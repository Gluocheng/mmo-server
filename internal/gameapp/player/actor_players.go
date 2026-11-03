package player

import (
	cfacade "github.com/cherry-game/cherry/facade"
	clog "github.com/cherry-game/cherry/logger"
	"github.com/cherry-game/cherry/net/parser/pomelo"
	"github.com/example/mmo-server/internal/gameapp/broadcast"
)

// ActorPlayers 玩家根 Actor，按 uid 动态创建子 Actor
type ActorPlayers struct {
	pomelo.ActorBase
}

func (p *ActorPlayers) AliasID() string {
	return "player"
}

// OnInit 启动进程内广播。移动和聊天先回包，再由这条循环推给周围的人。
func (p *ActorPlayers) OnInit() {
	broadcast.Bind(p)
}

func (p *ActorPlayers) OnFindChild(msg *cfacade.Message) (cfacade.IActor, bool) {
	childID := msg.TargetPath().ChildID
	childActor, err := p.Child().Create(childID, &actorPlayer{})
	if err != nil {
		clog.Warnf("create player child fail: %v", err)
		return nil, false
	}
	return childActor, true
}
