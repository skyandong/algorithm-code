// # 证书生成辅助
//
// 所有实验共用：在内存里生成一套完整的 PKI（根 CA → 中间 CA → 叶子证书）。
// 对应笔记《05-证书体系》。
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"time"
)

// certKeyPair 一张证书 + 配套私钥
type certKeyPair struct {
	Cert *x509.Certificate
	Key  *ecdsa.PrivateKey
	// DER 编码，用于 tls.Certificate
	CertDER []byte
}

// genCA 生成一张 CA 证书（可签发下级证书）
//
// 关键点：IsCA=true + BasicConstraintsValid，这是"能签别人的证书"的标志，
// 证书链验证时 x509 会检查这个字段。
func genCA(cn string, days int) *certKeyPair {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(0, 0, days),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, _ := x509.CreateCertificate(rand.Reader, tpl, tpl, key.Public(), key)
	cert, _ := x509.ParseCertificate(der)
	return &certKeyPair{Cert: cert, Key: key, CertDER: der}
}

// genLeaf 用上级 CA 签发一张叶子证书（服务端证书）
//
// 关键点：
//   - DNSNames 就是证书的 SAN（Subject Alternative Name）
//     浏览器/客户端做"域名匹配"看的就是它，不是 CommonName
//   - KeyUsage 含 KeyEncipherment/DigitalSignature，不含 CertSign
//     —— 叶子证书不能再签别人
func genLeaf(cn string, dnsNames []string, days int, parent *certKeyPair) *certKeyPair {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: cn},
		DNSNames:     dnsNames,
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(0, 0, days),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, _ := x509.CreateCertificate(rand.Reader, tpl, parent.Cert, key.Public(), parent.Key)
	cert, _ := x509.ParseCertificate(der)
	return &certKeyPair{Cert: cert, Key: key, CertDER: der}
}

// genChain 生成 完整三级链：Root CA → Intermediate CA → Leaf
//
// 对应真实世界：
//   Root CA        预装在操作系统/浏览器里（离线保管，十年有效）
//   Intermediate   Let's Encrypt 这类在线签发机构
//   Leaf           你的网站证书（几周~几个月有效）
func genChain() (root, inter, leaf *certKeyPair) {
	root = genCA("Demo Root CA", 3650)
	inter = genLeaf("Demo Intermediate CA", nil, 1825, root)
	// 中间 CA 要有签发权：改一下 IsCA（genLeaf 生成的没有 CA 权限，这里单独做）
	iKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	iTpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano() + 1),
		Subject:               pkix.Name{CommonName: "Demo Intermediate CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(0, 0, 1825),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	iDer, _ := x509.CreateCertificate(rand.Reader, iTpl, root.Cert, iKey.Public(), root.Key)
	iCert, _ := x509.ParseCertificate(iDer)
	inter = &certKeyPair{Cert: iCert, Key: iKey, CertDER: iDer}

	leaf = genLeaf("www.demo.local", []string{"www.demo.local", "demo.local"}, 90, inter)
	return
}

// expiredLeaf 签发一张已过期的叶子证书（用于演示验证失败）
func expiredLeaf(parent *certKeyPair) *certKeyPair {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "expired.demo.local"},
		DNSNames:     []string{"expired.demo.local"},
		NotBefore:    time.Now().AddDate(0, 0, -30),
		NotAfter:     time.Now().AddDate(0, 0, -1), // 昨天就过期了
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, _ := x509.CreateCertificate(rand.Reader, tpl, parent.Cert, key.Public(), parent.Key)
	cert, _ := x509.ParseCertificate(der)
	return &certKeyPair{Cert: cert, Key: key, CertDER: der}
}
