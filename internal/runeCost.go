package internal

import (
	"embed"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
)

var runeLevelCost = make(map[int]map[int]float64)

// InitializeRuneCosts carrega a tabela de custo das runas (gamedata atualizável primeiro,
// embed como fallback). Chamado uma vez no startup do monitor.
func InitializeRuneCosts(webFiles embed.FS, gameDataDir string) {
	var data []byte
	var err error
	if data, err = os.ReadFile(filepath.Join(gameDataDir, "runes.json")); err != nil {
		data, err = webFiles.ReadFile("web/runes.json")
		if err != nil {
			return
		}
	}
	var doc map[string]struct {
		Levels []struct {
			Level int     `json:"level"`
			Cost  float64 `json:"cost"`
		} `json:"levels"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return
	}
	for ks, r := range doc {
		k, err := strconv.Atoi(ks)
		if err != nil {
			continue
		}
		m := make(map[int]float64, len(r.Levels))
		for _, lv := range r.Levels {
			m[lv.Level] = lv.Cost
		}
		runeLevelCost[k] = m
	}
}

// runeSpendSince soma o custo (ouro) dos níveis de runa que subiram entre o snapshot
// LastRuneLevels (início da janela de medição) e o save atual — ou seja, quanto ouro
// foi gasto com upgrade de runa durante o ciclo. Runas que não subiram não contam.
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
