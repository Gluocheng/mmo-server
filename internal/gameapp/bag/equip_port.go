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

// ensureSheet 取出穿着内存，首次使用时按请求里的玩家建好。
func (p *actorBag) ensureSheet(playerID, uid int64) *equip.Sheet {
	if p.sheet == nil {
		p.sheet = equip.NewSheet()
	}
	if playerID > 0 {
		p.sheet.PlayerID = playerID
	}
	if uid > 0 {
		p.sheet.UID = uid
	}
	return p.sheet
}

// equipApply 在背包 Actor 内一次做完穿脱引起的格子变更和穿着变更。
// 两者同属本 Actor 的邮箱，任何一步失败都不会留下半套状态。
func (p *actorBag) equipApply(req *protocol.EquipApply) (*protocol.EquipApplyResult, int32) {
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
	sheet := p.ensureSheet(req.PlayerId, req.Uid)
	if c := sheet.Mutate(req, &memStock{mb: mb, bagType: bagType}); c != code.OK {
		return nil, c
	}
	state, c := sheet.State()
	if c != code.OK {
		return nil, c
	}
	return &protocol.EquipApplyResult{State: state, Bag: mb.toProto(bagType)}, code.OK
}

// equipEnhance 在背包 Actor 内扣材料并改穿着等级。失败时两边都不变。
func (p *actorBag) equipEnhance(req *protocol.EquipEnhanceApply) (*protocol.EquipEnhanceResult, int32) {
	if req == nil || req.PlayerId < 1 {
		return nil, code.PlayerNotEntered
	}
	mb, err := p.ensureBag(req.PlayerId, equip.MaterialBag)
	if err != nil {
		return nil, code.BagLoadFail
	}
	sheet := p.ensureSheet(req.PlayerId, req.Uid)
	out, c := sheet.Enhance(req.Uid, req.SlotId, req.UseProtect, &memMat{mb: mb})
	if c != code.OK {
		return nil, c
	}
	state, sc := sheet.State()
	if sc != code.OK {
		return nil, sc
	}
	return &protocol.EquipEnhanceResult{
		Success: out.Success, Dropped: out.Dropped, ProtectUsed: out.ProtectUsed,
		Level: out.Level, Attrs: out.Attrs, Bag: mb.toProto(equip.MaterialBag), State: state,
	}, code.OK
}

// equipState 返回穿着。穿脱和落库都在本 Actor，读到的必然是一致的快照。
func (p *actorBag) equipState(req *protocol.EquipStateRequest) (*protocol.EquipState, int32) {
	if req == nil || req.PlayerId < 1 {
		return nil, code.PlayerNotEntered
	}
	return p.ensureSheet(req.PlayerId, req.Uid).State()
}

// equipAttach 进场或切图后把穿着叠到战斗单位上。
func (p *actorBag) equipAttach(req *protocol.EquipStateRequest) int32 {
	if req == nil || req.PlayerId < 1 {
		return code.PlayerNotEntered
	}
	p.ensureSheet(req.PlayerId, req.Uid).Attach(req.Uid, req.PlayerId)
	return code.OK
}
