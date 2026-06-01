package config

import (
	"context"

	clog "github.com/cherry-game/cherry/logger"
	"github.com/cherry-game/cherry/net/parser/pomelo"
	cproto "github.com/cherry-game/cherry/net/proto"
	cprofile "github.com/cherry-game/cherry/profile"
	gcruntime "github.com/example/mmo-server/gameconfig/pkg/runtime"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/persistence"
	"github.com/example/mmo-server/internal/protocol"
)

// actorConfig 配置管理 Actor：手动 reload 等。
type actorConfig struct {
	pomelo.ActorBase
}

// OnInit 注册 game.config.reload。
func (p *actorConfig) OnInit() {
	p.Local().Register("reload", p.reload)
}

func (p *actorConfig) reload(session *cproto.Session, _ *protocol.None) {
	if !allowConfigReload() {
		p.ResponseCode(session, code.ConfigReloadDenied)
		return
	}
	ctx := context.Background()
	db, err := persistence.DB()
	if err != nil {
		clog.Warnf("config reload db: %v", err)
		p.ResponseCode(session, code.ConfigReloadFail)
		return
	}
	if err := gcruntime.Reload(ctx, db); err != nil {
		clog.Warnf("config reload fail: %v", err)
		p.ResponseCode(session, code.ConfigReloadFail)
		return
	}
	// 复用 RefreshTokenResponse 字段承载：accessExpireAt=version，refreshExpireAt=tableCount（待 genproto 专用消息后替换）。
	p.Response(session, &protocol.RefreshTokenResponse{
		AccessExpireAt:  gcruntime.Version(),
		RefreshExpireAt: int64(gcruntime.TableCount()),
	})
}

func allowConfigReload() bool {
	return cprofile.GetConfig("gameconfig").GetBool("allow_reload", false)
}
