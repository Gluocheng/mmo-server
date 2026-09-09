package bag

import (
	"errors"
	"strconv"

	clog "github.com/cherry-game/cherry/logger"
	"github.com/cherry-game/cherry/net/parser/pomelo"
	cproto "github.com/cherry-game/cherry/net/proto"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/persistence"
	"github.com/example/mmo-server/internal/protocol"
	"github.com/example/mmo-server/internal/sessionkey"
)

// actorBag 背包玩法 Actor：玩家进场（Session 含 PlayerID）后可访问。
type actorBag struct {
	pomelo.ActorBase
}

// OnInit 注册路由 list / add / remove / move / split（对应 game.bag.*）。
func (p *actorBag) OnInit() {
	p.Local().Register("list", p.list)
	p.Local().Register("add", p.add)
	p.Local().Register("remove", p.remove)
	p.Local().Register("move", p.move)
	p.Local().Register("split", p.split)
}

func (p *actorBag) playerIDFromSession(session *cproto.Session) (int64, bool) {
	if session == nil || !session.Contains(sessionkey.PlayerID) {
		return 0, false
	}
	playerID, err := strconv.ParseInt(session.GetString(sessionkey.PlayerID), 10, 64)
	if err != nil || playerID < 1 {
		return 0, false
	}
	return playerID, true
}

// list 返回指定背包内容；bag_type 必填。
func (p *actorBag) list(session *cproto.Session, req *protocol.BagListRequest) {
	playerID, ok := p.playerIDFromSession(session)
	if !ok {
		p.ResponseCode(session, code.PlayerNotEntered)
		return
	}
	if req == nil || req.BagType < 1 {
		p.ResponseCode(session, code.BagSlotInvalid)
		return
	}
	bag, err := persistence.GetBagByPlayerID(playerID, req.BagType)
	if err != nil {
		clog.Warnf("bag list fail player_id=%d bag_type=%d err=%v", playerID, req.BagType, err)
		p.ResponseCode(session, code.BagLoadFail)
		return
	}
	p.Response(session, bag)
}

// add 发放物品：默认按 item.bag_type 自动路由；req.BagType>0 时显式指定（GM），仍严格校验类别。
func (p *actorBag) add(session *cproto.Session, req *protocol.BagAddRequest) {
	playerID, ok := p.playerIDFromSession(session)
	if !ok {
		p.ResponseCode(session, code.PlayerNotEntered)
		return
	}
	if req == nil {
		p.ResponseCode(session, code.BagItemInvalid)
		return
	}
	count := max(req.Count, 1)

	var err error
	var bagType int32
	if req.BagType > 0 {
		// 显式指定目标背包（GM）：严格校验 item.bag_type == req.BagType
		bagType = req.BagType
		err = persistence.AddOrStackItemToBag(playerID, bagType, req.ItemId, count)
	} else {
		// 自动路由：按 item.bag_type
		err = persistence.AddOrStackItem(playerID, req.ItemId, count)
		if err == nil {
			bagType, _ = persistence.ResolveItemBagType(req.ItemId)
		}
	}
	if err != nil {
		p.respondBagError(session, playerID, err)
		return
	}
	p.respondBagMutate(session, playerID, bagType)
}

// remove 扣除物品：bySlot=true 按槽位扣减，否则按 itemId 跨槽合计扣减。
func (p *actorBag) remove(session *cproto.Session, req *protocol.BagRemoveRequest) {
	playerID, ok := p.playerIDFromSession(session)
	if !ok {
		p.ResponseCode(session, code.PlayerNotEntered)
		return
	}
	if req == nil {
		p.ResponseCode(session, code.BagItemInvalid)
		return
	}
	if req.BagType < 1 {
		p.ResponseCode(session, code.BagSlotInvalid)
		return
	}
	count := req.Count
	if count < 1 {
		count = 1
	}
	var err error
	if req.BySlot {
		err = persistence.RemoveItemAtSlot(playerID, req.BagType, req.Slot, count)
	} else {
		err = persistence.RemoveItem(playerID, req.BagType, req.ItemId, count)
	}
	if err != nil {
		p.respondBagError(session, playerID, err)
		return
	}
	p.respondBagMutate(session, playerID, req.BagType)
}

func (p *actorBag) move(session *cproto.Session, req *protocol.BagMoveRequest) {
	playerID, ok := p.playerIDFromSession(session)
	if !ok {
		p.ResponseCode(session, code.PlayerNotEntered)
		return
	}
	if req == nil {
		p.ResponseCode(session, code.BagItemInvalid)
		return
	}
	if req.BagType < 1 {
		p.ResponseCode(session, code.BagSlotInvalid)
		return
	}
	if err := persistence.MoveItem(playerID, req.BagType, req.FromSlot, req.ToSlot); err != nil {
		p.respondBagError(session, playerID, err)
		return
	}
	p.respondBagMutate(session, playerID, req.BagType)
}

func (p *actorBag) split(session *cproto.Session, req *protocol.BagSplitRequest) {
	playerID, ok := p.playerIDFromSession(session)
	if !ok {
		p.ResponseCode(session, code.PlayerNotEntered)
		return
	}
	if req == nil {
		p.ResponseCode(session, code.BagItemInvalid)
		return
	}
	if req.BagType < 1 {
		p.ResponseCode(session, code.BagSlotInvalid)
		return
	}
	count := req.Count
	if count < 1 {
		p.ResponseCode(session, code.BagItemInvalid)
		return
	}
	if err := persistence.SplitItem(playerID, req.BagType, req.FromSlot, count); err != nil {
		p.respondBagError(session, playerID, err)
		return
	}
	p.respondBagMutate(session, playerID, req.BagType)
}

func (p *actorBag) respondBagError(session *cproto.Session, playerID int64, err error) {
	switch {
	case errors.Is(err, persistence.ErrBagInvalid):
		p.ResponseCode(session, code.BagItemInvalid)
	case errors.Is(err, persistence.ErrBagNotEnough):
		p.ResponseCode(session, code.BagItemNotEnough)
	case errors.Is(err, persistence.ErrBagSlotInvalid):
		p.ResponseCode(session, code.BagSlotInvalid)
	case errors.Is(err, persistence.ErrBagFull):
		p.ResponseCode(session, code.BagFull)
	case errors.Is(err, persistence.ErrItemNotFound):
		p.ResponseCode(session, code.ItemNotFound)
	case errors.Is(err, persistence.ErrBagTypeMismatch):
		p.ResponseCode(session, code.BagTypeMismatch)
	default:
		clog.Warnf("bag op fail player_id=%d err=%v", playerID, err)
		p.ResponseCode(session, code.BagLoadFail)
	}
}

// respondBagMutate 变更成功后：RPC 返回最新 BagListResponse，并 Push onBagChange（内容相同）。
func (p *actorBag) respondBagMutate(session *cproto.Session, playerID int64, bagType int32) {
	bag, err := persistence.GetBagByPlayerID(playerID, bagType)
	if err != nil {
		clog.Warnf("bag list after mutate fail player_id=%d bag_type=%d err=%v", playerID, bagType, err)
		p.ResponseCode(session, code.BagLoadFail)
		return
	}
	p.Response(session, bag)
	p.Push(session, "onBagChange", bag)
}
