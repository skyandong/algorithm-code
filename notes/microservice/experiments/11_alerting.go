package main

import (
	"fmt"
	"sort"
	"strings"
)

// 实验 11：告警规则求值引擎 + Alertmanager 收敛（笔记 10 §2 §3）
// 实现: Pending/Firing 状态机（for 抗抖动）+ group_by 分组 + 抑制规则
// 演示: 一次机房故障在 10 个实例上触发 5 条规则 = 50 条告警, 看它们怎么收敛成 1 条通知
// 锚点: ① 抖动 20s 而 for=60s → 告警始终 Pending, 从不打扰人（10 §2）
//       ② 真实故障持续 → Pending 转 Firing, 通知发出
//       ③ 无分组 = 50 条通知; group_by [job] = 1 条（group_wait 攒批）
//       ④ 抑制: critical 抑制同 job 的 warning, 50 条告警只剩 20 条进通知

type alertRule struct {
	name     string
	forSec   int64 // for: 持续满足多久才转 Firing
	severity string
	on       bool // 模拟: 当前是否满足（故障/抖动由外部驱动）
}

type alert struct {
	name     string
	job      string
	instance string
	severity string
	state    string // inactive / pending / firing
	since    int64  // 进入当前状态的虚拟时刻（秒）
}

func (a *alert) key() string { return a.name + "@" + a.instance }

type simulator struct {
	now    int64
	alerts map[string]*alert
}

func newSimulator(rules []*alertRule, job string, instances int) *simulator {
	s := &simulator{alerts: map[string]*alert{}}
	for i := 0; i < instances; i++ {
		for _, r := range rules {
			a := &alert{
				name:     r.name,
				job:      job,
				instance: fmt.Sprintf("10.0.0.%d:8080", i+1),
				severity: r.severity,
				state:    "inactive",
				since:    0,
			}
			s.alerts[a.key()] = a
		}
	}
	return s
}

// step: 推进 dt 秒并按当前条件求值一次（对应 Prometheus 的 evaluation_interval）
func (s *simulator) step(rules []*alertRule, dt int64) {
	s.now += dt
	for _, r := range rules {
		for _, a := range s.alerts {
			if a.name != r.name {
				continue
			}
			switch a.state {
			case "inactive":
				if r.on {
					a.state, a.since = "pending", s.now
				}
			case "pending":
				if !r.on {
					a.state = "inactive" // 条件消失 → 回退, 这就是 for 抗抖的机制
				} else if s.now-a.since >= r.forSec {
					a.state, a.since = "firing", s.now
				}
			case "firing":
				if !r.on {
					a.state = "inactive" // 恢复 → resolved
				}
			}
		}
	}
}

func (s *simulator) count(state string) int {
	n := 0
	for _, a := range s.alerts {
		if a.state == state {
			n++
		}
	}
	return n
}

func (s *simulator) firing() []*alert {
	out := make([]*alert, 0)
	for _, a := range s.alerts {
		if a.state == "firing" {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].key() < out[j].key() })
	return out
}

// ---- Alertmanager 侧 ----

type notification struct {
	key    string
	alerts []*alert
}

// groupAlerts: group_by 指定的 label 组合成组（每组一条通知）
func groupAlerts(alerts []*alert, groupBy []string) []notification {
	if len(groupBy) == 0 {
		// 不分组: 每条告警一条独立通知——这就是"50 条告警 = 50 次打扰"
		out := make([]notification, 0, len(alerts))
		for _, a := range alerts {
			out = append(out, notification{key: a.key(), alerts: []*alert{a}})
		}
		return out
	}
	buckets := map[string][]*alert{}
	order := []string{}
	for _, a := range alerts {
		parts := make([]string, 0, len(groupBy))
		for _, l := range groupBy {
			switch l {
			case "alertname":
				parts = append(parts, a.name)
			case "job":
				parts = append(parts, a.job)
			case "instance":
				parts = append(parts, a.instance)
			case "severity":
				parts = append(parts, a.severity)
			}
		}
		key := strings.Join(parts, "/")
		if key == "" {
			key = "default"
		}
		if _, ok := buckets[key]; !ok {
			order = append(order, key)
		}
		buckets[key] = append(buckets[key], a)
	}
	out := make([]notification, 0, len(order))
	for _, k := range order {
		out = append(out, notification{key: k, alerts: buckets[k]})
	}
	return out
}

// inhibit: 高等级告警存在时, 静默同 job 的低等级告警（10 §3）
func inhibit(alerts []*alert) (kept, suppressed []*alert) {
	criticalJobs := map[string]bool{}
	for _, a := range alerts {
		if a.severity == "critical" {
			criticalJobs[a.job] = true
		}
	}
	for _, a := range alerts {
		if a.severity == "warning" && criticalJobs[a.job] {
			suppressed = append(suppressed, a)
			continue
		}
		kept = append(kept, a)
	}
	return kept, suppressed
}

func RunAlertingExperiments() {
	fmt.Println("=== 实验 11: 告警求值与 Alertmanager 收敛 ===")

	rules := []*alertRule{
		{name: "ServiceDown", forSec: 60, severity: "critical"},
		{name: "HighErrorRate", forSec: 300, severity: "critical"},
		{name: "HighLatency", forSec: 300, severity: "warning"},
		{name: "HighCPU", forSec: 300, severity: "warning"},
		{name: "QueueDeep", forSec: 300, severity: "warning"},
	}
	const instances = 10
	sim := newSimulator(rules, "pay-service", instances)
	total := len(sim.alerts)
	fmt.Printf("场景: 1 个 job × %d 实例 × %d 条规则 = %d 条潜在告警\n\n", instances, len(rules), total)

	// ---- 锚点 ①: 抖动被 for 吃掉 ----
	for _, r := range rules {
		r.on = true
	}
	sim.step(rules, 20) // 抖动只持续 20s
	pendingDuringGlitch := sim.count("pending")
	for _, r := range rules {
		r.on = false
	}
	sim.step(rules, 1)
	fmt.Printf("抖动期间进入 pending = %d 条, 恢复后 pending=%d firing=%d\n",
		pendingDuringGlitch, sim.count("pending"), sim.count("firing"))
	fmt.Printf("%s 锚点① for 抗抖动: 瞬时抖动 0 条告警发出（Pending 期间条件消失即回退）\n\n",
		mark(pendingDuringGlitch == total && sim.count("firing") == 0))

	// ---- 锚点 ②: 真实故障 ----
	for _, r := range rules {
		r.on = true
	}
	for t := int64(0); t < 400; t += 30 {
		sim.step(rules, 30)
	}
	firing := sim.firing()
	fmt.Printf("故障持续 400s 后: firing=%d / %d\n", len(firing), total)

	// ---- 锚点 ③: 分组 ----
	noGroup := groupAlerts(firing, nil)                  // 不分组
	byJob := groupAlerts(firing, []string{"job"})        // group_by [job]
	byName := groupAlerts(firing, []string{"alertname"}) // group_by [alertname]
	fmt.Printf("  不分组           → %d 条通知（每条 1 条告警）\n", len(noGroup))
	fmt.Printf("  group_by [job]  → %d 条通知（含全部 %d 条告警）\n", len(byJob), len(byJob[0].alerts))
	fmt.Printf("  group_by [alertname] → %d 条通知\n", len(byName))
	fmt.Printf("%s 锚点③ group_by 收敛: %d 条通知 → %d 条（把通知数量与故障规模解耦, 10 §3）\n\n",
		mark(len(noGroup) == total && len(byJob) == 1), len(noGroup), len(byJob))

	// ---- 锚点 ④: 抑制 ----
	kept, suppressed := inhibit(firing)
	fmt.Printf("  抑制前: %d 条（critical %d + warning %d）\n", len(firing), countSev(firing, "critical"), countSev(firing, "warning"))
	fmt.Printf("  抑制后: %d 条, 被抑制 %d 条\n", len(kept), len(suppressed))
	final := groupAlerts(kept, []string{"job"})
	fmt.Printf("%s 锚点④ 抑制规则: critical 存在时静默同 job 的 warning, 最终 %d 条告警进 %d 条通知\n",
		mark(len(suppressed) == 30 && len(kept) == 20), len(kept), len(final))

	fmt.Println("  最终通知摘要（10 §2: summary 必须自带足够信息）:")
	keptByName := groupAlerts(kept, []string{"alertname"})
	fmt.Printf("    [FIRING] pay-service: %d 条告警触发（%d critical / %d warning）\n",
		len(kept), countSev(kept, "critical"), countSev(kept, "warning"))
	for _, n := range keptByName {
		fmt.Printf("      - %s × %d\n", n.key, len(n.alerts))
	}
}

func countSev(alerts []*alert, sev string) int {
	n := 0
	for _, a := range alerts {
		if a.severity == sev {
			n++
		}
	}
	return n
}
