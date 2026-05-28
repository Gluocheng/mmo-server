package bag

import (
	cfacade "github.com/cherry-game/cherry/facade"
	clog "github.com/cherry-game/cherry/logger"
	"github.com/cherry-game/cherry/net/parser/pomelo"
)

// ActorBags 背包根 Actor，按 uid 动态创建子 Actor
type ActorBags struct {
	pomelo.ActorBase
}

func (p *ActorBags) AliasID() string {
	return "bag"
}

func (p *ActorBags) OnFindChild(msg *cfacade.Message) (cfacade.IActor, bool) {
	childID := msg.TargetPath().ChildID
	childActor, err := p.Child().Create(childID, &actorBag{})
	if err != nil {
		clog.Warnf("create bag child fail: %v", err)
		return nil, false
	}
	return childActor, true
}
