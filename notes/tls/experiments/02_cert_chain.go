// # 实验2：证书链验证 —— 亲手做一次浏览器的验签
//
// 生成 Root → Intermediate → Leaf 三级证书链，模拟浏览器验证：
//
//	1. 正常验证：叶子 → 中间 → 根，全部通过
//	2. 缺中间证书：链断了（Half chain 事故）
//	3. 证书过期：notAfter 校验失败
//	4. 域名不匹配：SAN 里没有访问的域名
//	5. 不受信任的根：换一个未知 CA 签发
//
// 对应笔记《05-证书体系》。
package main

import (
	"crypto/x509"
	"fmt"
)

// ExpCertChain 演示证书链验证的成功与各种失败场景。
func ExpCertChain() {
	fmt.Println("=== 实验2：证书链验证 ===")

	root, inter, leaf := genChain()
	roots := newX509Pool(root.Cert)

	verify := func(name string, cert *x509.Certificate, opts x509.VerifyOptions) {
		_, err := cert.Verify(opts)
		if err != nil {
			fmt.Printf("  %-16s ✗ %v\n", name, err)
		} else {
			fmt.Printf("  %-16s ✓ 验证通过\n", name)
		}
	}

	baseOpts := x509.VerifyOptions{
		DNSName: "www.demo.local",
		Roots:   roots,
	}

	fmt.Println("\n--- 场景1：完整证书链（叶子+中间都给到） ---")
	verify("完整链", leaf.Cert, x509.VerifyOptions{
		DNSName:       baseOpts.DNSName,
		Roots:         baseOpts.Roots,
		Intermediates: newX509Pool(inter.Cert),
	})

	fmt.Println("\n--- 场景2：服务器忘了发中间证书（只有叶子） ---")
	verify("缺中间证书", leaf.Cert, baseOpts)

	fmt.Println("\n--- 场景3：证书过期 ---")
	expired := expiredLeaf(inter)
	verify("已过期", expired.Cert, x509.VerifyOptions{
		DNSName:       "expired.demo.local",
		Roots:         baseOpts.Roots,
		Intermediates: newX509Pool(inter.Cert),
	})

	fmt.Println("\n--- 场景4：访问的域名不在证书 SAN 里 ---")
	verify("域名不匹配", leaf.Cert, x509.VerifyOptions{
		DNSName:       "other.site.com",
		Roots:         baseOpts.Roots,
		Intermediates: newX509Pool(inter.Cert),
	})

	fmt.Println("\n--- 场景5：未知 CA 签发（自签名/野CA） ---")
	rogue := genCA("Rogue CA", 365)
	rogueLeaf := genLeaf("www.demo.local", []string{"www.demo.local"}, 90, rogue)
	verify("不受信任的根", rogueLeaf.Cert, x509.VerifyOptions{
		DNSName:       "www.demo.local",
		Roots:         baseOpts.Roots, // 只信任我们的 Root，不认 Rogue CA
		Intermediates: newX509Pool(rogue.Cert),
	})

	fmt.Println()
	fmt.Println("结论:")
	fmt.Println("  - 验证 = 签名逐级上溯到受信任的根 + 有效期 + 域名匹配，缺一不可")
	fmt.Println("  - 场景2 是真实世界最高频事故：Nginx 只配了叶子证书没配 fullchain")
	fmt.Println("  - 场景5 就是中间人攻击的证书：CA 不受信任，浏览器直接拒绝")
}
