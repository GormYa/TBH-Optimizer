package internal

import (
	"path/filepath"
	"testing"
)

// Salvar e recarregar deve preservar as estatisticas, inclusive os campos
// acumulados (que sao json:"-" no StageStats) necessarios para continuar as
// medias cumulativas apos reiniciar o app.
func TestStageHistoryPersistRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stage_history.json")

	a := &StageHistoryStore{}
	a.Update(1204, 135, 3700, 9000, false, 0.2, 3, 40, 2, map[int]int{522031: 2})
	a.Update(1204, 140, 3800, 9100, false, 0.2, 3, 40, 1, map[int]int{410003: 1})

	if err := a.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	b := &StageHistoryStore{}
	if err := b.Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}

	sa, _ := a.Get(1204)
	sb, ok := b.Get(1204)
	if !ok {
		t.Fatal("fase 1204 nao foi recarregada")
	}
	if sb.TotalRuns != sa.TotalRuns {
		t.Errorf("TotalRuns: got %d want %d", sb.TotalRuns, sa.TotalRuns)
	}
	if sb.AvgGoldPerHour != sa.AvgGoldPerHour {
		t.Errorf("AvgGoldPerHour: got %v want %v", sb.AvgGoldPerHour, sa.AvgGoldPerHour)
	}
	if sb.AccumulatedGold != sa.AccumulatedGold {
		t.Errorf("AccumulatedGold: got %v want %v (acumulado nao persistido)", sb.AccumulatedGold, sa.AccumulatedGold)
	}
	if sb.AccumulatedTime != sa.AccumulatedTime {
		t.Errorf("AccumulatedTime: got %v want %v", sb.AccumulatedTime, sa.AccumulatedTime)
	}
	// O carimbo de nivel e o que separa fase medida de fase "antiga". Perde-lo no
	// restart marcava TODAS as fases como antigas e zerava as amostras do
	// multiplicador de XP -> xp/h reprojetado sem base depois de cada auto-update.
	if sb.MeasuredHeroLevel != 40 {
		t.Errorf("MeasuredHeroLevel: got %d want 40 (carimbo perdido no round-trip)", sb.MeasuredHeroLevel)
	}
}

// Arquivo inexistente (primeiro boot) nao e erro: comeca com historico vazio.
func TestStageHistoryLoadMissingFile(t *testing.T) {
	s := &StageHistoryStore{}
	if err := s.Load(filepath.Join(t.TempDir(), "nao_existe.json")); err != nil {
		t.Errorf("arquivo ausente nao deveria ser erro: %v", err)
	}
}
