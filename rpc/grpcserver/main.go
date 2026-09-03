package main

import (
	"context"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/anypb"

	"grpcserver/idl/err"
	"grpcserver/idl/hello"
)

// MyError
// 传入：自定义业务码 + 消息
// 返回：gRPC 标准结构化错误（status + Any 详情），客户端用 status.FromError 解包
func MyError(bizCode int32, msg string) error {
	errDetail := &err.BizError{
		Code: bizCode,
		Msg:  msg,
	}
	anyDetail, _ := anypb.New(errDetail)
	st := status.New(codes.FailedPrecondition, msg)
	st, _ = st.WithDetails(anyDetail)
	return st.Err()
}

type server struct {
	hello.UnimplementedHelloServiceServer
}

// ① 一元：保留原有的结构化错误演示
func (s *server) SayHello(
	ctx context.Context,
	req *hello.HelloRequest,
) (*hello.HelloReply, error) {
	log.Println("[业务] SayHello 收到：", req.Name)
	return nil, MyError(2334455, "hahahhaaxixixi")
}

// ② 服务端流：一问多答。像"往水管里写 N 次"，客户端逐条 Recv。
//    典型场景：列表拉取、行情推送、大结果分批下发。
func (s *server) StreamHello(req *hello.HelloRequest, stream hello.HelloService_StreamHelloServer) error {
	log.Println("[业务] StreamHello 开始推送")
	for i := 1; i <= 3; i++ {
		if err := stream.Send(&hello.HelloReply{
			Code:    int32(i),
			Message: "第" + strconv.Itoa(i) + "条推送给" + req.Name,
		}); err != nil {
			return err // 客户端断开时 Send 报错，流生命周期结束
		}
		time.Sleep(500 * time.Millisecond) // 模拟逐条产生的延迟
	}
	return nil // return = 服务端关流（客户端 Recv 返回 io.EOF）
}

// ③ 客户端流：多问一答。一直 Recv 直到客户端关流，最后回一个汇总。
//    典型场景：批量上报、文件分块上传。
func (s *server) UploadHello(stream hello.HelloService_UploadHelloServer) error {
	var names []string
	for {
		req, err := stream.Recv()
		if err == io.EOF { // 客户端 CloseSend → 数据收完
			summary := &hello.HelloSummary{Count: int32(len(names)), Names: strings.Join(names, ",")}
			log.Println("[业务] UploadHello 收满：", summary.Names)
			return stream.SendAndClose(summary) // 客户端流的"回包"用 SendAndClose
		}
		if err != nil {
			return err
		}
		names = append(names, req.Name)
	}
}

// ④ 双向流：Send/Recv 互不阻塞，先后顺序完全由业务定。
//    这里实现最简单的 echo：读到一条回一条；客户端关流后服务端跟着关。
func (s *server) ChatHello(stream hello.HelloService_ChatHelloServer) error {
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := stream.Send(&hello.HelloReply{Code: 0, Message: "echo:" + req.Name}); err != nil {
			return err
		}
	}
}

func main() {
	lis, err := net.Listen("tcp", ":8888")
	if err != nil {
		log.Fatal(err)
	}

	// 挂拦截器：Chain* 按声明顺序执行（类比 Gin 的 middleware 链）
	s := grpc.NewServer(
		grpc.ChainUnaryInterceptor(unaryLogging),
		grpc.ChainStreamInterceptor(streamLogging),
	)

	hello.RegisterHelloServiceServer(s, &server{})

	startTLSListener() // 8889: TLS 版监听（有证书才起）

	log.Println("gRPC 服务运行在 :8888（四种流模式 + 拦截器）")
	if err := s.Serve(lis); err != nil {
		log.Fatal(err)
	}
}
