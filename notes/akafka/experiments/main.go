// # Kafka 实验统一入口
//
// 对应笔记：notes/akafka/01-架构.md ~ 10-选型对比.md
// 实验代码：01_producer_basic.go ~ 11_health.go（本目录内）
//
// 用法（先 make up 启动 Kafka）：
//
//	go run ./experiments/ basic       # 案例1：生产者基础（同步/异步/批量发送）
//	go run ./experiments/ group       # 案例2：消费者组（Ctrl+C 退出）
//	go run ./experiments/ eos         # 案例3：Exactly-Once（幂等 + 事务）
//	go run ./experiments/ dlq         # 案例4：死信队列（Ctrl+C 退出）
//	go run ./experiments/ lag         # 案例5：消费积压监控（Ctrl+C 退出）
//	go run ./experiments/ pipeline    # 案例6：多 Topic 流水线（Ctrl+C 退出）
//	go run ./experiments/ ordering    # 案例7：顺序性保证（key 路由 + 消费端保序）
//	go run ./experiments/ storage     # 案例8：存储与读路径（offset/时间戳定位）
//	go run ./experiments/ concurrent  # 案例9：位移提交竞态 vs 滑动窗口
//	go run ./experiments/ skew        # 案例10：分区倾斜观测
//	go run ./experiments/ health      # 案例11：集群健康巡检
//	go run ./experiments/ all         # 运行可自动结束的案例（1、3、7~11）
//
// 等价于 Makefile 的 make run-01 ~ run-11。
package main

import (
	"fmt"
	"net"
	"os"
	"time"
)

// brokers 所有案例共享的 Kafka 地址。
const brokers = "localhost:9092"

type kafkaCase struct {
	name   string
	run    func()
	desc   string
	daemon bool // 常驻型（Ctrl+C 退出），不参与 all
}

var cases = []kafkaCase{
	{"basic", RunProducerBasic, "案例1：生产者基础（同步/异步/批量发送、acks、回调）", false},
	{"group", RunConsumerGroup, "案例2：消费者组（手动提交 Offset、Rebalance 感知，Ctrl+C 退出）", true},
	{"eos", RunExactlyOnce, "案例3：Exactly-Once（幂等生产者、事务）", false},
	{"dlq", RunDeadLetter, "案例4：死信队列（重试退避、DLQ、Header，Ctrl+C 退出）", true},
	{"lag", RunConsumerLag, "案例5：消费积压监控（kadm、HW vs CommitOffset，Ctrl+C 退出）", true},
	{"pipeline", RunPipeline, "案例6：多 Topic 流水线（下单→支付→通知→审计，Ctrl+C 退出）", true},
	{"ordering", RunOrdering, "案例7：顺序性保证（key 路由、消费端保序）", false},
	{"storage", RunStorage, "案例8：存储与读路径（start/end offset、时间戳定位、日志段）", false},
	{"concurrent", RunConcurrent, "案例9：位移提交竞态 vs 滑动窗口", false},
	{"skew", RunPartitionSkew, "案例10：分区倾斜观测（均匀 key vs 热点 key）", false},
	{"health", RunHealth, "案例11：集群健康巡检（broker/ISR/lag）", false},
}

func main() {
	if len(os.Args) < 2 {
		usage()
		return
	}
	exp := os.Args[1]

	checkKafka()

	if exp == "all" {
		for _, c := range cases {
			if !c.daemon {
				runCase(c)
			}
		}
		fmt.Println("\n常驻型案例（group/dlq/lag/pipeline）需要 Ctrl+C 退出，请单独运行：")
		for _, c := range cases {
			if c.daemon {
				fmt.Printf("  go run ./experiments/ %s\n", c.name)
			}
		}
		return
	}

	for _, c := range cases {
		if c.name == exp {
			runCase(c)
			return
		}
	}
	usage()
}

func usage() {
	fmt.Println("用法: go run ./experiments/ [basic|group|eos|dlq|lag|pipeline|ordering|storage|concurrent|skew|health|all]")
	fmt.Println()
	for _, c := range cases {
		fmt.Printf("  %-9s %s\n", c.name, c.desc)
	}
	fmt.Println()
	fmt.Println("提示: 先 make up 启动 Kafka（docker compose），再运行实验；常驻类实验用 Ctrl+C 退出。")
}

// checkKafka 轻量探测本地 9092 端口，未启动时给出提示（不强制拦截）。
func checkKafka() {
	conn, err := net.DialTimeout("tcp", brokers, 300*time.Millisecond)
	if err != nil {
		fmt.Println("⚠ 未检测到 localhost:9092 上的 Kafka，请先执行: make up")
		return
	}
	conn.Close()
}

func runCase(c kafkaCase) {
	fmt.Printf("\n===== %s =====\n", c.desc)
	c.run()
}
