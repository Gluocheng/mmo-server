package persistence

import (
	"context"
	"errors"
	"testing"
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
	err := AddOrStackItem(playerID, 1, 1)
	if !errors.Is(err, ErrBagInvalid) {
		t.Fatalf("expected ErrBagInvalid on overflow, got %v", err)
	}
}
