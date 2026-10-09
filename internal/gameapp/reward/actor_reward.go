package reward

import (
	"context"
	"errors"
	"strconv"

	cfacade "github.com/cherry-game/cherry/facade"
	clog "github.com/cherry-game/cherry/logger"
	"github.com/cherry-game/cherry/net/parser/pomelo"
	cproto "github.com/cherry-game/cherry/net/proto"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/persistence"
	"github.com/example/mmo-server/internal/protocol"
	"github.com/example/mmo-server/internal/sessionkey"
)

// ActorRewards 待领取根 Actor，按 uid 创建子 Actor。
type ActorRewards struct {
	pomelo.ActorBase
}

// AliasID 路由前缀 game.reward。
func (p *ActorRewards) AliasID() string { return "reward" }

// OnFindChild 按 uid 创建领奖子 Actor。
func (p *ActorRewards) OnFindChild(msg *cfacade.Message) (cfacade.IActor, bool) {
	childID := msg.TargetPath().ChildID
	childActor, err := p.Child().Create(childID, &actorReward{})
	if err != nil {
		clog.Warnf("create reward child fail: %v", err)
		return nil, false
	}
	return childActor, true
}

type actorReward struct {
	pomelo.ActorBase
}

// OnInit 注册 list 和 take。
func (p *actorReward) OnInit() {
	p.Local().Register("list", p.list)
	p.Local().Register("take", p.take)
}

// list 返回当前角色还没领走的奖励。未进场返回 40009。
func (p *actorReward) list(session *cproto.Session, _ *protocol.None) {
	playerID, ok := playerIDFrom(session)
	if !ok {
		p.ResponseCode(session, code.PlayerNotEntered)
		return
	}
	rows, err := persistence.ListRewardClaims(context.Background(), playerID)
	if err != nil {
		clog.Warnf("reward list player=%d err=%v", playerID, err)
		p.ResponseCode(session, code.BagLoadFail)
		return
	}
	rsp := &protocol.RewardListResponse{Claims: make([]*protocol.RewardClaim, 0, len(rows))}
	for _, row := range rows {
		rsp.Claims = append(rsp.Claims, &protocol.RewardClaim{
			Id: row.ID, ItemId: row.ItemID, Count: row.Count, Reason: row.Reason, MonsterId: row.MonsterID,
		})
	}
	p.Response(session, rsp)
}

// take 领取一条待领取。背包仍满返回 40022，这条继续留着；不存在返回 40023。
func (p *actorReward) take(session *cproto.Session, req *protocol.RewardTakeRequest) {
	playerID, ok := playerIDFrom(session)
	if !ok {
		p.ResponseCode(session, code.PlayerNotEntered)
		return
	}
	if req == nil || req.ClaimId < 1 {
		p.ResponseCode(session, code.BagItemInvalid)
		return
	}
	bagType, err := persistence.TakeRewardClaim(context.Background(), playerID, req.ClaimId)
	if err != nil {
		switch {
		case errors.Is(err, persistence.ErrBagFull):
			p.ResponseCode(session, code.BagFull)
		case errors.Is(err, persistence.ErrRewardClaimMissing):
			p.ResponseCode(session, code.ItemNotFound)
		default:
			clog.Warnf("reward take player=%d claim=%d err=%v", playerID, req.ClaimId, err)
			p.ResponseCode(session, code.BagLoadFail)
		}
		return
	}
	bag, err := persistence.GetBagByPlayerID(playerID, bagType)
	if err != nil {
		p.ResponseCode(session, code.BagLoadFail)
		return
	}
	p.Response(session, &protocol.RewardTakeResponse{Bag: bag})
	p.Push(session, "onBagChange", bag)
}

// playerIDFrom 从已进场的 session 取出角色 id。没有 player_id 时 ok 为 false。
func playerIDFrom(session *cproto.Session) (int64, bool) {
	if session == nil || !session.Contains(sessionkey.PlayerID) {
		return 0, false
	}
	playerID, err := strconv.ParseInt(session.GetString(sessionkey.PlayerID), 10, 64)
	if err != nil || playerID < 1 {
		return 0, false
	}
	return playerID, true
}
