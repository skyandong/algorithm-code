namespace go hello

// 先定义请求结构体
struct HelloRequest {
    1: string name
}

// 再定义响应结构体
struct HelloResponse {
    1: string message
}

service HelloService {
    // 传入结构体，返回结构体
    HelloResponse Hello(1: HelloRequest req)
}