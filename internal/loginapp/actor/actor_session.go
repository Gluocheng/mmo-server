package actor

import (
	"context"
	"errors"
	"strconv"
	"strings"

	cfacade "github.com/cherry-game/cherry/facade"
	clog "github.com/cherry-game/cherry/logger"
	cactor "github.com/cherry-game/cherry/net/actor"
	"github.com/example/mmo-server/internal/authcfg"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/gtime"
	"github.com/example/mmo-server/internal/persistence"
	"github.com/example/mmo-server/internal/protocol"
)

// ActorSession 登录服父 Actor，路径 {node}.session。只处理调时间，签发由工人并行处理。
type ActorSession struct {
	cactor.Base
}

func (p *ActorSession) AliasID() string {
	return "session"
}

// OnInit 注册调时间，并按配置建好 session 工人。
func (p *ActorSession) OnInit() {
	p.Remote().Register("setGameTime", p.setGameTime)
	n := authcfg.SessionWorkers()
	for i := 0; i < n; i++ {
		if _, err := p.Child().Create(strconv.Itoa(i), &ActorSessionWorker{}); err != nil {
			clog.Warnf("create login session worker %d: %v", i, err)
		}
	}
}

// OnFindChild 补建尚未存在的工人。编号超出配置范围时不创建。
func (p *ActorSession) OnFindChild(msg *cfacade.Message) (cfacade.IActor, bool) {
	if msg == nil || msg.TargetPath() == nil {
		return nil, false
	}
	id := msg.TargetPath().ChildID
	n, err := strconv.Atoi(id)
	if err != nil || n < 0 || n >= authcfg.SessionWorkers() {
		return nil, false
	}
	child, err := p.Child().Create(id, &ActorSessionWorker{})
	if err != nil {
		clog.Warnf("create login session worker %s: %v", id, err)
		return nil, false
	}
	return child, true
}

// ActorSessionWorker 处理签发、校验、刷新和登出。每个工人独占一个 goroutine。
type ActorSessionWorker struct {
	cactor.Base
}

func (p *ActorSessionWorker) OnInit() {
	p.Remote().Register("issueToken", p.issueToken)
	p.Remote().Register("authToken", p.authToken)
	p.Remote().Register("refreshToken", p.refreshToken)
	p.Remote().Register("logout", p.logout)
}

func (p *ActorSessionWorker) issueToken(req *protocol.IssueTokenRequest) (*protocol.IssueTokenResponse, int32) {
	if req == nil || strings.TrimSpace(req.Nickname) == "" || strings.TrimSpace(req.Password) == "" {
		return nil, code.LoginFail
	}
	if c := rejectIfMaintenance(); c != code.OK {
		return nil, c
	}
	if blocked, err := persistence.IsLoginBlocked(req.ClientIp, req.Nickname); err == nil && blocked {
		return nil, code.LoginRateLimited
	}

	deviceID := strings.TrimSpace(req.DeviceId)
	if deviceID == "" {
		return nil, code.DeviceIDRequired
	}

	uid, err := persistence.LoginOrCreateAccount(req.Nickname, req.Password)
	if err != nil || uid < 1 {
		clog.Warnf("issueToken account fail nickname=%s uid=%d err=%v", req.Nickname, uid, err)
		_ = persistence.RecordLoginFailure(req.ClientIp, req.Nickname)
		if errors.Is(err, persistence.ErrInvalidPassword) {
			return nil, code.InvalidPassword
		}
		return nil, code.LoginFail
	}
	if c := rejectIfBanned(uid); c != code.OK {
		return nil, c
	}
	_ = persistence.ClearLoginFailure(req.ClientIp, req.Nickname)
	accessToken, accessExpireAt, refreshToken, refreshExpireAt, err := persistence.IssueTokenPair(uid, deviceID)
	if err != nil {
		clog.Warnf("issueToken pair fail uid=%d err=%v", uid, err)
		return nil, code.LoginFail
	}
	return &protocol.IssueTokenResponse{
		Uid:             uid,
		AccessToken:     accessToken,
		AccessExpireAt:  accessExpireAt,
		RefreshToken:    refreshToken,
		RefreshExpireAt: refreshExpireAt,
	}, code.OK
}

func (p *ActorSessionWorker) authToken(req *protocol.TokenLoginRequest) (*protocol.TokenLoginResponse, int32) {
	if req == nil {
		return nil, code.LoginFail
	}
	if c := rejectIfMaintenance(); c != code.OK {
		return nil, c
	}
	accessToken := strings.TrimSpace(req.AccessToken)
	if accessToken == "" {
		accessToken = strings.TrimSpace(req.Token)
	}
	if accessToken == "" {
		return nil, code.LoginFail
	}
	if req.ServerId < 1 {
		return nil, code.InvalidServer
	}
	deviceID := strings.TrimSpace(req.DeviceId)
	if deviceID == "" {
		return nil, code.DeviceIDRequired
	}
	uid, _, err := persistence.VerifyAccessToken(accessToken, deviceID)
	if err != nil || uid < 1 {
		if errors.Is(err, persistence.ErrDeviceMismatch) {
			return nil, code.DeviceMismatch
		}
		if errors.Is(err, persistence.ErrAccessTokenInvalid) {
			return nil, code.AccessTokenInvalid
		}
		return nil, code.LoginFail
	}
	if c := rejectIfBanned(uid); c != code.OK {
		return nil, c
	}
	return &protocol.TokenLoginResponse{Uid: uid}, code.OK
}

func (p *ActorSessionWorker) refreshToken(req *protocol.RefreshTokenRequest) (*protocol.RefreshTokenResponse, int32) {
	if req == nil || strings.TrimSpace(req.RefreshToken) == "" {
		return nil, code.LoginFail
	}
	if c := rejectIfMaintenance(); c != code.OK {
		return nil, c
	}
	if uid, err := persistence.PeekRefreshTokenUID(req.RefreshToken); err == nil {
		if c := rejectIfBanned(uid); c != code.OK {
			return nil, c
		}
	}
	accessToken, accessExpireAt, refreshToken, refreshExpireAt, _, err := persistence.RotateTokenPairByRefreshToken(req.RefreshToken)
	if err != nil {
		if errors.Is(err, persistence.ErrRefreshTokenReplay) {
			return nil, code.RefreshTokenReplay
		}
		if errors.Is(err, persistence.ErrRefreshTokenInvalid) {
			return nil, code.RefreshTokenInvalid
		}
		return nil, code.LoginFail
	}
	return &protocol.RefreshTokenResponse{
		AccessToken:     accessToken,
		AccessExpireAt:  accessExpireAt,
		RefreshToken:    refreshToken,
		RefreshExpireAt: refreshExpireAt,
	}, code.OK
}

func (p *ActorSessionWorker) logout(req *protocol.LogoutRequest) (*protocol.LogoutResponse, int32) {
	if req == nil {
		return nil, code.LoginFail
	}
	accessToken := strings.TrimSpace(req.AccessToken)
	if accessToken == "" {
		accessToken = strings.TrimSpace(req.Token)
	}
	refreshToken := strings.TrimSpace(req.RefreshToken)
	if err := persistence.RevokeTokens(accessToken, refreshToken); err != nil {
		return nil, code.LoginFail
	}
	return &protocol.LogoutResponse{Ok: true}, code.OK
}

func rejectIfBanned(uid int64) int32 {
	banned, err := persistence.IsAccountBanned(uid)
	if err != nil {
		return code.LoginFail
	}
	if banned {
		return code.AccountBanned
	}
	return code.OK
}

func rejectIfMaintenance() int32 {
	on, err := persistence.IsMaintenance(context.Background())
	if err != nil {
		return code.OK
	}
	if on {
		return code.ServerMaintenance
	}
	return code.OK
}

// setGameTime 热更新本进程游戏时间偏置（Redis 已由 GM 写入）。
func (p *ActorSession) setGameTime(req *protocol.GmTimeSetRequest) (*protocol.GmTimeGetResponse, int32) {
	if req == nil || req.BiasSeconds < 0 {
		return nil, code.GmBadRequest
	}
	gtime.SetBiasSeconds(req.BiasSeconds)
	return &protocol.GmTimeGetResponse{
		BiasSeconds: gtime.BiasSeconds(),
		UnixNow:     gtime.UnixNow(),
		RealUnixNow: gtime.RealNow().Unix(),
	}, code.OK
}
