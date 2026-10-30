package bag

import (
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/gameapp/equip"
	"github.com/example/mmo-server/internal/persistence"
	"github.com/example/mmo-server/internal/protocol"
)

// memStock 把内存背包交给装备模块。装备模块不看见 memBag。
type memStock struct {
	mb      *memBag
	bagType int32
}

func (s *memStock) Peek(slot int32) (int32, int32, int32, bool) {
	it, ok := s.mb.slots[slot]
	if !ok || it.count < 1 {
		return 0, 0, 0, false
	}
	return it.itemID, it.count, it.level, true
}

func (s *memStock) Take(slot int32) (int32, int32, error) {
	it, ok := s.mb.slots[slot]
	if !ok || it.count < 1 {
		return 0, 0, persistence.ErrBagSlotInvalid
	}
	if err := s.mb.removeBySlot(s.bagType, slot, 1); err != nil {
		return 0, 0, err
	}
	return it.itemID, it.level, nil
}

func (s *memStock) PutAt(slot, itemID, level int32) error {
	return s.mb.putOne(s.bagType, slot, itemID, level)
}

func (s *memStock) PutFirst(itemID, level int32) (int32, error) {
	slot, err := s.mb.findEmptySlot(s.bagType)
	if err != nil {
		return 0, err
	}
	if err := s.mb.putOne(s.bagType, slot, itemID, level); err != nil {
		return 0, err
	}
	return slot, nil
}

// memMat 是材料背包，给强化扣强化石和保护符。
type memMat struct {
	mb *memBag
}

func (m *memMat) Count(itemID int32) int32 {
	var n int32
	for _, s := range m.mb.slots {
		if s.itemID == itemID {
			n += s.count
		}
	}
	return n
}

func (m *memMat) Take(itemID, count int32) error {
	return m.mb.removeByItemID(equip.MaterialBag, itemID, count)
}

// equipApply 在背包 Actor 里一次做完穿脱引起的格子变化，再改穿着。
func (p *actorBag) equipApply(req *protocol.EquipApply) (*protocol.BagListResponse, int32) {
	if req == nil || req.PlayerId < 1 {
		return nil, code.PlayerNotEntered
	}
	bagType := req.BagType
	if bagType < 1 {
		bagType = equip.BagType
	}
	mb, err := p.ensureBag(req.PlayerId, bagType)
	if err != nil {
		return nil, code.BagLoadFail
	}
	c := equip.Mutate(req, &memStock{mb: mb, bagType: bagType})
	if c != code.OK {
		return nil, c
	}
	return mb.toProto(bagType), code.OK
}

// equipEnhance 在背包 Actor 里扣材料并改穿着等级。失败时两边都不变。
func (p *actorBag) equipEnhance(req *protocol.EquipEnhanceApply) (*protocol.EquipEnhanceResult, int32) {
	if req == nil || req.PlayerId < 1 {
		return nil, code.PlayerNotEntered
	}
	mb, err := p.ensureBag(req.PlayerId, equip.MaterialBag)
	if err != nil {
		return nil, code.BagLoadFail
	}
	out, c := equip.Enhance(req.PlayerId, req.Uid, req.SlotId, req.UseProtect, &memMat{mb: mb})
	if c != code.OK {
		return nil, c
	}
	return &protocol.EquipEnhanceResult{
		Success: out.Success, Dropped: out.Dropped, ProtectUsed: out.ProtectUsed,
		Level: out.Level, Attrs: out.Attrs, Bag: mb.toProto(equip.MaterialBag),
	}, code.OK
}
