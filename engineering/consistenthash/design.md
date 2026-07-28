# 题目：多级缓存的"写穿透"与一致性哈希

## 场景

3 台独立 Redis 实例，客户端用一致性哈希自行分片（非 Redis Cluster）。服务层加本地内存缓存（Go map + LRU）：

```
请求 → 本地缓存(hit?) → Redis(hit?) → DB
         ↓ miss              ↓ miss
```

## 基础版：扩容时的脏读问题

集群从 3 节点扩到 4 节点后，一致性哈希导致部分 key 归属迁移：

```
扩容前：key="user:1001" → hash 落在 node-2
扩容后：key="user:1001" → hash 落在 node-3（新节点）
```

此时如果本地缓存不清理，写入请求走新路由写到 node-3，但本地缓存还存着 node-2 的旧值。后续读取命中本地缓存，返回的就是过期数据 —— **脏读**。

更隐蔽的情况：本地缓存中的旧值被回写到 Redis（写穿透），node-3 上新写入的数据被旧数据覆盖。

## 进阶版：精准失效，不清空全量

核心思路：**本地缓存记录 key 所属的物理节点 ID，集群变化后只失效"归属变了"的那部分 key**。

### 1. 一致性哈希抽象

把虚拟节点环抽象成有序数组（Sorted List），每个元素是 (hash值, 物理节点ID) 的二元组：

```
虚拟节点环（逻辑视图）:          有序数组（存储视图）:
                                 
     0                           [ hash:0,    node-a ]
   /  \                          [ hash:150,  node-b ]
  /    \                         [ hash:300,  node-c ]
 |  A   |                        [ hash:450,  node-a ]
 |  B   |                        [ hash:600,  node-b ]
  \ C  /                         [ hash:700,  node-c ]
   \  /                          [ hash:850,  node-a ]
    1000                         ...

key → hash(key) → 在有序数组中二分查找，返回该位置对应的物理节点 ID
```

### 2. 本地缓存结构

```go
type CacheEntry struct {
    Value    []byte
    NodeID   string  // 该 key 当前归属的物理节点
    ExpireAt int64
}
```

每次从 Redis 拿到数据时，计算 `lookup(key)` 确定它属于哪个物理节点，一起存入本地缓存。

### 3. 失效判断

集群拓扑变化后，旧环和新环的区别就是有序数组里某些 hash 值对应的物理节点变了。

```go
// 旧环和新环是两个有序数组
type Ring struct {
    nodes []RingNode  // 按 hash 升序排列
}

type RingNode struct {
    hash int64
    node string  // 物理节点 ID
}

// 查询 key 归属哪个物理节点（在环上二分查找）
func (r *Ring) Lookup(key string) string {
    h := hash(key)
    // 二分找到第一个 hash >= h 的节点
    i := sort.Search(len(r.nodes), func(i int) bool {
        return r.nodes[i].hash >= h
    })
    if i == len(r.nodes) {
        i = 0  // 环回绕
    }
    return r.nodes[i].node
}
```

本地缓存中的每个 key，如果满足下面条件就失效：

```
旧环.Lookup(key) != 新环.Lookup(key)
```

### 4. 二分查找定位要失效的范围（关键优化）

上面的方案需要遍历所有本地缓存 key 逐个重新 Lookup，比如 10 万个 key 就是 10 万次二分查找。

更好的方式是反过来想：旧环和新环的有序数组 diff 一下，找到**所有"物理节点变了"的 hash 区间**，然后本地缓存的 key 落在这个区间内的才需要失效。

```
旧环:  [0→A] [150→B] [300→C] [450→A] [600→B] [700→C] [850→A]
新环:  [0→A] [150→B] [300→C] [400→D] [550→A] [700→B] [800→C] [900→D]

diff:
  区间 [450, 550): A → D  (变了)
  区间 [550, 600): B → A  (变了)
  区间 [600, 700): B → B  (没变，跳过)
  区间 [700, 800): C → B  (变了)
  ...
```

具体算法：
1. 双指针遍历旧环和新环的有序数组，找到物理节点不一致的连续区间
2. 对每个"变了"的区间，将区间内 hash 值对应的本地缓存 key 失效
3. 本地缓存 key 的 hash 值也可以预先算好存着，避免二次计算

复杂度：O(m) 遍历两个有序数组（m = 虚拟节点数），而不是 O(n log m) 遍历所有缓存 key。

### 总结

| 方案 | 失效范围 | 复杂度 | 适用场景 |
|------|---------|--------|---------|
| 全量清空 | 全部 key | O(1) | 缓存小，可接受全部 miss |
| 逐个重新 Lookup | 只失效迁移的 key | O(n log m) | 缓存不大，n 较小时 |
| 有序数组 diff（推荐） | 只失效迁移的 key | O(m + k)，k = 失效 key 数 | 缓存大，精准失效 |

---

## 附录：Codis vs Redis Cluster

题目场景用的是客户端一致性哈希分片，实际上生产环境中类似的做法有 Codis。这里对比一下两种主流分片方案。

### 架构对比

```
Redis Cluster（官方方案）              Codis（豌豆荚开源，已停维）
═══════════════════════                ════════════════════════════
                                       
        客户端                            客户端
         │                                │
         ▼                                ▼
   ┌──────────┐                     ┌──────────┐
   │ slot 映射 │ (CRC16 % 16384)     │  Proxy   │ ← 无状态代理层
   └──────────┘                     └──────────┘
         │                                │
    ┌────┼────┐                      ┌────┼────┐
    ▼    ▼    ▼                      ▼    ▼    ▼
  node  node  node                node  node  node
                                   (普通 Redis，无 Cluster 模式)
```

| | Redis Cluster | Codis |
|---|---|---|
| **分片粒度** | 16384 个 hash slot | 1024 个 slot |
| **路由方式** | 客户端直连，缓存 slot→node 映射 | 客户端→Proxy→Redis，Proxy 做路由 |
| **hash 算法** | CRC16(key) % 16384 | 默认一致性哈希，也可配 hash slot |
| **节点角色** | 每个 Redis 节点都是 Cluster 模式（有 gossip 协议） | Redis 是普通 standalone 实例，无感知集群 |
| **扩缩容** | slot 在线迁移，`MOVED`/`ASK` 重定向 | 通过 Dashboard 在线迁移 slot，Proxy 热更新路由表 |
| **客户端复杂度** | 高 — 客户端要处理重定向、维护 slot 表 | 低 — 客户端只需连 Proxy（像连单机 Redis） |
| **运维复杂度** | 低 — 无额外组件，Redis 自带 | 高 — 需部署 Proxy + Dashboard + ZooKeeper/Etcd |
| **多 key 操作** | 需 key 在同一 slot（用 `{}` hash tag） | 需 key 在同一 slot |
| **状态** | 活跃维护，广泛使用 | 已停维，被 Redis Cluster 取代 |

### 关键区别

**架构上最大区别：Proxy 层**

Codis 在客户端和 Redis 之间加了一层 Proxy，好处是客户端零改造（像连单机 Redis），代价是多一跳网络延迟 + 运维 Proxy 集群。Redis Cluster 没有中间层，客户端直连，延迟更低但客户端逻辑更重。

**扩容时数据迁移粒度**

- Redis Cluster 按 slot 整批迁移（slot 是物理存储单元）
- 一致性哈希按 key 粒度自然漂移（key 重新 hash 后自然落在新节点）

这也是为什么本题的一致性哈希方案在扩容时会出现脏读——key 在环上的位置自动变了，而 slot 方案不会（slot 到 node 的映射是显式变更的，key 的 slot 编号本身不变）。

### 为什么现在基本都用 Redis Cluster

1. **无中间层** — Codis 的 Proxy 是额外运维负担和故障点
2. **官方支持** — Redis Cluster 是 Redis 内置功能，随版本迭代不断优化
3. **社区活跃度** — Codis 已停维多年，Redis Cluster 是事实标准
4. **客户端生态** — 主流语言都有成熟的 Cluster 客户端库（Jedis、go-redis 等），处理 `MOVED` 重定向对业务代码透明
