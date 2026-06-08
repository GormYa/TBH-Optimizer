package internal

import (
	"math"
	"testing"
)

// fitTimeModel ajusta tempo = a*HP + b*ondas por minimos quadrados sobre todos os
// pontos medidos. Com mais de 2 mapas, o DPS (1/a) e o custo por onda (b) saem
// robustos -- ao contrario do solver de 2 ancoras, que a 1-1 (HP~0) distorcia.
func TestFitTimeModelRecuperaCoeficientes(t *testing.T) {
	// pontos consistentes com a=0.0001 (s/HP) e b=5 (s/onda)
	pts := []timePoint{
		{HP: 560, Waves: 10, Time: 0.0001*560 + 5*10},
		{HP: 680580, Waves: 15, Time: 0.0001*680580 + 5*15},
		{HP: 891585, Waves: 15, Time: 0.0001*891585 + 5*15},
	}
	a, b, ok := fitTimeModel(pts)
	if !ok {
		t.Fatal("esperava ok=true")
	}
	if math.Abs(a-0.0001) > 1e-9 {
		t.Errorf("a = %v, quero ~0.0001", a)
	}
	if math.Abs(b-5) > 1e-6 {
		t.Errorf("b = %v, quero ~5", b)
	}
}

func TestFitTimeModelInsuficiente(t *testing.T) {
	if _, _, ok := fitTimeModel([]timePoint{{HP: 560, Waves: 10, Time: 46}}); ok {
		t.Error("1 ponto nao deve ajustar")
	}
	// HP identico em todos -> sistema singular
	pts := []timePoint{{HP: 1000, Waves: 10, Time: 50}, {HP: 1000, Waves: 12, Time: 60}}
	if _, _, ok := fitTimeModel(pts); ok {
		t.Error("HP constante deve falhar (singular)")
	}
}

// 2-6 (HP 680580) deve estimar MENOS tempo que 2-7 (HP 891585): mesma contagem de
// ondas, menos HP -> menos tempo. (O bug fazia o contrario.)
func TestEstimateTimeMonotonicoComHP(t *testing.T) {
	a, b := 0.0001, 5.0
	t26 := estimateTime(a, b, 680580, 15)
	t27 := estimateTime(a, b, 891585, 15)
	if !(t26 < t27) {
		t.Errorf("2-6 (%.1fs) deveria ser < 2-7 (%.1fs)", t26, t27)
	}
	if estimateTime(a, b, 0, 0) < 1 {
		t.Error("tempo minimo deve ser >= 1s")
	}
}
