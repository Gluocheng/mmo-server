package gateentry

import (
	"net"
	"testing"
)

func TestListenAccepts(t *testing.T) {
	ln, err := Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	done := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 4)
		_, _ = conn.Read(buf)
		_, _ = conn.Write([]byte("pong"))
		close(done)
	}()
	conn, err := net.Dial("tcp", ln.Addr().String())
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
	if string(buf) != "pong" {
		t.Fatalf("buf %q", buf)
	}
	<-done
}
