package combat

import (
	"time"

	cfacade "github.com/cherry-game/cherry/facade"
	clog "github.com/cherry-game/cherry/logger"
	"github.com/cherry-game/cherry/net/parser/pomelo"
	gcruntime "github.com/example/mmo-server/gameconfig/pkg/runtime"
	"github.com/example/mmo-server/internal/gameapp/reward"
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
	SyncSpawns(p)
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

// broadcast 同一拍先推怪物移动和新视野，再推战斗帧，最后结算击杀奖励。
func (p *ActorCombats) broadcast() {
	frames := Tick()
	PublishMonsterMoves(p, TakeMonsterSteps())
	for _, f := range frames {
		if f.AgentPath == "" {
			continue
		}
		pomelo.PushWithUID(p, f.AgentPath, f.ViewerUID, "onCombatFrame", toProto(f))
	}
	for _, kill := range TakeKills() {
		hits := make([]reward.Hurt, 0, len(kill.Hits))
		for _, h := range kill.Hits {
			hits = append(hits, reward.Hurt{UID: h.UID, PlayerID: h.PlayerID, Damage: h.Damage, At: h.At})
		}
		reward.Apply(p, reward.Kill{
			MonsterUID: kill.MonsterUID, TemplateID: kill.TemplateID,
			SceneID: kill.SceneID, Line: kill.Line, LastHit: kill.LastHit, Hits: hits,
		})
	}
}

// toProto 把这一拍的命中、Buff 变化、属性变化和截断时的自己转成协议包。
func toProto(f Frame) *protocol.CombatFrame {
	msg := &protocol.CombatFrame{
		Truncated: f.Truncated,
		Tick:      f.Tick,
		Hits:      make([]*protocol.CombatHit, 0, len(f.Hits)),
		Buffs:     make([]*protocol.CombatBuffChange, 0, len(f.Buffs)),
		Stats:     make([]*protocol.CombatStat, 0, len(f.Stats)),
	}
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
	for _, b := range f.Buffs {
		msg.Buffs = append(msg.Buffs, &protocol.CombatBuffChange{
			Uid: b.UID, Id: b.ID, Stacks: b.Stacks, ExpireAt: b.ExpireAt,
		})
	}
	for _, s := range f.Stats {
		msg.Stats = append(msg.Stats, &protocol.CombatStat{
			Uid: s.UID, Hp: s.HP, MaxHp: s.MaxHP, Attack: s.Attack, Defense: s.Defense,
		})
	}
	if f.Self != nil {
		self := &protocol.CombatSelf{Hp: f.Self.HP, MaxHp: f.Self.MaxHP, Attack: f.Self.Attack, Defense: f.Self.Defense}
		for _, b := range f.Self.Buffs {
			self.Buffs = append(self.Buffs, &protocol.CombatBuff{Id: b.ID, Stacks: b.Stacks, ExpireAt: b.ExpireAt})
		}
		msg.Self = self
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
