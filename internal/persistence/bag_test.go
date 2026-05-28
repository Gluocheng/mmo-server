package persistence

import (
	"context"
	"errors"
	"testing"

	"github.com/example/mmo-server/internal/protocol"
)

func TestAddOrStackItem(t *testing.T) {
	resetDBForTest(t)
	const playerID int64 = 1001

	if err := AddOrStackItem(playerID, 10, 2); err != nil {
		t.Fatalf("first add: %v", err)
	}
	if err := AddOrStackItem(playerID, 10, 3); err != nil {
		t.Fatalf("stack add: %v", err)
	}

	bag, err := GetBagByPlayerID(playerID)
	if err != nil {
		t.Fatalf("get bag: %v", err)
	}
	if len(bag.Items) != 1 || bag.Items[0].ItemId != 10 || bag.Items[0].Count != 5 {
		t.Fatalf("unexpected bag: %+v", bag.Items)
	}
}

func TestRemoveItemNotEnough(t *testing.T) {
	resetDBForTest(t)
	const playerID int64 = 1002

	if err := AddOrStackItem(playerID, 1, 1); err != nil {
		t.Fatalf("add: %v", err)
	}
	err := RemoveItem(playerID, 1, 2)
	if !errors.Is(err, ErrBagNotEnough) {
		t.Fatalf("expected ErrBagNotEnough, got %v", err)
	}
}

func TestRemoveItemDeletesRow(t *testing.T) {
	resetDBForTest(t)
	const playerID int64 = 1003

	if err := AddOrStackItem(playerID, 7, 4); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := RemoveItem(playerID, 7, 4); err != nil {
		t.Fatalf("remove: %v", err)
	}
	bag, err := GetBagByPlayerID(playerID)
	if err != nil {
		t.Fatalf("get bag: %v", err)
	}
	if len(bag.Items) != 0 {
		t.Fatalf("expected empty bag, got %+v", bag.Items)
	}
}

func TestBagTxRollback(t *testing.T) {
	resetDBForTest(t)
	const playerID int64 = 1004
	ctx := context.Background()

	err := WithinTx(ctx, func(txCtx context.Context) error {
		if err := addOrStackItemInTx(txCtx, playerID, 99, 1); err != nil {
			return err
		}
		return errors.New("force rollback")
	})
	if err == nil {
		t.Fatal("expected rollback error")
	}

	bag, err := GetBagByPlayerID(playerID)
	if err != nil {
		t.Fatalf("get bag: %v", err)
	}
	if len(bag.Items) != 0 {
		t.Fatalf("expected empty bag after rollback, got %+v", bag.Items)
	}
}

func TestValidateBagItemMaxStack(t *testing.T) {
	resetDBForTest(t)
	const playerID int64 = 1005

	if err := AddOrStackItem(playerID, 1, MaxBagStack); err != nil {
		t.Fatalf("add max: %v", err)
	}
	// 单槽已满时，同 item_id 会占新槽位（v2 多槽堆叠）
	if err := AddOrStackItem(playerID, 1, 1); err != nil {
		t.Fatalf("add to new slot: %v", err)
	}
	bag, err := GetBagByPlayerID(playerID)
	if err != nil {
		t.Fatalf("get bag: %v", err)
	}
	if len(bag.Items) != 2 {
		t.Fatalf("expected 2 slots, got %+v", bag.Items)
	}
	if err := AddOrStackItem(playerID, 1, MaxBagStack+1); !errors.Is(err, ErrBagInvalid) {
		t.Fatalf("expected ErrBagInvalid when count > max stack, got %v", err)
	}
}

func TestMoveItemToEmptySlot(t *testing.T) {
	resetDBForTest(t)
	const playerID int64 = 2001

	if err := AddOrStackItem(playerID, 5, 3); err != nil {
		t.Fatalf("add: %v", err)
	}
	bag, _ := GetBagByPlayerID(playerID)
	if len(bag.Items) != 1 {
		t.Fatalf("expected 1 stack")
	}
	fromSlot := bag.Items[0].Slot
	if err := MoveItem(playerID, fromSlot, 5); err != nil {
		t.Fatalf("move: %v", err)
	}
	bag, err := GetBagByPlayerID(playerID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(bag.Items) != 1 || bag.Items[0].Slot != 5 || bag.Items[0].Count != 3 {
		t.Fatalf("unexpected after move: %+v", bag.Items)
	}
}

func TestMoveItemSwap(t *testing.T) {
	resetDBForTest(t)
	const playerID int64 = 2002

	if err := AddOrStackItem(playerID, 1, 2); err != nil {
		t.Fatalf("add1: %v", err)
	}
	if err := AddOrStackItem(playerID, 2, 4); err != nil {
		t.Fatalf("add2: %v", err)
	}
	bag, _ := GetBagByPlayerID(playerID)
	if len(bag.Items) != 2 {
		t.Fatalf("expected 2 stacks")
	}
	s0, s1 := bag.Items[0].Slot, bag.Items[1].Slot
	if err := MoveItem(playerID, s0, s1); err != nil {
		t.Fatalf("swap move: %v", err)
	}
	bag, err := GetBagByPlayerID(playerID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	var a, b *protocol.BagItem
	for _, it := range bag.Items {
		if it.Slot == s0 {
			a = it
		}
		if it.Slot == s1 {
			b = it
		}
	}
	if a == nil || b == nil || a.ItemId != 2 || b.ItemId != 1 || a.Count != 4 || b.Count != 2 {
		t.Fatalf("swap failed: %+v", bag.Items)
	}
}

func TestSplitItem(t *testing.T) {
	resetDBForTest(t)
	const playerID int64 = 2003

	if err := AddOrStackItem(playerID, 9, 10); err != nil {
		t.Fatalf("add: %v", err)
	}
	slot := int32(0)
	bag, _ := GetBagByPlayerID(playerID)
	if len(bag.Items) > 0 {
		slot = bag.Items[0].Slot
	}
	if err := SplitItem(playerID, slot, 4); err != nil {
		t.Fatalf("split: %v", err)
	}
	bag, err := GetBagByPlayerID(playerID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(bag.Items) != 2 {
		t.Fatalf("expected 2 stacks after split, got %+v", bag.Items)
	}
	var total int32
	for _, it := range bag.Items {
		if it.ItemId != 9 {
			t.Fatalf("unexpected item: %+v", it)
		}
		total += it.Count
	}
	if total != 10 {
		t.Fatalf("expected total 10, got %d", total)
	}
}

func TestBagFull(t *testing.T) {
	resetDBForTest(t)
	const playerID int64 = 2004

	for i := int32(0); i < MaxBagSlots; i++ {
		if err := AddOrStackItem(playerID, 100+i, 1); err != nil {
			t.Fatalf("add slot %d: %v", i, err)
		}
	}
	err := AddOrStackItem(playerID, 999, 1)
	if !errors.Is(err, ErrBagFull) {
		t.Fatalf("expected ErrBagFull, got %v", err)
	}
}
