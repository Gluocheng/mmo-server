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

func (s *memStock) Peek(slot int32) (int32, int32, bool) {
	it, ok := s.mb.slots[slot]
	if !ok || it.count < 1 {
		return 0, 0, false
	}
	return it.itemID, it.count, true
}

func (s *memStock) Take(slot int32) (int32, error) {
	it, ok := s.mb.slots[slot]
	if !ok || it.count < 1 {
		return 0, persistence.ErrBagSlotInvalid
	}
	if err := s.mb.removeBySlot(s.bagType, slot, 1); err != nil {
		return 0, err
	}
	return it.itemID, nil
}

func (s *memStock) PutAt(slot, itemID int32) error {
	return s.mb.putOne(s.bagType, slot, itemID)
}

func (s *memStock) PutFirst(itemID int32) (int32, error) {
	slot, err := s.mb.findEmptySlot(s.bagType)
	if err != nil {
		return 0, err
	}
	if err := s.mb.putOne(s.bagType, slot, itemID); err != nil {
		return 0, err
	}
	return slot, nil
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
