// 题目：多级缓存的"写穿透"与一致性哈希
//
// 【场景】
// 3 台独立 Redis 实例，客户端用一致性哈希自行分片（非 Redis Cluster）。
// 服务层加本地内存缓存（Go map + LRU）：请求先查本地，再查 Redis。
//
// 【基础版缺陷】
// 当 Redis 节点扩容（3→4），一致性哈希导致部分 key 归属迁移。
// 此时本地缓存中的旧数据如果不清理：
//   - 写入走新路由→写到新节点, 读取命中本地缓存→返回旧值 → 脏读
//   - 更隐蔽: 本地缓存的旧值被回写到 Redis, 覆盖新节点上的正确数据
//
// 【本方案】
//   - 环抽象为有序数组, 二分查找 key 归属
//   - 本地缓存记录 key 所属物理节点 ID
//   - 集群变化后, diff 新旧两个有序数组, 找到"节点变了"的 hash 区间
//     只失效落在这些区间内的缓存 key (不清空全量)

package consistenthashcache

import (
	"container/list"
	"sort"
	"sync"
)

// ======================== 1. 一致性哈希环 ========================

// RingNode 环上一个点: 虚拟节点的 hash 值 + 物理节点 ID
type RingNode struct {
	hash uint32 // 虚拟节点在环上的位置
	node string // 对应的物理节点 (如 "redis-1")
}

// Ring 一致性哈希环, 内部存一个有序数组
type Ring struct {
	nodes []RingNode // 按 hash 升序排列
}

// BuildRing 从物理节点列表构建一致性哈希环。
// replicas 是每个物理节点的虚拟节点数（通常 100~200），越大分布越均匀。
//
// 生成逻辑：
//
//	对每个物理节点，构建 replicas 个虚拟节点：
//	  hash("redis-1:0")  → 环上一个点
//	  hash("redis-1:1")  → 环上另一个点
//	  ...
//	  hash("redis-1:149") → 又一个点
//	所有点按 hash 值排序后就是有序数组，Lookup 时二分查找。
func BuildRing(nodes []string, replicas int) *Ring {
	if len(nodes) == 0 {
		return &Ring{}
	}
	n := len(nodes) * replicas
	ringNodes := make([]RingNode, 0, n)
	for _, node := range nodes {
		for i := 0; i < replicas; i++ {
			// "redis-1:0", "redis-1:1", ...
			vkey := node + ":" + itoa(i)
			ringNodes = append(ringNodes, RingNode{hash: hash(vkey), node: node})
		}
	}
	sort.Slice(ringNodes, func(i, j int) bool {
		return ringNodes[i].hash < ringNodes[j].hash
	})
	return &Ring{nodes: ringNodes}
}

// itoa 简单整数转字符串, 避免 strconv 的额外堆分配
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte // 足够放 int 最大值
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// NewRing 从已计算好的 hash→node 映射构建环（用于从持久化配置恢复等场景）
func NewRing(virtualNodes map[uint32]string) *Ring {
	nodes := make([]RingNode, 0, len(virtualNodes))
	for hash, node := range virtualNodes {
		nodes = append(nodes, RingNode{hash: hash, node: node})
	}
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].hash < nodes[j].hash
	})
	return &Ring{nodes: nodes}
}

// hash 哈希函数 (FNV-1a 32bit)
func hash(key string) uint32 {
	h := uint32(2166136261)
	for i := 0; i < len(key); i++ {
		h ^= uint32(key[i])
		h *= 16777619
	}
	return h
}

// Lookup 二分查找 key 归属哪个物理节点
func (r *Ring) Lookup(key string) string {
	if len(r.nodes) == 0 {
		return ""
	}
	h := hash(key)
	// sort.Search 二分查找: 找到第一个 hash >= h 的节点
	idx := sort.Search(len(r.nodes), func(i int) bool {
		return r.nodes[i].hash >= h
	})
	if idx == len(r.nodes) {
		idx = 0 // 环的末尾回绕到开头
	}
	return r.nodes[idx].node
}

// ======================== 2. 本地缓存 ========================

type cacheEntry struct {
	value  []byte
	nodeID string // 该 key 当前归属的物理节点 (存这里是关键)
	// LRU 链表节点指针 ...
}

// Cache 本地 LRU 缓存 + 一致性哈希感知
type Cache struct {
	mu    sync.RWMutex
	data  map[string]*list.Element // key → LRU 节点
	lru   *list.List
	cap   int

	ring *Ring // 当前生效的哈希环 (原子替换)
}

type lruEntry struct {
	key      string
	cacheVal *cacheEntry
}

func NewCache(cap int, ring *Ring) *Cache {
	return &Cache{
		data: make(map[string]*list.Element),
		lru:  list.New(),
		cap:  cap,
		ring: ring,
	}
}

// ======================== 3. 读取链路 ========================

// Get 查询缓存: 本地 → Redis → DB
// cache.Get(key, func() { return redis.Get(key) })
func (c *Cache) Get(key string, fetchFromRedis func() ([]byte, bool)) ([]byte, bool) {
	c.mu.RLock()

	// 步骤1: 查本地缓存
	if elem, ok := c.data[key]; ok {
		entry := elem.Value.(*lruEntry).cacheVal

		// 检查这个 key 的归属节点是否变了
		// (ring 可能在拓扑变化时被替换)
		currentNode := c.ring.Lookup(key)
		if entry.nodeID == currentNode {
			// 归属未变, 直接命中
			c.mu.RUnlock()
			c.mu.Lock()
			c.lru.MoveToFront(elem)
			c.mu.Unlock()
			return entry.value, true
		}
		// 归属变了 → 这个缓存已失效, 删除
		c.mu.RUnlock()
		c.mu.Lock()
		c.lru.Remove(elem)
		delete(c.data, key)
		c.mu.Unlock()
	} else {
		c.mu.RUnlock()
	}

	// 步骤2: 本地未命中, 查 Redis
	val, ok := fetchFromRedis()
	if !ok {
		return nil, false
	}

	// 步骤3: 回填本地缓存 (记录归属节点)
	c.mu.Lock()
	c.setLocked(key, val, c.ring.Lookup(key))
	c.mu.Unlock()

	return val, true
}

// setLocked 写入本地缓存 (调用方需持有写锁)
func (c *Cache) setLocked(key string, val []byte, nodeID string) {
	if elem, ok := c.data[key]; ok {
		c.lru.MoveToFront(elem)
		elem.Value.(*lruEntry).cacheVal = &cacheEntry{value: val, nodeID: nodeID}
		return
	}

	if c.lru.Len() >= c.cap {
		oldest := c.lru.Back()
		if oldest != nil {
			c.lru.Remove(oldest)
			delete(c.data, oldest.Value.(*lruEntry).key)
		}
	}

	elem := c.lru.PushFront(&lruEntry{
		key: key,
		cacheVal: &cacheEntry{
			value:  val,
			nodeID: nodeID,
		},
	})
	c.data[key] = elem
}

// ======================== 4. 精准失效 (O(m+k)) ========================

// InvalidateMovedKeys 当集群拓扑变化后调用。
// 对比新旧环, 只失效"归属变了"的本地缓存 key。
// m = 虚拟节点数, k = 失效 key 数, 不清空全量缓存。
func (c *Cache) InvalidateMovedKeys(oldRing, newRing *Ring) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 更新环引用
	c.ring = newRing

	// 双指针遍历新旧有序数组，找到物理节点不一致的 hash 区间 -> 失效区间
	type hashRange struct{ lo, hi uint32 }
	var invalidRanges []hashRange

	i, j := 0, 0
	oldNodes := oldRing.nodes
	newNodes := newRing.nodes
	for i < len(oldNodes) && j < len(newNodes) {
		// 检查当前区间
		lo := oldNodes[i].hash
		if newNodes[j].hash > lo {
			lo = newNodes[j].hash
		}

		var hi uint32
		if i+1 < len(oldNodes) && j+1 < len(newNodes) {
			hi = oldNodes[i+1].hash
			if newNodes[j+1].hash < hi {
				hi = newNodes[j+1].hash
			}
		} else if i+1 < len(oldNodes) {
			hi = oldNodes[i+1].hash
		} else if j+1 < len(newNodes) {
			hi = newNodes[j+1].hash
		} else {
			hi = ^uint32(0) // 末尾
		}

		if oldNodes[i].node != newNodes[j].node {
			invalidRanges = append(invalidRanges, hashRange{lo: lo, hi: hi})
		}

		// 推进指针
		if i+1 < len(oldNodes) && oldNodes[i+1].hash <= hi {
			i++
		} else {
			j++
		}
		// 防止死循环
		if i >= len(oldNodes) || j >= len(newNodes) {
			break
		}
	}

	// 遍历本地缓存, 检查每个 key 的 hash 是否落在失效区间内
	for key, elem := range c.data {
		h := hash(key)
		for _, r := range invalidRanges {
			if h >= r.lo && h < r.hi {
				c.lru.Remove(elem)
				delete(c.data, key)
				break
			}
		}
	}
}

