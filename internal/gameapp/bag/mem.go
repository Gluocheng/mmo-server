package bag

import (
	"sort"

	gcruntime "github.com/example/mmo-server/gameconfig/pkg/runtime"
	"github.com/example/mmo-server/internal/persistence"
	"github.com/example/mmo-server/internal/protocol"
)

// memSlot 是内存背包里的一个槽位堆叠。level 跟着这件装备走，非装备为 0。
type memSlot struct {
	itemID int32
	count  int32
	level  int32
}

// memBag 是一个背包类型的内存态：slots 是权威最终态，touched 是本次未落库的变更集。
// touched 等价于策划里的 added/changed/removed 三集合并：flush 时按 slots 最终态决定 upsert 还是 delete。
type memBag struct {
	slots   map[int32]memSlot
	touched map[int32]struct{}
}

func newMemBag() *memBag {
	return &memBag{
		slots:   make(map[int32]memSlot),
		touched: make(map[int32]struct{}),
	}
}

// dirty 表示是否有未落库变更。
func (mb *memBag) dirty() bool { return len(mb.touched) > 0 }

// set 写入槽位并记为变更。
func (mb *memBag) set(slot int32, s memSlot) {
	mb.slots[slot] = s
	mb.touched[slot] = struct{}{}
}

// del 删除槽位并记为变更。
func (mb *memBag) del(slot int32) {
	delete(mb.slots, slot)
	mb.touched[slot] = struct{}{}
}

// toProto 组装 BagListResponse，只含非空槽。
func (mb *memBag) toProto(bagType int32) *protocol.BagListResponse {
	slots := mb.sortedSlots()
	rsp := &protocol.BagListResponse{}
	for _, s := range slots {
		it := mb.slots[s]
		rsp.Items = append(rsp.Items, &protocol.BagItem{
			Slot:         s,
			ItemId:       it.itemID,
			Count:        it.count,
			BagType:      bagType,
			EnhanceLevel: it.level,
		})
	}
	return rsp
}

// clearTouched 清空变更集（flush 成功后调用）。
func (mb *memBag) clearTouched() { mb.touched = make(map[int32]struct{}) }

// diff 返回落库变更：upserts 是槽位最终态，removes 是本次删除的槽位。
func (mb *memBag) diff() (upserts map[int32]memSlot, removes []int32) {
	upserts = make(map[int32]memSlot)
	for slot := range mb.touched {
		if s, ok := mb.slots[slot]; ok {
			upserts[slot] = s
		} else {
			removes = append(removes, slot)
		}
	}
	return upserts, removes
}

func (mb *memBag) sortedSlots() []int32 {
	slots := make([]int32, 0, len(mb.slots))
	for s := range mb.slots {
		slots = append(slots, s)
	}
	sort.Slice(slots, func(i, j int) bool { return slots[i] < slots[j] })
	return slots
}

func (mb *memBag) validateSlot(bagType, slot int32) error {
	if slot < 0 || slot >= memSlotCount(bagType) {
		return persistence.ErrBagSlotInvalid
	}
	return nil
}

func (mb *memBag) findEmptySlot(bagType int32) (int32, error) {
	limit := memSlotCount(bagType)
	for slot := int32(0); slot < limit; slot++ {
		if _, ok := mb.slots[slot]; !ok {
			return slot, nil
		}
	}
	return -1, persistence.ErrBagFull
}

// add 先向同 item 的已有槽堆叠，剩余数量占最小号空槽（可跨多槽）。
func (mb *memBag) add(bagType, itemID, count int32) error {
	if err := memValidateItem(itemID, count); err != nil {
		return err
	}
	if err := memValidateItemBagType(itemID, bagType); err != nil {
		return err
	}
	maxStack := memMaxStack(itemID)
	remaining := count
	for _, slot := range mb.sortedSlots() {
		if remaining < 1 {
			break
		}
		s := mb.slots[slot]
		if s.itemID != itemID {
			continue
		}
		room := maxStack - s.count
		if room < 1 {
			continue
		}
		add := remaining
		if add > room {
			add = room
		}
		s.count += add
		mb.set(slot, s)
		remaining -= add
	}
	for remaining > 0 {
		slot, err := mb.findEmptySlot(bagType)
		if err != nil {
			return err
		}
		put := remaining
		if put > maxStack {
			put = maxStack
		}
		mb.set(slot, memSlot{itemID: itemID, count: put})
		remaining -= put
	}
	return nil
}

// itemRoom 计算这个道具还能放进多少个（内存态）。返回该背包类型的剩余容量：
// 已有堆叠的空位，加上空槽能放下的数量。与 persistence.itemRoom 语义一致。
func (mb *memBag) itemRoom(bagType, itemID int32) int32 {
	maxStack := memMaxStack(itemID)
	if maxStack < 1 {
		maxStack = 1
	}
	var room int32
	for _, s := range mb.slots {
		if s.itemID != itemID {
			continue
		}
		if s.count < maxStack {
			room += maxStack - s.count
		}
	}
	empty := memSlotCount(bagType) - int32(len(mb.slots))
	if empty > 0 {
		room += empty * maxStack
	}
	return room
}

// putOne 把一件不可堆叠道具放进指定空格。格子已有东西时返回满。
func (mb *memBag) putOne(bagType, slot, itemID, level int32) error {
	if err := mb.validateSlot(bagType, slot); err != nil {
		return err
	}
	if _, ok := mb.slots[slot]; ok {
		return persistence.ErrBagFull
	}
	mb.set(slot, memSlot{itemID: itemID, count: 1, level: level})
	return nil
}

// removeBySlot 从指定槽位扣除 count；扣完则删，否则减数量。
func (mb *memBag) removeBySlot(bagType, slot, count int32) error {
	if err := mb.validateSlot(bagType, slot); err != nil {
		return err
	}
	s, ok := mb.slots[slot]
	if !ok || s.count < count {
		return persistence.ErrBagNotEnough
	}
	if s.count == count {
		mb.del(slot)
	} else {
		s.count -= count
		mb.set(slot, s)
	}
	return nil
}

// removeByItemID 按 item_id 从 slot 升序各槽合计扣除 count（可跨多槽）。
func (mb *memBag) removeByItemID(bagType, itemID, count int32) error {
	if itemID < 1 || count < 1 {
		return persistence.ErrBagInvalid
	}
	remaining := count
	for _, slot := range mb.sortedSlots() {
		if remaining < 1 {
			break
		}
		s := mb.slots[slot]
		if s.itemID != itemID {
			continue
		}
		take := s.count
		if take > remaining {
			take = remaining
		}
		if s.count == take {
			mb.del(slot)
		} else {
			s.count -= take
			mb.set(slot, s)
		}
		remaining -= take
	}
	if remaining > 0 {
		return persistence.ErrBagNotEnough
	}
	return nil
}

// move 移动/合并/交换：目标空槽则移过去；同 item 则合并（源槽可剩）；异 item 则交换两栈。
func (mb *memBag) move(bagType, fromSlot, toSlot int32) error {
	if err := mb.validateSlot(bagType, fromSlot); err != nil {
		return err
	}
	if err := mb.validateSlot(bagType, toSlot); err != nil {
		return err
	}
	if fromSlot == toSlot {
		return nil
	}
	from, ok := mb.slots[fromSlot]
	if !ok || from.itemID < 1 || from.count < 1 {
		return persistence.ErrBagSlotInvalid
	}
	to, ok := mb.slots[toSlot]
	if !ok {
		mb.del(fromSlot)
		mb.set(toSlot, from)
		return nil
	}
	if to.itemID == from.itemID {
		maxStack := memMaxStack(from.itemID)
		room := maxStack - to.count
		if room < 1 {
			return persistence.ErrBagInvalid
		}
		mv := from.count
		if mv > room {
			mv = room
		}
		to.count += mv
		mb.set(toSlot, to)
		if from.count == mv {
			mb.del(fromSlot)
		} else {
			from.count -= mv
			mb.set(fromSlot, from)
		}
		return nil
	}
	mb.set(fromSlot, to)
	mb.set(toSlot, from)
	return nil
}

// split 从源槽拆出 count 到新空槽；要求源槽数量严格大于 count。
func (mb *memBag) split(bagType, fromSlot, count int32) error {
	if err := mb.validateSlot(bagType, fromSlot); err != nil {
		return err
	}
	if count < 1 || count > persistence.MaxBagStack {
		return persistence.ErrBagInvalid
	}
	from, ok := mb.slots[fromSlot]
	if !ok || from.count <= count {
		return persistence.ErrBagNotEnough
	}
	emptySlot, err := mb.findEmptySlot(bagType)
	if err != nil {
		return err
	}
	mb.set(emptySlot, memSlot{itemID: from.itemID, count: count, level: from.level})
	from.count -= count
	mb.set(fromSlot, from)
	return nil
}

// memMaxStack 取 min(全局硬顶, 配置 max_stack)，与 persistence.effectiveMaxStack 一致。
func memMaxStack(itemID int32) int32 {
	ms := gcruntime.MaxStack(itemID)
	if ms < 1 {
		return 0
	}
	if ms > persistence.MaxBagStack {
		return persistence.MaxBagStack
	}
	return ms
}

// memSlotCount 返回背包槽位数；配置未加载或未配置时兜底 32（与 persistence 一致）。
func memSlotCount(bagType int32) int32 {
	if n := gcruntime.SlotCount(bagType); n > 0 {
		return n
	}
	return 32
}

// memValidateItem 校验 itemID/count 合法、item 存在、单次数量不超堆叠上限。
func memValidateItem(itemID, count int32) error {
	if itemID < 1 || count < 1 {
		return persistence.ErrBagInvalid
	}
	if err := gcruntime.ValidateItemID(itemID); err != nil {
		return persistence.ErrItemNotFound
	}
	max := memMaxStack(itemID)
	if max < 1 || count > max {
		return persistence.ErrBagInvalid
	}
	return nil
}

// memResolveBagType 返回道具目标背包类型；未声明时归入通用背包。
func memResolveBagType(itemID int32) (int32, error) {
	def, ok := gcruntime.Get(itemID)
	if !ok {
		return 0, persistence.ErrItemNotFound
	}
	if def.BagType < 1 {
		return persistence.GeneralBagType, nil
	}
	return def.BagType, nil
}

// memValidateItemBagType 严格校验道具类别与目标背包一致。
func memValidateItemBagType(itemID, bagType int32) error {
	expect, err := memResolveBagType(itemID)
	if err != nil {
		return err
	}
	if expect != bagType {
		return persistence.ErrBagTypeMismatch
	}
	return nil
}
