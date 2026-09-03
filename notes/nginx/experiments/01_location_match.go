// # location 匹配优先级验证
//
// 对应笔记: 01-核心架构与配置.md「location 匹配」
//
// 优先级: = 精确 > ^~ 前缀(跳过正则) > ~ 正则 > 最长前缀 > /
//
// conf/nginx.conf 里写了 5 个 location,本实验逐个路径断言命中的是哪一个:
//
//	/exact       → = /exact        精确匹配,命中即停
//	/exact/sub   → /               = 只匹配整串,子路径落到兜底(反代到后端)
//	/images/a.png→ ^~ /images/     ^~ 命中且跳过正则,不被 ~ \.php$ 干扰
//	/images      → /images         普通前缀(^~ /images/ 比路径长,匹配不上)
//	/index.php   → ~ \.php$        正则命中
//	/other/x     → /               兜底反代到后端,后端回 match:backend
package main

import (
	"fmt"
	"io"
	"net/http"
)

const base = "http://127.0.0.1:8080"

func getBody(url string) string {
	resp, err := http.Get(url)
	if err != nil {
		return "ERROR: " + err.Error() + " (nginx 起了吗?)"
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func Exp1LocationMatch() {
	cases := []struct {
		path string
		want string
	}{
		{"/exact", "match:exact"},
		{"/exact/sub", "match:backend"}, // = 是整串匹配,子路径不命中
		{"/images/a.png", "match:prefix-caret"},
		{"/images", "match:prefix"},
		{"/index.php", "match:regex"},
		{"/other/x", "match:backend"},
	}

	pass, fail := 0, 0
	for _, c := range cases {
		got := getBody(base + c.path)
		status := "PASS"
		if len(got) >= len(c.want) && got[:len(c.want)] == c.want {
			pass++
		} else {
			status = "FAIL"
			fail++
		}
		fmt.Printf("  [%s] %-16s → %s\n", status, c.path, got)
	}
	fmt.Printf("  结果: %d 通过, %d 失败\n\n", pass, fail)
}
