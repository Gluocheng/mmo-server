package combat

import "math"

// statKind 是本期参与公式的属性，取值与 cfg_stat.name 相同。
// 后面要结算新属性时在这里加一项，并在 refreshAttrs 里补分支。
type statKind string

const (
	statHP      statKind = "hp"
	statAttack  statKind = "attack"
	statDefense statKind = "defense"
)

// buffEffect 与 cfg_buff.effect 相同。
type buffEffect string

const (
	effectDot  buffEffect = "dot"
	effectHot  buffEffect = "hot"
	effectStun buffEffect = "stun"
	effectAttr buffEffect = "attr"
)

// attrMode 与 cfg_buff.mode 相同。flat 是固定值，percent 是百分比。
type attrMode string

const (
	modeFlat    attrMode = "flat"
	modePercent attrMode = "percent"
)

// refreshAttrs 用还没到期的属性 Buff 重算生命上限、攻击和防御。
// 生命上限升高时当前生命加上差额；降低时把当前生命压到新上限。死亡单位不在这里改。
func refreshAttrs(u *unit, now int64) {
	if u == nil || u.dead {
		return
	}
	var hpFlat, atkFlat, defFlat int32
	var hpPct, atkPct, defPct int32
	for _, b := range u.buffs {
		if b == nil || b.effect != effectAttr || now >= b.expireAt {
			continue
		}
		add := b.value
		if b.stacks > 1 {
			add *= b.stacks
		}
		switch b.stat {
		case statHP:
			if b.mode == modePercent {
				hpPct += add
			} else {
				hpFlat += add
			}
		case statAttack:
			if b.mode == modePercent {
				atkPct += add
			} else {
				atkFlat += add
			}
		case statDefense:
			if b.mode == modePercent {
				defPct += add
			} else {
				defFlat += add
			}
		}
	}
	u.attack = scaleStat(u.baseAttack, atkFlat, atkPct, 0)
	u.defense = scaleStat(u.baseDefense, defFlat, defPct, 0)
	setMaxHP(u, scaleStat(u.baseMaxHP, hpFlat, hpPct, 1))
}

// scaleStat 计算最终属性：(基础 + 固定加成) × (100 + 百分比) / 100，向下取整。
// 结果低于 min 时返回 min。攻击和防御的下限是 0，生命上限的下限是 1。
func scaleStat(base, flat, pct, min int32) int32 {
	v := (int64(base) + int64(flat)) * (100 + int64(pct)) / 100
	if v < int64(min) {
		return min
	}
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(v)
}

func setMaxHP(u *unit, next int32) {
	prev := u.maxHP
	u.maxHP = next
	if u.dead {
		return
	}
	if next > prev {
		u.hp += next - prev
	}
	if u.hp > u.maxHP {
		u.hp = u.maxHP
	}
}

// strikeDamage 用攻击方最终攻击和技能系数算出手值，再减去防守方最终防御。
// 出手值为 0 时造成 0。大于 0 时至少造成 1。
func strikeDamage(attacker *unit, skillDamage, factor int32, defender *unit, now int64) int32 {
	refreshAttrs(attacker, now)
	refreshAttrs(defender, now)
	raw := int64(skillDamage)
	if raw < 0 {
		raw = 0
	}
	if attacker != nil {
		raw += int64(attacker.attack) * int64(factor) / 100
	}
	if raw <= 0 {
		return 0
	}
	if defender != nil {
		raw -= int64(defender.defense)
	}
	if raw < 1 {
		return 1
	}
	if raw > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(raw)
}

func resetBaseStats(u *unit) {
	if u == nil {
		return
	}
	u.maxHP = u.baseMaxHP
	u.attack = u.baseAttack
	u.defense = u.baseDefense
}
