package internal

import "sort"

type StageOutlook struct {
	StageKey   int     `json:"stage_key"`
	Label      string  `json:"label"`
	Difficulty string  `json:"difficulty"`
	Level      int     `json:"level"`
	EstTime    float64 `json:"est_time"`
	Verdict    string  `json:"verdict"`
}

type HeroDamage struct {
	HeroKey int     `json:"hero_key"`
	Weight  float64 `json:"weight"`
}

type StalledGear struct {
	HeroKey   int `json:"hero_key"`
	SlotIndex int `json:"slot_index"`
	FromItem  int `json:"from_item"`
	ToItem    int `json:"to_item"`
}

type AdvisorReport struct {
	Calibrated   bool           `json:"calibrated"`
	CurrentStage int            `json:"current_stage"`
	NextWall     *StageOutlook  `json:"next_wall,omitempty"`
	Outlook      []StageOutlook `json:"outlook"`
	Heroes       []HeroDamage   `json:"heroes"`
	StalledGear  []StalledGear  `json:"stalled_gear"`
}

const (
	outlookViableSecs = 30.0
	outlookSlowSecs   = 120.0
	outlookCount      = 8
)

func stageVerdict(estTime float64, calibrated bool) string {
	if !calibrated || estTime <= 0 {
		return "parede"
	}
	switch {
	case estTime <= outlookViableSecs:
		return "viavel"
	case estTime <= outlookSlowSecs:
		return "lento"
	default:
		return "parede"
	}
}

func buildAdvisorReport(ctrl *Control, dps, overhead float64, calibrated bool) *AdvisorReport {
	rep := &AdvisorReport{Calibrated: calibrated, CurrentStage: ctrl.MaxCompletedStage}

	if ctrl.FarmStages != nil {
		var keys []int
		for k := range ctrl.FarmStages {
			if k > ctrl.MaxCompletedStage {
				keys = append(keys, k)
			}
		}
		sort.Ints(keys)
		if len(keys) > outlookCount {
			keys = keys[:outlookCount]
		}
		for _, k := range keys {
			info := ctrl.FarmStages[k]
			if info.Waves <= 0 {
				continue
			}
			est := 0.0
			if calibrated {
				est = estimateTimeDPS(dps, overhead, info.TotalHP, float64(info.Waves))
			}
			o := StageOutlook{StageKey: k, Label: info.Label, Difficulty: info.Difficulty, Level: info.Level, EstTime: est, Verdict: stageVerdict(est, calibrated)}
			rep.Outlook = append(rep.Outlook, o)
			if rep.NextWall == nil && o.Verdict != "viavel" {
				w := o
				rep.NextWall = &w
			}
		}
	}

	runes := resolveRuneContributions(ctrl.RuneLevels)
	for _, he := range ctrl.HeroEquipment {
		if !he.Active {
			continue
		}
		hs := aggregateHeroStats(he, runes)
		rep.Heroes = append(rep.Heroes, HeroDamage{HeroKey: he.HeroKey, Weight: damageWeight(hs)})
	}
	sort.SliceStable(rep.Heroes, func(i, j int) bool {
		if rep.Heroes[i].Weight != rep.Heroes[j].Weight {
			return rep.Heroes[i].Weight > rep.Heroes[j].Weight
		}
		return rep.Heroes[i].HeroKey < rep.Heroes[j].HeroKey
	})

	rep.StalledGear = buildStalledGear(ctrl, runes)
	return rep
}

func buildStalledGear(ctrl *Control, runes []statContribution) []StalledGear {
	var out []StalledGear
	for _, he := range ctrl.HeroEquipment {
		if !he.Active {
			continue
		}
		base := heroBaseStats[he.HeroKey]
		slotContribs := make([][]statContribution, len(he.Slots))
		for i, s := range he.Slots {
			slotContribs[i] = itemContributions(s.ItemKey, s.Enchants)
		}
		for si, s := range he.Slots {
			if s.ItemKey == 0 {
				continue
			}
			gear := itemMeta[s.ItemKey].Gear
			if gear == "" {
				continue
			}
			ctx := append([]statContribution{}, runes...)
			for sj := range he.Slots {
				if sj != si {
					ctx = append(ctx, slotContribs[sj]...)
				}
			}
			curVec := fieldValues(base, concat(ctx, slotContribs[si]))
			bestItem, bestFound := 0, false
			var bestVec [fIgnore]float64
			for _, ci := range ctrl.Inventory {
				if ci.ItemKey == 0 || itemMeta[ci.ItemKey].Gear != gear {
					continue
				}
				candVec := fieldValues(base, concat(ctx, itemContributions(ci.ItemKey, ci.EnchantData)))
				if !statsDominate(candVec, curVec) {
					continue
				}
				if bestFound && !statsDominate(candVec, bestVec) {
					continue // entre candidatos válidos, fica o mais forte
				}
				bestItem, bestVec, bestFound = ci.ItemKey, candVec, true
			}
			if bestFound {
				out = append(out, StalledGear{HeroKey: he.HeroKey, SlotIndex: si, FromItem: s.ItemKey, ToItem: bestItem})
			}
		}
	}
	return out
}

func concat(a, b []statContribution) []statContribution {
	out := make([]statContribution, 0, len(a)+len(b))
	out = append(out, a...)
	out = append(out, b...)
	return out
}
