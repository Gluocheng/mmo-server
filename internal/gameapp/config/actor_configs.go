package config

import (
	cfacade "github.com/cherry-game/cherry/facade"
	clog "github.com/cherry-game/cherry/logger"
	"github.com/cherry-game/cherry/net/parser/pomelo"
)

// ActorConfig 配置根 Actor（AliasID: config），按 uid 创建子 Actor。
type ActorConfig struct {
	pomelo.ActorBase
}

func (p *ActorConfig) AliasID() string {
	return "config"
}

func (p *ActorConfig) OnFindChild(msg *cfacade.Message) (cfacade.IActor, bool) {
	childID := msg.TargetPath().ChildID
	childActor, err := p.Child().Create(childID, &actorConfig{})
	if err != nil {
		clog.Warnf("create config child fail: %v", err)
		return nil, false
	}
	return childActor, true
}
