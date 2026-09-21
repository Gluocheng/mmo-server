package gm

import (
	"strconv"
	"strings"

	clog "github.com/cherry-game/cherry/logger"
	"github.com/cherry-game/cherry/net/parser/pomelo"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/gameapp/world"
	"github.com/example/mmo-server/internal/persistence"
	"github.com/example/mmo-server/internal/protocol"
)

// actorGMAccount GM 账号查询、封禁与禁言。
type actorGMAccount struct {
	pomelo.ActorBase
}

func (p *actorGMAccount) OnInit() {
	p.Remote().Register("query", p.queryCluster)
	p.Remote().Register("ban", p.banCluster)
	p.Remote().Register("unban", p.unbanCluster)
	p.Remote().Register("mute", p.muteCluster)
	p.Remote().Register("unmute", p.unmuteCluster)
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

// banCluster 封禁账号、吊销 token 并踢下线；durationSeconds=0 为永久。
func (p *actorGMAccount) banCluster(req *protocol.GmBanRequest) (*protocol.GmBanResponse, int32) {
	operator := ""
	var duration int64
	if req != nil {
		operator = req.Operator
		duration = req.DurationSeconds
	}
	if duration < 0 {
		persistence.WriteGMOpLog(operator, persistence.GMActionBan, 0, 0, "", code.GmBadRequest)
		return nil, code.GmBadRequest
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
	found, err := persistence.SetAccountBannedFor(view.Uid, true, reason, duration)
	if err != nil {
		clog.Warnf("gm ban uid=%d err=%v", view.Uid, err)
		persistence.WriteGMOpLog(operator, persistence.GMActionBan, view.Uid, 0, reason, code.GmTargetNotFound)
		return nil, code.GmTargetNotFound
	}
	if !found {
		persistence.WriteGMOpLog(operator, persistence.GMActionBan, view.Uid, 0, reason, code.GmTargetNotFound)
		return nil, code.GmTargetNotFound
	}
	tokensRevoked := true
	if err := persistence.RevokeAllTokensForUID(view.Uid); err != nil {
		clog.Warnf("gm ban revoke tokens uid=%d err=%v", view.Uid, err)
		tokensRevoked = false
	}
	_, inRoom := world.AgentPath(view.Uid)
	kickUIDOnGates(p, view.Uid)
	after, _, _ := persistence.GetAccountByUID(view.Uid)
	until := int64(0)
	if after != nil {
		until = after.BannedUntil
	}
	detail := reason
	if duration > 0 {
		detail = reason + " duration=" + strconv.FormatInt(duration, 10)
	}
	persistence.WriteGMOpLog(operator, persistence.GMActionBan, view.Uid, 0, detail, code.OK)
	return &protocol.GmBanResponse{
		Uid:           view.Uid,
		Banned:        true,
		Kicked:        inRoom,
		BannedUntil:   until,
		TokensRevoked: tokensRevoked,
	}, code.OK
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

// muteCluster 禁言账号，不踢线。
func (p *actorGMAccount) muteCluster(req *protocol.GmBanRequest) (*protocol.GmMuteResponse, int32) {
	operator := ""
	var duration int64
	if req != nil {
		operator = req.Operator
		duration = req.DurationSeconds
	}
	if duration < 0 {
		persistence.WriteGMOpLog(operator, persistence.GMActionMute, 0, 0, "", code.GmBadRequest)
		return nil, code.GmBadRequest
	}
	view, c := p.resolveAccount(req)
	if c != code.OK {
		persistence.WriteGMOpLog(operator, persistence.GMActionMute, 0, 0, "", c)
		return nil, c
	}
	reason := ""
	if req != nil {
		reason = strings.TrimSpace(req.Reason)
	}
	found, err := persistence.SetAccountMuted(view.Uid, true, reason, duration)
	if err != nil || !found {
		clog.Warnf("gm mute uid=%d found=%v err=%v", view.Uid, found, err)
		persistence.WriteGMOpLog(operator, persistence.GMActionMute, view.Uid, 0, reason, code.GmTargetNotFound)
		return nil, code.GmTargetNotFound
	}
	after, _, _ := persistence.GetAccountByUID(view.Uid)
	until := int64(0)
	if after != nil {
		until = after.MutedUntil
	}
	detail := reason
	if duration > 0 {
		detail = reason + " duration=" + strconv.FormatInt(duration, 10)
	}
	persistence.WriteGMOpLog(operator, persistence.GMActionMute, view.Uid, 0, detail, code.OK)
	return &protocol.GmMuteResponse{Uid: view.Uid, Muted: true, MutedUntil: until}, code.OK
}

// unmuteCluster 解除禁言。
func (p *actorGMAccount) unmuteCluster(req *protocol.GmBanRequest) (*protocol.GmMuteResponse, int32) {
	operator := ""
	if req != nil {
		operator = req.Operator
	}
	view, c := p.resolveAccount(req)
	if c != code.OK {
		persistence.WriteGMOpLog(operator, persistence.GMActionUnmute, 0, 0, "", c)
		return nil, c
	}
	found, err := persistence.SetAccountMuted(view.Uid, false, "", 0)
	if err != nil || !found {
		clog.Warnf("gm unmute uid=%d found=%v err=%v", view.Uid, found, err)
		persistence.WriteGMOpLog(operator, persistence.GMActionUnmute, view.Uid, 0, "", code.GmTargetNotFound)
		return nil, code.GmTargetNotFound
	}
	persistence.WriteGMOpLog(operator, persistence.GMActionUnmute, view.Uid, 0, "", code.OK)
	return &protocol.GmMuteResponse{Uid: view.Uid, Muted: false}, code.OK
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
