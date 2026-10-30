package equip

import (
	"context"
	"errors"
	"sort"

	gcruntime "github.com/example/mmo-server/gameconfig/pkg/runtime"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/gameapp/combat"
	"github.com/example/mmo-server/internal/persistence"
	"github.com/example/mmo-server/internal/protocol"
)

const (
	// BagType 是装备背包。穿着不占这里的格子。
	BagType  int32 = 4
	opWear         = 1
	opUnwear       = 2
)

// Stock 是装备背包的格子操作。一次穿脱里由调用方保证不会被别人插队。
type Stock interface {
	Peek(slot int32) (itemID, count int32, ok bool)
	Take(slot int32) (int32, error)
	PutAt(slot, itemID int32) error
	PutFirst(itemID int32) (int32, error)
}

// Row 是当前穿着的一个部位，给落库用。
type Row struct {
	SlotID int32
	ItemID int32
}

type sheet struct {
	slots  map[int32]int32
	dirty  bool
	loaded bool
}

var (
	sheets      = map[int64]*sheet{}
	uidByPlayer = map[int64]int64{}
)

// ResetForTest 清空穿着内存。仅测试调用。
func ResetForTest() {
	sheets = map[int64]*sheet{}
	uidByPlayer = map[int64]int64{}
}

// OpenTest 准备一个不读库的空穿着。仅测试调用。
func OpenTest(playerID int64) {
	sheets[playerID] = &sheet{slots: map[int32]int32{}, loaded: true}
}

// Mutate 按 EquipApply 穿上或脱下。失败时格子和穿着都不变。
func Mutate(req *protocol.EquipApply, stock Stock) int32 {
	if req == nil || req.PlayerId < 1 || stock == nil {
		return code.PlayerNotEntered
	}
	if req.Uid > 0 {
		uidByPlayer[req.PlayerId] = req.Uid
	}
	switch req.Op {
	case opWear:
		return wear(req.PlayerId, stock, req.BagType, req.BagSlot, req.TargetSlot)
	case opUnwear:
		return unwear(req.PlayerId, stock, req.TargetSlot)
	default:
		return code.EquipInvalid
	}
}

// Attach 进场或切图后把已保存的穿着叠到战斗单位上，并记成下一拍的比较基准。
func Attach(uid, playerID int64) {
	if uid < 1 || playerID < 1 {
		return
	}
	uidByPlayer[playerID] = uid
	if _, c := prepare(playerID); c != code.OK {
		return
	}
	combat.SetEquipBonus(uid, bonusOf(playerID), true)
}

// Unload 丢掉这个角色的穿着内存。调用前应已经落库。
func Unload(playerID int64) {
	delete(sheets, playerID)
	delete(uidByPlayer, playerID)
}

// PeekDirty 返回尚未落库的完整穿着。没有变更时 dirty 为 false。
func PeekDirty(playerID int64) ([]Row, bool) {
	s, ok := sheets[playerID]
	if !ok || !s.dirty {
		return nil, false
	}
	rows := make([]Row, 0, len(s.slots))
	for slot, item := range s.slots {
		if item > 0 {
			rows = append(rows, Row{SlotID: slot, ItemID: item})
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].SlotID < rows[j].SlotID })
	return rows, true
}

// AckDirty 落库成功后清掉脏标记。
func AckDirty(playerID int64) {
	if s, ok := sheets[playerID]; ok {
		s.dirty = false
	}
}

// State 按部位 id 顺序列出穿着。空部位的道具 id 为 0。
func State(playerID int64) (*protocol.EquipState, int32) {
	s, c := prepare(playerID)
	if c != code.OK {
		return nil, c
	}
	ids := slotIDs(s)
	msg := &protocol.EquipState{Slots: make([]*protocol.EquipSlotState, 0, len(ids))}
	for _, id := range ids {
		item := s.slots[id]
		msg.Slots = append(msg.Slots, &protocol.EquipSlotState{
			SlotId: id, ItemId: item, Attrs: attrProto(item),
		})
	}
	return msg, code.OK
}

func wear(playerID int64, stock Stock, bagType, bagSlot, target int32) int32 {
	if bagType != BagType {
		return code.EquipInvalid
	}
	s, c := prepare(playerID)
	if c != code.OK {
		return c
	}
	itemID, count, ok := stock.Peek(bagSlot)
	if !ok {
		return code.BagSlotInvalid
	}
	if count != 1 || itemID < 1 {
		return code.EquipInvalid
	}
	def, found := gcruntime.Get(itemID)
	if !found {
		return code.ItemNotFound
	}
	if def.Type != "equipment" || def.EquipSlot < 1 {
		return code.EquipInvalid
	}
	slotID, c := pickSlot(def.EquipSlot, target, s.slots)
	if c != code.OK {
		return c
	}
	taken, err := stock.Take(bagSlot)
	if err != nil {
		return bagCode(err)
	}
	if old := s.slots[slotID]; old != 0 {
		if err := stock.PutAt(bagSlot, old); err != nil {
			_ = stock.PutAt(bagSlot, taken)
			return bagCode(err)
		}
	}
	s.slots[slotID] = taken
	s.dirty = true
	syncCombat(playerID)
	return code.OK
}

func unwear(playerID int64, stock Stock, slotID int32) int32 {
	s, c := prepare(playerID)
	if c != code.OK {
		return c
	}
	item := s.slots[slotID]
	if item < 1 {
		return code.EquipSlotInvalid
	}
	if _, err := stock.PutFirst(item); err != nil {
		return bagCode(err)
	}
	delete(s.slots, slotID)
	s.dirty = true
	syncCombat(playerID)
	return code.OK
}

func pickSlot(group, target int32, worn map[int32]int32) (int32, int32) {
	ids := gcruntime.EquipSlotIDsByGroup(group)
	if len(ids) == 0 {
		return 0, code.EquipSlotInvalid
	}
	if target == 0 {
		if len(ids) == 1 {
			return ids[0], code.OK
		}
		for _, id := range ids {
			if worn[id] == 0 {
				return id, code.OK
			}
		}
		return ids[0], code.OK
	}
	for _, id := range ids {
		if id == target {
			return id, code.OK
		}
	}
	return 0, code.EquipSlotInvalid
}

func prepare(playerID int64) (*sheet, int32) {
	if s, ok := sheets[playerID]; ok && s.loaded {
		if s.slots == nil {
			s.slots = map[int32]int32{}
		}
		return s, code.OK
	}
	rows, err := persistence.LoadEquipment(context.Background(), playerID)
	if err != nil {
		return nil, code.BagLoadFail
	}
	s := &sheet{slots: map[int32]int32{}, loaded: true}
	for _, row := range rows {
		if row.ItemID > 0 {
			s.slots[row.SlotID] = row.ItemID
		}
	}
	sheets[playerID] = s
	return s, code.OK
}

func slotIDs(s *sheet) []int32 {
	seen := map[int32]struct{}{}
	var ids []int32
	for _, slot := range gcruntime.EquipSlots() {
		ids = append(ids, slot.ID)
		seen[slot.ID] = struct{}{}
	}
	for id, item := range s.slots {
		if item < 1 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func attrProto(itemID int32) []*protocol.EquipAttr {
	if itemID < 1 {
		return nil
	}
	rows := gcruntime.EquipAttrs(itemID)
	out := make([]*protocol.EquipAttr, 0, len(rows))
	for _, row := range rows {
		out = append(out, &protocol.EquipAttr{Stat: row.Stat, Mode: row.Mode, Value: row.Value})
	}
	return out
}

func bonusOf(playerID int64) combat.EquipBonus {
	s := sheets[playerID]
	var b combat.EquipBonus
	if s == nil {
		return b
	}
	for _, item := range s.slots {
		for _, row := range gcruntime.EquipAttrs(item) {
			addBonus(&b, row.Stat, row.Mode, row.Value)
		}
	}
	return b
}

func addBonus(b *combat.EquipBonus, stat, mode string, value int32) {
	switch stat {
	case "hp":
		if mode == "percent" {
			b.HPPct += value
		} else {
			b.HPFlat += value
		}
	case "attack":
		if mode == "percent" {
			b.AtkPct += value
		} else {
			b.AtkFlat += value
		}
	case "defense":
		if mode == "percent" {
			b.DefPct += value
		} else {
			b.DefFlat += value
		}
	}
}

func syncCombat(playerID int64) {
	uid := uidByPlayer[playerID]
	if uid < 1 {
		return
	}
	combat.SetEquipBonus(uid, bonusOf(playerID), false)
}

func bagCode(err error) int32 {
	switch {
	case errors.Is(err, persistence.ErrBagSlotInvalid):
		return code.BagSlotInvalid
	case errors.Is(err, persistence.ErrBagFull):
		return code.BagFull
	case errors.Is(err, persistence.ErrBagNotEnough):
		return code.BagItemNotEnough
	default:
		return code.BagLoadFail
	}
}
