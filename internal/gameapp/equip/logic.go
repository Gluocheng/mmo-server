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
	BagType int32 = 4
	// MaterialBag 是材料背包。强化石和保护符在这里。
	MaterialBag int32 = 3
	opWear      int32 = 1
	opUnwear    int32 = 2
)

// Stock 是装备背包的格子操作。一次穿脱里由调用方保证不会被别人插队。
// level 是这一件装备的强化等级，跟着道具走。
type Stock interface {
	Peek(slot int32) (itemID, count, level int32, ok bool)
	Take(slot int32) (itemID, level int32, err error)
	PutAt(slot, itemID, level int32) error
	PutFirst(itemID, level int32) (int32, error)
}

// Row 是当前穿着的一个部位，给落库用。
type Row struct {
	SlotID int32
	ItemID int32
	Level  int32
}

// piece 是一个部位上的一件装备。
type piece struct {
	item  int32
	level int32
}

// sheet 是一个角色的穿着内存。slots 的键是部位 id，空部位不在表里。
// dirty 表示有尚未落库的穿脱。loaded 表示已经从库载入，或测试直接建好，避免重复读库。
type sheet struct {
	slots  map[int32]piece
	dirty  bool
	loaded bool
}

// sheets 按 playerID 保存穿着。uidByPlayer 把角色映射到账号，穿脱后用来刷新战斗属性。
var (
	sheets      = map[int64]*sheet{}
	uidByPlayer = map[int64]int64{}
)

// ResetForTest 清空穿着内存。仅测试调用。
func ResetForTest() {
	sheets = map[int64]*sheet{}
	uidByPlayer = map[int64]int64{}
	resetRoll()
}

// OpenTest 准备一个不读库的空穿着。仅测试调用。
func OpenTest(playerID int64) {
	sheets[playerID] = &sheet{slots: map[int32]piece{}, loaded: true}
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
	for slot, p := range s.slots {
		if p.item > 0 {
			rows = append(rows, Row{SlotID: slot, ItemID: p.item, Level: p.level})
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
		p := s.slots[id]
		msg.Slots = append(msg.Slots, &protocol.EquipSlotState{
			SlotId: id, ItemId: p.item, EnhanceLevel: p.level, Attrs: attrProto(p.item, p.level),
		})
	}
	return msg, code.OK
}

// wear 从装备背包穿到部位。失败时背包格子和穿着都保持原样。
// 部位上已有装备时，旧装备放回刚空出的背包格。
func wear(playerID int64, stock Stock, bagType, bagSlot, target int32) int32 {
	if bagType != BagType {
		return code.EquipInvalid
	}
	s, c := prepare(playerID)
	if c != code.OK {
		return c
	}
	itemID, count, _, ok := stock.Peek(bagSlot)
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
	taken, takenLevel, err := stock.Take(bagSlot)
	if err != nil {
		return bagCode(err)
	}
	if old := s.slots[slotID]; old.item != 0 {
		// 旧装备放不回刚空出的格子时，把取出的新装备放回去，穿着不动。
		if err := stock.PutAt(bagSlot, old.item, old.level); err != nil {
			_ = stock.PutAt(bagSlot, taken, takenLevel)
			return bagCode(err)
		}
	}
	s.slots[slotID] = piece{item: taken, level: takenLevel}
	s.dirty = true
	syncCombat(playerID)
	return code.OK
}

// unwear 把部位上的装备放进装备背包第一个空格。背包满时穿着不变。
func unwear(playerID int64, stock Stock, slotID int32) int32 {
	s, c := prepare(playerID)
	if c != code.OK {
		return c
	}
	cur := s.slots[slotID]
	if cur.item < 1 {
		return code.EquipSlotInvalid
	}
	if _, err := stock.PutFirst(cur.item, cur.level); err != nil {
		return bagCode(err)
	}
	delete(s.slots, slotID)
	s.dirty = true
	syncCombat(playerID)
	return code.OK
}

// pickSlot 在部位组里选定一个部位。target 为 0 时，只有一个部位就用它；
// 有多个部位时先取 id 最小的空位，都占着则用 id 最小的那个并换下旧装备。
// target 不在这组里返回 40067。
func pickSlot(group, target int32, worn map[int32]piece) (int32, int32) {
	ids := gcruntime.EquipSlotIDsByGroup(group)
	if len(ids) == 0 {
		return 0, code.EquipSlotInvalid
	}
	if target == 0 {
		if len(ids) == 1 {
			return ids[0], code.OK
		}
		for _, id := range ids {
			if worn[id].item == 0 {
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

// prepare 取出已载入的穿着。还没载入时从库读一次，读失败返回 40020。
func prepare(playerID int64) (*sheet, int32) {
	if s, ok := sheets[playerID]; ok && s.loaded {
		if s.slots == nil {
			s.slots = map[int32]piece{}
		}
		return s, code.OK
	}
	rows, err := persistence.LoadEquipment(context.Background(), playerID)
	if err != nil {
		return nil, code.BagLoadFail
	}
	s := &sheet{slots: map[int32]piece{}, loaded: true}
	for _, row := range rows {
		if row.ItemID > 0 {
			s.slots[row.SlotID] = piece{item: row.ItemID, level: row.Level}
		}
	}
	sheets[playerID] = s
	return s, code.OK
}

// slotIDs 返回要展示的部位。先放配表里的全部部位，再补上配表已删但仍穿着的部位，按 id 排序。
func slotIDs(s *sheet) []int32 {
	seen := map[int32]struct{}{}
	var ids []int32
	for _, slot := range gcruntime.EquipSlots() {
		ids = append(ids, slot.ID)
		seen[slot.ID] = struct{}{}
	}
	for id, p := range s.slots {
		if p.item < 1 {
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

// attrProto 把一件装备追加等级后的属性转成协议。没有道具时为空。
func attrProto(itemID, level int32) []*protocol.EquipAttr {
	if itemID < 1 {
		return nil
	}
	rows := gcruntime.EquipAttrs(itemID)
	out := make([]*protocol.EquipAttr, 0, len(rows))
	for _, row := range rows {
		out = append(out, &protocol.EquipAttr{
			Stat: row.Stat, Mode: row.Mode, Value: boosted(row.Value, level),
		})
	}
	return out
}

// bonusOf 把当前穿着里生命、攻击、防御的固定值和百分比加总。其它属性名忽略。
func bonusOf(playerID int64) combat.EquipBonus {
	s := sheets[playerID]
	var b combat.EquipBonus
	if s == nil {
		return b
	}
	for _, p := range s.slots {
		if p.item < 1 {
			continue
		}
		for _, row := range gcruntime.EquipAttrs(p.item) {
			addBonus(&b, row.Stat, row.Mode, boosted(row.Value, p.level))
		}
	}
	return b
}

// addBonus 按属性名和模式累加。percent 进百分比，其余进固定值。
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

// syncCombat 把加成写到战斗单位，但不提交外观。下一拍有变化时才推属性帧。没有账号时跳过。
func syncCombat(playerID int64) {
	uid := uidByPlayer[playerID]
	if uid < 1 {
		return
	}
	combat.SetEquipBonus(uid, bonusOf(playerID), false)
}

// bagCode 把背包错误转成协议码。对不上的按加载失败。
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
