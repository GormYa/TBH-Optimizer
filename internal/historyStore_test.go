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

// A primeira corrida real MISTURA com a semente (nao a descarta com alpha=1).
func TestManualSeedBlendsWithRealRun(t *testing.T) {
	var s StageHistoryStore
	s.SetManualTime(1101, 45, 14, 16, 1)

	s.Update(1101, 50, 14, 16, true, 0.2, 1, 0, nil)
	st, _ := s.Get(1101)
	if st.TotalRuns != 1 {
		t.Fatalf("a corrida real deveria contar: TotalRuns=%d", st.TotalRuns)
	}
	if d := st.AvgTimeSpent - 46.0; d > 0.001 || d < -0.001 {
		t.Fatalf("esperava blend ~46s, veio %.3f", st.AvgTimeSpent)
	}
}

// Sem semente, a primeira corrida real define a media (alpha=1, comportamento antigo).
func TestFirstRealRunWithoutSeedReplaces(t *testing.T) {
	var s StageHistoryStore
	s.Update(1101, 50, 14, 16, true, 0.2, 1, 0, nil)
	if st, _ := s.Get(1101); st.AvgTimeSpent != 50 {
		t.Fatalf("sem semente, 1a corrida deveria definir 50, veio %.2f", st.AvgTimeSpent)
	}
}
