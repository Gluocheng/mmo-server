package gm

import (
	"fmt"

	clog "github.com/cherry-game/cherry/logger"
	"github.com/cherry-game/cherry/net/parser/pomelo"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/gameapp/world"
	"github.com/example/mmo-server/internal/persistence"
	"github.com/example/mmo-server/internal/protocol"
)

// actorGMBag GM 背包查询与发放。
type actorGMBag struct {
	pomelo.ActorBase
}

func (p *actorGMBag) OnInit() {
	p.Remote().Register("query", p.queryCluster)
	p.Remote().Register("grant", p.grantCluster)
	p.Remote().Register("deduct", p.deductCluster)
}

// queryCluster 按 playerId + bagType 查看背包。
func (p *actorGMBag) queryCluster(req *protocol.GmBagQueryRequest) (*protocol.BagListResponse, int32) {
	if req == nil || req.PlayerId < 1 || req.BagType < 1 {
		return nil, code.GmBadRequest
	}
	_, found, err := persistence.GetPlayerByID(req.PlayerId)
	if err != nil {
		clog.Warnf("gm bag query player: %v", err)
		return nil, code.BagLoadFail
	}
	if !found {
		return nil, code.GmTargetNotFound
	}
	bag, err := persistence.GetBagByPlayerID(req.PlayerId, req.BagType)
	if err != nil {
		clog.Warnf("gm bag query fail player_id=%d bag_type=%d err=%v", req.PlayerId, req.BagType, err)
		return nil, code.BagLoadFail
	}
	return bag, code.OK
}

// grantCluster 按 player_id 发放道具；已删除角色拒绝。在线则尽力 Push onBagChange。
func (p *actorGMBag) grantCluster(req *protocol.GmGrantRequest) (*protocol.GmGrantResponse, int32) {
	operator := ""
	var playerID int64
	if req != nil {
		operator = req.Operator
		playerID = req.PlayerId
	}
	rsp, c := p.doGrant(req)
	detail := ""
	if req != nil {
		detail = fmt.Sprintf("itemId=%d count=%d bagType=%d", req.ItemId, req.Count, req.BagType)
	}
	var uid int64
	if rec, found, err := persistence.GetPlayerByID(playerID); err == nil && found {
		uid = rec.Uid
	}
	persistence.WriteGMOpLog(operator, persistence.GMActionGrant, uid, playerID, detail, c)
	return rsp, c
}

func (p *actorGMBag) doGrant(req *protocol.GmGrantRequest) (*protocol.GmGrantResponse, int32) {
	if req == nil || req.PlayerId < 1 || req.ItemId < 1 {
		return nil, code.GmBadRequest
	}
	rec, found, err := persistence.GetPlayerByID(req.PlayerId)
	if err != nil {
		clog.Warnf("gm grant lookup player_id=%d err=%v", req.PlayerId, err)
		return nil, code.BagLoadFail
	}
	if !found || rec.Deleted {
		return nil, code.GmTargetNotFound
	}
	count := req.Count
	if count < 1 {
		count = 1
	}

	var bagType int32
	if req.BagType > 0 {
		bagType = req.BagType
		err = persistence.AddOrStackItemToBag(req.PlayerId, bagType, req.ItemId, count)
	} else {
		err = persistence.AddOrStackItem(req.PlayerId, req.ItemId, count)
		if err == nil {
			bagType, _ = persistence.ResolveItemBagType(req.ItemId)
		}
	}
	if err != nil {
		return nil, bagErrorCode(err)
	}
	bag, err := persistence.GetBagByPlayerID(req.PlayerId, bagType)
	if err != nil {
		clog.Warnf("gm grant list fail player_id=%d bag_type=%d err=%v", req.PlayerId, bagType, err)
		return nil, code.BagLoadFail
	}
	pushed := false
	if path, ok := world.AgentPath(rec.Uid); ok {
		pomelo.PushWithUID(p, path, rec.Uid, "onBagChange", bag)
		pushed = true
	}
	return &protocol.GmGrantResponse{BagType: bagType, Bag: bag, Pushed: pushed}, code.OK
}

// deductCluster 按 player_id 扣道具；itemId 优先，否则按槽位。已删除角色拒绝。
func (p *actorGMBag) deductCluster(req *protocol.GmDeductRequest) (*protocol.GmGrantResponse, int32) {
	operator := ""
	var playerID int64
	if req != nil {
		operator = req.Operator
		playerID = req.PlayerId
	}
	rsp, c := p.doDeduct(req)
	detail := ""
	if req != nil {
		detail = fmt.Sprintf("itemId=%d slot=%d count=%d bagType=%d", req.ItemId, req.Slot, req.Count, req.BagType)
	}
	var uid int64
	if rec, found, err := persistence.GetPlayerByID(playerID); err == nil && found {
		uid = rec.Uid
	}
	persistence.WriteGMOpLog(operator, persistence.GMActionDeduct, uid, playerID, detail, c)
	return rsp, c
}

func (p *actorGMBag) doDeduct(req *protocol.GmDeductRequest) (*protocol.GmGrantResponse, int32) {
	if req == nil || req.PlayerId < 1 {
		return nil, code.GmBadRequest
	}
	if req.ItemId < 1 && req.BagType < 1 {
		return nil, code.GmBadRequest
	}
	rec, found, err := persistence.GetPlayerByID(req.PlayerId)
	if err != nil {
		clog.Warnf("gm deduct lookup player_id=%d err=%v", req.PlayerId, err)
		return nil, code.BagLoadFail
	}
	if !found || rec.Deleted {
		return nil, code.GmTargetNotFound
	}
	count := req.Count
	if count < 1 {
		count = 1
	}

	var bagType int32
	if req.ItemId > 0 {
		if req.BagType > 0 {
			bagType = req.BagType
		} else {
			bagType, err = persistence.ResolveItemBagType(req.ItemId)
			if err != nil {
				return nil, bagErrorCode(err)
			}
		}
		err = persistence.RemoveItem(req.PlayerId, bagType, req.ItemId, count)
	} else {
		bagType = req.BagType
		err = persistence.RemoveItemAtSlot(req.PlayerId, bagType, req.Slot, count)
	}
	if err != nil {
		return nil, bagErrorCode(err)
	}
	bag, err := persistence.GetBagByPlayerID(req.PlayerId, bagType)
	if err != nil {
		clog.Warnf("gm deduct list fail player_id=%d bag_type=%d err=%v", req.PlayerId, bagType, err)
		return nil, code.BagLoadFail
	}
	pushed := false
	if path, ok := world.AgentPath(rec.Uid); ok {
		pomelo.PushWithUID(p, path, rec.Uid, "onBagChange", bag)
		pushed = true
	}
	return &protocol.GmGrantResponse{BagType: bagType, Bag: bag, Pushed: pushed}, code.OK
}
