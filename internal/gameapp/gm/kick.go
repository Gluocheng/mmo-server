package gm

import (
	cfacade "github.com/cherry-game/cherry/facade"
	clog "github.com/cherry-game/cherry/logger"
	"github.com/cherry-game/cherry/net/parser/pomelo"
	cproto "github.com/cherry-game/cherry/net/proto"
	"github.com/example/mmo-server/internal/protocol"
)

type gmKicker interface {
	App() cfacade.IApplication
	Call(targetPath, funcName string, arg any) int32
}

type gmWaitCaller interface {
	gmKicker
	CallWait(targetPath, funcName string, arg, reply any) int32
}

// kickUIDOnGates 通知全部 gate 踢掉该 uid 的连接。
func kickUIDOnGates(p gmKicker, uid int64) {
	if p == nil || p.App() == nil || p.App().Discovery() == nil {
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

// kickAllOnGates 通知全部 gate 关闭本节点所有连接，返回踢到的连接数之和。
func kickAllOnGates(p gmWaitCaller) int32 {
	if p == nil || p.App() == nil || p.App().Discovery() == nil {
		return 0
	}
	var total int32
	members := p.App().Discovery().ListByType(gateNodeType)
	for _, member := range members {
		target := cfacade.NewPath(member.GetNodeID(), "ops")
		var rsp protocol.GmKickAllResponse
		if rc := p.CallWait(target, "kickAll", &protocol.GmKickAllRequest{}, &rsp); rc != 0 {
			clog.Warnf("gm kickAll call gate=%s code=%d", member.GetNodeID(), rc)
			continue
		}
		total += rsp.Kicked
	}
	return total
}
