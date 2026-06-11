package internal

import (
	"encoding/json"
	"os"
)

// HistoryFilePath e o snapshot em disco que sobrevive ao restart do app.
const HistoryFilePath = "stage_history.json"

// persistedStageStats espelha StageStats incluindo os campos acumulados (que em
// StageStats sao json:"-" para nao vazar na API). Eles sao necessarios para
// continuar as medias cumulativas depois de reiniciar. RawGoldPerHour/RawXpPerHour
// nao entram: sao recalculados em GenerateReportWithEstimates.
type persistedStageStats struct {
	StageKey          int         `json:"stage_key"`
	TotalRuns         int         `json:"total_runs"`
	ManualTime        float64     `json:"manual_time"`
	AvgTimeSpent      float64     `json:"avg_time_spent"`
	AvgGoldPerRun     float64     `json:"avg_gold_per_run"`
	AvgGoldPerHour    float64     `json:"avg_gold_per_hour"`
	AvgXpPerRun       float64     `json:"avg_xp_per_run"`
	AvgXpPerHour      float64     `json:"avg_xp_per_hour"`
	AvgItemsPerHour   float64     `json:"avg_items_per_hour"`
	MeasuredHeroLevel int         `json:"measured_hero_level"`
	ItemCatalog       map[int]int `json:"item_catalog"`
	AccumulatedGold   float64     `json:"accumulated_gold"`
	AccumulatedXp     float64     `json:"accumulated_xp"`
	AccumulatedTime   float64     `json:"accumulated_time"`
	AccumulatedItems  float64     `json:"accumulated_items"`
}

// Save grava o historico em disco de forma atomica (temp + rename), para nunca
// deixar o arquivo corrompido se o processo morrer no meio da escrita.
func (s *StageHistoryStore) Save(path string) error {
	s.mu.RLock()
	snapshot := make([]persistedStageStats, 0, len(s.history))
	for _, st := range s.history {
		snapshot = append(snapshot, persistedStageStats{
			StageKey:          st.StageKey,
			TotalRuns:         st.TotalRuns,
			ManualTime:        st.ManualTime,
			AvgTimeSpent:      st.AvgTimeSpent,
			AvgGoldPerRun:     st.AvgGoldPerRun,
			AvgGoldPerHour:    st.AvgGoldPerHour,
			AvgXpPerRun:       st.AvgXpPerRun,
			AvgXpPerHour:      st.AvgXpPerHour,
			AvgItemsPerHour:   st.AvgItemsPerHour,
			MeasuredHeroLevel: st.MeasuredHeroLevel,
			ItemCatalog:       st.ItemCatalog,
			AccumulatedGold:   st.AccumulatedGold,
			AccumulatedXp:     st.AccumulatedXp,
			AccumulatedTime:   st.AccumulatedTime,
			AccumulatedItems:  st.AccumulatedItems,
		})
	}
	s.mu.RUnlock()

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Load restaura o historico do disco. Arquivo inexistente (primeiro boot) nao e
// erro: o app simplesmente comeca com o historico vazio.
func (s *StageHistoryStore) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var snapshot []persistedStageStats
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = make(map[int]*StageStats, len(snapshot))
	for _, p := range snapshot {
		s.history[p.StageKey] = &StageStats{
			StageKey:          p.StageKey,
			TotalRuns:         p.TotalRuns,
			ManualTime:        p.ManualTime,
			AvgTimeSpent:      p.AvgTimeSpent,
			AvgGoldPerRun:     p.AvgGoldPerRun,
			AvgGoldPerHour:    p.AvgGoldPerHour,
			AvgXpPerRun:       p.AvgXpPerRun,
			AvgXpPerHour:      p.AvgXpPerHour,
			AvgItemsPerHour:   p.AvgItemsPerHour,
			MeasuredHeroLevel: p.MeasuredHeroLevel,
			ItemCatalog:       p.ItemCatalog,
			AccumulatedGold:   p.AccumulatedGold,
			AccumulatedXp:     p.AccumulatedXp,
			AccumulatedTime:   p.AccumulatedTime,
			AccumulatedItems:  p.AccumulatedItems,
		}
	}
	return nil
}
