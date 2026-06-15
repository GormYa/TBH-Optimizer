package internal

import "testing"

func approxW(a, b float64) bool { d := a - b; if d < 0 { d = -d }; return d < 1e-6 }

func TestAggregateHeroStatsLayering(t *testing.T) {
	combatScale = CombatScale{CritChance: 100, CritDmg: 1000, AtkSpeed: 1, Atk: 1, EnchantPercentDivisor: 1000}
	heroBaseStats = map[int]HeroBaseStats{101: {ATK: 100, AtkSpeed: 2, CritChance: 2500, CritDmg: 1500}}
	itemBaseStats = map[int][]ItemBaseStat{
		300001: {{Stat: "AttackDamage", Mod: "FLAT", Value: 50}},
	}
	itemMeta = map[int]ItemMeta{300001: {Part: "weapon", Gear: "Espada"}}
	he := HeroEquipment{HeroKey: 101, Active: true, Slots: []EquippedSlot{{SlotIndex: 0, ItemKey: 300001}}}
	hs := aggregateHeroStats(he, nil)
	if !approxW(hs.ATK, 150) {
		t.Fatalf("ATK=%v queria 150", hs.ATK)
	}
	if hs.AtkSpeed != 2 {
		t.Fatalf("AtkSpeed=%v queria 2", hs.AtkSpeed)
	}
}

func TestDamageWeightRatio(t *testing.T) {
	a := HeroStats{ATK: 100, AtkSpeed: 1, CritChance: 0, CritDmg: 1}
	b := HeroStats{ATK: 200, AtkSpeed: 1, CritChance: 0, CritDmg: 1}
	wa, wb := damageWeight(a), damageWeight(b)
	if wb <= wa || !approxW(wb/wa, 2) {
		t.Fatalf("peso deve dobrar: wa=%v wb=%v", wa, wb)
	}
	c := HeroStats{ATK: 100, AtkSpeed: 1, CritChance: 0.5, CritDmg: 1.4}
	if !approxW(damageWeight(c), 120) {
		t.Fatalf("peso com crit=%v queria 120", damageWeight(c))
	}
}
