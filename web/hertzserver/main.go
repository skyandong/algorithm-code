package main

import (
	"context"
	"kitexserver/kitex_gen/hello"
	"kitexserver/kitex_gen/hello/helloservice"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/cloudwego/kitex/client"
	"github.com/cloudwego/kitex/pkg/kerrors"
	"github.com/cloudwego/kitex/pkg/transmeta"
	"github.com/cloudwego/kitex/transport"
)

func main() {
	
	// 连接 Kitex RPC 服务
	cli, err := helloservice.NewClient("hello", client.WithHostPorts("127.0.0.1:8888"), client.WithTransportProtocol(transport.TTHeader),
		client.WithMetaHandler(transmeta.ClientTTHeaderHandler))
	if err != nil {
		panic(err)
	}

	h := server.Default(server.WithHostPorts("127.0.0.1:8080"))

	// HTTP 接口调用 RPC
	h.GET("/hello", func(c context.Context, ctx *app.RequestContext) {
		req := &hello.HelloRequest{
			Name: "nihao",
		}
		resp, err := cli.Hello(c, req)
		if err != nil {
			// 👇 这是唯一能解开 Kitex 业务错误的方法
			biz, ok := kerrors.FromBizStatusError(err)
			if ok {
				// 拿到你定义的 错误码 + 消息
				ctx.JSON(200, utils.H{
					"code": biz.BizStatusCode(),
					"msg":  biz.BizMessage(),
				})
				return
			}

			// 其他错误
			ctx.JSON(500, utils.H{"msg": err.Error()})
			return
		}

		ctx.JSON(consts.StatusOK, utils.H{
			"message": resp,
		})
	})

	h.Spin()
}
