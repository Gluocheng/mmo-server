package actor

import (
	cactor "github.com/cherry-game/cherry/net/actor"
	"github.com/cherry-game/cherry/net/parser/pomelo"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/protocol"
)

// ActorOps 网关运营远程：全量踢连接（维护模式）。
type ActorOps struct {
	cactor.Base
}

func (p *ActorOps) AliasID() string {
	return "ops"
}

func (p *ActorOps) OnInit() {
	p.Remote().Register("kickAll", p.kickAll)
}

// kickAll 关闭本节点全部 WebSocket 连接。
func (p *ActorOps) kickAll(_ *protocol.GmKickAllRequest) (*protocol.GmKickAllResponse, int32) {
	var n int32
	pomelo.ForeachAgent(func(agent *pomelo.Agent) {
		agent.Kick(&protocol.CodeOnly{Code: code.ServerMaintenance}, true)
		n++
	})
	return &protocol.GmKickAllResponse{Kicked: n}, code.OK
}
