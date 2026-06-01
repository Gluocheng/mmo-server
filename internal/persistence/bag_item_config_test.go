package persistence

import (
	"errors"
	"testing"
)

func TestAddOrStackItemNotInConfig(t *testing.T) {
	resetBagTestDB(t)
	const playerID int64 = 9001
	err := AddOrStackItem(playerID, 99999, 1)
	if !errors.Is(err, ErrItemNotFound) {
		t.Fatalf("expected ErrItemNotFound, got %v", err)
	}
}

func TestAddOrStackItemRespectsConfigMaxStack(t *testing.T) {
	resetBagTestDB(t)
	const playerID int64 = 9002
	// SeedTestItems 默认 max_stack=9999；单独测 effectiveMaxStack 在 runtime 单测覆盖。
	if err := AddOrStackItem(playerID, 1, 9999); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := AddOrStackItem(playerID, 1, 1); err != nil {
		t.Fatalf("add second stack: %v", err)
	}
}
