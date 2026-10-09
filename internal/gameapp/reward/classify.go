package reward

// Fighter 是一只怪物这一条命里造成过伤害的玩家。
// OnLine 表示结算时还在死怪的那张图、那条线。PartyID 为 0 表示不在队伍里。
type Fighter struct {
	UID      int64
	PlayerID int64
	Damage   int32
	At       int64
	OnLine   bool
	PartyID  int64
}

// Outcome 是一次击杀的归类和谁拿到哪一种奖。
// Class 为 solo、party 或 open。Rank 只含仍在本线的人，名次从 1 起。
// Solo 或 Party 二选一，表示参与奖。Last 为应拿最后一击的 uid，没有则为 0。
type Outcome struct {
	Class string
	Rank  map[int64]int32
	Solo  map[int64]bool
	Party map[int64]bool
	Last  int64
}

// Classify 按伤害归属决定参与奖、名次和最后一击。
// 只有一名玩家造成过伤害时是单独击杀，他在不在队伍里都一样。
// 同一支队伍至少两人造成过伤害且结算时都还在本线，才是组队击杀。
// 两支队伍都合格时，仍在本线的出力人数多的获胜；人数相同比这些人的伤害合计；再相同取较小的队伍 id。
func Classify(lastHit int64, fighters []Fighter) Outcome {
	out := Outcome{
		Class: "open",
		Rank:  map[int64]int32{},
		Solo:  map[int64]bool{},
		Party: map[int64]bool{},
	}
	var dealt []Fighter
	for _, f := range fighters {
		if f.Damage > 0 && f.UID > 0 {
			dealt = append(dealt, f)
		}
	}
	if len(dealt) == 0 {
		return out
	}
	if len(dealt) == 1 {
		out.Class = "solo"
		if dealt[0].OnLine {
			out.Solo[dealt[0].UID] = true
		}
	} else if win, ok := winningParty(dealt); ok {
		out.Class = "party"
		for _, f := range dealt {
			if f.OnLine && f.PartyID == win {
				out.Party[f.UID] = true
			}
		}
	}
	var ranked []Fighter
	for _, f := range dealt {
		if f.OnLine {
			ranked = append(ranked, f)
		}
	}
	sortFighters(ranked)
	for i := 0; i < len(ranked) && i < 3; i++ {
		out.Rank[ranked[i].UID] = int32(i + 1)
	}
	for _, f := range dealt {
		if f.UID == lastHit && f.OnLine {
			out.Last = lastHit
			break
		}
	}
	return out
}

// partyScore 是一支合格候选队伍：仍在本线且造成过伤害的人数，以及这些人的伤害合计。
type partyScore struct {
	id     int64
	count  int
	damage int32
}

// winningParty 选出拿到组队参与奖的队伍。不足 2 名在线出力成员的队伍不合格。
// 没有合格队伍时 ok 为 false，调用方按公开争夺处理，不发参与奖。
func winningParty(dealt []Fighter) (int64, bool) {
	scores := map[int64]*partyScore{}
	for _, f := range dealt {
		if !f.OnLine || f.PartyID < 1 {
			continue
		}
		sc := scores[f.PartyID]
		if sc == nil {
			sc = &partyScore{id: f.PartyID}
			scores[f.PartyID] = sc
		}
		sc.count++
		sc.damage += f.Damage
	}
	var best *partyScore
	for _, sc := range scores {
		if sc.count < 2 {
			continue
		}
		if best == nil || betterParty(sc, best) {
			best = sc
		}
	}
	if best == nil {
		return 0, false
	}
	return best.id, true
}

// betterParty 判断 a 是否压过 b：先比在线出力人数，再比伤害合计，最后取较小的队伍 id。
func betterParty(a, b *partyScore) bool {
	if a.count != b.count {
		return a.count > b.count
	}
	if a.damage != b.damage {
		return a.damage > b.damage
	}
	return a.id < b.id
}

// sortFighters 按名次规则排序：伤害高的在前；伤害相同则先达到该伤害的在前；仍相同则 uid 更小的在前。
func sortFighters(in []Fighter) {
	for i := 1; i < len(in); i++ {
		j := i
		for j > 0 && fighterBefore(in[j], in[j-1]) {
			in[j], in[j-1] = in[j-1], in[j]
			j--
		}
	}
}

// fighterBefore 表示按名次规则 a 应排在 b 前面。
func fighterBefore(a, b Fighter) bool {
	if a.Damage != b.Damage {
		return a.Damage > b.Damage
	}
	if a.At != b.At {
		return a.At < b.At
	}
	return a.UID < b.UID
}
