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

func (p *actorBag) list(session *cproto.Session, _ *protocol.None) {
	playerID, ok := p.playerIDFromSession(session)
	if !ok {
		p.ResponseCode(session, code.PlayerNotEntered)
		return
	}
	bag, err := persistence.GetBagByPlayerID(playerID)
	if err != nil {
		clog.Warnf("bag list fail player_id=%d err=%v", playerID, err)
		p.ResponseCode(session, code.BagLoadFail)
		return
	}
	p.Response(session, bag)
}

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
	count := req.Count
	if count < 1 {
		count = 1
	}
	if err := persistence.AddOrStackItem(playerID, req.ItemId, count); err != nil {
		p.respondBagError(session, playerID, err)
		return
	}
	p.respondBagMutate(session, playerID)
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
	count := req.Count
	if count < 1 {
		count = 1
	}
	var err error
	if req.BySlot {
		err = persistence.RemoveItemAtSlot(playerID, req.Slot, count)
	} else {
		err = persistence.RemoveItem(playerID, req.ItemId, count)
	}
	if err != nil {
		p.respondBagError(session, playerID, err)
		return
	}
	p.respondBagMutate(session, playerID)
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
	if err := persistence.MoveItem(playerID, req.FromSlot, req.ToSlot); err != nil {
		p.respondBagError(session, playerID, err)
		return
	}
	p.respondBagMutate(session, playerID)
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
	count := req.Count
	if count < 1 {
		p.ResponseCode(session, code.BagItemInvalid)
		return
	}
	if err := persistence.SplitItem(playerID, req.FromSlot, count); err != nil {
		p.respondBagError(session, playerID, err)
		return
	}
	p.respondBagMutate(session, playerID)
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
	default:
		clog.Warnf("bag op fail player_id=%d err=%v", playerID, err)
		p.ResponseCode(session, code.BagLoadFail)
	}
}

// respondBagMutate 变更成功后：RPC 返回最新 BagListResponse，并 Push onBagChange（内容相同）。
func (p *actorBag) respondBagMutate(session *cproto.Session, playerID int64) {
	bag, err := persistence.GetBagByPlayerID(playerID)
	if err != nil {
		clog.Warnf("bag list after mutate fail player_id=%d err=%v", playerID, err)
		p.ResponseCode(session, code.BagLoadFail)
		return
	}
	p.Response(session, bag)
	p.Push(session, "onBagChange", bag)
}
