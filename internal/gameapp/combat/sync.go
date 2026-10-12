package combat

import (
	"sync"

	cfacade "github.com/cherry-game/cherry/facade"
	"github.com/cherry-game/cherry/net/parser/pomelo"
	"github.com/example/mmo-server/internal/gameapp/world"
	"github.com/example/mmo-server/internal/protocol"
)

// seenBefore 记下怪物移动前的视野，移动后再和现在对比。
type seenBefore struct {
	UID    int64
	Before []int64
}

var (
	seenMu      sync.Mutex
	pendingSeen []seenBefore
)

// storeSeen 暂存这一拍怪物移动前能看见的单位，推送时再取出。
func storeSeen(v []seenBefore) {
	seenMu.Lock()
	pendingSeen = v
	seenMu.Unlock()
}

// takeSeen 取出并清空上一拍记下的移动前视野。
func takeSeen() []seenBefore {
	seenMu.Lock()
	defer seenMu.Unlock()
	out := pendingSeen
	pendingSeen = nil
	return out
}

// Snap 是推给客户端的一份战斗外观。
type Snap struct {
	UID     int64
	HP      int32
	MaxHP   int32
	Attack  int32
	Defense int32
	Dead    bool
	Buffs   []BuffView
}

// PublishEnter 在进场或切图成功后，给自己推自己和附近单位，并给附近玩家推自己。
func PublishEnter(sender cfacade.IActor, uid int64) {
	self, ok := readSnap(uid)
	if !ok {
		return
	}
	pushSnap(sender, uid, self)
	for _, id := range world.NearbyUIDs(uid) {
		if other, ok := readSnap(id); ok {
			pushSnap(sender, uid, other)
		}
		if isPlayer(id) {
			pushSnap(sender, id, self)
		}
	}
}

// PublishLeft 切走之后，自己丢掉旧视野，附近玩家丢掉自己。
func PublishLeft(sender cfacade.IActor, uid int64, before []int64) {
	for _, id := range before {
		if id == uid {
			continue
		}
		pushGone(sender, uid, id)
		if isPlayer(id) {
			pushGone(sender, id, uid)
		}
	}
}

// PublishMove 对比移动前的视野。新进来的发快照，走出去的发消失。
func PublishMove(sender cfacade.IActor, uid int64, before []int64) {
	after := world.NearbyUIDs(uid)
	gain, loss := diffIDs(before, after)
	moverPlayer := isPlayer(uid)
	var self Snap
	var hasSelf bool
	if moverPlayer {
		self, hasSelf = readSnap(uid)
	}
	for _, id := range gain {
		if moverPlayer {
			if other, ok := readSnap(id); ok {
				pushSnap(sender, uid, other)
			}
		}
		if isPlayer(id) && hasSelf {
			pushSnap(sender, id, self)
		}
	}
	for _, id := range loss {
		if id == uid {
			continue
		}
		if moverPlayer {
			pushGone(sender, uid, id)
		}
		if isPlayer(id) {
			pushGone(sender, id, uid)
		}
	}
}

// PublishMonsterMoves 先发怪物位移，再按移动前的视野补快照和消失。
func PublishMonsterMoves(sender cfacade.IActor, steps []MonsterStep) {
	seen := takeSeen()
	beforeOf := map[int64][]int64{}
	for _, s := range seen {
		beforeOf[s.UID] = s.Before
	}
	for _, step := range steps {
		world.BroadcastMove(sender, step.UID, &protocol.MoveBroadcast{
			Uid: step.UID, X: step.X, Y: step.Y, Z: step.Z,
		})
	}
	for _, step := range steps {
		PublishMove(sender, step.UID, beforeOf[step.UID])
	}
}

// diffIDs 对比移动前后的视野。gain 是新进入的单位，loss 是离开的单位。
func diffIDs(before, after []int64) (gain, loss []int64) {
	old := map[int64]struct{}{}
	for _, id := range before {
		old[id] = struct{}{}
	}
	now := map[int64]struct{}{}
	for _, id := range after {
		now[id] = struct{}{}
		if _, ok := old[id]; !ok {
			gain = append(gain, id)
		}
	}
	for _, id := range before {
		if _, ok := now[id]; !ok {
			loss = append(loss, id)
		}
	}
	return gain, loss
}

// isPlayer 判断这个 uid 是玩家。怪物不收快照和消失。
func isPlayer(uid int64) bool {
	kind, ok := world.ActorTypeOf(uid)
	return ok && kind == world.ActorPlayer
}

// readSnap 抄下单位当前的生命、三项属性和 Buff。单位不存在时返回 false。
func readSnap(uid int64) (Snap, bool) {
	mu.Lock()
	defer mu.Unlock()
	u := units[uid]
	if u == nil {
		return Snap{}, false
	}
	return Snap{
		UID: uid, HP: u.hp, MaxHP: u.maxHP, Attack: u.attack, Defense: u.defense,
		Dead: u.dead, Buffs: buffViews(u),
	}, true
}

// pushSnap 沿观众进场时记下的网关路径推 onCombatSnapshot。没有连接则跳过。
func pushSnap(sender cfacade.IActor, viewer int64, snap Snap) {
	path, ok := world.AgentPath(viewer)
	if !ok || path == "" || sender == nil {
		return
	}
	buffs := make([]*protocol.CombatBuff, 0, len(snap.Buffs))
	for _, b := range snap.Buffs {
		buffs = append(buffs, &protocol.CombatBuff{Id: b.ID, Stacks: b.Stacks, ExpireAt: b.ExpireAt})
	}
	pomelo.PushWithUID(sender, path, viewer, "onCombatSnapshot", &protocol.CombatSnapshot{
		Uid: snap.UID, Hp: snap.HP, MaxHp: snap.MaxHP, Attack: snap.Attack, Defense: snap.Defense,
		Dead: snap.Dead, Buffs: buffs,
	})
}

// pushGone 推 onCombatGone。自己不会收到自己的消失。
func pushGone(sender cfacade.IActor, viewer, uid int64) {
	if viewer == uid {
		return
	}
	path, ok := world.AgentPath(viewer)
	if !ok || path == "" || sender == nil {
		return
	}
	pomelo.PushWithUID(sender, path, viewer, "onCombatGone", &protocol.CombatGone{Uid: uid})
}
