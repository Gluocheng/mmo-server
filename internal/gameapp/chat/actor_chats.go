package chat

import (
	cfacade "github.com/cherry-game/cherry/facade"
	clog "github.com/cherry-game/cherry/logger"
	"github.com/cherry-game/cherry/net/parser/pomelo"
)

// ActorChats 聊天根 Actor，按 uid 动态创建子 Actor
type ActorChats struct {
	pomelo.ActorBase
}

func (p *ActorChats) AliasID() string {
	return "chat"
}

func (p *ActorChats) OnFindChild(msg *cfacade.Message) (cfacade.IActor, bool) {
	childID := msg.TargetPath().ChildID
	childActor, err := p.Child().Create(childID, &actorChat{})
	if err != nil {
		clog.Warnf("create chat child fail: %v", err)
		return nil, false
	}
	return childActor, true
}

