package internal

import (
	"math"
	"testing"
)

// effectiveDPS ajusta tempo = HP/DPS + b*ondas por minimos quadrados. Com pontos
// consistentes com DPS=10000 HP/s e b=5s/onda, recupera os dois.
func TestEffectiveDPSRecupera(t *testing.T) {
	dps0, b0 := 10000.0, 5.0
	pts := []timePoint{
		{HP: 560, Waves: 10, Time: 560/dps0 + b0*10},
		{HP: 680580, Waves: 15, Time: 680580/dps0 + b0*15},
		{HP: 891585, Waves: 15, Time: 891585/dps0 + b0*15},
	}
	dps, b, ok := effectiveDPS(pts)
	if !ok {
		t.Fatal("esperava ok=true")
	}
	if math.Abs(dps-dps0) > 1 {
		t.Errorf("dps = %v, quero ~%v", dps, dps0)
	}
	if math.Abs(b-b0) > 1e-6 {
		t.Errorf("custo por onda = %v, quero ~%v", b, b0)
	}
}

func TestEffectiveDPSInsuficiente(t *testing.T) {
	if _, _, ok := effectiveDPS([]timePoint{{HP: 560, Waves: 10, Time: 46}}); ok {
		t.Error("1 ponto nao estima DPS")
	}
	// HP identico em todos -> sistema singular
	pts := []timePoint{{HP: 1000, Waves: 10, Time: 50}, {HP: 1000, Waves: 12, Time: 60}}
	if _, _, ok := effectiveDPS(pts); ok {
		t.Error("HP constante deve falhar (singular)")
	}
}

// Monotonicidade: mesma contagem de ondas, mais HP -> mais tempo; piso de 1s.
func TestEstimateTimeDPSMonotonico(t *testing.T) {
	if !(estimateTimeDPS(10000, 5, 680580, 15) < estimateTimeDPS(10000, 5, 891585, 15)) {
		t.Error("mais HP deveria dar mais tempo")
	}
	if estimateTimeDPS(10000, 5, 0, 0) < 1 {
		t.Error("piso de 1s")
	}
}
