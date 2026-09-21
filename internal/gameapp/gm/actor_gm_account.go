package gm

import (
	"strings"

	clog "github.com/cherry-game/cherry/logger"
	"github.com/cherry-game/cherry/net/parser/pomelo"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/gameapp/world"
	"github.com/example/mmo-server/internal/persistence"
	"github.com/example/mmo-server/internal/protocol"
)

// actorGMAccount GM 账号查询与封禁。
type actorGMAccount struct {
	pomelo.ActorBase
}

func (p *actorGMAccount) OnInit() {
	p.Remote().Register("query", p.queryCluster)
	p.Remote().Register("ban", p.banCluster)
	p.Remote().Register("unban", p.unbanCluster)
}

// queryCluster 按 uid 或 nickname 二选一查账号。
func (p *actorGMAccount) queryCluster(req *protocol.GmAccountQueryRequest) (*protocol.GmAccountView, int32) {
	if req == nil {
		return nil, code.GmBadRequest
	}
	uidSet := req.Uid > 0
	nick := strings.TrimSpace(req.Nickname)
	nickSet := nick != ""
	if !exactlyOne(uidSet, nickSet) {
		return nil, code.GmBadRequest
	}
	var (
		view  *protocol.GmAccountView
		found bool
		err   error
	)
	if uidSet {
		view, found, err = persistence.GetAccountByUID(req.Uid)
	} else {
		view, found, err = persistence.GetAccountByNickname(nick)
	}
	if err != nil {
		return nil, code.GmTargetNotFound
	}
	if !found {
		return nil, code.GmTargetNotFound
	}
	return view, code.OK
}

// banCluster 永久封禁账号并踢下线。
func (p *actorGMAccount) banCluster(req *protocol.GmBanRequest) (*protocol.GmBanResponse, int32) {
	operator := ""
	if req != nil {
		operator = req.Operator
	}
	view, c := p.resolveAccount(req)
	if c != code.OK {
		persistence.WriteGMOpLog(operator, persistence.GMActionBan, 0, 0, "", c)
		return nil, c
	}
	reason := ""
	if req != nil {
		reason = strings.TrimSpace(req.Reason)
	}
	found, err := persistence.SetAccountBanned(view.Uid, true, reason)
	if err != nil {
		clog.Warnf("gm ban uid=%d err=%v", view.Uid, err)
		persistence.WriteGMOpLog(operator, persistence.GMActionBan, view.Uid, 0, reason, code.GmTargetNotFound)
		return nil, code.GmTargetNotFound
	}
	if !found {
		persistence.WriteGMOpLog(operator, persistence.GMActionBan, view.Uid, 0, reason, code.GmTargetNotFound)
		return nil, code.GmTargetNotFound
	}
	_, inRoom := world.AgentPath(view.Uid)
	kickUIDOnGates(p, view.Uid)
	persistence.WriteGMOpLog(operator, persistence.GMActionBan, view.Uid, 0, reason, code.OK)
	return &protocol.GmBanResponse{Uid: view.Uid, Banned: true, Kicked: inRoom}, code.OK
}

// unbanCluster 解除账号封禁。
func (p *actorGMAccount) unbanCluster(req *protocol.GmBanRequest) (*protocol.GmBanResponse, int32) {
	operator := ""
	if req != nil {
		operator = req.Operator
	}
	view, c := p.resolveAccount(req)
	if c != code.OK {
		persistence.WriteGMOpLog(operator, persistence.GMActionUnban, 0, 0, "", c)
		return nil, c
	}
	found, err := persistence.SetAccountBanned(view.Uid, false, "")
	if err != nil {
		clog.Warnf("gm unban uid=%d err=%v", view.Uid, err)
		persistence.WriteGMOpLog(operator, persistence.GMActionUnban, view.Uid, 0, "", code.GmTargetNotFound)
		return nil, code.GmTargetNotFound
	}
	if !found {
		persistence.WriteGMOpLog(operator, persistence.GMActionUnban, view.Uid, 0, "", code.GmTargetNotFound)
		return nil, code.GmTargetNotFound
	}
	persistence.WriteGMOpLog(operator, persistence.GMActionUnban, view.Uid, 0, "", code.OK)
	return &protocol.GmBanResponse{Uid: view.Uid, Banned: false}, code.OK
}

func (p *actorGMAccount) resolveAccount(req *protocol.GmBanRequest) (*protocol.GmAccountView, int32) {
	if req == nil {
		return nil, code.GmBadRequest
	}
	uidSet := req.Uid > 0
	nick := strings.TrimSpace(req.Nickname)
	nickSet := nick != ""
	if !exactlyOne(uidSet, nickSet) {
		return nil, code.GmBadRequest
	}
	var (
		view  *protocol.GmAccountView
		found bool
		err   error
	)
	if uidSet {
		view, found, err = persistence.GetAccountByUID(req.Uid)
	} else {
		view, found, err = persistence.GetAccountByNickname(nick)
	}
	if err != nil || !found {
		return nil, code.GmTargetNotFound
	}
	return view, code.OK
}
