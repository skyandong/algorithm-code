// # 限流验证:漏桶的 rate / burst / nodelay 语义
//
// 对应笔记: 03-限流与安全.md「limit_req」
//
// conf(8083): limit_req zone=req_limit rate=5r/s burst=10 nodelay; limit_req_status 429
//
// 预期: 瞬发 20 个请求 ——
//
//	漏桶没有"rate 窗口额度"概念,rate 只决定漏出速度,不提供初始配额。
//	放行数由 burst 决定: 第 n 个请求的 excess = n,超过 burst=10 即拒。
//	即约 10 个放行(burst 容量,nodelay 让它们不等排队),
//	429 的个数 ≈ 20 - 10 = 10 个左右(有毫秒级平滑误差)。
//
// 变体实验(改 conf 后 reload 重跑):
//
//	去掉 nodelay → 请求变慢但不 429(在 nginx 里排队,响应延迟可见)
//	rate=5r/m    → 更严,注意是每分钟 5 个
package main

import (
	"fmt"
	"net/http"
	"sync"
)

func Exp4RateLimit() {
	const total = 20
	statuses := make([]int, total)
	var wg sync.WaitGroup

	client := &http.Client{Transport: &http.Transport{MaxConnsPerHost: total}}
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			resp, err := client.Get("http://127.0.0.1:8083/hello")
			if err != nil {
				statuses[i] = -1
				return
			}
			statuses[i] = resp.StatusCode
			resp.Body.Close()
		}(i)
	}
	wg.Wait()

	ok, limited, other := 0, 0, 0
	for _, s := range statuses {
		switch s {
		case 200:
			ok++
		case 429:
			limited++
		default:
			other++
		}
	}

	fmt.Printf("  瞬发 %d 个并发请求 → 200: %d 个, 429: %d 个, 其他: %d 个\n", total, ok, limited, other)
	fmt.Printf("  符合漏桶+burst=10+nodelay 的预期(放行 ≈ burst=10,其余 429;rate 只决定漏出速度)\n")
	fmt.Println("  变体: conf 里去掉 nodelay 再 reload,429 会消失,但延迟显著上升(排队)")
}
