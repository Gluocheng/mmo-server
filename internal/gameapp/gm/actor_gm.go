package gm

import (
	cfacade "github.com/cherry-game/cherry/facade"
	clog "github.com/cherry-game/cherry/logger"
	"github.com/cherry-game/cherry/net/parser/pomelo"
)

// ActorGM GM 根 Actor（AliasID: gm），按 domain 创建子 Actor。
type ActorGM struct {
	pomelo.ActorBase
}

func (p *ActorGM) AliasID() string {
	return "gm"
}

// OnFindChild 按 domain 创建子 Actor；路由 game.gm.<domain>.<method>。
func (p *ActorGM) OnFindChild(msg *cfacade.Message) (cfacade.IActor, bool) {
	childID := msg.TargetPath().ChildID
	var childActor cfacade.IActor
	var err error
	switch childID {
	case "config":
		childActor, err = p.Child().Create(childID, &actorGMConfig{})
	default:
		clog.Warnf("gm: unknown domain %s", childID)
		return nil, false
	}
	if err != nil {
		clog.Warnf("create gm child fail: %v", err)
		return nil, false
	}
	return childActor, true
}
