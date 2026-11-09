package equip

import (
	"testing"

	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/gameapp/combat"
	"github.com/example/mmo-server/internal/protocol"
)

func TestSeedGearStats(t *testing.T) {
	seedEquip(t)
	sh := OpenTest(1, 0)
	bag := newFake(32)
	bag.set(0, 2001)
	if c := sh.Mutate(wearReq(1, 9, 0, 0), bag); c != code.OK {
		t.Fatal(c)
	}
	cr, cd, pen, bonus, red, steal, ok := combat.Rates(9)
	if !ok || cr != 10 || cd != 0 || pen != 0 || bonus != 0 || red != 0 || steal != 0 {
		t.Fatalf("sword %d %d %d %d %d %d", cr, cd, pen, bonus, red, steal)
	}
	if c := sh.Mutate(&protocol.EquipApply{PlayerId: 1, Uid: 9, Op: opUnwear, BagType: BagType, TargetSlot: 1}, bag); c != code.OK {
		t.Fatal(c)
	}
	bag.set(1, 2002)
	if c := sh.Mutate(wearReq(1, 9, 1, 0), bag); c != code.OK {
		t.Fatal(c)
	}
	_, _, _, _, red, _, _ = combat.Rates(9)
	if red != 5 {
		t.Fatalf("cloth %d", red)
	}
	bag.set(2, 2003)
	if c := sh.Mutate(wearReq(1, 9, 2, 10), bag); c != code.OK {
		t.Fatal(c)
	}
	_, _, _, _, _, steal, _ = combat.Rates(9)
	_, _, atk, _, _ := combat.Stats(9)
	if steal != 5 || atk != 11 {
		t.Fatalf("ring steal=%d atk=%d", steal, atk)
	}
	bag.set(3, 2004)
	if c := sh.Mutate(wearReq(1, 9, 3, 11), bag); c != code.OK {
		t.Fatal(c)
	}
	_, cd, _, _, _, steal, _ = combat.Rates(9)
	if cd != 20 || steal != 5 {
		t.Fatalf("sharp cd=%d steal=%d", cd, steal)
	}
}
