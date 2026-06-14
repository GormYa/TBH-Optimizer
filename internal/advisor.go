package internal

import (
	"sort"
	"strconv"
	"strings"
)

// ===================== Papéis (carry × tank) =====================

// roleMargin: um herói só é "tank" se sua fatia de HP efetivo do time superar
// sua fatia de DPS por esta margem. Sem margem, balanceados caem em "carry".
const roleMargin = 1.2

// detectRoles classifica cada herói ATIVO por papel comparando sua fatia de
// sobrevivência vs sua fatia de dano no time (unit-free, comparativo).
// Retorna map heroKey -> "carry"|"tank".
func detectRoles(team []HeroCombat) map[int]string {
	totDPS, totEHP := 0.0, 0.0
	for _, h := range team {
		totDPS += h.ModeledDPS
		totEHP += h.EffectiveHP
	}
	roles := map[int]string{}
	for _, h := range team {
		ds, es := 0.0, 0.0
		if totDPS > 0 {
			ds = h.ModeledDPS / totDPS
		}
		if totEHP > 0 {
			es = h.EffectiveHP / totEHP
		}
		if es > ds*roleMargin {
			roles[h.Key] = "tank"
		} else {
			roles[h.Key] = "carry"
		}
	}
	return roles
}

// ===================== Upgrades-da-bag (DPS e HP efetivo) =====================

// BagUpgrade: melhor item POSSUÍDO (bag + baú + trade, via ctrl.Inventory) que
// melhora um slot do herói (stats-base; a bag não guarda rolls de enchant). Para
// DPS, Delta já é k-escalado (ΔDPS); para EHP, Delta é o ganho bruto de HP efetivo
// (estimativa) e DeltaPct o ganho %.
type BagUpgrade struct {
	ItemKey   int     `json:"item_key"`
	Grade     string  `json:"grade"`
	SlotIndex int     `json:"slot_index"`
	Metric    string  `json:"metric"` // "dps" | "ehp"
	Delta    float64 `json:"delta"`
	// DeltaPct: para DPS, é o % no espaço do MODELO (igual à % calibrada, pois k cancela na razão);
	// para EHP, é o % de ganho de HP efetivo (estimativa).
	DeltaPct float64 `json:"delta_pct"`
}

// heroWithSlots recomputa os stats do herói trocando itens por slot.
// override[slot] = itemKey (>0 troca e zera enchants do slot; -1 esvazia; ausente mantém).
func heroWithSlots(he HeroEquipment, extra []statContribution, override map[int]int) HeroCombat {
	clone := HeroEquipment{HeroKey: he.HeroKey, Level: he.Level, Active: he.Active}
	clone.Slots = make([]EquippedSlot, len(he.Slots))
	for i, s := range he.Slots {
		ns := s
		if ik, ok := override[s.SlotIndex]; ok {
			if ik < 0 {
				ns.ItemKey = 0
				ns.Enchants = nil
			} else if ik > 0 {
				ns.ItemKey = ik
				ns.Enchants = nil
			}
		}
		clone.Slots[i] = ns
	}
	return aggregateHeroCombat(clone, extra)
}

// bestBagUpgrades varre TODOS os itens possuídos (inv = bag + baú + trade) por slot
// do herói (mesmo Part do item equipado) e devolve o melhor upgrade de DPS e o melhor
// de HP efetivo. Itens travados (IsBlocked) CONTAM — cadeado não impede equipar.
// Seleção por delta do MODELO (funciona sem calibração); para DPS, Delta sai k-escalado.
func bestBagUpgrades(he HeroEquipment, extra []statContribution, k float64, inv []StoredItem) (dpsUp, ehpUp *BagUpgrade) {
	cur := aggregateHeroCombat(he, extra)
	bestDPSModeled, bestEHP := 0.0, 0.0
	for _, s := range he.Slots {
		if s.ItemKey == 0 {
			// Limitação v1 deliberada: slots VAZIOS não recebem sugestão porque não há
			// mapa slotIndex→Part confirmado nos dados; preencher slot vazio fica como follow-up.
			continue
		}
		part := itemMeta[s.ItemKey].Part
		if part == "" {
			continue
		}
		for _, it := range inv {
			if it.ItemKey == 0 {
				continue
			}
			m, ok := itemMeta[it.ItemKey]
			if !ok || m.Part != part {
				continue
			}
			nc := heroWithSlots(he, extra, map[int]int{s.SlotIndex: it.ItemKey})

			if dM := nc.ModeledDPS - cur.ModeledDPS; dM > 0 && (dpsUp == nil || dM > bestDPSModeled) {
				bestDPSModeled = dM
				pct := 0.0
				if cur.ModeledDPS > 0 {
					pct = dM / cur.ModeledDPS * 100
				}
				dpsUp = &BagUpgrade{ItemKey: it.ItemKey, Grade: m.Grade, SlotIndex: s.SlotIndex, Metric: "dps", Delta: k * dM, DeltaPct: pct}
			}
			if dE := nc.EffectiveHP - cur.EffectiveHP; dE > 0 && (ehpUp == nil || dE > bestEHP) {
				bestEHP = dE
				pct := 0.0
				if cur.EffectiveHP > 0 {
					pct = dE / cur.EffectiveHP * 100
				}
				ehpUp = &BagUpgrade{ItemKey: it.ItemKey, Grade: m.Grade, SlotIndex: s.SlotIndex, Metric: "ehp", Delta: dE, DeltaPct: pct}
			}
		}
	}
	return dpsUp, ehpUp
}

// ===================== Composição do build =====================

// Composition: distribuição APROXIMADA do investimento de combate do time entre
// os três níveis de dano (magnitudes das contribuições roteadas, normalizadas).
// É indicador qualitativo, não decomposição exata do DPS — a UI rotula "aprox.".
type Composition struct {
	AtkPct   float64 `json:"atk_pct"`
	CritPct  float64 `json:"crit_pct"`
	SpeedPct float64 `json:"speed_pct"`
}

func absf(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func teamComposition(equip []HeroEquipment, runes []statContribution) Composition {
	atk, crit, spd := 0.0, 0.0, 0.0
	add := func(cs []statContribution) {
		for _, c := range cs {
			switch c.field {
			case fAtk:
				atk += absf(c.value)
			case fCritChance, fCritDmg:
				crit += absf(c.value)
			case fAtkSpeed:
				spd += absf(c.value)
			}
		}
	}
	for _, he := range equip {
		if he.Active {
			add(heroContributions(he))
		}
	}
	add(runes)
	tot := atk + crit + spd
	if tot <= 0 {
		return Composition{}
	}
	return Composition{AtkPct: atk / tot * 100, CritPct: crit / tot * 100, SpeedPct: spd / tot * 100}
}

// ===================== Ranking de runas (determinístico) =====================

type RuneDPSRank struct {
	RuneKey    int     `json:"rune_key"`
	NextLevel  int     `json:"next_level"`
	Cost       float64 `json:"cost"`
	DeltaDPS   float64 `json:"delta_dps"`
	DeltaPct   float64 `json:"delta_pct"`
	DPSPerGold float64 `json:"dps_per_gold"`
	Affordable bool    `json:"affordable"`
}
type RuneGoldXPRank struct {
	RuneKey     int     `json:"rune_key"`
	NextLevel   int     `json:"next_level"`
	Cost        float64 `json:"cost"`
	Stat        string  `json:"stat"`
	GainPct     float64 `json:"gain_pct"`
	GainPerGold float64 `json:"gain_per_gold"`
	Affordable  bool    `json:"affordable"`
}
type RuneRanking struct {
	DPS    []RuneDPSRank    `json:"dps"`
	GoldXP []RuneGoldXPRank `json:"gold_xp"`
}

func isGoldXPStat(stat string) bool {
	for _, p := range []string{"AdditionalGold", "AdditionalExp", "IncreaseGoldAmount", "IncreaseExpAmount", "OfflineRewardGold", "OfflineRewardExp"} {
		if strings.HasPrefix(stat, p) {
			return true
		}
	}
	return false
}

// prereqOK: prevReq é a chave de uma runa que precisa estar LEVELADA (>=1). "" = sem prereq.
func prereqOK(prevReq string, runeLevels map[int]int) bool {
	if prevReq == "" || prevReq == "0" {
		return true
	}
	pk, err := strconv.Atoi(prevReq)
	if err != nil {
		return true
	}
	return runeLevels[pk] >= 1
}

// computeRuneRanking: ΔDPS via projeção por runa comprável. Filtra DPS<=0 e ordena
// de forma DETERMINÍSTICA (por dps/ouro, tiebreak por RuneKey) — fim do reshuffle.
func computeRuneRanking(equip []HeroEquipment, runeLevels map[int]int, k, gold float64) RuneRanking {
	curRunes := resolveRuneContributions(runeLevels)
	baseTeam := 0.0
	var activeHeroes []HeroEquipment
	for _, he := range equip {
		if he.Active {
			activeHeroes = append(activeHeroes, he)
			baseTeam += aggregateHeroCombat(he, curRunes).ModeledDPS
		}
	}

	var out RuneRanking
	for key, info := range runeCatalog {
		next := runeLevels[key] + 1
		if next > info.MaxLevel || next > len(info.Levels) {
			continue
		}
		if !prereqOK(info.PrevReq, runeLevels) {
			continue
		}
		lv := info.Levels[next-1]
		affordable := gold >= lv.Cost

		if isGoldXPStat(lv.Stat) {
			prev := 0.0
			if next-1 >= 1 && next-1 <= len(info.Levels) {
				prev = info.Levels[next-2].Value
			}
			gain := lv.Value - prev
			gp := 0.0
			if lv.Cost > 0 {
				gp = gain / lv.Cost
			}
			out.GoldXP = append(out.GoldXP, RuneGoldXPRank{
				RuneKey: key, NextLevel: next, Cost: lv.Cost, Stat: lv.Stat,
				GainPct: gain, GainPerGold: gp, Affordable: affordable,
			})
			continue
		}

		projLevels := map[int]int{}
		for kk, vv := range runeLevels {
			projLevels[kk] = vv
		}
		projLevels[key] = next
		projRunes := resolveRuneContributions(projLevels)
		newTeam := 0.0
		for _, he := range activeHeroes {
			newTeam += aggregateHeroCombat(he, projRunes).ModeledDPS
		}
		modeledDelta := newTeam - baseTeam
		if modeledDelta <= 0 {
			continue // runa que não move DPS (utilidade) fica fora deste ranking
		}
		delta := k * modeledDelta
		pct := 0.0
		if baseTeam > 0 {
			pct = modeledDelta / baseTeam * 100
		}
		dpg := 0.0
		if lv.Cost > 0 {
			dpg = modeledDelta / lv.Cost // por-ouro estável mesmo sem k
		}
		out.DPS = append(out.DPS, RuneDPSRank{
			RuneKey: key, NextLevel: next, Cost: lv.Cost,
			DeltaDPS: delta, DeltaPct: pct, DPSPerGold: dpg, Affordable: affordable,
		})
	}
	sort.SliceStable(out.DPS, func(i, j int) bool {
		if out.DPS[i].DPSPerGold != out.DPS[j].DPSPerGold {
			return out.DPS[i].DPSPerGold > out.DPS[j].DPSPerGold
		}
		return out.DPS[i].RuneKey < out.DPS[j].RuneKey
	})
	sort.SliceStable(out.GoldXP, func(i, j int) bool {
		if out.GoldXP[i].GainPerGold != out.GoldXP[j].GainPerGold {
			return out.GoldXP[i].GainPerGold > out.GoldXP[j].GainPerGold
		}
		return out.GoldXP[i].RuneKey < out.GoldXP[j].RuneKey
	})
	return out
}

// ===================== Relatório v2 + top-ações =====================

// HeroAdvice: conselho por herói ativo. Carrega AMBOS os modelos; a UI escolhe o
// que mostrar conforme o papel (auto ou override do usuário).
type HeroAdvice struct {
	HeroKey         int         `json:"hero_key"`
	RoleAuto        string      `json:"role_auto"`
	ModeledDPS      float64     `json:"modeled_dps"`
	DPSShare        float64     `json:"dps_share"`
	EffectiveHP     float64     `json:"effective_hp"`
	BestDPSUpgrade  *BagUpgrade `json:"best_dps_upgrade,omitempty"`
	BestSurvUpgrade *BagUpgrade `json:"best_surv_upgrade,omitempty"`
}

// TopAction: ação heterogênea pro bloco "O que fazer agora".
type TopAction struct {
	Kind      string  `json:"kind"` // "runa" | "equipar" | "tank" | "ouro"
	HeroKey   int     `json:"hero_key,omitempty"`
	RefKey    int     `json:"ref_key"` // rune_key ou item_key
	SlotIndex int     `json:"slot_index,omitempty"`
	DeltaDPS  float64 `json:"delta_dps,omitempty"`
	DeltaPct  float64 `json:"delta_pct,omitempty"` // tank: % HP ef. (est.) ; ouro: % ganho
}

type AdvisorReport struct {
	Calibrated  bool         `json:"calibrated"`
	Heroes      []HeroAdvice `json:"heroes"`
	Runes       RuneRanking  `json:"runes"`
	Composition Composition  `json:"composition"`
	TopActions  []TopAction  `json:"top_actions"`
}

func buildAdvisorReport(equip []HeroEquipment, runeLevels map[int]int, inv []StoredItem,
	calibrated bool, k, gold float64, stageLevel int) *AdvisorReport {

	setArmorStageLevel(stageLevel) // ajusta o limiar de armadura (HP efetivo) pro nível de fase
	runes := resolveRuneContributions(runeLevels)
	var team []HeroCombat
	for _, he := range equip {
		if he.Active {
			team = append(team, aggregateHeroCombat(he, runes))
		}
	}
	roles := detectRoles(team)
	totDPS := teamModeledDPS(team)

	adv := &AdvisorReport{Calibrated: calibrated && k > 0}
	for _, he := range equip {
		if !he.Active {
			continue
		}
		hc := aggregateHeroCombat(he, runes)
		dpsUp, survUp := bestBagUpgrades(he, runes, k, inv)
		share := 0.0
		if totDPS > 0 {
			share = hc.ModeledDPS / totDPS
		}
		adv.Heroes = append(adv.Heroes, HeroAdvice{
			HeroKey:         he.HeroKey,
			RoleAuto:        roles[he.HeroKey],
			ModeledDPS:      hc.ModeledDPS,
			DPSShare:        share,
			EffectiveHP:     hc.EffectiveHP,
			BestDPSUpgrade:  dpsUp,
			BestSurvUpgrade: survUp,
		})
	}
	adv.Runes = computeRuneRanking(equip, runeLevels, k, gold)
	adv.Composition = teamComposition(equip, runes)
	adv.TopActions = topActions(adv, roles)
	return adv
}

// topActions junta as melhores ações num bloco. Ações de DPS (runa + equipar carry)
// rankeadas por ΔDPS; depois acrescenta a melhor de tank e a melhor de ouro.
// Tudo determinístico (tiebreak por RefKey).
func topActions(adv *AdvisorReport, roles map[int]string) []TopAction {
	var dpsActs []TopAction
	for _, r := range adv.Runes.DPS {
		if r.DeltaDPS > 0 {
			dpsActs = append(dpsActs, TopAction{Kind: "runa", RefKey: r.RuneKey, DeltaDPS: r.DeltaDPS, DeltaPct: r.DeltaPct})
		}
	}
	for _, h := range adv.Heroes {
		if roles[h.HeroKey] != "tank" && h.BestDPSUpgrade != nil && h.BestDPSUpgrade.Delta > 0 {
			dpsActs = append(dpsActs, TopAction{
				Kind: "equipar", HeroKey: h.HeroKey, RefKey: h.BestDPSUpgrade.ItemKey,
				SlotIndex: h.BestDPSUpgrade.SlotIndex, DeltaDPS: h.BestDPSUpgrade.Delta, DeltaPct: h.BestDPSUpgrade.DeltaPct,
			})
		}
	}
	sort.SliceStable(dpsActs, func(i, j int) bool {
		if dpsActs[i].DeltaDPS != dpsActs[j].DeltaDPS {
			return dpsActs[i].DeltaDPS > dpsActs[j].DeltaDPS
		}
		return dpsActs[i].RefKey < dpsActs[j].RefKey
	})
	if len(dpsActs) > 4 {
		dpsActs = dpsActs[:4]
	}

	// melhor ação de tank (maior Δ% HP ef.)
	// A ordem de adv.Heroes (slice, não mapa) torna empates determinísticos.
	var bestTank *TopAction
	for _, h := range adv.Heroes {
		if roles[h.HeroKey] == "tank" && h.BestSurvUpgrade != nil && h.BestSurvUpgrade.DeltaPct > 0 {
			if bestTank == nil || h.BestSurvUpgrade.DeltaPct > bestTank.DeltaPct {
				bestTank = &TopAction{Kind: "tank", HeroKey: h.HeroKey, RefKey: h.BestSurvUpgrade.ItemKey,
					SlotIndex: h.BestSurvUpgrade.SlotIndex, DeltaPct: h.BestSurvUpgrade.DeltaPct}
			}
		}
	}
	if bestTank != nil {
		dpsActs = append(dpsActs, *bestTank)
	}

	// melhor ação de ouro (runa de ouro comprável agora, maior por-ouro)
	for _, r := range adv.Runes.GoldXP {
		if r.Affordable {
			dpsActs = append(dpsActs, TopAction{Kind: "ouro", RefKey: r.RuneKey, DeltaPct: r.GainPct})
			break
		}
	}
	return dpsActs
}
