package linkedlist

import (
	"container/list"
)

type cacheNode struct {
	key int
	val int
}

// LRU 缓存
// https://leetcode.cn/problems/lru-cache/description/
type LRUCache struct {
	// TODO：用范型链表避免运行时断言
	data    *list.List
	dataMap map[int]*list.Element
	maxSize int
}

func Constructor(capacity int) LRUCache {
	return LRUCache{
		data:    list.New(),
		dataMap: make(map[int]*list.Element, capacity),
		maxSize: capacity,
	}
}

func (this *LRUCache) Get(key int) int {
	listElement, ok := this.dataMap[key]
	if !ok {
		return -1
	}

	this.data.MoveToFront(listElement)
	return listElement.Value.(*cacheNode).val
}

func (this *LRUCache) Put(key int, value int) {
	val, ok := this.dataMap[key]
	if !ok {
		if this.maxSize == this.data.Len() {
			endElement := this.data.Back()
			delete(this.dataMap, endElement.Value.(*cacheNode).key)
			this.data.Remove(endElement)
		}

		listElement := this.data.PushFront(&cacheNode{key: key, val: value})
		this.dataMap[key] = listElement
		return
	}

	n := val.Value.(*cacheNode)
	this.data.MoveToFront(val)
	n.val = value
}
