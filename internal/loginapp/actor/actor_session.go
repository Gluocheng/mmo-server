package actor

import (
	"errors"
	"strings"

	cactor "github.com/cherry-game/cherry/net/actor"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/persistence"
	"github.com/example/mmo-server/internal/protocol"
)

// ActorSession 登录服单例 Actor，路径 login-1.session
type ActorSession struct {
	cactor.Base
}

func (p *ActorSession) AliasID() string {
	return "session"
}

func (p *ActorSession) OnInit() {
	p.Remote().Register("issueToken", p.issueToken)
	p.Remote().Register("authToken", p.authToken)
	p.Remote().Register("refreshToken", p.refreshToken)
	p.Remote().Register("logout", p.logout)
}

func (p *ActorSession) issueToken(req *protocol.IssueTokenRequest) (*protocol.IssueTokenResponse, int32) {
	if req == nil || strings.TrimSpace(req.Nickname) == "" || strings.TrimSpace(req.Password) == "" {
		return nil, code.LoginFail
	}
	if blocked, err := persistence.IsLoginBlocked(req.ClientIp, req.Nickname); err == nil && blocked {
		return nil, code.LoginRateLimited
	}

	deviceID := strings.TrimSpace(req.DeviceId)
	if deviceID == "" {
		return nil, code.DeviceIDRequired
	}

	uid, err := persistence.FindOrCreateAccount(req.Nickname, req.Password)
	if err != nil || uid < 1 {
		_ = persistence.RecordLoginFailure(req.ClientIp, req.Nickname)
		if errors.Is(err, persistence.ErrInvalidPassword) {
			return nil, code.InvalidPassword
		}
		return nil, code.LoginFail
	}
	_ = persistence.ClearLoginFailure(req.ClientIp, req.Nickname)
	accessToken, accessExpireAt, refreshToken, refreshExpireAt, err := persistence.IssueTokenPair(uid, deviceID)
	if err != nil {
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

func (p *ActorSession) authToken(req *protocol.TokenLoginRequest) (*protocol.TokenLoginResponse, int32) {
	if req == nil {
		return nil, code.LoginFail
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
	return &protocol.TokenLoginResponse{Uid: uid}, code.OK
}

func (p *ActorSession) refreshToken(req *protocol.RefreshTokenRequest) (*protocol.RefreshTokenResponse, int32) {
	if req == nil || strings.TrimSpace(req.RefreshToken) == "" {
		return nil, code.LoginFail
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

func (p *ActorSession) logout(req *protocol.LogoutRequest) (*protocol.LogoutResponse, int32) {
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
