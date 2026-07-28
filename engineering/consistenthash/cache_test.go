package consistenthashcache

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ======================== 虚拟节点环构建测试 ========================

func TestBuildRing_AllNodesCovered(t *testing.T) {
	r := BuildRing([]string{"redis-1", "redis-2", "redis-3"}, 150)

	// 3 节点 × 150 虚拟节点 = 450 个点
	assert.Len(t, r.nodes, 450)

	// 有序
	for i := 1; i < len(r.nodes); i++ {
		assert.True(t, r.nodes[i-1].hash <= r.nodes[i].hash, "环数组须有序")
	}
}

func TestBuildRing_Distribution(t *testing.T) {
	// 500 虚拟节点 + 50000 真实风格 key，各节点落在 25%~42% 即为均匀
	nodes := []string{"redis-1", "redis-2", "redis-3"}
	r := BuildRing(nodes, 500)

	counts := map[string]int{}
	types := []string{"user", "order", "product", "session", "cache", "lock"}
	for i := 0; i < 50000; i++ {
		k := types[i%len(types)] + ":" + itoa(i*137)
		counts[r.Lookup(k)]++
	}

	for _, node := range nodes {
		pct := float64(counts[node]) / 50000
		assert.True(t, pct > 0.25, "%s 占比 %.2f 偏低", node, pct)
		assert.True(t, pct < 0.42, "%s 占比 %.2f 偏高", node, pct)
	}
}

// ======================== 环 Lookup 测试 ========================

func TestRing_Lookup_Basic(t *testing.T) {
	r := BuildRing([]string{"redis-1", "redis-2", "redis-3"}, 150)

	keys := []string{"alpha", "beta", "gamma", "delta", "epsilon"}
	nodes := make(map[string]bool)
	for _, k := range keys {
		node := r.Lookup(k)
		assert.NotEmpty(t, node)
		nodes[node] = true
	}
	assert.True(t, len(nodes) >= 1, "所有 key 应该映射到有效节点")
}

func TestRing_Lookup_SpecificValue(t *testing.T) {
	r := NewRing(map[uint32]string{
		0:          "node-a",
		1000000000: "node-b",
		3000000000: "node-c",
	})

	// hash("a") → 3826002220 落在 [3×10^9, 0) 即末尾回绕区间 → node-a
	assert.Equal(t, "node-a", r.Lookup("a"))
	assert.NotEmpty(t, r.Lookup("hello"))
}

func TestRing_Lookup_WrapAround(t *testing.T) {
	r := NewRing(map[uint32]string{
		10:  "node-a",
		200: "node-b",
	})

	// hash 落到环末尾 (≥200) → 回绕到 10 → node-a
	assert.Equal(t, "node-a", r.Lookup("some-key"))
}

func TestRing_Lookup_Empty(t *testing.T) {
	r := NewRing(nil)
	assert.Equal(t, "", r.Lookup("any-key"))
}

func TestRing_Lookup_SameKeySameNode(t *testing.T) {
	r := BuildRing([]string{"redis-1", "redis-2", "redis-3"}, 150)
	node := r.Lookup("my-key")
	for i := 0; i < 100; i++ {
		assert.Equal(t, node, r.Lookup("my-key"))
	}
}

// ======================== 缓存基础测试 ========================

func TestCache_Get_LocalHit(t *testing.T) {
	r := BuildRing([]string{"redis-1", "redis-2"}, 100)
	c := NewCache(10, r)

	callCount := 0
	fetch := func() ([]byte, bool) {
		callCount++
		return []byte("hello"), true
	}

	val, ok := c.Get("key1", fetch)
	assert.True(t, ok)
	assert.Equal(t, []byte("hello"), val)
	assert.Equal(t, 1, callCount, "首次应穿透到 Redis")

	// 二次命中本地缓存
	callCount = 0
	val, ok = c.Get("key1", func() ([]byte, bool) {
		callCount++
		return []byte("SHOULD-NOT-BE-CALLED"), true
	})
	assert.True(t, ok)
	assert.Equal(t, []byte("hello"), val)
	assert.Equal(t, 0, callCount, "命中本地缓存，不应调用 Redis")
}

func TestCache_Get_RedisMiss(t *testing.T) {
	r := BuildRing([]string{"redis-1"}, 100)
	c := NewCache(10, r)

	val, ok := c.Get("nokey", func() ([]byte, bool) {
		return nil, false
	})
	assert.False(t, ok)
	assert.Nil(t, val)
}

func TestCache_Get_NodeChanged(t *testing.T) {
	oldRing := BuildRing([]string{"redis-1", "redis-2", "redis-3"}, 100)
	c := NewCache(10, oldRing)

	fetchCount := 0
	val, ok := c.Get("key1", func() ([]byte, bool) {
		fetchCount++
		return []byte("v1"), true
	})
	assert.True(t, ok)
	assert.Equal(t, []byte("v1"), val)
	assert.Equal(t, 1, fetchCount)

	// 扩容：3→4
	newRing := BuildRing([]string{"redis-1", "redis-2", "redis-3", "redis-4"}, 100)
	c.ring = newRing

	fetchCount = 0
	val2, _ := c.Get("key1", func() ([]byte, bool) {
		fetchCount++
		return []byte("v2"), true
	})
	assert.True(t, len(val2) > 0)
}

func TestCache_LRU_Eviction(t *testing.T) {
	r := BuildRing([]string{"redis-1"}, 100)
	c := NewCache(2, r)

	redis := map[string][]byte{
		"a": []byte("a"), "b": []byte("b"), "c": []byte("c"),
	}

	fetch := func(key string) func() ([]byte, bool) {
		return func() ([]byte, bool) {
			v, ok := redis[key]
			return v, ok
		}
	}

	c.Get("a", fetch("a"))
	c.Get("b", fetch("b"))

	c.mu.RLock()
	_, okA := c.data["a"]
	_, okB := c.data["b"]
	c.mu.RUnlock()
	assert.True(t, okA)
	assert.True(t, okB)

	// 插入 c → 淘汰最久未使用的 a
	c.Get("c", fetch("c"))

	c.mu.RLock()
	_, okA = c.data["a"]
	_, okB = c.data["b"]
	_, okC := c.data["c"]
	c.mu.RUnlock()
	assert.False(t, okA, "a 应该被 LRU 淘汰")
	assert.True(t, okB)
	assert.True(t, okC)
}

// ======================== 精准失效测试 ========================

func TestInvalidateMovedKeys_SameRing(t *testing.T) {
	r := BuildRing([]string{"redis-1", "redis-2", "redis-3"}, 100)
	c := NewCache(10, r)

	c.Get("k1", func() ([]byte, bool) { return []byte("v1"), true })
	c.Get("k2", func() ([]byte, bool) { return []byte("v2"), true })

	c.InvalidateMovedKeys(r, r)
	assert.Len(t, c.data, 2)
}

func TestInvalidateMovedKeys_DifferentRing(t *testing.T) {
	oldRing := BuildRing([]string{"redis-1", "redis-2", "redis-3"}, 100)
	c := NewCache(10, oldRing)

	c.Get("k1", func() ([]byte, bool) { return []byte("v1"), true })
	c.Get("k2", func() ([]byte, bool) { return []byte("v2"), true })

	// 新环：节点变了
	newRing := BuildRing([]string{"redis-1", "redis-2", "redis-3", "redis-4"}, 100)
	c.InvalidateMovedKeys(oldRing, newRing)
	assert.Equal(t, newRing, c.ring)
}

func TestInvalidateMovedKeys_AddNode(t *testing.T) {
	oldRing := BuildRing([]string{"redis-1", "redis-2", "redis-3"}, 100)
	c := NewCache(100, oldRing)

	keys := []string{"u:1", "u:2", "u:3", "u:4", "u:5"}
	for _, k := range keys {
		c.Get(k, func() ([]byte, bool) { return []byte(k + "-val"), true })
	}
	oldLen := len(c.data)

	newRing := BuildRing([]string{"redis-1", "redis-2", "redis-3", "redis-4"}, 100)
	c.InvalidateMovedKeys(oldRing, newRing)

	assert.True(t, len(c.data) <= oldLen, "失效后缓存数量不应增加")
}

func TestInvalidateMovedKeys_Concurrent(t *testing.T) {
	oldRing := BuildRing([]string{"redis-1", "redis-2", "redis-3"}, 100)
	c := NewCache(100, oldRing)

	for i := 0; i < 50; i++ {
		k := "key-" + string(rune('A'+i%26)) + string(rune('a'+i))
		c.Get(k, func() ([]byte, bool) { return []byte(k), true })
	}

	newRing := BuildRing([]string{"redis-1", "redis-2", "redis-3", "redis-4"}, 100)
	c.InvalidateMovedKeys(oldRing, newRing)
}

// ======================== 其他测试 ========================

func TestCache_EntryHasNodeID(t *testing.T) {
	r := BuildRing([]string{"redis-1", "redis-2"}, 100)
	c := NewCache(10, r)

	c.Get("mykey", func() ([]byte, bool) { return []byte("v"), true })

	c.mu.RLock()
	elem := c.data["mykey"]
	c.mu.RUnlock()
	assert.NotNil(t, elem)

	entry := elem.Value.(*lruEntry).cacheVal
	assert.Equal(t, r.Lookup("mykey"), entry.nodeID, "缓存 entry 须记录其归属节点 ID")
}

func TestHash_Deterministic(t *testing.T) {
	h1 := hash("hello-world")
	for i := 0; i < 1000; i++ {
		assert.Equal(t, h1, hash("hello-world"))
	}
}

func TestHash_Distribution(t *testing.T) {
	seen := make(map[uint32]bool)
	for i := 0; i < 100; i++ {
		seen[hash("key-"+string(rune('a'+i%26))+string(rune('0'+i/10)))] = true
	}
	assert.True(t, len(seen) >= 90, "hash 分布应基本均匀")
}

func TestItoa(t *testing.T) {
	assert.Equal(t, "0", itoa(0))
	assert.Equal(t, "149", itoa(149))
	assert.Equal(t, "1000", itoa(1000))
}
