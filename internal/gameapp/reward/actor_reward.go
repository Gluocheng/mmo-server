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
	"github.com/example/mmo-server/internal/gameapp/world"
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
// 在线改经 bag Actor 内存，离线直写 DB。
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
	// 在线：CallWait 到 bag Actor 领取并同步拿回最新背包。框架码（超时/未找到）降级直写 DB。
	if _, ok := world.AgentPath(session.Uid); ok {
		claimReq := &protocol.RewardClaimRequest{PlayerId: playerID, ClaimId: req.ClaimId}
		var rsp protocol.RewardClaimResponse
		rc := p.CallWait(p.NewChildPath("bag", session.Uid), "rewardClaim", claimReq, &rsp)
		if rc == code.OK {
			p.Response(session, &protocol.RewardTakeResponse{Bag: rsp.Bag})
			return
		}
		if rc >= 40000 {
			p.ResponseCode(session, rc)
			return
		}
		clog.Warnf("reward take in-memory fail uid=%d rc=%d, fallback to db", session.Uid, rc)
		bagType, err := persistence.TakeRewardClaim(context.Background(), playerID, req.ClaimId)
		if err != nil {
			p.respondTakeErr(session, playerID, req.ClaimId, err)
			return
		}
		// 直写 DB 成功；失效内存让下次操作懒加载重读，避免与 DB 分叉。
		p.Call(p.NewChildPath("bag", session.Uid), "invalidate", nil)
		bag, err := persistence.GetBagByPlayerID(playerID, bagType)
		if err != nil {
			p.ResponseCode(session, code.BagLoadFail)
			return
		}
		p.Response(session, &protocol.RewardTakeResponse{Bag: bag})
		p.Push(session, "onBagChange", bag)
		return
	}
	bagType, err := persistence.TakeRewardClaim(context.Background(), playerID, req.ClaimId)
	if err != nil {
		p.respondTakeErr(session, playerID, req.ClaimId, err)
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

// respondTakeErr 把领取错误映射为业务码。
func (p *actorReward) respondTakeErr(session *cproto.Session, playerID, claimID int64, err error) {
	switch {
	case errors.Is(err, persistence.ErrBagFull):
		p.ResponseCode(session, code.BagFull)
	case errors.Is(err, persistence.ErrRewardClaimMissing):
		p.ResponseCode(session, code.ItemNotFound)
	default:
		clog.Warnf("reward take player=%d claim=%d err=%v", playerID, claimID, err)
		p.ResponseCode(session, code.BagLoadFail)
	}
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
