package combat

import (
	"time"

	cfacade "github.com/cherry-game/cherry/facade"
	clog "github.com/cherry-game/cherry/logger"
	"github.com/cherry-game/cherry/net/parser/pomelo"
	gcruntime "github.com/example/mmo-server/gameconfig/pkg/runtime"
	"github.com/example/mmo-server/internal/protocol"
)

// ActorCombats 战斗根 Actor：按配表心跳结算，并按 uid 创建施法子 Actor。
type ActorCombats struct {
	pomelo.ActorBase
}

// AliasID 路由前缀 game.combat。
func (p *ActorCombats) AliasID() string {
	return "combat"
}

// OnInit 启动心跳。间隔读配表，未配置时 100ms 空转。
func (p *ActorCombats) OnInit() {
	p.schedule()
}

func (p *ActorCombats) schedule() {
	ms := int32(100)
	if c, ok := gcruntime.CombatConstRow(); ok && c.TickMs >= 50 {
		ms = c.TickMs
	}
	p.Timer().AddOnce(time.Duration(ms)*time.Millisecond, func() {
		p.broadcast()
		p.schedule()
	})
}

func (p *ActorCombats) broadcast() {
	for _, f := range Tick() {
		if f.AgentPath == "" || len(f.Hits) == 0 {
			continue
		}
		pomelo.PushWithUID(p, f.AgentPath, f.ViewerUID, "onCombatFrame", toProto(f))
	}
}

func toProto(f Frame) *protocol.CombatFrame {
	msg := &protocol.CombatFrame{Truncated: f.Truncated, Hits: make([]*protocol.CombatHit, 0, len(f.Hits))}
	for _, h := range f.Hits {
		msg.Hits = append(msg.Hits, &protocol.CombatHit{
			SourceUid: h.SourceUID,
			TargetUid: h.TargetUID,
			SkillId:   h.SkillID,
			BuffId:    h.BuffID,
			Amount:    h.Amount,
			Heal:      h.Heal,
			TargetHp:  h.TargetHP,
			Dead:      h.Dead,
		})
	}
	return msg
}

// OnFindChild 按 uid 创建施法 Actor。
func (p *ActorCombats) OnFindChild(msg *cfacade.Message) (cfacade.IActor, bool) {
	childID := msg.TargetPath().ChildID
	childActor, err := p.Child().Create(childID, &actorCombat{})
	if err != nil {
		clog.Warnf("create combat child fail: %v", err)
		return nil, false
	}
	return childActor, true
}
