package gateentry

import "net"

// Listen 在 address 上监听。
// 握手队列长度由系统的 somaxconn 决定。容器里在 compose 中把 net.core.somaxconn 调到 4096，避免同一时刻大量连接被直接拒绝。
func Listen(address string) (net.Listener, error) {
	return net.Listen("tcp", address)
}
