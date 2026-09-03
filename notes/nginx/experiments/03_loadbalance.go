// # 负载均衡分布验证:轮询策略
//
// 对应笔记: 02-反向代理与负载均衡.md「upstream」
//
// conf 里 upstream backend 配了 127.0.0.1:8081 和 8082 两个等权 server,
// 默认轮询(round-robin):打 10 发,预期两个后端各接约 5 个。
//
// 想验证 weight:把 conf 里 8081 加 weight=3,reload 后重跑,预期约 7:3。
// 想验证被动健康检查:杀掉 8082 进程,观察请求全落 8081 且 error.log 出现
// "upstream server temporarily disabled"。
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func Exp3LoadBalance() {
	count := map[string]int{}
	const total = 10

	for i := 0; i < total; i++ {
		resp, err := http.Get("http://127.0.0.1:8080/hello")
		if err != nil {
			fmt.Printf("  请求 %d 失败: %v\n", i, err)
			continue
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var r struct {
			Backend string `json:"backend"`
		}
		if json.Unmarshal(b, &r) == nil && r.Backend != "" {
			count[r.Backend]++
		}
	}

	fmt.Printf("  共 %d 次请求,分布: %v\n", total, count)
	fmt.Println("  预期: 轮询下两实例各约 " + fmt.Sprint(total/2) + " 次")
	fmt.Println("  延伸: 后端挂一个再跑,nginx 自动摘除(被动健康检查),error.log 可见记录")
}
