package internal

import "testing"

func setupCombatScaleForAdvisor() {
	combatScale = CombatScale{CritChance: 100, CritDmg: 1000, AtkSpeed: 1, Atk: 1, EnchantPercentDivisor: 1000}
	armorThreshold = 1000 // limiar de armadura fixo p/ testes determinísticos
}

func TestDetectRoles(t *testing.T) {
	// carry: muito DPS, pouco HP ; tank: pouco DPS, muito HP
	carry := HeroCombat{Key: 1, ModeledDPS: 1000, EffectiveHP: 100}
	tank := HeroCombat{Key: 2, ModeledDPS: 50, EffectiveHP: 5000}
	roles := detectRoles([]HeroCombat{carry, tank})
	if roles[1] != "carry" {
		t.Fatalf("herói 1 devia ser carry, foi %q", roles[1])
	}
	if roles[2] != "tank" {
		t.Fatalf("herói 2 devia ser tank, foi %q", roles[2])
	}
}

func TestDetectRolesUnicoHeroiViraCarry(t *testing.T) {
	roles := detectRoles([]HeroCombat{{Key: 9, ModeledDPS: 100, EffectiveHP: 100}})
	if roles[9] != "carry" {
		t.Fatalf("herói único (sem margem defensiva) devia ser carry, foi %q", roles[9])
	}
}

func TestBestBagUpgradesDPS(t *testing.T) {
	setupCombatScaleForAdvisor()
	heroBaseStats = map[int]HeroBaseStats{101: {ATK: 0, AtkSpeed: 1, MaxHp: 100}}
	itemBaseStats = map[int][]ItemBaseStat{
		300001: {{Stat: "AttackDamage", Mod: "FLAT", Value: 10}},  // equipado (fraco)
		400001: {{Stat: "AttackDamage", Mod: "FLAT", Value: 60}},  // na bag (forte, mesmo part)
		900001: {{Stat: "AttackDamage", Mod: "FLAT", Value: 5}},   // na bag, part errado
	}
	itemMeta = map[int]ItemMeta{
		300001: {Grade: "COMMON", Part: "weapon", Gear: "Espada"},
		400001: {Grade: "RARE", Part: "weapon", Gear: "Espada"},
		900001: {Grade: "RARE", Part: "armor", Gear: "Elmo"},
	}
	he := HeroEquipment{HeroKey: 101, Active: true, Slots: []EquippedSlot{{SlotIndex: 0, ItemKey: 300001}}}
	inv := []StoredItem{{ItemKey: 400001}, {ItemKey: 900001}}

	dpsUp, _ := bestBagUpgrades(he, nil, 2.0, inv)
	if dpsUp == nil || dpsUp.ItemKey != 400001 || dpsUp.SlotIndex != 0 {
		t.Fatalf("upgrade de DPS errado: %+v", dpsUp)
	}
	// ATK 10->60 no slot weapon: modeled cur=10*1, novo=60*1 ; ΔDPS=k*(60-10)=2*50=100
	if !approx(dpsUp.Delta, 100) {
		t.Fatalf("ΔDPS = %v, queria 100", dpsUp.Delta)
	}
}

func TestBestBagUpgradesEHP(t *testing.T) {
	setupCombatScaleForAdvisor()
	heroBaseStats = map[int]HeroBaseStats{102: {ATK: 1, AtkSpeed: 1, MaxHp: 1000, Armor: 0}}
	itemBaseStats = map[int][]ItemBaseStat{
		300010: {{Stat: "MaxHp", Mod: "FLAT", Value: 0}},      // equipado (nada de HP)
		400010: {{Stat: "MaxHp", Mod: "FLAT", Value: 1000}},   // bag: +1000 HP
	}
	itemMeta = map[int]ItemMeta{
		300010: {Grade: "COMMON", Part: "armor", Gear: "Peito"},
		400010: {Grade: "RARE", Part: "armor", Gear: "Peito"},
	}
	he := HeroEquipment{HeroKey: 102, Active: true, Slots: []EquippedSlot{{SlotIndex: 0, ItemKey: 300010}}}
	inv := []StoredItem{{ItemKey: 400010}}

	_, ehpUp := bestBagUpgrades(he, nil, 0.0 /*sem calibração*/, inv)
	if ehpUp == nil || ehpUp.ItemKey != 400010 {
		t.Fatalf("upgrade de EHP errado: %+v", ehpUp)
	}
	// effHP cur = 1000 ; novo = 2000 ; ΔEHP=1000 ; DeltaPct=100
	if !approx(ehpUp.Delta, 1000) || !approx(ehpUp.DeltaPct, 100) {
		t.Fatalf("ΔEHP=%v pct=%v, queria 1000/100", ehpUp.Delta, ehpUp.DeltaPct)
	}
}

func TestTeamComposition(t *testing.T) {
	setupCombatScaleForAdvisor()
	heroBaseStats = map[int]HeroBaseStats{101: {ATK: 1, AtkSpeed: 1}}
	itemBaseStats = map[int][]ItemBaseStat{
		300001: {
			{Stat: "AttackDamage", Mod: "FLAT", Value: 70},   // fAtk 70
			{Stat: "CriticalChance", Mod: "FLAT", Value: 2000}, // /100 => 20 em fCritChance
			{Stat: "AttackSpeed", Mod: "FLAT", Value: 10},    // fAtkSpeed 10
		},
	}
	itemMeta = map[int]ItemMeta{300001: {Part: "weapon"}}
	equip := []HeroEquipment{{HeroKey: 101, Active: true, Slots: []EquippedSlot{{SlotIndex: 0, ItemKey: 300001}}}}
	c := teamComposition(equip, nil)
	// magnitudes: atk 70, crit 20, speed 10 -> total 100 -> 70/20/10 %
	if !approx(c.AtkPct, 70) || !approx(c.CritPct, 20) || !approx(c.SpeedPct, 10) {
		t.Fatalf("composição = %+v, queria 70/20/10", c)
	}
}

func TestRuneRankingFiltraEDetermina(t *testing.T) {
	setupCombatScaleForAdvisor()
	heroBaseStats = map[int]HeroBaseStats{101: {ATK: 10, AtkSpeed: 1}}
	itemBaseStats = map[int][]ItemBaseStat{}
	runeCatalog = map[int]RuneInfo{
		// duas runas de DPS com MESMO dps_por_ouro (empate -> tiebreak por chave: 1 antes de 2)
		1: {Key: 1, Name: "A", MaxLevel: 1, Levels: []RuneLevel{{Level: 1, Cost: 100, Stat: "AllHeroAttackDamage", Value: 5}}},
		2: {Key: 2, Name: "B", MaxLevel: 1, Levels: []RuneLevel{{Level: 1, Cost: 100, Stat: "AllHeroAttackDamage", Value: 5}}},
		// runa utilidade (não-DPS, não ouro/XP): MovementSpeed -> ΔDPS 0 -> NÃO entra
		3: {Key: 3, Name: "C", MaxLevel: 1, Levels: []RuneLevel{{Level: 1, Cost: 100, Stat: "AllHeroMoveSpeed", Value: 50}}},
	}
	equip := []HeroEquipment{{HeroKey: 101, Active: true}}
	r := computeRuneRanking(equip, map[int]int{}, 2.0, 1000)

	if len(r.DPS) != 2 {
		t.Fatalf("esperava 2 runas de DPS (utilidade filtrada), veio %d: %+v", len(r.DPS), r.DPS)
	}
	if r.DPS[0].RuneKey != 1 || r.DPS[1].RuneKey != 2 {
		t.Fatalf("ordem não-determinística em empate: %+v", r.DPS)
	}
}

func TestBuildAdvisorReportV2(t *testing.T) {
	setupCombatScaleForAdvisor()
	heroBaseStats = map[int]HeroBaseStats{
		101: {ATK: 100, AtkSpeed: 2, CritChance: 25, CritDmg: 1400, MaxHp: 100},
		301: {ATK: 1, AtkSpeed: 1, MaxHp: 9000, Armor: 500},
	}
	itemBaseStats = map[int][]ItemBaseStat{
		300001: {{Stat: "AttackDamage", Mod: "FLAT", Value: 0}},
		400001: {{Stat: "AttackDamage", Mod: "FLAT", Value: 50}},
	}
	itemMeta = map[int]ItemMeta{
		300001: {Grade: "COMMON", Part: "weapon", Gear: "Espada"},
		400001: {Grade: "RARE", Part: "weapon", Gear: "Espada"},
	}
	runeCatalog = map[int]RuneInfo{}
	equip := []HeroEquipment{
		{HeroKey: 101, Active: true, Slots: []EquippedSlot{{SlotIndex: 0, ItemKey: 300001}}},
		{HeroKey: 301, Active: true},
	}
	inv := []StoredItem{{ItemKey: 400001}}

	adv := buildAdvisorReport(equip, map[int]int{}, inv, true, 2.0, 100000, 0)
	if !adv.Calibrated {
		t.Fatalf("devia estar calibrado (k>0)")
	}
	if len(adv.Heroes) != 2 {
		t.Fatalf("esperava 2 heróis, veio %d", len(adv.Heroes))
	}
	// herói 101 = carry ; 301 = tank
	roleByHero := map[int]string{}
	for _, h := range adv.Heroes {
		roleByHero[h.HeroKey] = h.RoleAuto
	}
	if roleByHero[101] != "carry" || roleByHero[301] != "tank" {
		t.Fatalf("papéis errados: %+v", roleByHero)
	}
	// carry 101 tem upgrade de DPS da bag (400001)
	for _, h := range adv.Heroes {
		if h.HeroKey == 101 && (h.BestDPSUpgrade == nil || h.BestDPSUpgrade.ItemKey != 400001) {
			t.Fatalf("carry sem upgrade de DPS esperado: %+v", h.BestDPSUpgrade)
		}
	}
	// top-ações: a melhor é equipar 400001 no carry
	if len(adv.TopActions) == 0 || adv.TopActions[0].Kind != "equipar" || adv.TopActions[0].RefKey != 400001 {
		t.Fatalf("top-ações erradas: %+v", adv.TopActions)
	}
}
