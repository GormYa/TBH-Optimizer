package internal

import "testing"

// A curva real de XP (LevelInfoData) substitui a formula 15*L^4. Sem ela, o XP no
// level-up estourava (ex.: L40 dava 38.4M em vez dos 25.77M reais).
func TestLevelCurveOverridesFormula(t *testing.T) {
	LoadLevelCurve([]byte(`{"40":25772024,"64":253816273}`))
	if got := GetXPRequiredForLevel(40); got != 25772024 {
		t.Fatalf("L40 = %.0f, esperava a curva real 25772024 (nao a formula 15*L^4=38.4M)", got)
	}
	// nivel ausente na curva -> fallback pra formula
	if got := GetXPRequiredForLevel(2); got != 15.0*2*2*2*2 {
		t.Fatalf("L2 ausente deveria cair no fallback 15*L^4=240, veio %.0f", got)
	}
}
