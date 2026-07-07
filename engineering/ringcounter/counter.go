package ringcounter

import (
	"sync"
	"time"
)

const (
	bucketNum = 50                  // 环形数组长度：50 个桶 × 100ms = 5 秒窗口
	shardNum  = 32                  // 分段锁个数
	bucketMS  = 100                 // 每个桶覆盖的毫秒数
	windowS   = 5                   // 滑动窗口秒数
)

// bucket 一个 100ms 时间桶，记录该时段内的请求数
type bucket struct {
	tick  int64 // 桶的序号：第几个 100ms（单调递增）
	count int64 // 该时段内的请求计数
}

// ringBuf 固定大小的环形数组，新数据覆盖旧数据
type ringBuf struct {
	buckets [bucketNum]bucket
}

type shard struct {
	mu sync.Mutex
	m  map[string]*ringBuf
}

// Counter 分段锁 + 环形数组计数器，统计过去 5 秒每个路径的 QPS。
// 写入 O(1)，读取 O(1)。
type Counter struct {
	shards [shardNum]shard
}

func New() *Counter {
	c := &Counter{}
	for i := range c.shards {
		c.shards[i].m = make(map[string]*ringBuf)
	}
	return c
}

// tick 当前时刻对应的桶序号
func tick() int64 {
	return time.Now().UnixMilli() / bucketMS
}

const (
	fnvOffset = 2166136261
	fnvPrime  = 16777619
)

func (c *Counter) shardOf(path string) uint32 {
	h := uint32(fnvOffset)
	for i := 0; i < len(path); i++ {
		h ^= uint32(path[i])
		h *= fnvPrime
	}
	return h % shardNum
}

// Add 记录一次请求。O(1)
func (c *Counter) Add(path string) {
	s := &c.shards[c.shardOf(path)]
	s.mu.Lock()
	defer s.mu.Unlock()

	rb, ok := s.m[path]
	if !ok {
		rb = &ringBuf{}
		s.m[path] = rb
	}

	t := tick()
	i := int(t % bucketNum) // 环形数组中的位置

	if rb.buckets[i].tick != t {
		rb.buckets[i].tick = t
		rb.buckets[i].count = 1
	} else {
		rb.buckets[i].count++
	}
}

// GetQPS 返回指定路径过去 5 秒的 QPS 近似值。O(1)
func (c *Counter) GetQPS(path string) float64 {
	s := &c.shards[c.shardOf(path)]
	s.mu.Lock()
	defer s.mu.Unlock()

	rb, ok := s.m[path]
	if !ok {
		return 0
	}

	// 50 个桶以前的数据都算过期
	oldest := tick() - bucketNum

	var total int64
	for i := 0; i < bucketNum; i++ {
		if rb.buckets[i].tick > oldest {
			total += rb.buckets[i].count
		}
	}
	return float64(total) / windowS
}

// Clean 清理所有桶都已过期的路径，防止 map 无限增长。
func (c *Counter) Clean() {
	oldest := tick() - bucketNum
	for i := range c.shards {
		s := &c.shards[i]
		s.mu.Lock()
		for path, rb := range s.m {
			allExpired := true
			for j := 0; j < bucketNum; j++ {
				if rb.buckets[j].tick > oldest {
					allExpired = false
					break
				}
			}
			if allExpired {
				delete(s.m, path)
			}
		}
		s.mu.Unlock()
	}
}
