package helo

import (
	"context"

	"github.com/cloudwego/kitex/pkg/kerrors"
	"kitexserver/kitex_gen/hello"
)

// Hello implements the HelloServiceImpl interface.
func (s *HelloServiceImpl) Hello(ctx context.Context, req *hello.HelloRequest) (resp *hello.HelloResponse, err error) {
	resp = &hello.HelloResponse{
		Message: "hello " + req.Name,
	}

	return resp, kerrors.NewBizStatusError(400000000, "用户不存在")
}
