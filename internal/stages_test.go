package internal

import (
	"math"
	"testing"
)

// O multiplicador de XP deve sair so das medicoes carimbadas (nivel conhecido),
// usando a retencao DA EPOCA. Uma fase antiga (MeasuredHeroLevel==0), medida com
// retencao alta, nao pode entrar — senao infla o multiplicador e, por tabela, todas
// as estimativas (foi o bug que jogava fases antigas pro topo do ranking).
func TestYieldMultipliersIgnoresStaleXP(t *testing.T) {
	farm := map[int]FarmStageInfo{
		// carimbada, medida no proprio nivel (retencao = 1.0): multiplicador real = 3.0
		3050: {Key: 3050, Level: 50, ExpectedGold: 100, ExpectedEXP: 1000},
		// antiga (sem carimbo): se entrasse, daria 5.0 e puxaria a mediana pra cima
		2040: {Key: 2040, Level: 40, ExpectedGold: 100, ExpectedEXP: 1000},
	}
	stats := []StageStats{
		{StageKey: 3050, TotalRuns: 6, MeasuredHeroLevel: 50, AvgGoldPerRun: 200, AvgXpPerRun: 3000},
		{StageKey: 2040, TotalRuns: 6, MeasuredHeroLevel: 0, AvgGoldPerRun: 400, AvgXpPerRun: 5000},
	}

	gold, xp := yieldMultipliers(stats, farm)

	if math.Abs(xp-3.0) > 1e-9 {
		t.Fatalf("multiplicador de XP deveria vir so da fase carimbada (3.0), veio %.4f", xp)
	}
	// Ouro nao depende de nivel: usa as duas -> mediana de [2.0, 4.0] = 3.0
	if math.Abs(gold-3.0) > 1e-9 {
		t.Fatalf("multiplicador de ouro deveria usar as duas medicoes (mediana 3.0), veio %.4f", gold)
	}
}
