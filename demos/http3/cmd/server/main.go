package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"time"

	"github.com/quic-go/quic-go/http3"
)

func main() {
	// 生成自签名证书
	certFile := "localhost.pem"
	keyFile := "localhost-key.pem"
	if _, err := os.Stat(certFile); os.IsNotExist(err) {
		generateCert(certFile, keyFile)
	}

	// 路由
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello HTTP/3! 协议: %s\n", r.Proto)
	})
	mux.HandleFunc("/time", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"time": "%s"}`, time.Now().Format(time.RFC3339))
	})

	// 启动 HTTP/3 服务器
	srv := http3.Server{
		Addr:    ":4433",
		Handler: mux,
	}

	fmt.Println("🚀 HTTP/3 服务器启动: https://localhost:4433")
	fmt.Println("   测试方式:")
	fmt.Println("   curl --http3 -k https://localhost:4433/")
	fmt.Println("   curl --http3 -k https://localhost:4433/time")
	fmt.Println()

	log.Fatal(srv.ListenAndServeTLS(certFile, keyFile))
}

func generateCert(certFile, keyFile string) {
	priv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		DNSNames:     []string{"localhost", "127.0.0.1"},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)

	certOut, _ := os.Create(certFile)
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	certOut.Close()

	keyOut, _ := os.Create(keyFile)
	keyBytes, _ := x509.MarshalECPrivateKey(priv)
	pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	keyOut.Close()

	fmt.Println("✅ 已生成自签名证书:", certFile, keyFile)
}
