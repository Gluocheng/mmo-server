package gm

import (
	"strings"

	cfacade "github.com/cherry-game/cherry/facade"
	clog "github.com/cherry-game/cherry/logger"
	"github.com/cherry-game/cherry/net/parser/pomelo"
	cproto "github.com/cherry-game/cherry/net/proto"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/gameapp/world"
	"github.com/example/mmo-server/internal/persistence"
	"github.com/example/mmo-server/internal/protocol"
)

const gateNodeType = "gate"

// actorGMPlayer GM 角色查询与踢人。
type actorGMPlayer struct {
	pomelo.ActorBase
}

func (p *actorGMPlayer) OnInit() {
	p.Remote().Register("query", p.queryCluster)
	p.Remote().Register("kick", p.kickCluster)
}

// queryCluster 按 playerId / name / uid 三选一查角色（含软删）。
func (p *actorGMPlayer) queryCluster(req *protocol.GmPlayerQueryRequest) (*protocol.GmPlayerQueryResponse, int32) {
	if req == nil {
		return nil, code.GmBadRequest
	}
	name := strings.TrimSpace(req.Name)
	if !exactlyOne(req.PlayerId > 0, name != "", req.Uid > 0) {
		return nil, code.GmBadRequest
	}
	rsp := &protocol.GmPlayerQueryResponse{}
	switch {
	case req.PlayerId > 0:
		rec, found, err := persistence.GetPlayerByID(req.PlayerId)
		if err != nil {
			clog.Warnf("gm player query by id: %v", err)
			return nil, code.GmTargetNotFound
		}
		if !found {
			return nil, code.GmTargetNotFound
		}
		rsp.List = []*protocol.GmPlayerRecord{rec}
	case name != "":
		list, err := persistence.GetPlayersByName(name)
		if err != nil {
			clog.Warnf("gm player query by name: %v", err)
			return nil, code.GmTargetNotFound
		}
		if len(list) == 0 {
			return nil, code.GmTargetNotFound
		}
		rsp.List = list
	default:
		_, accFound, err := persistence.GetAccountByUID(req.Uid)
		if err != nil || !accFound {
			return nil, code.GmTargetNotFound
		}
		list, err := persistence.ListPlayersByUIDAll(req.Uid)
		if err != nil {
			clog.Warnf("gm player query by uid: %v", err)
			return nil, code.GmTargetNotFound
		}
		rsp.List = list
	}
	return rsp, code.OK
}

// kickCluster 按 uid 或 playerId 二选一踢下线；离线也返回成功。
func (p *actorGMPlayer) kickCluster(req *protocol.GmKickRequest) (*protocol.GmKickResponse, int32) {
	operator := ""
	var uid, playerID int64
	if req != nil {
		operator = req.Operator
		uid = req.Uid
		playerID = req.PlayerId
	}
	if !exactlyOne(uid > 0, playerID > 0) {
		persistence.WriteGMOpLog(operator, persistence.GMActionKick, uid, playerID, "", code.GmBadRequest)
		return nil, code.GmBadRequest
	}
	if playerID > 0 {
		rec, found, err := persistence.GetPlayerByID(playerID)
		if err != nil || !found {
			persistence.WriteGMOpLog(operator, persistence.GMActionKick, 0, playerID, "", code.GmTargetNotFound)
			return nil, code.GmTargetNotFound
		}
		uid = rec.Uid
	}
	_, inRoom := world.AgentPath(uid)
	p.kickUIDOnGates(uid)
	rsp := &protocol.GmKickResponse{Uid: uid, Kicked: inRoom}
	persistence.WriteGMOpLog(operator, persistence.GMActionKick, uid, playerID, "", code.OK)
	return rsp, code.OK
}

func (p *actorGMPlayer) kickUIDOnGates(uid int64) {
	if p.App() == nil || p.App().Discovery() == nil {
		return
	}
	kick := &cproto.PomeloKick{Uid: uid, Reason: []byte{}, Close: true}
	members := p.App().Discovery().ListByType(gateNodeType)
	for _, member := range members {
		target := cfacade.NewPath(member.GetNodeID(), "user")
		if rc := p.Call(target, pomelo.KickFuncName, kick); rc != 0 {
			clog.Warnf("gm kick call gate=%s uid=%d code=%d", member.GetNodeID(), uid, rc)
		}
	}
}
