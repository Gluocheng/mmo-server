package equip

import (
	"testing"

	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/gameapp/combat"
	"github.com/example/mmo-server/internal/persistence"
	"github.com/example/mmo-server/internal/protocol"
)

func TestEnhancePlusOneRaisesAttack(t *testing.T) {
	seedEquip(t)
	OpenTest(1)
	wearSword(t)
	mat := newMat(1, 1)
	out, c := Enhance(1, 9, 1, true, mat)
	if c != code.OK || !out.Success || out.Dropped || out.ProtectUsed || out.Level != 1 {
		t.Fatalf("code=%d out=%+v", c, out)
	}
	if len(out.Attrs) != 1 || out.Attrs[0].Value != 6 {
		t.Fatalf("attrs=%v", out.Attrs)
	}
	_, _, atk, _, _ := combat.Stats(9)
	if atk != 16 {
		t.Fatalf("attack=%d", atk)
	}
	if mat.n[stoneItem] != 0 || mat.n[protectItem] != 1 {
		t.Fatalf("mat=%v", mat.n)
	}
}

func TestEnhanceNotWornKeepsStones(t *testing.T) {
	seedEquip(t)
	OpenTest(1)
	mat := newMat(1, 0)
	_, c := Enhance(1, 9, 1, false, mat)
	if c != code.EquipNotWorn {
		t.Fatalf("code=%d", c)
	}
	if mat.n[stoneItem] != 1 {
		t.Fatalf("stones=%d", mat.n[stoneItem])
	}
	if _, c := Enhance(1, 9, 99, false, mat); c != code.EquipSlotInvalid {
		t.Fatalf("bad slot %d", c)
	}
}

func TestEnhanceLevelFollowsItem(t *testing.T) {
	seedEquip(t)
	OpenTest(1)
	bag := newFake(32)
	bag.set(0, 2001)
	if c := Mutate(wearReq(1, 9, 0, 0), bag); c != code.OK {
		t.Fatal(c)
	}
	if _, c := Enhance(1, 9, 1, false, newMat(1, 0)); c != code.OK {
		t.Fatal(c)
	}
	if c := Mutate(&protocol.EquipApply{PlayerId: 1, Uid: 9, Op: opUnwear, BagType: BagType, TargetSlot: 1}, bag); c != code.OK {
		t.Fatal(c)
	}
	if bag.slots[0].level != 1 {
		t.Fatalf("bag level=%d", bag.slots[0].level)
	}
	if c := Mutate(wearReq(1, 9, 0, 0), bag); c != code.OK {
		t.Fatal(c)
	}
	st, _ := State(1)
	if st.Slots[0].EnhanceLevel != 1 || st.Slots[0].ItemId != 2001 {
		t.Fatalf("state=%+v", st.Slots[0])
	}
	_, _, atk, _, _ := combat.Stats(9)
	if atk != 16 {
		t.Fatalf("attack=%d", atk)
	}
}

func TestEnhanceFailDropsLevel(t *testing.T) {
	seedEquip(t)
	OpenTest(1)
	wearSword(t)
	sheets[1].slots[1] = piece{item: 2001, level: 7}
	mat := newMat(8, 0)
	SetRollsForTest(false, true)
	out, c := Enhance(1, 9, 1, false, mat)
	if c != code.OK || out.Success || !out.Dropped || out.ProtectUsed || out.Level != 6 {
		t.Fatalf("code=%d out=%+v", c, out)
	}
	if mat.n[stoneItem] != 0 {
		t.Fatalf("stones=%d", mat.n[stoneItem])
	}
}

func TestEnhanceProtectBlocksDrop(t *testing.T) {
	seedEquip(t)
	OpenTest(1)
	wearSword(t)
	sheets[1].slots[1] = piece{item: 2001, level: 7}
	mat := newMat(8, 1)
	SetRollsForTest(false, true)
	out, c := Enhance(1, 9, 1, true, mat)
	if c != code.OK || out.Success || out.Dropped || !out.ProtectUsed || out.Level != 7 {
		t.Fatalf("code=%d out=%+v", c, out)
	}
	if mat.n[stoneItem] != 0 || mat.n[protectItem] != 0 {
		t.Fatalf("mat=%v", mat.n)
	}
}

func TestEnhanceMissingProtect(t *testing.T) {
	seedEquip(t)
	OpenTest(1)
	wearSword(t)
	sheets[1].slots[1] = piece{item: 2001, level: 7}
	mat := newMat(8, 0)
	_, c := Enhance(1, 9, 1, true, mat)
	if c != code.BagItemNotEnough {
		t.Fatalf("code=%d", c)
	}
	if mat.n[stoneItem] != 8 || sheets[1].slots[1].level != 7 {
		t.Fatalf("stones=%d level=%d", mat.n[stoneItem], sheets[1].slots[1].level)
	}
}

func TestEnhanceMaxLevel(t *testing.T) {
	seedEquip(t)
	OpenTest(1)
	wearSword(t)
	sheets[1].slots[1] = piece{item: 2001, level: 10}
	mat := newMat(10, 0)
	_, c := Enhance(1, 9, 1, false, mat)
	if c != code.EquipLevelMax {
		t.Fatalf("code=%d", c)
	}
	if mat.n[stoneItem] != 10 {
		t.Fatalf("stones=%d", mat.n[stoneItem])
	}
}

func TestEnhancePushesStatsNextTick(t *testing.T) {
	seedEquip(t)
	OpenTest(1)
	wearSword(t)
	combat.Tick()
	if _, c := Enhance(1, 9, 1, false, newMat(1, 0)); c != code.OK {
		t.Fatal(c)
	}
	frames := combat.Tick()
	atk, ok := viewerAttack(frames, 9)
	if !ok || atk != 16 {
		t.Fatalf("attack=%d ok=%v frames=%d", atk, ok, len(frames))
	}
}

func wearSword(t *testing.T) {
	t.Helper()
	bag := newFake(32)
	bag.set(0, 2001)
	if c := Mutate(wearReq(1, 9, 0, 0), bag); c != code.OK {
		t.Fatal(c)
	}
}

func viewerAttack(frames []combat.Frame, viewer int64) (int32, bool) {
	for _, f := range frames {
		if f.ViewerUID != viewer {
			continue
		}
		for _, s := range f.Stats {
			if s.UID == viewer {
				return s.Attack, true
			}
		}
	}
	return 0, false
}

type fakeMat struct {
	n map[int32]int32
}

func newMat(stones, protects int32) *fakeMat {
	return &fakeMat{n: map[int32]int32{stoneItem: stones, protectItem: protects}}
}

func (f *fakeMat) Count(itemID int32) int32 { return f.n[itemID] }

func (f *fakeMat) Take(itemID, count int32) error {
	if f.n[itemID] < count {
		return persistence.ErrBagNotEnough
	}
	f.n[itemID] -= count
	return nil
}
