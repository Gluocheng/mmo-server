package reward

import (
	"context"

	cfacade "github.com/cherry-game/cherry/facade"
	clog "github.com/cherry-game/cherry/logger"
	"github.com/cherry-game/cherry/net/parser/pomelo"
	gcruntime "github.com/example/mmo-server/gameconfig/pkg/runtime"
	"github.com/example/mmo-server/internal/gameapp/world"
	"github.com/example/mmo-server/internal/gtime"
	"github.com/example/mmo-server/internal/persistence"
	"github.com/example/mmo-server/internal/protocol"
)

// Hurt 是一名玩家对死掉的这只怪造成的实际伤害。
type Hurt struct {
	UID      int64
	PlayerID int64
	Damage   int32
	At       int64
}

// Kill 是一只怪物死亡时抄下的伤害榜。
type Kill struct {
	MonsterUID int64
	TemplateID int32
	SceneID    int32
	Line       int32
	LastHit    int64
	Hits       []Hurt
}

// Apply 按队伍名单和是否还在本线发奖，并推 onKillSettle。推送失败不回滚已经进包或待领取的记录。
func Apply(sender cfacade.IActor, kill Kill) {
	if kill.MonsterUID < 1 || len(kill.Hits) == 0 {
		return
	}
	now := gtime.Now().UnixMilli()
	fighters := make([]Fighter, 0, len(kill.Hits))
	for _, h := range kill.Hits {
		pose, on := world.PoseOf(h.UID)
		onLine := on && pose.SceneID == kill.SceneID && pose.Line == kill.Line
		partyID := int64(0)
		st, c := persistence.PartyStateOf(context.Background(), h.UID, now)
		if c == 0 {
			partyID = st.PartyID
		}
		fighters = append(fighters, Fighter{
			UID: h.UID, PlayerID: h.PlayerID, Damage: h.Damage, At: h.At,
			OnLine: onLine, PartyID: partyID,
		})
	}
	out := Classify(kill.LastHit, fighters)
	for _, f := range fighters {
		if !f.OnLine {
			continue
		}
		items := grantKinds(sender, f, kill.TemplateID, out)
		pushSettle(sender, f.UID, kill, out, items)
	}
}

// grantKinds 按参与奖、名次和最后一击查奖励表并发放。
// 配表没有这一行就跳过。进了背包的种类记下来，稍后推 onBagChange；放不下的整份进待领取。
func grantKinds(sender cfacade.IActor, f Fighter, monsterID int32, out Outcome) []*protocol.KillRewardItem {
	var kinds []string
	if out.Solo[f.UID] {
		kinds = append(kinds, "solo")
	}
	if out.Party[f.UID] {
		kinds = append(kinds, "party")
	}
	switch out.Rank[f.UID] {
	case 1:
		kinds = append(kinds, "rank1")
	case 2:
		kinds = append(kinds, "rank2")
	case 3:
		kinds = append(kinds, "rank3")
	}
	if out.Last == f.UID {
		kinds = append(kinds, "last")
	}
	var items []*protocol.KillRewardItem
	bags := map[int32]struct{}{}
	for _, kind := range kinds {
		def, ok := gcruntime.KillReward(monsterID, kind)
		if !ok {
			continue
		}
		if f.PlayerID < 1 {
			continue
		}
		got, err := persistence.DeliverKillItem(context.Background(), f.PlayerID, def.ItemID, def.Count, monsterID)
		if err != nil {
			clog.Warnf("kill reward uid=%d player=%d kind=%s err=%v", f.UID, f.PlayerID, kind, err)
			continue
		}
		items = append(items, &protocol.KillRewardItem{
			ItemId: def.ItemID, Count: def.Count, Pending: !got.Placed,
		})
		if got.Placed {
			bags[got.BagType] = struct{}{}
		}
	}
	pushBags(sender, f.UID, f.PlayerID, bags)
	return items
}

// pushSettle 把这次击杀的归类、名次和自己拿到的道具推给该玩家。没有网关路径时不推。
func pushSettle(sender cfacade.IActor, uid int64, kill Kill, out Outcome, items []*protocol.KillRewardItem) {
	if sender == nil {
		return
	}
	path, ok := world.AgentPath(uid)
	if !ok {
		return
	}
	pomelo.PushWithUID(sender, path, uid, "onKillSettle", &protocol.KillSettle{
		MonsterUid: kill.MonsterUID,
		MonsterId:  kill.TemplateID,
		Class:      out.Class,
		Rank:       out.Rank[uid],
		LastHitUid: kill.LastHit,
		Items:      items,
	})
}

// pushBags 把已经写入背包的种类对应背包再推一次 onBagChange。待领取不走这里。
func pushBags(sender cfacade.IActor, uid, playerID int64, bags map[int32]struct{}) {
	if sender == nil || playerID < 1 || len(bags) == 0 {
		return
	}
	path, ok := world.AgentPath(uid)
	if !ok {
		return
	}
	for bagType := range bags {
		bag, err := persistence.GetBagByPlayerID(playerID, bagType)
		if err != nil || bag == nil {
			continue
		}
		pomelo.PushWithUID(sender, path, uid, "onBagChange", bag)
	}
}
