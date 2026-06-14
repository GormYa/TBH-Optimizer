package internal

import (
	"math"
	"testing"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestAggregateHeroFlatAdditiveMult(t *testing.T) {
	combatScale = CombatScale{CritChance: 100, CritDmg: 1000, AtkSpeed: 1, Atk: 1, EnchantPercentDivisor: 1000}
	heroBaseStats = map[int]HeroBaseStats{101: {ATK: 2, AtkSpeed: 90, CritChance: 25, CritDmg: 1400}}
	itemBaseStats = map[int][]ItemBaseStat{
		300001: {{Stat: "AttackDamage", Mod: "FLAT", Value: 10}, {Stat: "AttackSpeed", Mod: "FLAT", Value: 10}},
	}

	he := HeroEquipment{
		HeroKey: 101, Level: 1, Active: true,
		Slots: []EquippedSlot{
			{SlotIndex: 0, ItemKey: 300001, Enchants: []ItemEnchant{
				{StatType: 1, ModType: 1, Value: 50, StatModKey: 7},
				{StatType: 3, ModType: 0, Value: 10, StatModKey: 8},
			}},
		},
	}

	hc := aggregateHeroCombat(he, nil)
	if !approx(hc.ATK, 12.6) {
		t.Fatalf("ATK = %v, queria 12.6", hc.ATK)
	}
	if !approx(hc.AtkSpeed, 100) {
		t.Fatalf("AtkSpeed = %v, queria 100", hc.AtkSpeed)
	}
	if !approx(hc.CritChance, 0.35) {
		t.Fatalf("CritChance = %v, queria 0.35", hc.CritChance)
	}
	if !approx(hc.CritDmg, 1.4) {
		t.Fatalf("CritDmg = %v, queria 1.4", hc.CritDmg)
	}
}

func TestAggregateHeroRunasTimeInteiro(t *testing.T) {
	combatScale = CombatScale{CritChance: 100, CritDmg: 1000, AtkSpeed: 1, Atk: 1, EnchantPercentDivisor: 1000}
	heroBaseStats = map[int]HeroBaseStats{201: {ATK: 1, AtkSpeed: 100}}
	itemBaseStats = map[int][]ItemBaseStat{}
	he := HeroEquipment{HeroKey: 201, Active: true, Slots: nil}
	runes := []statContribution{
		{field: fAtk, mod: modFlat, value: 5},
		{field: fAtk, mod: modAdditive, value: 0.10},
	}
	hc := aggregateHeroCombat(he, runes)
	if !approx(hc.ATK, 6.6) {
		t.Fatalf("ATK com runas = %v, queria 6.6", hc.ATK)
	}
}

func TestModeledDPSFormula(t *testing.T) {
	hc := HeroCombat{ATK: 100, AtkSpeed: 2, CritChance: 0.25, CritDmg: 1.4}
	// critDmg é multiplicador cheio: 100*2*(1 + 0.25*(1.4-1)) = 200*1.1 = 220
	if !approx(modeledDPS(hc), 220) {
		t.Fatalf("modeledDPS = %v, queria 220", modeledDPS(hc))
	}
}

func TestCritChanceClampada(t *testing.T) {
	hc := HeroCombat{ATK: 10, AtkSpeed: 1, CritChance: 1.5, CritDmg: 2}
	// chance clampa em 1.0 ; bônus = 2-1 = 1 -> 10*1*(1+1*1) = 20
	if !approx(modeledDPS(hc), 20) {
		t.Fatalf("clamp falhou: %v", modeledDPS(hc))
	}
}

func TestCalibrationK(t *testing.T) {
	k, ok := calibrationK(540, 270, true)
	if !ok || !approx(k, 2.0) {
		t.Fatalf("k = %v ok=%v, queria 2.0/true", k, ok)
	}
	if _, ok := calibrationK(540, 0, true); ok {
		t.Fatalf("modeled=0 devia dar ok=false")
	}
	if _, ok := calibrationK(540, 270, false); ok {
		t.Fatalf("nao-calibrado devia dar ok=false")
	}
}

func TestDeltaDPS(t *testing.T) {
	if !approx(deltaDPS(2.0, 270, 300), 60) {
		t.Fatalf("deltaDPS errado: %v", deltaDPS(2.0, 270, 300))
	}
}

func TestBuildCombatReport(t *testing.T) {
	combatScale = CombatScale{CritChance: 100, CritDmg: 1000, AtkSpeed: 1, Atk: 1, EnchantPercentDivisor: 1000}
	heroBaseStats = map[int]HeroBaseStats{101: {ATK: 100, AtkSpeed: 2, CritChance: 25, CritDmg: 1400}}
	itemBaseStats = map[int][]ItemBaseStat{}
	runeCatalog = map[int]RuneInfo{}

	equip := []HeroEquipment{{HeroKey: 101, Active: true}}
	// modeled = 100*2*(1+0.25*(1.4-1)) = 220 ; effective 440 -> k = 2.0
	rep := buildCombatReport(equip, map[int]int{}, 440, true)
	if !rep.Calibrated || !approx(rep.CalibrationK, 2.0) {
		t.Fatalf("relatório: %+v", rep)
	}
	if len(rep.PerHero) != 1 || !approx(rep.ModeledDPSTotal, 220) {
		t.Fatalf("perHero/total errado: %+v", rep)
	}
}

func TestAggregateHeroSobrevivencia(t *testing.T) {
	combatScale = CombatScale{CritChance: 100, CritDmg: 1000, AtkSpeed: 1, Atk: 1, EnchantPercentDivisor: 1000}
	armorThreshold = 1000 // limiar fixo p/ teste determinístico (red = 1000/2000 = 0.5)
	heroBaseStats = map[int]HeroBaseStats{301: {ATK: 1, AtkSpeed: 1, MaxHp: 1000, Armor: 0}}
	itemBaseStats = map[int][]ItemBaseStat{
		500001: {{Stat: "MaxHp", Mod: "FLAT", Value: 1000}, {Stat: "Armor", Mod: "FLAT", Value: 1000}},
	}
	he := HeroEquipment{HeroKey: 301, Active: true, Slots: []EquippedSlot{
		{SlotIndex: 0, ItemKey: 500001},
	}}
	hc := aggregateHeroCombat(he, nil)
	if !approx(hc.MaxHp, 2000) {
		t.Fatalf("MaxHp = %v, queria 2000", hc.MaxHp)
	}
	// red = armor/(armor+threshold) = 1000/2000 = 0.5 ; effHP = 2000/(1-0.5) = 4000
	if !approx(hc.Armor, 1000) || !approx(hc.EffectiveHP, 4000) {
		t.Fatalf("armor=%v effHP=%v, queria 1000/4000", hc.Armor, hc.EffectiveHP)
	}
}

func TestEffectiveHPSemHpZero(t *testing.T) {
	if effectiveHP(0, 500) != 0 {
		t.Fatalf("sem maxHp, effHP deve ser 0")
	}
}

func TestEffectiveHPTeto(t *testing.T) {
	armorThreshold = 1000
	// armadura enorme -> redução capada em 75% -> effHP = maxHp/(1-0.75) = maxHp*4
	if got := effectiveHP(1000, 1_000_000); !approx(got, 4000) {
		t.Fatalf("teto de 75%% falhou: effHP = %v, queria 4000", got)
	}
}

func TestSetArmorStageLevel(t *testing.T) {
	setArmorStageLevel(50) // 14*50+12 = 712
	if !approx(armorThreshold, 712) {
		t.Fatalf("armorThreshold = %v, queria 712", armorThreshold)
	}
	setArmorStageLevel(0) // <=0 não altera
	if !approx(armorThreshold, 712) {
		t.Fatalf("stageLevel<=0 não deveria alterar: %v", armorThreshold)
	}
}
