/*
# RESP 协议与 Pipelining

## RESP 是什么
RESP(REdis Serialization Protocol)是 Redis 客户端和服务端之间的 TCP 线协议。
文本格式,人能读,用首字节区分类型:

	+ Simple String   →  +OK\r\n
	- Error           →  -ERR unknown command\r\n
	: Integer         →  :1000\r\n
	$ Bulk String     →  $5\r\nhello\r\n          (带长度,二进制安全)
	* Array           →  *3\r\n$3\r\nSET\r\n...   (命令就是字符串数组)

SET name redis 在协议层的真实字节:

	*3\r\n$3\r\nSET\r\n$4\r\nname\r\n$5\r\nredis\r\n

## 为什么 Pipelining 能省 RTT
Pipelining 不是协议特性,是客户端用法:
把 N 条命令的字节拼好一次 write(),再一次性 read() 所有回包。
服务端按序执行、按序回包,客户端按序解析。
省的就是那 N-1 次网络往返。

	逐条:  write → flush → read | write → flush → read | ...   N 次 RTT
	pipeline: write×N → flush → read×N                          1 次 RTT

## 实验
本文件对比 10000 次 SET:
- 逐条:每条 write+flush+read
- pipeline:攒齐后一次 flush,再一次性读所有回包
*/
package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

const (
	addr     = "localhost:6379"
	password = "123456" // 没密码就改空串, AUTH 会自动跳过
	n        = 10000
)

// Client 就是一个 TCP 连接 + 读写缓冲. 不依赖任何库.
type Client struct {
	net.Conn
	br *bufio.Reader
	bw *bufio.Writer
}

func dial() *Client {
	c, err := net.Dial("tcp", addr)
	if err != nil {
		panic(err)
	}
	cl := &Client{Conn: c, br: bufio.NewReader(c), bw: bufio.NewWriter(c)}

	// AUTH (没设密码时 Redis 会回 "no password is set", 忽略即可)
	cl.writeCmd("AUTH", password)
	cl.bw.Flush()
	rep := cl.readReply()
	if strings.HasPrefix(rep, "-") && !strings.Contains(rep, "no password is set") {
		panic("AUTH 失败: " + rep)
	}
	return cl
}

// encode 把一条命令编码成 RESP 字节.
// SET k v  ->  *3\r\n$3\r\nSET\r\n$1\r\nk\r\n$1\r\nv\r\n
func encode(cmd string, args ...string) []byte {
	parts := append([]string{cmd}, args...)
	var b []byte
	b = append(b, '*')
	b = append(b, []byte(fmt.Sprintf("%d\r\n", len(parts)))...)
	for _, s := range parts {
		b = append(b, '$')
		b = append(b, []byte(fmt.Sprintf("%d\r\n", len(s)))...)
		b = append(b, s...)
		b = append(b, "\r\n"...)
	}
	return b
}

func (c *Client) writeCmd(cmd string, args ...string) {
	c.bw.Write(encode(cmd, args...))
}

// readReply 读一个 RESP 回包. 简化版: 只取值, 不解析数组.
func (c *Client) readReply() string {
	line, err := c.br.ReadString('\n')
	if err != nil {
		panic(err)
	}
	line = strings.TrimRight(line, "\r\n")
	switch line[0] {
	case '+', '-':
		return line
	case ':':
		return line[1:]
	case '$':
		var length int
		fmt.Sscanf(line[1:], "%d", &length)
		if length < 0 {
			return "<nil>"
		}
		buf := make([]byte, length)
		io.ReadFull(c.br, buf)
		c.br.ReadByte() // \r
		c.br.ReadByte() // \n
		return string(buf)
	default:
		return line
	}
}

func main() {
	c := dial()
	defer c.Close()

	// 先看一条命令在协议层长什么样
	fmt.Printf("SET k 0 的 RESP 字节: %q\n", string(encode("SET", "k", "0")))
	fmt.Println("(即 *3\\r\\n$3\\r\\nSET\\r\\n$1\\r\\nk\\r\\n$1\\r\\n0\\r\\n)\n")

	// 逐条: 每条都 write + flush + read, 一个 RTT 跑一次
	start := time.Now()
	for i := 0; i < n; i++ {
		c.writeCmd("SET", fmt.Sprintf("seq:%d", i), fmt.Sprintf("%d", i))
		c.bw.Flush()
		c.readReply()
	}
	seq := time.Since(start)

	// pipeline: N 条命令全 write 进缓冲, flush 一次, 再一次性读 N 个回包
	start = time.Now()
	for i := 0; i < n; i++ {
		c.writeCmd("SET", fmt.Sprintf("pipe:%d", i), fmt.Sprintf("%d", i))
	}
	c.bw.Flush()
	for i := 0; i < n; i++ {
		c.readReply()
	}
	pipe := time.Since(start)

	fmt.Printf("逐条 %d 次:     %v\n", n, seq)
	fmt.Printf("pipeline %d 次: %v\n", n, pipe)
	fmt.Printf("提速: %.1fx  <- pipeline 只等 1 次 RTT, 逐条等了 %d 次\n",
		float64(seq.Nanoseconds())/float64(pipe.Nanoseconds()), n)
}
