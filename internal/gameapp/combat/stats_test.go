package combat

import (
	"testing"

	"github.com/example/mmo-server/internal/code"
)

func setPoints(uid int64, critRate, critDamage, penetrate, bonus, reduction, steal int32) {
	mu.Lock()
	defer mu.Unlock()
	u := units[uid]
	u.eqCritRate = critRate
	u.eqCritDamage = critDamage
	u.eqPenetrate = penetrate
	u.eqDamageBonus = bonus
	u.eqDamageReduction = reduction
	u.eqLifesteal = steal
	refreshAttrs(u, nowFn())
}

func damageHit(frames []Frame, src, dst int64) (Hit, bool) {
	for _, f := range frames {
		for _, h := range f.Hits {
			if h.SourceUID == src && h.TargetUID == dst && !h.Heal {
				return h, true
			}
		}
	}
	return Hit{}, false
}

func healHit(frames []Frame, uid int64) (Hit, bool) {
	for _, f := range frames {
		for _, h := range f.Hits {
			if h.Heal && h.SourceUID == uid && h.TargetUID == uid && h.BuffID == 0 {
				return h, true
			}
		}
	}
	return Hit{}, false
}

func TestNoCritMatchesOldDamage(t *testing.T) {
	seedAttr(t)
	join(t, 92001, 0)
	join(t, 92002, 0)
	placeWolf(t, 92003)
	if c := Cast(92001, 1, 92002); c != code.OK {
		t.Fatal(c)
	}
	Tick()
	if hp, _, _, _ := Snapshot(92002); hp != 90 {
		t.Fatalf("player %d", hp)
	}
	SetClockForTest(1000)
	if c := Cast(92001, 1, 92003); c != code.OK {
		t.Fatal(c)
	}
	Tick()
	if hp, _, _, _ := Snapshot(92003); hp != 72 {
		t.Fatalf("wolf %d", hp)
	}
}

func TestCritThenDefense(t *testing.T) {
	seedAttr(t)
	join(t, 92101, 0)
	join(t, 92102, 0)
	placeWolf(t, 92103)
	setPoints(92101, 100, 0, 0, 0, 0, 0)
	if c := Cast(92101, 1, 92102); c != code.OK {
		t.Fatal(c)
	}
	frames := Tick()
	hit, ok := damageHit(frames, 92101, 92102)
	if !ok || !hit.Crit || hit.Amount != 15 || hit.TargetHP != 85 {
		t.Fatalf("hit %+v", hit)
	}
	SetClockForTest(1000)
	if c := Cast(92101, 1, 92103); c != code.OK {
		t.Fatal(c)
	}
	frames = Tick()
	hit, ok = damageHit(frames, 92101, 92103)
	if !ok || !hit.Crit || hit.Amount != 13 || hit.TargetHP != 67 {
		t.Fatalf("wolf %+v", hit)
	}
}

func TestCritRollCanMiss(t *testing.T) {
	seedAttr(t)
	join(t, 92201, 0)
	join(t, 92202, 0)
	setPoints(92201, 50, 0, 0, 0, 0, 0)
	SetCritRollsForTest(false)
	if c := Cast(92201, 1, 92202); c != code.OK {
		t.Fatal(c)
	}
	frames := Tick()
	hit, ok := damageHit(frames, 92201, 92202)
	if !ok || hit.Crit || hit.Amount != 10 {
		t.Fatalf("miss %+v", hit)
	}
}

func TestPenetrateIgnoresDefense(t *testing.T) {
	seedAttr(t)
	join(t, 92301, 0)
	placeWolf(t, 92303)
	setPoints(92301, 0, 0, 100, 0, 0, 0)
	if c := Cast(92301, 1, 92303); c != code.OK {
		t.Fatal(c)
	}
	Tick()
	if hp, _, _, _ := Snapshot(92303); hp != 70 {
		t.Fatalf("wolf %d", hp)
	}
}

func TestBonusThenReduction(t *testing.T) {
	seedAttr(t)
	join(t, 92401, 0)
	join(t, 92402, 0)
	setPoints(92401, 0, 0, 0, 100, 0, 0)
	setPoints(92402, 0, 0, 0, 0, 75, 0)
	if c := Cast(92401, 1, 92402); c != code.OK {
		t.Fatal(c)
	}
	Tick()
	if hp, _, _, _ := Snapshot(92402); hp != 95 {
		t.Fatalf("quarter %d", hp)
	}
}

func TestReductionDoesNotZeroDamage(t *testing.T) {
	seedAttr(t)
	join(t, 92501, 0)
	join(t, 92502, 0)
	mu.Lock()
	units[92501].baseAttack = 1
	refreshAttrs(units[92501], nowFn())
	mu.Unlock()
	setPoints(92502, 0, 0, 0, 0, 75, 0)
	if c := Cast(92501, 1, 92502); c != code.OK {
		t.Fatal(c)
	}
	Tick()
	if hp, _, _, _ := Snapshot(92502); hp != 99 {
		t.Fatalf("floor %d", hp)
	}
}

func TestLifestealCapsAtMissingHP(t *testing.T) {
	seedAttr(t)
	join(t, 92601, 0)
	join(t, 92602, 0)
	setPoints(92601, 0, 0, 0, 0, 0, 50)
	mu.Lock()
	units[92601].hp = 90
	mu.Unlock()
	if c := Cast(92601, 1, 92602); c != code.OK {
		t.Fatal(c)
	}
	frames := Tick()
	heal, ok := healHit(frames, 92601)
	if !ok || heal.Amount != 5 || heal.TargetHP != 95 {
		t.Fatalf("heal %+v", heal)
	}
	mu.Lock()
	units[92601].hp = 98
	mu.Unlock()
	SetClockForTest(1000)
	if c := Cast(92601, 1, 92602); c != code.OK {
		t.Fatal(c)
	}
	frames = Tick()
	heal, ok = healHit(frames, 92601)
	if !ok || heal.Amount != 2 || heal.TargetHP != 100 {
		t.Fatalf("capped %+v", heal)
	}
}

func TestDotUsesStrikeTail(t *testing.T) {
	seedAttr(t)
	join(t, 92701, 0)
	setPoints(92701, 0, 0, 0, 0, 0, 100)
	if c := ApplyBuff(92701, 1); c != code.OK {
		t.Fatal(c)
	}
	SetClockForTest(1000)
	frames := Tick()
	hit, ok := buffHit(frames, 92701, 92701, false)
	if !ok || hit.Amount != 4 || hit.Crit || hit.BuffID != 1 {
		t.Fatalf("dot %+v", hit)
	}
	heal, ok := buffHit(frames, 92701, 92701, true)
	if !ok || heal.Amount != 4 || heal.BuffID != 1 {
		t.Fatalf("leech %+v", heal)
	}
	if hp, _, _, _ := Snapshot(92701); hp != 100 {
		t.Fatalf("hp %d", hp)
	}
}

func TestDotCritPenBonusAndReduction(t *testing.T) {
	seedAttr(t)
	join(t, 92801, 0)
	join(t, 92802, 0)
	if c := ApplyBuff(92802, 1); c != code.OK {
		t.Fatal(c)
	}
	mu.Lock()
	units[92802].buffs[0].source = 92801
	units[92802].baseDefense = 5
	mu.Unlock()
	setPoints(92801, 100, 0, 100, 100, 0, 0)
	setPoints(92802, 0, 0, 0, 0, 50, 0)
	SetClockForTest(1000)
	frames := Tick()
	hit, ok := buffHit(frames, 92801, 92802, false)
	// 4 暴击成 6，穿透后不减 5 防，加成翻倍成 12，减免 50% 成 6。
	if !ok || !hit.Crit || hit.Amount != 6 {
		t.Fatalf("dot %+v", hit)
	}
}

func TestHotCritAndBonusIgnoreReduction(t *testing.T) {
	seedAttr(t)
	join(t, 92901, 0)
	if c := ApplyBuff(92901, 8); c != code.OK {
		t.Fatal(c)
	}
	mu.Lock()
	units[92901].hp = 50
	mu.Unlock()
	setPoints(92901, 0, 0, 100, 0, 75, 100)
	SetClockForTest(1000)
	frames := Tick()
	heal, ok := buffHit(frames, 92901, 92901, true)
	if !ok || heal.Crit || heal.Amount != 10 || heal.TargetHP != 60 {
		t.Fatalf("plain %+v", heal)
	}
	mu.Lock()
	units[92901].hp = 40
	mu.Unlock()
	setPoints(92901, 100, 0, 0, 100, 75, 0)
	SetClockForTest(2000)
	frames = Tick()
	heal, ok = buffHit(frames, 92901, 92901, true)
	// 10 暴击成 15，伤害加成再翻倍成 30。减免和吸血不改治疗。
	if !ok || !heal.Crit || heal.Amount != 30 || heal.TargetHP != 70 {
		t.Fatalf("amped %+v", heal)
	}
}

func buffHit(frames []Frame, src, dst int64, heal bool) (Hit, bool) {
	for _, f := range frames {
		for _, h := range f.Hits {
			if h.SourceUID == src && h.TargetUID == dst && h.Heal == heal && h.BuffID != 0 {
				return h, true
			}
		}
	}
	return Hit{}, false
}
