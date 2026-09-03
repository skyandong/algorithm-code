package main

import (
	"fmt"
)

// 实验 07：时间轮（单轮 + 轮数字段）
// 实现笔记 07 §2 的完整结构: 60 格环形数组, 每格挂任务链表, 指针每 tick 走一格;
// 轮数 > 0 的任务留守减一, 轮数 = 0 的执行摘除。
// 用虚拟 tick 驱动（确定性输出, 3 次运行一致）。
// 锚点: 1000 个跨多圈任务 100% 按预期 tick 触发; 取消的任务 0 执行。

const (
	wheelSlots = 60 // 60 格 = 一圈 60 tick
)

// twTask: 时间轮任务
type twTask struct {
	id     int
	round  int // 剩余轮数: 指针扫过时 -1, 为 0 时执行
	runAt  int // 预期执行 tick（用于验收）
	cancel bool
}

// timeWheel: 单层时间轮
type timeWheel struct {
	slots [][]*twTask // 每格一个任务链表
	now   int         // 当前 tick（虚拟时钟）
}

func newTimeWheel() *timeWheel {
	return &timeWheel{slots: make([][]*twTask, wheelSlots)}
}

// add: 插入延迟 delay tick 的任务, 返回任务指针（可用于取消）
// 计算: 轮数 = (delay-1)/60, 槽 = (now+delay)%60 —— 笔记 07 §2 公式
func (w *timeWheel) add(id, delay int) *twTask {
	round := (delay - 1) / wheelSlots
	slot := (w.now + delay) % wheelSlots
	t := &twTask{id: id, round: round, runAt: w.now + delay}
	w.slots[slot] = append(w.slots[slot], t)
	return t
}

// tick: 指针走一格, 扫描该格链表
func (w *timeWheel) tick() (fired []*twTask) {
	w.now++
	slot := w.now % wheelSlots
	remaining := w.slots[slot][:0]
	for _, t := range w.slots[slot] {
		switch {
		case t.cancel:
			// 已取消: 直接摘除
		case t.round > 0:
			t.round-- // 留守: 等下一圈
			remaining = append(remaining, t)
		default:
			fired = append(fired, t) // 到点执行
		}
	}
	w.slots[slot] = remaining
	return fired
}

func RunTimeWheelExperiments() {
	fmt.Println("== 实验 07: 时间轮——单轮+轮数, 跨多圈任务全部按时触发 ==")

	w := newTimeWheel()

	// ---- 插入 1000 个任务: 延迟 1..600 tick（跨 1~10 圈）----
	const taskCount = 1000
	tasks := make([]*twTask, 0, taskCount)
	for i := 1; i <= taskCount; i++ {
		delay := i % 600 // 1..599,0→600 处理
		if delay == 0 {
			delay = 600
		}
		tasks = append(tasks, w.add(i, delay))
	}
	// 取消其中 100 个（模拟"订单提前支付, 取消延迟任务"）
	for i := 0; i < taskCount; i += 10 {
		tasks[i].cancel = true
	}
	active := taskCount - taskCount/10

	fmt.Printf("插入 %d 个任务, 延迟 1~600 tick（跨 1~10 圈）, 取消 %d 个\n", taskCount, taskCount/10)
	fmt.Printf("轮数分布验证: delay=61 → 轮数=%d; delay=600 → 轮数=%d\n",
		(61-1)/wheelSlots, (600-1)/wheelSlots)

	// ---- 驱动 600 tick, 收集触发 ----
	firedAt := make(map[int]int) // taskID → 实际触发 tick
	for tick := 1; tick <= 600; tick++ {
		for _, t := range w.tick() {
			firedAt[t.id] = tick
		}
	}

	// ---- 验收 ----
	fmt.Println("\n--- 验收 ---")

	// 1. 未取消的任务 100% 按时触发
	onTime, late, early, notFired, cancelledFired := 0, 0, 0, 0, 0
	for _, t := range tasks {
		if t.cancel {
			if _, ok := firedAt[t.id]; ok {
				cancelledFired++
			}
			continue
		}
		at, ok := firedAt[t.id]
		switch {
		case !ok:
			notFired++
		case at == t.runAt:
			onTime++
		case at > t.runAt:
			late++
		default:
			early++
		}
	}
	fmt.Printf("  按时触发  : %d/%d (100%%) → %s\n", onTime, active, mark(onTime == active))
	fmt.Printf("  提前/迟到 : %d/%d (必须为 0) → %s\n", early, late, mark(early == 0 && late == 0))
	fmt.Printf("  未触发    : %d (必须为 0) → %s\n", notFired, mark(notFired == 0))
	fmt.Printf("  已取消执行 : %d (必须为 0) → %s\n", cancelledFired, mark(cancelledFired == 0))

	// 2. 轮数字段的正确性抽查: delay=62 应在第 62 tick 触发（避开被取消的 id=61）
	if t := firedAt[62]; t == 62 { // id=62 → delay=62, 轮数=(62-1)/60=1
		fmt.Printf("  抽查 id=62 (delay=62, 轮数=1): 第 %d tick 触发 → %s\n", t, mark(true))
	} else {
		fmt.Printf("  抽查 id=62: 第 %v tick 触发 (预期 62) → %s\n", firedAt[62], mark(false))
	}

	// 3. 槽占用统计（插入 O(1) 的佐证: 1000 任务 / 60 槽 ≈ 17/格）
	totalInSlots := 0
	for _, s := range w.slots {
		totalInSlots += len(s)
	}
	fmt.Printf("  600 tick 后轮内残留: %d 个（未到轮数任务应为 0——全部到期）→ %s\n",
		totalInSlots, mark(totalInSlots == 0))

	ok := onTime == active && early == 0 && late == 0 && notFired == 0 && cancelledFired == 0 && totalInSlots == 0
	if ok {
		fmt.Println("\n→ 结论: 1000 个跨多圈任务 100% 按预期 tick 触发, 取消 100% 生效 ✓")
		fmt.Println("  复杂度: 插入=除法取模+链表尾插 O(1); 到期检查=指针步进, 与任务总量无关")
	} else {
		fmt.Println("\n→ 结论: 时间轮实现有误 ✗")
	}

	// ---- 对照: 与 zset 轮询的复杂度对比（叙事输出）----
	fmt.Println("\n--- 复杂度对照（叙事） ---")
	fmt.Println("  zset 轮询 : 每秒 ZRANGEBYSCORE 全量扫描, 百万任务时 Redis CPU 显著")
	fmt.Println("  时间轮    : 每秒只看 1/60 格的链表, 检查成本与任务总量无关")
	fmt.Printf("  本实验    : %d 任务摊到 %d 格, 平均 %.1f 任务/格——指针每 tick 只扫这些\n",
		taskCount, wheelSlots, float64(taskCount)/wheelSlots)
}
