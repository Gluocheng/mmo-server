package persistence

import (
	"context"
	"errors"
	"testing"
)

func TestDeliverKillItemGoesPendingUntilBagHasRoom(t *testing.T) {
	resetBagTestDB(t)
	const playerID int64 = 42
	for id := int32(1); id <= 32; id++ {
		if err := AddOrStackItem(playerID, id, 1); err != nil {
			t.Fatalf("fill %d: %v", id, err)
		}
	}
	got, err := DeliverKillItem(context.Background(), playerID, 33, 2, 2)
	if err != nil || got.Placed || got.ClaimID < 1 {
		t.Fatalf("delivery %+v err %v", got, err)
	}
	if _, err := TakeRewardClaim(context.Background(), playerID, got.ClaimID); !errors.Is(err, ErrBagFull) {
		t.Fatalf("take while full: %v", err)
	}
	rows, err := ListRewardClaims(context.Background(), playerID)
	if err != nil || len(rows) != 1 || rows[0].ItemID != 33 || rows[0].Count != 2 {
		t.Fatalf("claims %+v err %v", rows, err)
	}
	if err := RemoveItem(playerID, testBag, 1, 1); err != nil {
		t.Fatal(err)
	}
	bagType, err := TakeRewardClaim(context.Background(), playerID, got.ClaimID)
	if err != nil {
		t.Fatal(err)
	}
	bag, err := GetBagByPlayerID(playerID, bagType)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, it := range bag.Items {
		if it.ItemId == 33 && it.Count == 2 {
			found = true
		}
	}
	if !found {
		t.Fatalf("bag %+v", bag.Items)
	}
	rows, err = ListRewardClaims(context.Background(), playerID)
	if err != nil || len(rows) != 0 {
		t.Fatalf("left %+v err %v", rows, err)
	}
}
