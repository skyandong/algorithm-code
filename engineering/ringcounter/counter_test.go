package ringcounter

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCounter_AddAndGetQPS(t *testing.T) {
	c := New()

	c.Add("/api/users")
	c.Add("/api/users")
	c.Add("/api/users")

	qps := c.GetQPS("/api/users")
	assert.InDelta(t, 3.0/5.0, qps, 0.01)

	assert.Zero(t, c.GetQPS("/api/nonexistent"))
}

func TestCounter_ShardSeparation(t *testing.T) {
	c := New()

	c.Add("/api/users")
	c.Add("/api/orders")

	assert.NotZero(t, c.GetQPS("/api/users"))
	assert.NotZero(t, c.GetQPS("/api/orders"))
	assert.Zero(t, c.GetQPS("/api/products"))
}

func TestCounter_Concurrent(t *testing.T) {
	c := New()
	var wg sync.WaitGroup

	n := 1000
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Add("/api/users")
		}()
	}
	wg.Wait()

	qps := c.GetQPS("/api/users")
	expected := float64(n) / 5.0
	assert.InDelta(t, expected, qps, expected*0.01)
}

func TestCounter_ExpiredSlotsIgnored(t *testing.T) {
	c := New()

	c.Add("/api/users")

	// 把桶的 tick 设为极早的值，模拟数据过期
	idx := c.shardOf("/api/users")
	s := &c.shards[idx]
	s.mu.Lock()
	rb := s.m["/api/users"]
	for i := range rb.buckets {
		rb.buckets[i].tick = 1
	}
	s.mu.Unlock()

	assert.Zero(t, c.GetQPS("/api/users"))
}

func TestCounter_MultiplePaths(t *testing.T) {
	c := New()

	paths := []string{"/a", "/b", "/c", "/d"}
	for _, p := range paths {
		for i := 0; i < 10; i++ {
			c.Add(p)
		}
	}

	for _, p := range paths {
		assert.InDelta(t, 2.0, c.GetQPS(p), 0.01)
	}
}

func TestCounter_Clean(t *testing.T) {
	c := New()

	c.Add("/api/users")

	// 把所有桶的 tick 设为过期值
	idx := c.shardOf("/api/users")
	s := &c.shards[idx]
	s.mu.Lock()
	rb := s.m["/api/users"]
	for i := range rb.buckets {
		rb.buckets[i].tick = 1
	}
	s.mu.Unlock()

	c.Clean()

	s.mu.Lock()
	_, ok := s.m["/api/users"]
	s.mu.Unlock()
	assert.False(t, ok, "过期的路径应该被 Clean 移除")
}

func TestCounter_WrapAround(t *testing.T) {
	c := New()
	path := "/api/users"
	idx := c.shardOf(path)
	s := &c.shards[idx]

	// 预填所有桶为过期数据
	s.mu.Lock()
	rb := &ringBuf{}
	s.m[path] = rb
	old := tick() - bucketNum - 1
	for i := 0; i < bucketNum; i++ {
		rb.buckets[i] = bucket{tick: old + int64(i), count: 10}
	}
	s.mu.Unlock()

	// Add 应该覆盖当前位置的旧桶
	c.Add(path)
	s.mu.Lock()
	pos := int(tick() % bucketNum)
	assert.Equal(t, int64(1), rb.buckets[pos].count)
	s.mu.Unlock()
}
