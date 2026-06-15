package internal

var runeLevelCost = make(map[int]map[int]float64)

func InitializeRuneCosts() {
	for k, info := range runeCatalog {
		m := make(map[int]float64, len(info.Levels))
		for _, lv := range info.Levels {
			m[lv.Level] = lv.Cost
		}
		runeLevelCost[k] = m
	}
}

func (ctrl *Control) runeSpendSince(save *InnerSaveData) float64 {
	now := runeLevels(save)
	spend := 0.0
	for key, lvl := range now {
		before := ctrl.LastRuneLevels[key]
		if lvl <= before {
			continue
		}
		costs := runeLevelCost[key]
		for l := before + 1; l <= lvl; l++ {
			spend += costs[l]
		}
	}
	return spend
}
