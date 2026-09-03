package main

// 拦截器（interceptor）：gRPC 版的 middleware。
// 四象限：一元/流式 × 服务端/客户端，服务端最常用的是这两个：
//
//	UnaryInterceptor:  func(ctx, req, info, handler) (resp, error)
//	StreamInterceptor: func(srv, ss 流对象, info, handler) error —— 流对象包一层即可拦截收发
//
// 典型用途：日志、认证（读 metadata）、panic recovery、超时兜底。

import (
	"context"
	"fmt"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

// 一元拦截器：记录耗时 + 演示从 metadata 里拿客户端附加信息
func unaryLogging(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	start := time.Now()
	resp, err := handler(ctx, req)
	log.Printf("[unary] %-20s 耗时 %v err=%v uid=%q from=%s",
		info.FullMethod, time.Since(start), err, uidFrom(ctx), fromAddr(ctx))
	return resp, err
}

// 流式拦截器：handler 里就是业务代码对流的收发，包住它 = 包住整个流的生命周期
func streamLogging(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	start := time.Now()
	err := handler(srv, ss)
	log.Printf("[stream] %-20s 耗时 %v err=%v uid=%q client_stream=%v server_stream=%v",
		info.FullMethod, time.Since(start), err, uidFrom(ss.Context()),
		info.IsClientStream, info.IsServerStream)
	return err
}

// 从 metadata 取客户端带来的自定义头（类似 HTTP header）
func uidFrom(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	if v := md.Get("x-uid"); len(v) > 0 {
		return v[0]
	}
	return ""
}

// 对端地址（连接级信息不走 metadata，走 peer）
func fromAddr(ctx context.Context) string {
	if p, ok := peer.FromContext(ctx); ok {
		return p.Addr.String()
	}
	return fmt.Sprintf("unknown")
}
