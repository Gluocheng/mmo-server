package bag

import (
	"time"

	cfacade "github.com/cherry-game/cherry/facade"
	clog "github.com/cherry-game/cherry/logger"
	"github.com/cherry-game/cherry/net/parser/pomelo"
)

// flushInterval 定时 flush 周期。
const flushInterval = 5 * time.Second

// ActorBags 背包根 Actor，按 uid 动态创建子 Actor。
type ActorBags struct {
	pomelo.ActorBase
}

func (p *ActorBags) AliasID() string {
	return "bag"
}

// OnInit 启动定时 flush：只遍历本机所有 bag 子 Actor，把带脏标记的包落库。
func (p *ActorBags) OnInit() {
	p.schedule()
}

func (p *ActorBags) schedule() {
	p.Timer().AddOnce(flushInterval, func() {
		p.flushAll()
		p.schedule()
	})
}

// flushAll 遍历全部 bag 子 Actor，flush 脏包。
func (p *ActorBags) flushAll() {
	p.Child().Each(func(i cfacade.IActor) {
		if a, ok := i.(*actorBag); ok {
			a.flushDirty()
		}
	})
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
