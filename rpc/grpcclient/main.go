package main

// gRPC 客户端全量演示：
//	① 一元 + 结构化错误解包（原有）
//	② 服务端流 / ③ 客户端流 / ④ 双向流
//	⑤ metadata 附加自定义头（服务端拦截器会读 x-uid）
//	⑥ deadline 超时控制（ctx 到期，服务端业务被自动取消）
//	⑦ TLS 凭证（8889，信任自签 CA 的正规写法，见 tls.go）

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/anypb"

	errs "grpcserver/idl/err"
	"grpcserver/idl/hello"
)

func main() {
	// 连接服务端（明文 8888）
	conn, err := grpc.NewClient(":8888", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	client := hello.NewHelloServiceClient(conn)

	// ⑤ metadata：类似 HTTP header，拦截器/业务都能读
	ctx := metadata.AppendToOutgoingContext(context.Background(), "x-uid", "skyandong")

	demoUnary(ctx, client)
	demoServerStream(ctx, client)
	demoClientStream(ctx, client)
	demoBidiStream(ctx, client)
	demoDeadline(client)
	demoTLS()
}

// ① 一元 + 错误详情解包：status.FromError → 遍历 details → Any 解包成 BizError
func demoUnary(ctx context.Context, client hello.HelloServiceClient) {
	fmt.Println("------ ① 一元 + 结构化错误 ------")
	resp, err := client.SayHello(ctx, &hello.HelloRequest{Name: "grpc"})
	if err != nil {
		st, _ := status.FromError(err)
		for _, detail := range st.Details() {
			anyMsg, ok := detail.(*anypb.Any)
			if !ok {
				continue
			}
			var bizErr errs.BizError
			if err := anyMsg.UnmarshalTo(&bizErr); err == nil {
				fmt.Printf("业务码：%d，错误信息：%s\n", bizErr.Code, bizErr.Msg)
				return
			}
		}
		fmt.Println("未解出业务详情:", st.String())
		return
	}
	log.Println("响应：", resp)
}

// ② 服务端流：一次请求，循环 Recv 到 io.EOF
func demoServerStream(ctx context.Context, client hello.HelloServiceClient) {
	fmt.Println("------ ② 服务端流（一问多答） ------")
	stream, err := client.StreamHello(ctx, &hello.HelloRequest{Name: "andong"})
	if err != nil {
		log.Fatal(err)
	}
	for {
		reply, err := stream.Recv()
		if err == io.EOF { // 服务端 return = 关流
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("收到推送: [%d] %s\n", reply.Code, reply.Message)
	}
}

// ③ 客户端流：循环 Send，CloseAndRecv 收唯一一个汇总回包
func demoClientStream(ctx context.Context, client hello.HelloServiceClient) {
	fmt.Println("------ ③ 客户端流（多问一答） ------")
	stream, err := client.UploadHello(ctx)
	if err != nil {
		log.Fatal(err)
	}
	for _, name := range []string{"a", "b", "c"} {
		if err := stream.Send(&hello.HelloRequest{Name: name}); err != nil {
			log.Fatal(err)
		}
	}
	summary, err := stream.CloseAndRecv() // 关发送方向 + 等汇总
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("汇总: 共 %d 个 -> %s\n", summary.Count, summary.Names)
}

// ④ 双向流：两个 goroutine 各管一头，互不阻塞
func demoBidiStream(ctx context.Context, client hello.HelloServiceClient) {
	fmt.Println("------ ④ 双向流（边发边收） ------")
	stream, err := client.ChatHello(ctx)
	if err != nil {
		log.Fatal(err)
	}

	go func() { // 发送方
		for _, name := range []string{"x1", "x2", "x3"} {
			stream.Send(&hello.HelloRequest{Name: name})
		}
		stream.CloseSend() // 发完关发送方向，服务端 Recv 会拿到 EOF
	}()

	for { // 接收方
		reply, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("收到:", reply.Message)
	}
}

// ⑥ deadline：ctx 带超时。StreamHello 每条间隔 500ms，
//    700ms 后 ctx 到期 → Recv 返回 DeadlineExceeded，
//    服务端 Send 也会因 ctx 取消而失败 —— 超时是全链路取消，不是客户端单方面断开。
func demoDeadline(client hello.HelloServiceClient) {
	fmt.Println("------ ⑥ deadline 超时 ------")
	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()

	stream, err := client.StreamHello(ctx, &hello.HelloRequest{Name: "timeout-demo"})
	if err != nil {
		log.Fatal(err)
	}
	for {
		reply, err := stream.Recv()
		if err != nil {
			st, _ := status.FromError(err)
			fmt.Printf("超时退出: code=%s(%v)\n", st.Code(), st.Message())
			break
		}
		fmt.Println("超时前收到:", reply.Message)
	}
}

// ⑦ TLS：信任自签 CA + 校验 SAN 的正规写法（tls.go 提供凭证）
func demoTLS() {
	fmt.Println("------ ⑦ TLS 凭证 ------")
	creds := tlsCreds()
	if creds == nil {
		fmt.Println("未找到证书（notes/nginx/conf/gen-certs.sh 生成后重试），跳过")
		return
	}

	conn, err := grpc.NewClient("localhost:8889", grpc.WithTransportCredentials(creds))
	if err != nil {
		log.Println(err)
		return
	}
	defer conn.Close()

	stream, err := hello.NewHelloServiceClient(conn).StreamHello(
		metadata.AppendToOutgoingContext(context.Background(), "x-uid", "tls-user"),
		&hello.HelloRequest{Name: "tls-user"},
	)
	if err != nil {
		fmt.Println("TLS 调用失败:", err)
		return
	}
	for {
		reply, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("TLS 加密通道收到:", reply.Message)
	}
}
