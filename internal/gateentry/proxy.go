package gateentry

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"
)

// Config 是对外入口。Listen 是客户端连接的地址，网关地址来自 profile 里启用的 node.gate。
type Config struct {
	Listen                string
	MaxConnectionsPerGate int
	Gates                 []Gate
}

type profileFile struct {
	GateEntry struct {
		Listen                string `json:"listen"`
		MaxConnectionsPerGate int    `json:"max_connections_per_gate"`
	} `json:"gate_entry"`
	Node struct {
		Gate []struct {
			NodeID  string `json:"node_id"`
			Address string `json:"address"`
			Enable  bool   `json:"enable"`
		} `json:"gate"`
	} `json:"node"`
}

// LoadConfig 读取 profile。没有启用的网关时返回错误。
func LoadConfig(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var raw profileFile
	if err := json.Unmarshal(b, &raw); err != nil {
		return Config{}, err
	}
	cfg := Config{
		Listen:                strings.TrimSpace(raw.GateEntry.Listen),
		MaxConnectionsPerGate: raw.GateEntry.MaxConnectionsPerGate,
	}
	if cfg.Listen == "" {
		cfg.Listen = ":10100"
	}
	if cfg.MaxConnectionsPerGate < 1 {
		cfg.MaxConnectionsPerGate = 2000
	}
	for _, g := range raw.Node.Gate {
		if !g.Enable {
			continue
		}
		addr := listenAddr(g.Address)
		if addr == "" || g.NodeID == "" {
			continue
		}
		cfg.Gates = append(cfg.Gates, Gate{
			ID: g.NodeID, Address: addr, Max: cfg.MaxConnectionsPerGate,
		})
	}
	if len(cfg.Gates) == 0 {
		return Config{}, fmt.Errorf("no enabled gate in %s", path)
	}
	return cfg, nil
}

func listenAddr(address string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		return ""
	}
	if strings.HasPrefix(address, ":") {
		return "127.0.0.1" + address
	}
	return address
}

// Serve 接受客户端连接并转发到选中的网关。全部满员时返回 HTTP 503。
func Serve(ln net.Listener, bal *Balancer) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go handle(conn, bal)
	}
}

func handle(client net.Conn, bal *Balancer) {
	defer client.Close()
	addr, release, ok := bal.Acquire()
	if !ok {
		rejectBusy(client)
		return
	}
	defer release()
	backend, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		rejectBusy(client)
		return
	}
	defer backend.Close()
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(backend, client)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(client, backend)
		done <- struct{}{}
	}()
	<-done
}

func rejectBusy(conn net.Conn) {
	_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, _ = io.WriteString(conn, "HTTP/1.1 503 Service Unavailable\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
}
