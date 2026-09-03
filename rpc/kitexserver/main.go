package main

import (
	"kitexserver/handle/helo"
	"log"
	"net"

	"github.com/cloudwego/kitex/pkg/transmeta"
	"github.com/cloudwego/kitex/server"
	"kitexserver/kitex_gen/hello/helloservice"
)

func main() {
	addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:8888")
	if err != nil {
		log.Fatal(err)
	}

	svr := server.NewServer(
		server.WithServiceAddr(addr),
		server.WithMetaHandler(transmeta.ServerTTHeaderHandler),
	)
	err = svr.RegisterService(helloservice.NewServiceInfo(), new(helo.HelloServiceImpl))
	if err != nil {
		log.Fatal(err)
	}

	err = svr.Run()
	if err != nil {
		log.Println(err.Error())
	}
}
