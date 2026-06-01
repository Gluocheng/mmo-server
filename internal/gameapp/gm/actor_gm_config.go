package gm

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

// actorGMConfig GM 配置域：game.gm.config.reload。
type actorGMConfig struct {
	pomelo.ActorBase
}

func (p *actorGMConfig) OnInit() {
	// Pomelo 客户端调用（session + req 两参）
	p.Local().Register("reload", p.reload)
	// 跨节点 Cluster 调用（req 单参，通过 NATS 从 GM 独立进程进入）
	p.Remote().Register("reload", p.reloadCluster)
}

// reload 热更配置表（Pomelo 客户端调用）。
func (p *actorGMConfig) reload(session *cproto.Session, req *protocol.RefreshTokenRequest) {
	if !allowGMConfigReload() {
		p.ResponseCode(session, code.ConfigReloadDenied)
		return
	}
	tableName := ""
	if req != nil {
		tableName = req.RefreshToken
	}
	ctx := context.Background()
	db, err := persistence.DB()
	if err != nil {
		clog.Warnf("gm config reload db: %v", err)
		p.ResponseCode(session, code.ConfigReloadFail)
		return
	}
	if tableName == "" {
		if err := gcruntime.Reload(ctx, db); err != nil {
			clog.Warnf("gm config reload all fail: %v", err)
			p.ResponseCode(session, code.ConfigReloadFail)
			return
		}
	} else {
		if err := gcruntime.ReloadTable(ctx, db, tableName); err != nil {
			clog.Warnf("gm config reload table=%s fail: %v", tableName, err)
			p.ResponseCode(session, code.ConfigReloadFail)
			return
		}
	}
	p.Response(session, &protocol.RefreshTokenResponse{
		AccessExpireAt:  gcruntime.Version(),
		RefreshExpireAt: int64(gcruntime.TableCount()),
	})
}

// reloadCluster 热更配置表（跨节点 Cluster / NATS 调用）。
// 返回值符合 Cherry 远程调用的约定：首值为 protobuf 负载，次值为业务码。
func (p *actorGMConfig) reloadCluster(req *protocol.RefreshTokenRequest) (*protocol.RefreshTokenResponse, int32) {
	if !allowGMConfigReload() {
		return nil, code.ConfigReloadDenied
	}
	tableName := ""
	if req != nil {
		tableName = req.RefreshToken
	}
	ctx := context.Background()
	db, err := persistence.DB()
	if err != nil {
		clog.Warnf("gm config reload db: %v", err)
		return nil, code.ConfigReloadFail
	}
	if tableName == "" {
		if err := gcruntime.Reload(ctx, db); err != nil {
			clog.Warnf("gm config reload all fail: %v", err)
			return nil, code.ConfigReloadFail
		}
	} else {
		if err := gcruntime.ReloadTable(ctx, db, tableName); err != nil {
			clog.Warnf("gm config reload table=%s fail: %v", tableName, err)
			return nil, code.ConfigReloadFail
		}
	}
	return &protocol.RefreshTokenResponse{
		AccessExpireAt:  gcruntime.Version(),
		RefreshExpireAt: int64(gcruntime.TableCount()),
	}, code.OK
}

func allowGMConfigReload() bool {
	return cprofile.GetConfig("gameconfig").GetBool("allow_reload", false)
}
