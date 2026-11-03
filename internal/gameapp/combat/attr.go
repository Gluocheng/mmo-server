package combat

import (
	"math"
	"math/rand/v2"
)

// statKind 是本期参与公式的属性，取值与 cfg_stat.name 相同。
// 后面要结算新属性时在这里加一项，并在 refreshAttrs 里补分支。
type statKind string

const (
	statHP              statKind = "hp"
	statAttack          statKind = "attack"
	statDefense         statKind = "defense"
	statCritRate        statKind = "crit_rate"
	statCritDamage      statKind = "crit_damage"
	statPenetrate       statKind = "penetrate"
	statDamageBonus     statKind = "damage_bonus"
	statDamageReduction statKind = "damage_reduction"
	statLifesteal       statKind = "lifesteal"
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

// EquipBonus 是当前穿着折成的属性加成。后六项的单位已经是百分点。
type EquipBonus struct {
	HPFlat          int32
	AtkFlat         int32
	DefFlat         int32
	HPPct           int32
	AtkPct          int32
	DefPct          int32
	CritRate        int32
	CritDamage      int32
	Penetrate       int32
	DamageBonus     int32
	DamageReduction int32
	Lifesteal       int32
}

// SetEquipBonus 记下穿着加成并立刻重算。commit 为真时把这次结果记成下一拍的比较基准。
// 换装传 false，让下一拍把变化推出去。进场叠穿着传 true，避免进场后再多发一拍。
func SetEquipBonus(uid int64, b EquipBonus, commit bool) {
	mu.Lock()
	defer mu.Unlock()
	u := units[uid]
	if u == nil || u.monster {
		return
	}
	u.eqHPFlat, u.eqAtkFlat, u.eqDefFlat = b.HPFlat, b.AtkFlat, b.DefFlat
	u.eqHPPct, u.eqAtkPct, u.eqDefPct = b.HPPct, b.AtkPct, b.DefPct
	u.eqCritRate, u.eqCritDamage, u.eqPenetrate = b.CritRate, b.CritDamage, b.Penetrate
	u.eqDamageBonus, u.eqDamageReduction, u.eqLifesteal = b.DamageBonus, b.DamageReduction, b.Lifesteal
	refreshAttrs(u, nowFn())
	if commit {
		commitAppearance(uid)
	}
}

// Stats 返回当前生命、生命上限、攻击和防御。
func Stats(uid int64) (hp, maxHP, attack, defense int32, ok bool) {
	mu.Lock()
	defer mu.Unlock()
	u := units[uid]
	if u == nil {
		return 0, 0, 0, 0, false
	}
	return u.hp, u.maxHP, u.attack, u.defense, true
}

// Rates 返回暴击率、暴击伤害、穿透、伤害加成、受伤减免和吸血的最终百分点。
func Rates(uid int64) (critRate, critDamage, penetrate, damageBonus, damageReduction, lifesteal int32, ok bool) {
	mu.Lock()
	defer mu.Unlock()
	u := units[uid]
	if u == nil {
		return 0, 0, 0, 0, 0, 0, false
	}
	return u.critRate, u.critDamage, u.penetrate, u.damageBonus, u.damageReduction, u.lifesteal, true
}

// refreshAttrs 用还没到期的属性 Buff 和穿着重算生命、攻击、防御和六项百分比。
// 生命上限升高时当前生命加上差额；降低时把当前生命压到新上限。死亡单位不在这里改。
// 六项基础为 0。配表里的 flat 和 percent 都是百分点，加总后再套现有公式。
func refreshAttrs(u *unit, now int64) {
	if u == nil || u.dead {
		return
	}
	hpFlat, atkFlat, defFlat := u.eqHPFlat, u.eqAtkFlat, u.eqDefFlat
	hpPct, atkPct, defPct := u.eqHPPct, u.eqAtkPct, u.eqDefPct
	critRate := u.eqCritRate
	critDamage := u.eqCritDamage
	penetrate := u.eqPenetrate
	damageBonus := u.eqDamageBonus
	damageReduction := u.eqDamageReduction
	lifesteal := u.eqLifesteal
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
		case statCritRate:
			critRate += add
		case statCritDamage:
			critDamage += add
		case statPenetrate:
			penetrate += add
		case statDamageBonus:
			damageBonus += add
		case statDamageReduction:
			damageReduction += add
		case statLifesteal:
			lifesteal += add
		}
	}
	u.attack = scaleStat(u.baseAttack, atkFlat, atkPct, 0)
	u.defense = scaleStat(u.baseDefense, defFlat, defPct, 0)
	setMaxHP(u, scaleStat(u.baseMaxHP, hpFlat, hpPct, 1))
	u.critRate = clampStat(scaleStat(0, critRate, 0, 0), 0, 100)
	u.critDamage = clampStat(scaleStat(0, critDamage, 0, 0), 0, math.MaxInt32)
	u.penetrate = clampStat(scaleStat(0, penetrate, 0, 0), 0, 100)
	u.damageBonus = clampStat(scaleStat(0, damageBonus, 0, 0), 0, math.MaxInt32)
	u.damageReduction = clampStat(scaleStat(0, damageReduction, 0, 0), 0, 75)
	u.lifesteal = clampStat(scaleStat(0, lifesteal, 0, 0), 0, 100)
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

func clampStat(v, min, max int32) int32 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
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

// strikeDamage 算出手伤害，并标明这一下是否暴击。
// 顺序是出手值、暴击、穿透后的防御、伤害加成、受伤减免。出手值为 0 时造成 0。
// 第 1 步大于 0 时，后面的结果至少为 1。
func strikeDamage(attacker *unit, skillDamage, factor int32, defender *unit, now int64) (int32, bool) {
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
		return 0, false
	}
	outgoing := raw
	crit := false
	if attacker != nil && critRoll(attacker.critRate) {
		outgoing = outgoing * (150 + int64(attacker.critDamage)) / 100
		crit = true
	}
	if outgoing > 0 && defender != nil {
		pen := int64(0)
		if attacker != nil {
			pen = int64(attacker.penetrate)
		}
		if pen < 0 {
			pen = 0
		}
		if pen > 100 {
			pen = 100
		}
		effDef := int64(defender.defense) * (100 - pen) / 100
		outgoing -= effDef
		if outgoing < 1 {
			outgoing = 1
		}
	}
	bonus := int64(0)
	if attacker != nil {
		bonus = int64(attacker.damageBonus)
	}
	reduction := int64(0)
	if defender != nil {
		reduction = int64(defender.damageReduction)
	}
	outgoing = outgoing * (100 + bonus) / 100
	outgoing = outgoing * (100 - reduction) / 100
	if outgoing < 1 {
		outgoing = 1
	}
	if outgoing > math.MaxInt32 {
		outgoing = math.MaxInt32
	}
	return int32(outgoing), crit
}

// critRoll 按暴击率掷点。测试可换成固定结果。
var critRoll = defaultCritRoll

func defaultCritRoll(rate int32) bool {
	if rate >= 100 {
		return true
	}
	if rate <= 0 {
		return false
	}
	return int32(rand.IntN(100))+1 <= rate
}

// SetCritRollsForTest 按顺序返回固定暴击结果，用完后再掷都落空。仅测试调用。
func SetCritRollsForTest(hits ...bool) {
	i := 0
	critRoll = func(int32) bool {
		if i >= len(hits) {
			return false
		}
		v := hits[i]
		i++
		return v
	}
}

func resetCritRoll() { critRoll = defaultCritRoll }

func resetBaseStats(u *unit) {
	if u == nil {
		return
	}
	u.maxHP = u.baseMaxHP
	u.attack = u.baseAttack
	u.defense = u.baseDefense
	u.critRate = 0
	u.critDamage = 0
	u.penetrate = 0
	u.damageBonus = 0
	u.damageReduction = 0
	u.lifesteal = 0
}
