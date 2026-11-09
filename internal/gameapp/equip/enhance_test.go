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
	sh := OpenTest(1, 0)
	wearSword(t, sh)
	mat := newMat(1, 1)
	out, c := sh.Enhance(9, 1, true, mat)
	if c != code.OK || !out.Success || out.Dropped || out.ProtectUsed || out.Level != 1 {
		t.Fatalf("code=%d out=%+v", c, out)
	}
	if attackAttr(out.Attrs) != 6 {
		t.Fatalf("attrs=%v", out.Attrs)
	}
	_, _, atk, _, _ := combat.Stats(9)
	if atk != 16 {
		t.Fatalf("attack=%d", atk)
	}
	if mat.n[stoneItem] != 0 || mat.n[protectItem] != 1 {
		t.Fatalf("mat=%v", mat.n)
	}
	cr, _, _, _, _, _, ok := combat.Rates(9)
	if !ok || cr != 11 {
		t.Fatalf("crit=%d", cr)
	}
}

func TestEnhanceNotWornKeepsStones(t *testing.T) {
	seedEquip(t)
	sh := OpenTest(1, 0)
	mat := newMat(1, 0)
	_, c := sh.Enhance(9, 1, false, mat)
	if c != code.EquipNotWorn {
		t.Fatalf("code=%d", c)
	}
	if mat.n[stoneItem] != 1 {
		t.Fatalf("stones=%d", mat.n[stoneItem])
	}
	if _, c := sh.Enhance(9, 99, false, mat); c != code.EquipSlotInvalid {
		t.Fatalf("bad slot %d", c)
	}
}

func TestEnhanceLevelFollowsItem(t *testing.T) {
	seedEquip(t)
	sh := OpenTest(1, 0)
	bag := newFake(32)
	bag.set(0, 2001)
	if c := sh.Mutate(wearReq(1, 9, 0, 0), bag); c != code.OK {
		t.Fatal(c)
	}
	if _, c := sh.Enhance(9, 1, false, newMat(1, 0)); c != code.OK {
		t.Fatal(c)
	}
	if c := sh.Mutate(&protocol.EquipApply{PlayerId: 1, Uid: 9, Op: opUnwear, BagType: BagType, TargetSlot: 1}, bag); c != code.OK {
		t.Fatal(c)
	}
	if bag.slots[0].level != 1 {
		t.Fatalf("bag level=%d", bag.slots[0].level)
	}
	if c := sh.Mutate(wearReq(1, 9, 0, 0), bag); c != code.OK {
		t.Fatal(c)
	}
	st, _ := sh.State()
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
	sh := OpenTest(1, 0)
	wearSword(t, sh)
	sh.SetWorn(1, 2001, 7)
	mat := newMat(8, 0)
	SetRollsForTest(false, true)
	out, c := sh.Enhance(9, 1, false, mat)
	if c != code.OK || out.Success || !out.Dropped || out.ProtectUsed || out.Level != 6 {
		t.Fatalf("code=%d out=%+v", c, out)
	}
	if mat.n[stoneItem] != 0 {
		t.Fatalf("stones=%d", mat.n[stoneItem])
	}
}

func TestEnhanceProtectBlocksDrop(t *testing.T) {
	seedEquip(t)
	sh := OpenTest(1, 0)
	wearSword(t, sh)
	sh.SetWorn(1, 2001, 7)
	mat := newMat(8, 1)
	SetRollsForTest(false, true)
	out, c := sh.Enhance(9, 1, true, mat)
	if c != code.OK || out.Success || out.Dropped || !out.ProtectUsed || out.Level != 7 {
		t.Fatalf("code=%d out=%+v", c, out)
	}
	if mat.n[stoneItem] != 0 || mat.n[protectItem] != 0 {
		t.Fatalf("mat=%v", mat.n)
	}
}

func TestEnhanceMissingProtect(t *testing.T) {
	seedEquip(t)
	sh := OpenTest(1, 0)
	wearSword(t, sh)
	sh.SetWorn(1, 2001, 7)
	mat := newMat(8, 0)
	_, c := sh.Enhance(9, 1, true, mat)
	if c != code.BagItemNotEnough {
		t.Fatalf("code=%d", c)
	}
	if mat.n[stoneItem] != 8 || mustLevel(sh, 1) != 7 {
		t.Fatalf("stones=%d level=%d", mat.n[stoneItem], mustLevel(sh, 1))
	}
}

func TestEnhanceMaxLevel(t *testing.T) {
	seedEquip(t)
	sh := OpenTest(1, 0)
	wearSword(t, sh)
	sh.SetWorn(1, 2001, 10)
	mat := newMat(10, 0)
	_, c := sh.Enhance(9, 1, false, mat)
	if c != code.EquipLevelMax {
		t.Fatalf("code=%d", c)
	}
	if mat.n[stoneItem] != 10 {
		t.Fatalf("stones=%d", mat.n[stoneItem])
	}
}

func TestEnhancePushesStatsNextTick(t *testing.T) {
	seedEquip(t)
	sh := OpenTest(1, 0)
	wearSword(t, sh)
	combat.Tick()
	if _, c := sh.Enhance(9, 1, false, newMat(1, 0)); c != code.OK {
		t.Fatal(c)
	}
	frames := combat.Tick()
	atk, ok := viewerAttack(frames, 9)
	if !ok || atk != 16 {
		t.Fatalf("attack=%d ok=%v frames=%d", atk, ok, len(frames))
	}
}

func attackAttr(attrs []*protocol.EquipAttr) int32 {
	for _, attr := range attrs {
		if attr.Stat == "attack" {
			return attr.Value
		}
	}
	return -1
}

func wearSword(t *testing.T, sh *Sheet) {
	t.Helper()
	bag := newFake(32)
	bag.set(0, 2001)
	if c := sh.Mutate(wearReq(1, 9, 0, 0), bag); c != code.OK {
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
