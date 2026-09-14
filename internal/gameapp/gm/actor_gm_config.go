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

// actorGMConfig GM 配置域：reload。
type actorGMConfig struct {
	pomelo.ActorBase
}

func (p *actorGMConfig) OnInit() {
	p.Remote().Register("reload", p.reloadCluster)
}

// reloadCluster 热更配置表（跨节点 Cluster / NATS）。
func (p *actorGMConfig) reloadCluster(req *protocol.GmReloadRequest) (*protocol.GmReloadResponse, int32) {
	operator := ""
	tableName := ""
	if req != nil {
		operator = req.Operator
		tableName = req.TableName
	}
	rsp, c := p.doReload(tableName)
	persistence.WriteGMOpLog(operator, persistence.GMActionReload, 0, 0, tableName, c)
	return rsp, c
}

func (p *actorGMConfig) doReload(tableName string) (*protocol.GmReloadResponse, int32) {
	if !allowGMConfigReload() {
		return nil, code.ConfigReloadDenied
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
	return &protocol.GmReloadResponse{
		Version: gcruntime.Version(),
		Tables:  int64(gcruntime.TableCount()),
	}, code.OK
}

func allowGMConfigReload() bool {
	return cprofile.GetConfig("gameconfig").GetBool("allow_reload", false)
}
