package gateentry

import "testing"

func TestAcquirePrefersFewerConnections(t *testing.T) {
	b := NewBalancer([]Gate{
		{ID: "gate-1", Address: "127.0.0.1:10110", Active: 3, Max: 5},
		{ID: "gate-2", Address: "127.0.0.1:10111", Active: 1, Max: 5},
	})
	addr, release, ok := b.Acquire()
	if !ok || addr != "127.0.0.1:10111" {
		t.Fatalf("addr %s ok %v", addr, ok)
	}
	release()
	addr, release, ok = b.Acquire()
	if !ok || addr != "127.0.0.1:10111" {
		t.Fatalf("after release %s", addr)
	}
	release()
}

func TestAcquireSkipsFullGate(t *testing.T) {
	b := NewBalancer([]Gate{
		{ID: "gate-1", Address: "127.0.0.1:10110", Max: 1},
		{ID: "gate-2", Address: "127.0.0.1:10111", Max: 1},
	})
	_, rel1, ok := b.Acquire()
	if !ok {
		t.Fatal("first")
	}
	_, rel2, ok := b.Acquire()
	if !ok {
		t.Fatal("second")
	}
	if _, _, ok := b.Acquire(); ok {
		t.Fatal("expected full")
	}
	rel1()
	addr, rel3, ok := b.Acquire()
	if !ok || addr == "" {
		t.Fatal("after one release")
	}
	rel2()
	rel3()
}
