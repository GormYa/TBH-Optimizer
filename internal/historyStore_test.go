package internal

import "testing"

// O tempo digitado na calibracao e uma SEMENTE: nao conta como corrida e e
// substituido (nao empilhado) a cada recalibracao.
func TestSetManualTimeIsSeedNotRun(t *testing.T) {
	var s StageHistoryStore
	s.SetManualTime(1101, 45, 14, 16, 1)
	if st, _ := s.Get(1101); st.TotalRuns != 0 {
		t.Fatalf("semente nao deveria contar como corrida, TotalRuns=%d", st.TotalRuns)
	}
	if st, _ := s.Get(1101); st.AvgTimeSpent != 45 {
		t.Fatalf("media deveria ser a semente 45, veio %.2f", st.AvgTimeSpent)
	}

	s.SetManualTime(1101, 50, 14, 16, 1)
	if st, _ := s.Get(1101); st.TotalRuns != 0 || st.AvgTimeSpent != 50 {
		t.Fatalf("recalibrar deveria substituir: runs=%d avg=%.2f", st.TotalRuns, st.AvgTimeSpent)
	}
}

// Medicao legada (sem carimbo de nivel) e curada pela proxima corrida real: snap
// alpha=1 substitui o valor podre de uma vez e carimba o nivel atual.
func TestLegacyMeasurementHealsOnNextRun(t *testing.T) {
	var s StageHistoryStore
	// Simula legado: corridas antigas sem nivel (heroLevel=0 -> MeasuredHeroLevel fica 0).
	s.Update(2207, 100, 1000, 5000, true, 0.2, 1, 0, 0, nil)
	s.Update(2207, 100, 1000, 5000, true, 0.2, 1, 0, 0, nil)
	if st, _ := s.Get(2207); st.MeasuredHeroLevel != 0 {
		t.Fatalf("medicao legada deveria ficar sem carimbo, veio %d", st.MeasuredHeroLevel)
	}

	// Corrida real nova, agora com nivel: deve dar snap (substituir, nao misturar).
	s.Update(2207, 50, 9999, 99999, true, 0.2, 1, 44, 0, nil)
	st, _ := s.Get(2207)
	if st.MeasuredHeroLevel != 44 {
		t.Fatalf("deveria carimbar o nivel 44, veio %d", st.MeasuredHeroLevel)
	}
	if st.AvgTimeSpent != 50 || st.AvgXpPerRun != 99999 {
		t.Fatalf("a corrida que cura deveria substituir (snap): avg=%.0f xp=%.0f", st.AvgTimeSpent, st.AvgXpPerRun)
	}
}

// A primeira corrida real SUBSTITUI a semente (alpha=1): o tempo digitado e so um
// placeholder ate o jogo dar a medicao de verdade, nunca prende a media.
func TestManualSeedReplacedByFirstRealRun(t *testing.T) {
	var s StageHistoryStore
	s.SetManualTime(1101, 45, 14, 16, 1)

	s.Update(1101, 50, 800, 900, true, 0.2, 1, 30, 0, nil)
	st, _ := s.Get(1101)
	if st.TotalRuns != 1 {
		t.Fatalf("a corrida real deveria contar: TotalRuns=%d", st.TotalRuns)
	}
	if st.AvgTimeSpent != 50 {
		t.Fatalf("a 1a corrida real deveria virar 50s (snap), veio %.3f", st.AvgTimeSpent)
	}
	if st.AvgGoldPerRun != 800 || st.AvgXpPerRun != 900 {
		t.Fatalf("ouro/xp deveriam ser os da corrida (800/900), veio %.0f/%.0f", st.AvgGoldPerRun, st.AvgXpPerRun)
	}
}

// Sem semente, a primeira corrida real define a media (alpha=1, comportamento antigo).
func TestFirstRealRunWithoutSeedReplaces(t *testing.T) {
	var s StageHistoryStore
	s.Update(1101, 50, 14, 16, true, 0.2, 1, 30, 0, nil)
	if st, _ := s.Get(1101); st.AvgTimeSpent != 50 {
		t.Fatalf("sem semente, 1a corrida deveria definir 50, veio %.2f", st.AvgTimeSpent)
	}
}
