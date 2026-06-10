package internal

func (s *StageHistoryStore) Update(stageKey int, timeSpent float64, goldGain float64, xpGain float64, useEMA bool, alphaConfig float64, numHeroes int, dropCount int, dropsByKey map[int]int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.history == nil {
		s.history = make(map[int]*StageStats)
	}
	stats, exists := s.history[stageKey]

	if !exists {
		stats = &StageStats{StageKey: stageKey}
		s.history[stageKey] = stats
	}

	stats.TotalRuns++

	stats.AccumulatedTime += timeSpent
	stats.AccumulatedGold += goldGain
	stats.AccumulatedXp += xpGain

	stats.AccumulatedItems += float64(dropCount)
	if len(dropsByKey) > 0 {
		if stats.ItemCatalog == nil {
			stats.ItemCatalog = make(map[int]int)
		}
		for k, c := range dropsByKey {
			stats.ItemCatalog[k] += c
		}
	}
	if stats.AccumulatedTime > 0 {
		stats.AvgItemsPerHour = stats.AccumulatedItems / (stats.AccumulatedTime / 3600.0)
	}

	if !useEMA {
		stats.AvgTimeSpent = stats.AccumulatedTime / float64(stats.TotalRuns)
		stats.AvgXpPerRun = stats.AccumulatedXp / float64(stats.TotalRuns)
		stats.AvgGoldPerRun = stats.AccumulatedGold / float64(stats.TotalRuns)

		totalTimeInHours := stats.AccumulatedTime / 3600.0
		if totalTimeInHours > 0 {
			stats.AvgGoldPerHour = stats.AccumulatedGold / totalTimeInHours
			if numHeroes > 0 {
				stats.AvgXpPerHour = (stats.AccumulatedXp / float64(numHeroes)) / totalTimeInHours
			} else {
				stats.AvgXpPerHour = stats.AccumulatedXp / totalTimeInHours
			}
		}
		return
	}
	alpha := alphaConfig
	if stats.TotalRuns == 1 {
		alpha = 1.0
	}

	stats.AvgTimeSpent = (timeSpent * alpha) + (stats.AvgTimeSpent * (1.0 - alpha))
	stats.AvgGoldPerRun = (goldGain * alpha) + (stats.AvgGoldPerRun * (1.0 - alpha))
	stats.AvgXpPerRun = (xpGain * alpha) + (stats.AvgXpPerRun * (1.0 - alpha))

	if timeSpent <= 0 {
		return
	}

	runGoldPerHour := (goldGain / timeSpent) * 3600
	stats.AvgGoldPerHour = (runGoldPerHour * alpha) + (stats.AvgGoldPerHour * (1.0 - alpha))

	if numHeroes > 0 {
		runXpPerHour := ((xpGain / float64(numHeroes)) / timeSpent) * 3600
		stats.AvgXpPerHour = (runXpPerHour * alpha) + (stats.AvgXpPerHour * (1.0 - alpha))
	}
}

// SetManualTime grava o tempo digitado pelo usuario na calibracao como uma SEMENTE,
func (s *StageHistoryStore) SetManualTime(stageKey int, timeSpent, goldGain, xpGain float64, numHeroes int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.history == nil {
		s.history = make(map[int]*StageStats)
	}
	stats, exists := s.history[stageKey]
	if !exists {
		stats = &StageStats{StageKey: stageKey}
		s.history[stageKey] = stats
	}

	stats.ManualTime = timeSpent

	if stats.TotalRuns == 0 && timeSpent > 0 {
		stats.AvgTimeSpent = timeSpent
		stats.AvgGoldPerRun = goldGain
		stats.AvgXpPerRun = xpGain
		stats.AvgGoldPerHour = (goldGain / timeSpent) * 3600.0
		if numHeroes > 0 {
			stats.AvgXpPerHour = (xpGain / float64(numHeroes) / timeSpent) * 3600.0
		} else {
			stats.AvgXpPerHour = (xpGain / timeSpent) * 3600.0
		}
	}
}

// AllStats devolve uma copia das estatisticas de todas as fases (snapshot sob lock).
func (s *StageHistoryStore) AllStats() []StageStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]StageStats, 0, len(s.history))
	for _, st := range s.history {
		out = append(out, *st)
	}
	return out
}

// MostRunStage devolve a fase com mais corridas registradas (e tempo medio > 0),
// usada como referencia para estimar o tempo de fases ainda sem historico proprio.
func (s *StageHistoryStore) MostRunStage() (key, runs int, avgTime float64) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for k, st := range s.history {
		if st.TotalRuns > runs && st.AvgTimeSpent > 0 {
			key, runs, avgTime = k, st.TotalRuns, st.AvgTimeSpent
		}
	}
	return key, runs, avgTime
}

func (s *StageHistoryStore) Get(stageKey int) (*StageStats, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats, exists := s.history[stageKey]

	return stats, exists
}

func (s *StageHistoryStore) GenerateReport() AnalyticsReport {
	s.mu.RLock()
	defer s.mu.RUnlock()

	bestGold := 0
	bestXp := 0
	maxGoldPerHour := 0.0
	maxXpPerHour := 0.0

	stagesCopy := make(map[int]*StageStats)
	for stageKey, stats := range s.history {
		statsCopy := *stats
		stagesCopy[stageKey] = &statsCopy

		if stats.AvgGoldPerHour > maxGoldPerHour {
			maxGoldPerHour = stats.AvgGoldPerHour
			bestGold = stageKey
		}
		if stats.AvgXpPerHour > maxXpPerHour {
			maxXpPerHour = stats.AvgXpPerHour
			bestXp = stageKey
		}
	}
	return AnalyticsReport{
		BestGoldStage: bestGold,
		BestXpStage:   bestXp,
		Stages:        stagesCopy,
	}
}

func (s *StageHistoryStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.history = make(map[int]*StageStats)
}

// Remove apaga as estatisticas de uma unica fase.
func (s *StageHistoryStore) Remove(stageKey int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.history, stageKey)
}
