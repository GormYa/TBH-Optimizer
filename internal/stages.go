package internal

import (
	"encoding/json"
)

func LoadFarmStages(data []byte) (map[int]FarmStageInfo, error) {
	var stages []FarmStageInfo
	if err := json.Unmarshal(data, &stages); err != nil {
		return nil, err
	}
	stagesMap := make(map[int]FarmStageInfo)
	for _, s := range stages {
		stagesMap[s.Key] = s
	}
	return stagesMap, nil
}

func yieldMultipliers(stats []StageStats, farm map[int]FarmStageInfo) (gold, xp float64) {
	var gm []float64
	for _, st := range stats {
		if st.TotalRuns < 3 {
			continue
		}
		info, ok := farm[st.StageKey]
		if !ok {
			continue
		}
		if info.ExpectedGold > 0 {
			gm = append(gm, st.AvgGoldPerRun/info.ExpectedGold)
		}
	}
	gold = 1.0
	if len(gm) > 0 {
		gold = median(gm)
	}
	xp, _ = xpMultiplierMeasured(stats, farm)
	return
}

func xpMultiplierMeasured(stats []StageStats, farm map[int]FarmStageInfo) (float64, bool) {
	var xm []float64
	for _, st := range stats {
		if st.TotalRuns < 3 || st.MeasuredHeroLevel <= 0 {
			continue
		}
		info, ok := farm[st.StageKey]
		if !ok || info.ExpectedEXP <= 0 {
			continue
		}
		retThen := expRetention(info.Level, st.MeasuredHeroLevel)
		if retThen > 0 {
			xm = append(xm, st.AvgXpPerRun/(info.ExpectedEXP*retThen))
		}
	}
	if len(xm) == 0 {
		return 1.0, false
	}
	return median(xm), true
}

func (ctrl *Control) GenerateReportWithEstimates() AnalyticsReport {
	ctrl.StageHistory.mu.RLock()
	defer ctrl.StageHistory.mu.RUnlock()

	stagesReport := make(map[int]*StageStats)
	for stageKey, stats := range ctrl.StageHistory.history {
		statsCopy := *stats
		stagesReport[stageKey] = &statsCopy
	}

	// --- Calibracao por regressao sobre TODOS os mapas medidos ---
	// tempo = a*HP + b*ondas, ajustado por minimos quadrados. Usar todos os pontos
	// (em vez de 2 ancoras) evita que a 1-1, de HP desprezivel, distorca o DPS e
	// gere absurdos (ex.: estimar uma fase de menos HP como mais demorada).
	var points []timePoint
	var measured []StageStats
	for key, stats := range ctrl.StageHistory.history {
		isMeasured := stats.TotalRuns > 0
		seeded := stats.ManualTime > 0
		if (!isMeasured && !seeded) || stats.AvgTimeSpent <= 0 {
			continue
		}
		info, ok := ctrl.FarmStages[key]
		if !ok || info.TotalHP <= 0 {
			continue
		}
		points = append(points, timePoint{HP: info.TotalHP, Waves: float64(info.Waves), Time: stats.AvgTimeSpent})
		measured = append(measured, *stats)
	}

	dps, overhead, calibrated := effectiveDPS(points)

	goldMultiplier, xpMultiplier := yieldMultipliers(measured, ctrl.FarmStages)

	numHeroes := ctrl.numActiveHeroes()

	for key, st := range stagesReport {
		info, ok := ctrl.FarmStages[key]
		if !ok || st.TotalRuns == 0 {
			continue
		}
		retNow := expRetention(info.Level, ctrl.HeroLevel)
		if st.MeasuredHeroLevel == 0 {
			st.Stale = true
			if calibrated {
				estTime := estimateTimeDPS(dps, overhead, info.TotalHP, float64(info.Waves))
				if estTime > 0 {
					st.AvgTimeSpent = estTime
					st.AvgGoldPerRun = info.ExpectedGold * goldMultiplier
					st.AvgGoldPerHour = (st.AvgGoldPerRun / estTime) * 3600.0
					if numHeroes > 0 {
						st.AvgXpPerRun = info.ExpectedEXP * xpMultiplier * retNow
						st.AvgXpPerHour = (st.AvgXpPerRun / float64(numHeroes) / estTime) * 3600.0
					}
				}
			}
			continue
		}
		retThen := expRetention(info.Level, st.MeasuredHeroLevel)
		if retThen > 0 && retNow != retThen {
			st.AvgXpPerHour *= retNow / retThen
			st.AvgXpPerRun *= retNow / retThen
		}
	}

	bestGoldStage := 0
	bestXpStage := 0

	if calibrated && ctrl.FarmStages != nil {
		lastRegistered := 0
		for stageKey, st := range ctrl.StageHistory.history {
			if (st.TotalRuns > 0 || st.ManualTime > 0) && stageKey > lastRegistered {
				lastRegistered = stageKey
			}
		}
		if lastRegistered <= 0 {
			lastRegistered = 1101
		}

		maxGoldPerHour := 0.0
		maxXpPerHour := 0.0

		for key, info := range ctrl.FarmStages {
			if key > lastRegistered {
				continue
			}
			if info.Waves <= 0 {
				continue
			}

			estTime := estimateTimeDPS(dps, overhead, info.TotalHP, float64(info.Waves))
			estGoldPerRun := info.ExpectedGold * goldMultiplier
			estXpPerRun := info.ExpectedEXP * xpMultiplier * expRetention(info.Level, ctrl.HeroLevel)
			estGoldPerHour := (estGoldPerRun / estTime) * 3600.0
			estXpPerHour := (estXpPerRun / float64(numHeroes) / estTime) * 3600.0

			if _, exists := stagesReport[key]; !exists {
				stagesReport[key] = &StageStats{
					StageKey:       key,
					TotalRuns:      0,
					AvgTimeSpent:   estTime,
					AvgGoldPerRun:  estGoldPerRun,
					AvgGoldPerHour: estGoldPerHour,
					AvgXpPerRun:    estXpPerRun,
					AvgXpPerHour:   estXpPerHour,
				}
			}

			statsInReport := stagesReport[key]
			if statsInReport.AvgGoldPerHour > maxGoldPerHour {
				maxGoldPerHour = statsInReport.AvgGoldPerHour
				bestGoldStage = key
			}
			if statsInReport.AvgXpPerHour > maxXpPerHour {
				maxXpPerHour = statsInReport.AvgXpPerHour
				bestXpStage = key
			}
		}
	} else {
		maxGold := 0.0
		maxXp := 0.0
		for key, stats := range ctrl.StageHistory.history {
			if stats.AvgGoldPerHour > maxGold {
				maxGold = stats.AvgGoldPerHour
				bestGoldStage = key
			}
			if stats.AvgXpPerHour > maxXp {
				maxXp = stats.AvgXpPerHour
				bestXpStage = key
			}
		}
	}

	for stageKey, stats := range stagesReport {
		if ctrl.FarmStages != nil {
			if info, exists := ctrl.FarmStages[stageKey]; exists {
				stats.ExpRetained = expRetention(info.Level, ctrl.HeroLevel)
				if stats.AvgTimeSpent > 0 {
					stats.RawGoldPerHour = (info.ExpectedGold / stats.AvgTimeSpent) * 3600.0
					stats.RawXpPerHour = (info.ExpectedEXP / stats.AvgTimeSpent) * 3600.0
				}
			}
		}
		if stats.RawGoldPerHour <= 0 {
			stats.RawGoldPerHour = stats.AvgGoldPerHour
		}
		if stats.RawXpPerHour <= 0 {
			stats.RawXpPerHour = stats.AvgXpPerHour
		}
	}

	return AnalyticsReport{
		BestGoldStage: bestGoldStage,
		BestXpStage:   bestXpStage,
		Stages:        stagesReport,
		Calibrated:    calibrated,
		EffectiveDPS:  dps,
		HeroLevel:     ctrl.HeroLevel,
		ActiveHeroes:  ctrl.ActiveHeroes,
		Gold:          ctrl.Gold,
		RuneLevels:    ctrl.RuneLevels,
		OwnedPets:     ctrl.OwnedPets,
		ActivePet:     ctrl.ActivePet,
		ChestTracker:  GetChestTrackerStatus(ctrl),
	}
}
