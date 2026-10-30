package equip

import (
	"context"
	"testing"

	gcruntime "github.com/example/mmo-server/gameconfig/pkg/runtime"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/gameapp/combat"
	"github.com/example/mmo-server/internal/gameapp/world"
	"github.com/example/mmo-server/internal/persistence"
	"github.com/example/mmo-server/internal/protocol"
)

func TestWearSwordRaisesAttackAndStrike(t *testing.T) {
	seedEquip(t)
	OpenTest(1)
	bag := newFake(32)
	bag.set(0, 2001)
	if c := Mutate(wearReq(1, 9, 0, 0), bag); c != code.OK {
		t.Fatal(c)
	}
	_, _, atk, _, ok := combat.Stats(9)
	if !ok || atk != 15 {
		t.Fatalf("attack=%d", atk)
	}
	combat.Enter(8)
	if c := combat.Cast(9, 1, 8); c != code.OK {
		t.Fatal(c)
	}
	combat.Tick()
	hp, _, _, _ := combat.Snapshot(8)
	if hp != 85 {
		t.Fatalf("hp=%d", hp)
	}
	if c := Mutate(&protocol.EquipApply{PlayerId: 1, Uid: 9, Op: opUnwear, BagType: BagType, TargetSlot: 1}, bag); c != code.OK {
		t.Fatal(c)
	}
	_, _, atk, _, _ = combat.Stats(9)
	if atk != 10 {
		t.Fatalf("attack after unwear=%d", atk)
	}
}

func TestSwordAndBuffStack(t *testing.T) {
	seedEquip(t)
	OpenTest(1)
	bag := newFake(32)
	bag.set(0, 2001)
	if c := Mutate(wearReq(1, 9, 0, 0), bag); c != code.OK {
		t.Fatal(c)
	}
	if c := combat.ApplyBuff(9, 5); c != code.OK {
		t.Fatal(c)
	}
	_, _, atk, _, _ := combat.Stats(9)
	if atk != 20 {
		t.Fatalf("attack=%d", atk)
	}
}

func TestClothRaisesHPThenClamp(t *testing.T) {
	seedEquip(t)
	OpenTest(1)
	bag := newFake(32)
	bag.set(0, 2002)
	if c := Mutate(wearReq(1, 9, 0, 0), bag); c != code.OK {
		t.Fatal(c)
	}
	hp, maxHP, _, def, _ := combat.Stats(9)
	if hp != 120 || maxHP != 120 || def != 3 {
		t.Fatalf("hp=%d max=%d def=%d", hp, maxHP, def)
	}
	if c := Mutate(&protocol.EquipApply{PlayerId: 1, Uid: 9, Op: opUnwear, BagType: BagType, TargetSlot: 4}, bag); c != code.OK {
		t.Fatal(c)
	}
	hp, maxHP, _, def, _ = combat.Stats(9)
	if maxHP != 100 || hp != 100 || def != 0 {
		t.Fatalf("after hp=%d max=%d def=%d", hp, maxHP, def)
	}
}

func TestSwapReturnsOldToFreedSlot(t *testing.T) {
	seedEquip(t)
	OpenTest(1)
	bag := newFake(32)
	bag.set(3, 2001)
	if c := Mutate(wearReq(1, 9, 3, 0), bag); c != code.OK {
		t.Fatal(c)
	}
	bag.set(4, 2011)
	if c := Mutate(wearReq(1, 9, 4, 1), bag); c != code.OK {
		t.Fatal(c)
	}
	if bag.slots[4].item != 2001 {
		t.Fatalf("old sword slot=%v", bag.slots)
	}
	_, _, atk, _, _ := combat.Stats(9)
	if atk != 17 {
		t.Fatalf("attack=%d", atk)
	}
}

func TestUnwearFullBag(t *testing.T) {
	seedEquip(t)
	OpenTest(1)
	bag := newFake(1)
	bag.set(0, 2001)
	if c := Mutate(wearReq(1, 0, 0, 0), bag); c != code.OK {
		t.Fatal(c)
	}
	bag.set(0, 2003)
	if c := Mutate(&protocol.EquipApply{PlayerId: 1, Op: opUnwear, BagType: BagType, TargetSlot: 1}, bag); c != code.BagFull {
		t.Fatalf("code=%d", c)
	}
	st, _ := State(1)
	if st.Slots[0].ItemId != 2001 {
		t.Fatalf("still worn %+v", st.Slots[0])
	}
}

func TestRingsAndWrongSlot(t *testing.T) {
	seedEquip(t)
	OpenTest(1)
	bag := newFake(32)
	bag.set(0, 2003)
	bag.set(1, 2003)
	if c := Mutate(wearReq(1, 9, 0, 10), bag); c != code.OK {
		t.Fatal(c)
	}
	if c := Mutate(wearReq(1, 9, 1, 11), bag); c != code.OK {
		t.Fatal(c)
	}
	_, _, atk, _, _ := combat.Stats(9)
	if atk != 12 {
		t.Fatalf("attack=%d", atk)
	}
	bag.set(2, 2003)
	if c := Mutate(wearReq(1, 9, 2, 1), bag); c != code.EquipSlotInvalid {
		t.Fatalf("code=%d", c)
	}
}

func TestRejectPotionAndEmpty(t *testing.T) {
	seedEquip(t)
	OpenTest(1)
	bag := newFake(32)
	bag.set(0, 1001)
	if c := Mutate(wearReq(1, 0, 0, 0), bag); c != code.EquipInvalid {
		t.Fatalf("potion %d", c)
	}
	if c := Mutate(wearReq(1, 0, 3, 0), bag); c != code.BagSlotInvalid {
		t.Fatalf("empty %d", c)
	}
	if c := Mutate(wearReq(1, 0, 0, 99), bag); c != code.EquipInvalid && c != code.EquipSlotInvalid {
		t.Fatalf("bad slot %d", c)
	}
}

func TestWearPersistsAcrossReload(t *testing.T) {
	persistence.UseMemoryDBForTest(t)
	seedEquip(t)
	OpenTest(7)
	bag := newFake(32)
	bag.set(0, 2001)
	if c := Mutate(wearReq(7, 0, 0, 0), bag); c != code.OK {
		t.Fatal(c)
	}
	rows, dirty := PeekDirty(7)
	if !dirty || len(rows) != 1 {
		t.Fatalf("dirty=%v rows=%v", dirty, rows)
	}
	if err := persistence.FlushPlayer(context.Background(), 7, nil, []persistence.EquipRow{{SlotID: rows[0].SlotID, ItemID: rows[0].ItemID}}, true); err != nil {
		t.Fatal(err)
	}
	AckDirty(7)
	Unload(7)
	st, c := State(7)
	if c != code.OK || st.Slots[0].ItemId != 2001 {
		t.Fatalf("code=%d state=%v", c, st)
	}
}

func wearReq(player, uid int64, slot, target int32) *protocol.EquipApply {
	return &protocol.EquipApply{
		PlayerId: player, Uid: uid, Op: opWear, BagType: BagType, BagSlot: slot, TargetSlot: target,
	}
}

func seedEquip(t *testing.T) {
	t.Helper()
	ResetForTest()
	combat.ResetForTest()
	gcruntime.BuildCombat(
		[]gcruntime.SkillDef{{ID: 1, Name: "普攻", Target: "single", CastRange: 30, Damage: 0, Factor: 100}},
		[]gcruntime.BuffDef{{ID: 5, Name: "战意", DurationMs: 5000, Effect: "attr", Value: 5, MaxStack: 1, Stat: "attack", Mode: "flat"}},
		gcruntime.CombatConst{MaxHP: 100, TickMs: 100, RespawnMs: 5000, FrameEventCap: 64, Attack: 10},
	)
	gcruntime.BuildStats([]gcruntime.StatDef{
		{ID: 1, Name: "hp", Settle: true, AllowFlat: true, AllowPercent: true},
		{ID: 2, Name: "attack", Settle: true, AllowFlat: true, AllowPercent: true},
		{ID: 3, Name: "defense", Settle: true, AllowFlat: true, AllowPercent: true},
	})
	slots := make([]gcruntime.EquipSlotDef, 0, 12)
	for id := int32(1); id <= 12; id++ {
		group := id
		if id == 11 {
			group = 10
		}
		slots = append(slots, gcruntime.EquipSlotDef{ID: id, GroupID: group})
	}
	gcruntime.BuildEquip(slots, []gcruntime.EquipAttrDef{
		{ItemID: 2001, Stat: "attack", Mode: "flat", Value: 5},
		{ItemID: 2011, Stat: "attack", Mode: "flat", Value: 7},
		{ItemID: 2002, Stat: "hp", Mode: "flat", Value: 20},
		{ItemID: 2002, Stat: "defense", Mode: "flat", Value: 3},
		{ItemID: 2003, Stat: "attack", Mode: "flat", Value: 1},
	}, []gcruntime.ItemDef{
		{ID: 1001, Type: "consumable", MaxStack: 99, BagType: 2},
		{ID: 2001, Type: "equipment", MaxStack: 1, BagType: 4, EquipSlot: 1},
		{ID: 2011, Type: "equipment", MaxStack: 1, BagType: 4, EquipSlot: 1},
		{ID: 2002, Type: "equipment", MaxStack: 1, BagType: 4, EquipSlot: 4},
		{ID: 2003, Type: "equipment", MaxStack: 1, BagType: 4, EquipSlot: 10},
	})
	rules := make([]gcruntime.EnhanceRule, 0, 10)
	for lv := int32(1); lv <= 7; lv++ {
		rules = append(rules, gcruntime.EnhanceRule{Level: lv, StoneCount: lv, SuccessPercent: 100})
	}
	rules = append(rules,
		gcruntime.EnhanceRule{Level: 8, StoneCount: 8, SuccessPercent: 70, DropPercent: 50},
		gcruntime.EnhanceRule{Level: 9, StoneCount: 9, SuccessPercent: 50, DropPercent: 70},
		gcruntime.EnhanceRule{Level: 10, StoneCount: 10, SuccessPercent: 30, DropPercent: 100},
	)
	gcruntime.BuildEnhance(rules)
	gcruntime.BuildScenes([]gcruntime.SceneDef{{
		ID: world.DefaultSceneID, Name: "测试", AllowCombat: true, MaxOnline: 10, MaxLines: 1,
	}})
	world.Enter(9, "gate", world.DefaultSceneID)
	world.Enter(8, "gate", world.DefaultSceneID)
	combat.Enter(9)
	t.Cleanup(func() {
		combat.Leave(9)
		combat.Leave(8)
		world.Leave(9)
		world.Leave(8)
	})
}

type fakeSlot struct {
	item  int32
	level int32
}

type fakeStock struct {
	slots map[int32]fakeSlot
	limit int32
}

func newFake(limit int32) *fakeStock {
	return &fakeStock{slots: map[int32]fakeSlot{}, limit: limit}
}

func (f *fakeStock) set(slot, item int32) { f.slots[slot] = fakeSlot{item: item} }

func (f *fakeStock) Peek(slot int32) (int32, int32, int32, bool) {
	it, ok := f.slots[slot]
	if !ok {
		return 0, 0, 0, false
	}
	return it.item, 1, it.level, true
}

func (f *fakeStock) Take(slot int32) (int32, int32, error) {
	it, ok := f.slots[slot]
	if !ok {
		return 0, 0, persistence.ErrBagSlotInvalid
	}
	delete(f.slots, slot)
	return it.item, it.level, nil
}

func (f *fakeStock) PutAt(slot, item, level int32) error {
	if slot < 0 || slot >= f.limit {
		return persistence.ErrBagSlotInvalid
	}
	if _, ok := f.slots[slot]; ok {
		return persistence.ErrBagFull
	}
	f.slots[slot] = fakeSlot{item: item, level: level}
	return nil
}

func (f *fakeStock) PutFirst(item, level int32) (int32, error) {
	for slot := int32(0); slot < f.limit; slot++ {
		if _, ok := f.slots[slot]; !ok {
			f.slots[slot] = fakeSlot{item: item, level: level}
			return slot, nil
		}
	}
	return 0, persistence.ErrBagFull
}
