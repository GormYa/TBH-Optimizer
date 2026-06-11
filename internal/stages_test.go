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

// As COLUNAS de uma fase antiga (stale) precisam contar a mesma historia: tempo,
// por-corrida e por-hora todos reprojetados — nao o por-corrida da medicao velha ao
// lado de um por-hora recalculado. E numa fase carimbada cujo nivel mudou, o
// xp/corrida reescala junto com o xp/hora.
func TestReportReprojectsStaleAndRescalesStamped(t *testing.T) {
	const sA, sB, sLegacy = 2206, 2208, 2050
	ctrl := Control{
		UseEMA: true, EMAAlpha: 0.2,
		HeroLevel:       55,
		ActiveHeroCount: 1,
		FarmStages: map[int]FarmStageInfo{
			// carimbada medida no nivel 50 (ret 1.0), hoje no 55 (ret 0.8367): reescala
			sA: {Key: sA, TotalHP: 100000, Waves: 17, Level: 49, ExpectedGold: 4000, ExpectedEXP: 60000},
			// carimbada estavel (diff <= 2 nos dois niveis): ancora o multiplicador em 9
			sB: {Key: sB, TotalHP: 500000, Waves: 20, Level: 53, ExpectedGold: 5000, ExpectedEXP: 70000},
			// antiga (sem carimbo): reprojetada inteira
			sLegacy: {Key: sLegacy, TotalHP: 120000, Waves: 23, Level: 50, ExpectedGold: 4500, ExpectedEXP: 65000},
		},
	}
	for range 3 {
		// sA medida no 50: ret(49,50)=1.0 -> multiplicador 540000/60000 = 9
		ctrl.StageHistory.Update(sA, 148, 10000, 540000, true, 0.2, 1, 50, 0, nil)
		// sB medida no 55: ret(53,55)=1.0 -> multiplicador 630000/70000 = 9
		ctrl.StageHistory.Update(sB, 262, 12000, 630000, true, 0.2, 1, 55, 0, nil)
	}
	// fase antiga: 2 corridas legadas (sem nivel), media velha de 237s / 5.2M xp
	for range 2 {
		ctrl.StageHistory.Update(sLegacy, 237, 8000, 5215968, true, 0.2, 1, 0, 0, nil)
	}

	rep := ctrl.GenerateReportWithEstimates()
	if !rep.Calibrated {
		t.Fatal("com 3 fases medidas a regressão deveria calibrar")
	}

	leg := rep.Stages[sLegacy]
	if !leg.Stale {
		t.Fatal("fase sem carimbo deveria sair como stale")
	}
	// reprojecao: xp/corrida = 65000 x 9 x ret(50,55)=0.90816... ~= 531.275 (nao os 5.2M velhos)
	wantXp := 65000 * 9.0 * expRetention(50, 55)
	if math.Abs(leg.AvgXpPerRun-wantXp) > wantXp*0.01 {
		t.Fatalf("xp/corrida da fase antiga deveria ser reprojetado (~%.0f), veio %.0f", wantXp, leg.AvgXpPerRun)
	}
	if leg.AvgTimeSpent == 237 {
		t.Fatal("tempo da fase antiga deveria ser o estimado, não a medição velha")
	}
	// coerencia interna da linha: por-hora == por-corrida / tempo
	wantHr := leg.AvgXpPerRun / leg.AvgTimeSpent * 3600.0
	if math.Abs(leg.AvgXpPerHour-wantHr) > wantHr*0.001 {
		t.Fatalf("xp/h (%.0f) não bate com xp/corrida ÷ tempo (%.0f)", leg.AvgXpPerHour, wantHr)
	}
	// fase carimbada com nivel mudado: por-corrida reescala junto com o por-hora
	a := rep.Stages[sA]
	ratio := expRetention(49, 55) / expRetention(49, 50) // 0.8367 / 1.0
	wantA := 540000 * ratio
	if math.Abs(a.AvgXpPerRun-wantA) > wantA*0.01 {
		t.Fatalf("xp/corrida da fase carimbada deveria reescalar pra retenção atual (~%.0f), veio %.0f", wantA, a.AvgXpPerRun)
	}
}
