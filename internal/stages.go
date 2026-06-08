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
	var goldMultSum, xpMultSum float64
	var multCount int
	for key, stats := range ctrl.StageHistory.history {
		measured := stats.TotalRuns > 0
		seeded := stats.ManualTime > 0
		if (!measured && !seeded) || stats.AvgTimeSpent <= 0 {
			continue
		}
		info, ok := ctrl.FarmStages[key]
		if !ok || info.TotalHP <= 0 {
			continue
		}
		points = append(points, timePoint{HP: info.TotalHP, Waves: float64(info.Waves), Time: stats.AvgTimeSpent})
		if measured && info.ExpectedGold > 0 && info.ExpectedEXP > 0 {
			goldMultSum += stats.AvgGoldPerRun / info.ExpectedGold
			xpMultSum += stats.AvgXpPerRun / info.ExpectedEXP
			multCount++
		}
	}

	a, b, calibrated := fitTimeModel(points)

	goldMultiplier := 1.0
	xpMultiplier := 1.0
	if multCount > 0 {
		goldMultiplier = goldMultSum / float64(multCount)
		xpMultiplier = xpMultSum / float64(multCount)
	}

	numHeroes := len(ctrl.HeroStates)
	if numHeroes < 1 {
		numHeroes = 1
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

			estTime := estimateTime(a, b, info.TotalHP, float64(info.Waves))
			estGoldPerRun := info.ExpectedGold * goldMultiplier
			estXpPerRun := info.ExpectedEXP * xpMultiplier
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
	}
}
