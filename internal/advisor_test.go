package internal

import "testing"

func TestBuildAdvisorOutlook(t *testing.T) {
	ctrl := &Control{
		MaxCompletedStage: 1102,
		FarmStages: map[int]FarmStageInfo{
			1101: {Key: 1101, Label: "1-1", Level: 1, TotalHP: 100, Waves: 5},
			1103: {Key: 1103, Label: "1-3", Level: 3, TotalHP: 300, Waves: 5},   // 3s -> viavel
			1104: {Key: 1104, Label: "1-4", Level: 4, TotalHP: 6000, Waves: 5},  // 60s -> lento
			1105: {Key: 1105, Label: "1-5", Level: 5, TotalHP: 20000, Waves: 5}, // 200s -> parede
		},
	}
	rep := buildAdvisorReport(ctrl, 100.0, 0.0, true)
	if !rep.Calibrated || rep.CurrentStage != 1102 {
		t.Fatalf("cabeçalho: %+v", rep)
	}
	if len(rep.Outlook) != 3 {
		t.Fatalf("3 fases, veio %d", len(rep.Outlook))
	}
	if rep.Outlook[0].StageKey != 1103 || rep.Outlook[0].Verdict != "viavel" {
		t.Fatalf("1103 viável: %+v", rep.Outlook[0])
	}
	if rep.Outlook[1].Verdict != "lento" || rep.Outlook[2].Verdict != "parede" {
		t.Fatalf("verdicts: %+v", rep.Outlook)
	}
	if rep.NextWall == nil || rep.NextWall.StageKey != 1104 {
		t.Fatalf("next_wall=1104: %+v", rep.NextWall)
	}
}

func TestBuildStalledGearEnchantAware(t *testing.T) {
	combatScale = CombatScale{CritChance: 100, CritDmg: 1000, AtkSpeed: 1, Atk: 1, EnchantPercentDivisor: 1000}
	heroBaseStats = map[int]HeroBaseStats{101: {ATK: 0, AtkSpeed: 1, CritChance: 0, CritDmg: 1000}}
	// item equipado (key 1) base 100 ATK; candidatos de mesmo gear "sword".
	itemBaseStats = map[int][]ItemBaseStat{
		1: {{Stat: "AttackDamage", Mod: "FLAT", Value: 100}},
		2: {{Stat: "AttackDamage", Mod: "FLAT", Value: 120}}, // base maior, SEM enchant
		3: {{Stat: "AttackDamage", Mod: "FLAT", Value: 90}},  // pior
		4: {{Stat: "AttackDamage", Mod: "FLAT", Value: 999}}, // gear diferente -> ignorar
		5: {{Stat: "AttackDamage", Mod: "FLAT", Value: 200}}, // de fato superior
	}
	itemMeta = map[int]ItemMeta{
		1: {Gear: "sword"}, 2: {Gear: "sword"}, 3: {Gear: "sword"}, 4: {Gear: "bow"}, 5: {Gear: "sword"},
	}
	// equipado tem enchant +50 ATK (StatType 1 = AttackDamage, FLAT) -> efetivo 150.
	ctrl := &Control{
		HeroEquipment: []HeroEquipment{{HeroKey: 101, Active: true, Slots: []EquippedSlot{
			{SlotIndex: 0, ItemKey: 1, Enchants: []ItemEnchant{{StatModKey: 5001, StatType: 1, ModType: 0, Value: 50}}},
		}}},
		Inventory: []StoredItem{
			{ItemKey: 2}, {ItemKey: 3}, {ItemKey: 4}, {ItemKey: 5},
		},
	}
	got := buildStalledGear(ctrl, nil)
	// key 2 (120) < equipado efetivo 150 -> NÃO sugere (prova que o enchant conta).
	// key 5 (200) > 150 -> única sugestão.
	if len(got) != 1 {
		t.Fatalf("esperava 1 sugestão (só key 5 domina), veio %d: %+v", len(got), got)
	}
	g := got[0]
	if g.HeroKey != 101 || g.SlotIndex != 0 || g.FromItem != 1 || g.ToItem != 5 {
		t.Fatalf("esperava troca 1->5 no slot 0 do herói 101, veio %+v", g)
	}
}

func TestBuildAdvisorPesosHeroi(t *testing.T) {
	combatScale = CombatScale{CritChance: 100, CritDmg: 1000, AtkSpeed: 1, Atk: 1, EnchantPercentDivisor: 1000}
	heroBaseStats = map[int]HeroBaseStats{
		101: {ATK: 200, AtkSpeed: 1, CritChance: 0, CritDmg: 1000},
		102: {ATK: 100, AtkSpeed: 1, CritChance: 0, CritDmg: 1000},
	}
	itemBaseStats = map[int][]ItemBaseStat{}
	ctrl := &Control{
		MaxCompletedStage: 0, FarmStages: map[int]FarmStageInfo{},
		HeroEquipment: []HeroEquipment{
			{HeroKey: 101, Active: true}, {HeroKey: 102, Active: true}, {HeroKey: 103, Active: false},
		},
		RuneLevels: map[int]int{},
	}
	rep := buildAdvisorReport(ctrl, 0, 0, false)
	if len(rep.Heroes) != 2 {
		t.Fatalf("só heróis ativos: %d", len(rep.Heroes))
	}
	var w101, w102 float64
	for _, h := range rep.Heroes {
		if h.HeroKey == 101 {
			w101 = h.Weight
		}
		if h.HeroKey == 102 {
			w102 = h.Weight
		}
	}
	if w101 <= w102 {
		t.Fatalf("101 deveria ter mais peso: %v vs %v", w101, w102)
	}
}
