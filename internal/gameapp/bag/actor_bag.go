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

type actorBag struct {
	pomelo.ActorBase
}

func (p *actorBag) OnInit() {
	p.Local().Register("list", p.list)
	p.Local().Register("add", p.add)
	p.Local().Register("remove", p.remove)
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
		if errors.Is(err, persistence.ErrBagInvalid) {
			p.ResponseCode(session, code.BagItemInvalid)
			return
		}
		clog.Warnf("bag add fail player_id=%d item_id=%d err=%v", playerID, req.ItemId, err)
		p.ResponseCode(session, code.BagLoadFail)
		return
	}
	p.listAfterMutate(session, playerID)
}

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
	if err := persistence.RemoveItem(playerID, req.ItemId, count); err != nil {
		if errors.Is(err, persistence.ErrBagInvalid) {
			p.ResponseCode(session, code.BagItemInvalid)
			return
		}
		if errors.Is(err, persistence.ErrBagNotEnough) {
			p.ResponseCode(session, code.BagItemNotEnough)
			return
		}
		clog.Warnf("bag remove fail player_id=%d item_id=%d err=%v", playerID, req.ItemId, err)
		p.ResponseCode(session, code.BagLoadFail)
		return
	}
	p.listAfterMutate(session, playerID)
}

func (p *actorBag) listAfterMutate(session *cproto.Session, playerID int64) {
	bag, err := persistence.GetBagByPlayerID(playerID)
	if err != nil {
		clog.Warnf("bag list after mutate fail player_id=%d err=%v", playerID, err)
		p.ResponseCode(session, code.BagLoadFail)
		return
	}
	p.Response(session, bag)
}
