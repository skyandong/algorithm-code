// # Slice 与 Map 底层实验
//
// 对应笔记：notes/golang/01-slice与map底层.md
// 全部结论已改为断言式验证，见 01_slice_map_test.go（无打印观察）。
//
// —— runtime 源码对照 ——
//
// src/runtime/slice.go
//
//	type slice struct {
//		array unsafe.Pointer // 指向底层数组
//		len   int
//		cap   int
//	}
//
// src/internal/runtime/maps/map.go（当前 map 实现顶层）
//
//	type Map struct {
//		used        uint64         // 已用槽位数，len() 直接读它（必须放首字段，编译器依赖）
//		seed        uintptr        // hash 种子（每次创建随机）
//		dirPtr      unsafe.Pointer // 目录：正常为 *[dirLen]*table；小 map 直接指向单个 group
//		dirLen      int
//		globalDepth uint8 // 目录查找用的 hash 位数
//		globalShift uint8
//	}
//
// src/internal/runtime/maps/table.go（单张表）
//
//	type table struct {
//		used       uint16 // 本表已用槽位
//		capacity   uint16 // 总槽位（恒为 2^N）
//		growthLeft uint16 // 距下次 rehash 还能填的空槽数（含 tombstone 计入）
//		localDepth uint8
//		index      int
//	}
//
// src/internal/runtime/maps/group.go（一组 8 槽，key/elem 编译期按类型展开）
//
//	type group struct {
//		ctrls ctrlGroup                // 8 个控制字：hash 低 7 位 + 空闲/已删标记
//		slots [abi.MapGroupSlots]slot // abi.MapGroupSlots = 8
//	}
package main

// appendInside 修改的是参数副本（header 拷贝）的 len，调用方无感。
func appendInside(s []int) {
	s = append(s, 7)
}

// appendAndReturn 正确姿势：把新 header 返回（或传 *[]int）。
func appendAndReturn(s []int) []int {
	return append(s, 7)
}
