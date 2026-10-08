package gateentry

import (
	"net"
	"testing"
)

func TestLoadConfigUsesEnabledGates(t *testing.T) {
	cfg, err := LoadConfig("../../configs/mmo-cluster.json")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != ":10100" {
		t.Fatalf("listen %s", cfg.Listen)
	}
	if len(cfg.Gates) != 2 || cfg.Gates[0].Address != "127.0.0.1:10110" || cfg.Gates[1].Address != "127.0.0.1:10111" {
		t.Fatalf("gates %+v", cfg.Gates)
	}
}

func TestProxyReachesChosenGate(t *testing.T) {
	backend, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	got := make(chan string, 1)
	go func() {
		conn, err := backend.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 4)
		n, _ := conn.Read(buf)
		got <- string(buf[:n])
		_, _ = conn.Write([]byte("pong"))
	}()
	front, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer front.Close()
	bal := NewBalancer([]Gate{{ID: "g", Address: backend.Addr().String(), Max: 2}})
	go Serve(front, bal)
	conn, err := net.Dial("tcp", front.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4)
	if _, err := conn.Read(buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "pong" || <-got != "ping" {
		t.Fatalf("buf %q", buf)
	}
}
