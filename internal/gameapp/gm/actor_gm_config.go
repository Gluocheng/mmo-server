package gm

import (
	"context"

	clog "github.com/cherry-game/cherry/logger"
	"github.com/cherry-game/cherry/net/parser/pomelo"
	cprofile "github.com/cherry-game/cherry/profile"
	gcruntime "github.com/example/mmo-server/gameconfig/pkg/runtime"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/persistence"
	"github.com/example/mmo-server/internal/protocol"
)

// actorGMConfig GM 配置域：仅 NATS Remote reload（GM HTTP）。
type actorGMConfig struct {
	pomelo.ActorBase
}

func (p *actorGMConfig) OnInit() {
	// 仅接受 GM 进程经 NATS Remote 调用；不向 Pomelo 客户端暴露 Local reload。
	p.Remote().Register("reload", p.reloadCluster)
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
