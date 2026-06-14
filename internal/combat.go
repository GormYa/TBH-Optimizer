package internal

// Campos de combate que o modelo agrega.
const (
	fAtk = iota
	fAtkSpeed
	fCritChance
	fCritDmg
	fMaxHp
	fArmor
	fIgnore // stat fora do modelo (utilidade pura)
)

// ModType (do save / items.json): 0=FLAT soma, 1=ADDITIVE soma %, 2=MULTIPLICATIVE multiplica.
const (
	modFlat           = 0
	modAdditive       = 1
	modMultiplicative = 2
)

// divisores de % por fonte (runa/gear-base usam centésimos; enchant usa milésimos — vem do combatScale).
const (
	runePercentDivisor = 100.0
	gearPercentDivisor = 100.0
)

// statContribution já está roteada/escalada pro espaço do modelo.
type statContribution struct {
	field int
	mod   int
	value float64
	slot  int // índice do slot de origem; -1 = não-de-slot (base/runa/pet)
}

// HeroCombat: stats agregados (unidades internas) + DPS modelado.
type HeroCombat struct {
	Key        int     `json:"key"`
	ATK        float64 `json:"atk"`
	AtkSpeed   float64 `json:"atk_speed"`
	CritChance  float64 `json:"crit_chance"`
	CritDmg     float64 `json:"crit_dmg"`
	MaxHp       float64 `json:"max_hp"`
	Armor       float64 `json:"armor"`
	EffectiveHP float64 `json:"effective_hp"`
	ModeledDPS  float64 `json:"modeled_dps"`
}

// modName mapeia o campo string ("FLAT"/"ADDITIVE"/"MULTIPLICATIVE") do items.json pro int.
func modName(s string) int {
	switch s {
	case "ADDITIVE", "PERCENT":
		return modAdditive
	case "MULTIPLICATIVE", "MULT":
		return modMultiplicative
	default:
		return modFlat
	}
}

// routeStat decide em qual campo do modelo um stat entra e como escalar o valor cru.
// percentDiv é o divisor de % daquela fonte (enchant vs runa/gear). Devolve ok=false p/ ignorar.
func routeStat(stat string, mod int, raw, percentDiv float64) (field int, modOut int, value float64, ok bool) {
	switch stat {
	case "AttackDamage", "AllHeroAttackDamage":
		if mod == modAdditive {
			return fAtk, modAdditive, raw / percentDiv, true
		}
		return fAtk, modFlat, raw / combatScale.Atk, true
	case "AttackDamagePercent", "AllHeroAttackDamagePercent", "PhysicalDamagePercent", "IncreaseMeleeDamage":
		return fAtk, modAdditive, raw / percentDiv, true
	case "AttackSpeed", "AllHeroAttackSpeed":
		if mod == modAdditive {
			return fAtkSpeed, modAdditive, raw / percentDiv, true
		}
		return fAtkSpeed, modFlat, raw / combatScale.AtkSpeed, true
	case "CriticalChance":
		return fCritChance, modFlat, raw / combatScale.CritChance, true
	case "CriticalDamage":
		return fCritDmg, modFlat, raw / combatScale.CritDmg, true
	case "MaxHp", "AllHeroMaxHp":
		if mod == modAdditive {
			return fMaxHp, modAdditive, raw / percentDiv, true
		}
		return fMaxHp, modFlat, raw, true
	case "MaxHpPercent", "AllHeroMaxHpPercent":
		return fMaxHp, modAdditive, raw / percentDiv, true
	case "Armor", "AllHeroArmor":
		if mod == modAdditive {
			return fArmor, modAdditive, raw / percentDiv, true
		}
		return fArmor, modFlat, raw, true
	case "ArmorPercent", "AllHeroArmorPercent":
		return fArmor, modAdditive, raw / percentDiv, true
	}
	return fIgnore, 0, 0, false
}

// statAccum: valor final = flat * (1+pct) * mult.
type statAccum struct {
	flat, pct, mult float64
}

func newAccum(base float64) statAccum { return statAccum{flat: base, pct: 0, mult: 1} }
func (a *statAccum) apply(mod int, v float64) {
	switch mod {
	case modAdditive:
		a.pct += v
	case modMultiplicative:
		a.mult *= (1 + v)
	default:
		a.flat += v
	}
}
func (a statAccum) value() float64 { return a.flat * (1 + a.pct) * a.mult }

// heroContributions monta a lista de contribuições de um herói (gear base + enchants).
// Cada uma marcada com o slot de origem (p/ projeção em C2). Runas/pet entram via extra.
func heroContributions(he HeroEquipment) []statContribution {
	var out []statContribution
	for _, s := range he.Slots {
		if s.ItemKey != 0 {
			for _, b := range itemBaseStats[s.ItemKey] {
				if f, m, v, ok := routeStat(b.Stat, modName(b.Mod), b.Value, gearPercentDivisor); ok {
					out = append(out, statContribution{field: f, mod: m, value: v, slot: s.SlotIndex})
				}
			}
		}
		for _, e := range s.Enchants {
			name := statTypeName(e.StatType)
			if f, m, v, ok := routeStat(name, e.ModType, float64(e.Value), combatScale.EnchantPercentDivisor); ok {
				out = append(out, statContribution{field: f, mod: m, value: v, slot: s.SlotIndex})
			}
		}
	}
	return out
}

// aggregateHeroCombat dobra base + contribuições (do herói) + extra (runas/pet, slot=-1).
func aggregateHeroCombat(he HeroEquipment, extra []statContribution) HeroCombat {
	base := heroBaseStats[he.HeroKey]
	atk := newAccum(base.ATK)
	spd := newAccum(base.AtkSpeed)
	critC := newAccum(base.CritChance / combatScale.CritChance)
	critD := newAccum(base.CritDmg / combatScale.CritDmg)
	maxHp := newAccum(base.MaxHp)
	armor := newAccum(base.Armor)

	fold := func(c statContribution) {
		switch c.field {
		case fAtk:
			atk.apply(c.mod, c.value)
		case fAtkSpeed:
			spd.apply(c.mod, c.value)
		case fCritChance:
			critC.apply(c.mod, c.value)
		case fCritDmg:
			critD.apply(c.mod, c.value)
		case fMaxHp:
			maxHp.apply(c.mod, c.value)
		case fArmor:
			armor.apply(c.mod, c.value)
		}
	}
	for _, c := range heroContributions(he) {
		fold(c)
	}
	for _, c := range extra {
		fold(c)
	}

	hc := HeroCombat{
		Key:        he.HeroKey,
		ATK:        atk.value(),
		AtkSpeed:   spd.value(),
		CritChance: critC.value(),
		CritDmg:    critD.value(),
		MaxHp:      maxHp.value(),
		Armor:      armor.value(),
	}
	hc.ModeledDPS = modeledDPS(hc)
	hc.EffectiveHP = effectiveHP(hc.MaxHp, hc.Armor)
	return hc
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// Mitigação de armadura (wiki taskbarhero/mechanics, constantes verificadas no binário):
//   Reduction = Armor / (Armor + 14*StageLevel + 12), com teto de 75%.
// armorThreshold = 14*StageLevel + 12. Default ~fase 50; o advisor ajusta via
// setArmorStageLevel() com o nível da fase atual do jogador.
const armorMaxReduction = 0.75

var armorThreshold = 14.0*50 + 12

// setArmorStageLevel atualiza o limiar de armadura pro nível de fase informado.
func setArmorStageLevel(stageLevel int) {
	if stageLevel > 0 {
		armorThreshold = 14.0*float64(stageLevel) + 12
	}
}

// effectiveHP: HP dividido por (1 - mitigação de armadura). ESTIMATIVA — depende do
// nível de fase (armorThreshold) e ignora dodge/block/resistências. Sem maxHp => 0.
func effectiveHP(maxHp, armor float64) float64 {
	if maxHp <= 0 {
		return 0
	}
	red := armor / (armor + armorThreshold)
	if red > armorMaxReduction {
		red = armorMaxReduction
	}
	return maxHp / (1 - red)
}

// modeledDPS: fórmula confirmada (web/combat_model.json + wiki). Valor em unidades
// internas; só usado via razão k / deltas, nunca absoluto.
func modeledDPS(hc HeroCombat) float64 {
	// critDmg é o multiplicador CHEIO (wiki taskbarhero/mechanics, verificado no binário):
	// DPS = atkSpeed × atk × (1 + critChance × (critDamage − 1)). Bônus = mult − 1.
	critBonus := hc.CritDmg - 1
	if critBonus < 0 {
		critBonus = 0
	}
	critFactor := 1 + clamp01(hc.CritChance)*critBonus
	return hc.ATK * hc.AtkSpeed * critFactor
}

// teamModeledDPS soma o DPS modelado dos heróis ATIVOS.
func teamModeledDPS(team []HeroCombat) float64 {
	total := 0.0
	for _, hc := range team {
		total += hc.ModeledDPS
	}
	return total
}

// calibrationK = effective_dps / modeled_dps_total, só quando calibrado e ambos > 0.
func calibrationK(effectiveDPS, modeledTotal float64, calibrated bool) (float64, bool) {
	if !calibrated || modeledTotal <= 0 || effectiveDPS <= 0 {
		return 0, false
	}
	return effectiveDPS / modeledTotal, true
}

// deltaDPS converte uma mudança no DPS modelado de UM herói em ΔDPS real, via k.
func deltaDPS(k, oldHeroModeled, newHeroModeled float64) float64 {
	return k * (newHeroModeled - oldHeroModeled)
}

type CombatReport struct {
	PerHero         []HeroCombat `json:"per_hero"`
	ModeledDPSTotal float64      `json:"modeled_dps_total"`
	CalibrationK    float64      `json:"calibration_k"`
	Calibrated      bool         `json:"calibrated"`
}

// resolveRuneContributions traduz as runas LEVELADAS do jogador em contribuições de time
// (aplicadas a todo herói ativo). Usa o valor no nível atual de cada runa.
func resolveRuneContributions(runeLevels map[int]int) []statContribution {
	var out []statContribution
	for key, lvl := range runeLevels {
		if lvl <= 0 {
			continue
		}
		info, ok := runeCatalog[key]
		if !ok || lvl > len(info.Levels) {
			continue
		}
		lv := info.Levels[lvl-1]
		if f, m, v, ok := routeStat(lv.Stat, modAdditiveIfPercent(lv.Stat), lv.Value, runePercentDivisor); ok {
			out = append(out, statContribution{field: f, mod: m, value: v, slot: -1})
		}
	}
	return out
}

// modAdditiveIfPercent: runas "*Percent" são ADDITIVE; o resto FLAT (routeStat reconfirma).
func modAdditiveIfPercent(stat string) int {
	switch stat {
	case "AllHeroAttackDamagePercent", "AllHeroArmorPercent":
		return modAdditive
	}
	return modFlat
}

// buildCombatReport agrega o time ativo, calcula modeled_total e k.
func buildCombatReport(equip []HeroEquipment, runeLevels map[int]int, effectiveDPS float64, calibrated bool) *CombatReport {
	runes := resolveRuneContributions(runeLevels)
	var per []HeroCombat
	for _, he := range equip {
		if he.Active {
			per = append(per, aggregateHeroCombat(he, runes))
		}
	}
	total := teamModeledDPS(per)
	k, ok := calibrationK(effectiveDPS, total, calibrated)
	return &CombatReport{
		PerHero:         per,
		ModeledDPSTotal: total,
		CalibrationK:    k,
		Calibrated:      ok,
	}
}
