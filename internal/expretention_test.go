package internal

import "testing"

// Valida a curva REAL de exp mantida (emulada do GameAssembly, metodo vy.jwe) para
// heroi nivel 32. Tres fases: 100% -> parabola ate factor -> geometrico ate o piso 0.01.
//   over  (heroi>=fase): b1 2, e1 8,  factor 0.5, e2 = 8+round(32/3)  = 19
//   under (heroi< fase): b1 6, e1 14, factor 0.4, e2 = 14+round(32/3) = 25
func TestExpRetention(t *testing.T) {
	cases := []struct {
		stage, hero int
		want        float64
	}{
		// over-level (fase <= heroi): diff = 32-stage
		{32, 32, 1.00},   // diff 0
		{30, 32, 1.00},   // diff 2 (= b1)
		{26, 32, 0.7778}, // diff 6: 1 - 0.5*(4/6)^2
		{20, 32, 0.1206}, // diff 12: cauda 0.5*(0.02)^(4/11)
		{13, 32, 0.0100}, // diff 19 (= e2) -> piso 1%
		{1, 32, 0.0100},  // diff 31 (1-1 com heroi alto) -> piso 1% (era 50%!)
		// under-level (fase > heroi): diff = stage-32
		{36, 32, 1.00},   // diff 4
		{38, 32, 1.00},   // diff 6 (= b1)
		{40, 32, 0.9625}, // diff 8: 1 - 0.6*(2/8)^2
		{44, 32, 0.6625}, // diff 12: 1 - 0.6*(6/8)^2
		{50, 32, 0.1046}, // diff 18: cauda 0.4*(0.025)^(4/11)
		{60, 32, 0.0100}, // diff 28 >= e2 -> piso 1%
		// guardas
		{0, 32, 1.00}, {40, 0, 1.00},
	}
	for _, c := range cases {
		got := expRetention(c.stage, c.hero)
		if d := got - c.want; d > 5e-3 || d < -5e-3 {
			t.Errorf("expRetention(%d,%d)=%.4f, quero %.4f", c.stage, c.hero, got, c.want)
		}
	}
}

// Uma medicao so calibra os multiplicadores se estiver DENTRO da parabola de retencao
// (diff <= e1) — antes da cauda de decaimento. Na cauda, base x retencao desacopla do
// ganho real (heroi nv83 x fase nv1: ~176k XP, nao base x piso). Heroi 32: over e1=8,
// under e1=14. Par indefinido (nivel <=0) conta como confiavel (retencao 1.0).
func TestExpBandReliable(t *testing.T) {
	cases := []struct {
		stage, hero int
		want        bool
	}{
		{32, 32, true},  // diff 0
		{24, 32, true},  // over, diff 8 (= e1)
		{23, 32, false}, // over, diff 9 (> e1) -> cauda
		{1, 32, false},  // 1-1 com heroi alto -> cauda (o ponto envenenado)
		{46, 32, true},  // under, diff 14 (= e1)
		{47, 32, false}, // under, diff 15 (> e1) -> cauda
		{0, 32, true},   // fase indefinida
		{40, 0, true},   // medicao legada sem carimbo de nivel
	}
	for _, c := range cases {
		if got := expBandReliable(c.stage, c.hero); got != c.want {
			t.Errorf("expBandReliable(%d,%d)=%v, quero %v", c.stage, c.hero, got, c.want)
		}
	}
}

// XP/hora deve ser dividido pelos herois ATIVOS (3), nao por todos do save (6),
// senao mostra metade do valor (descasando do wiki, que e por heroi ativo).
func TestNumActiveHeroes(t *testing.T) {
	ctrl := Control{ActiveHeroCount: 3, HeroStates: map[int]HeroState{1: {}, 2: {}, 3: {}, 4: {}, 5: {}, 6: {}}}
	if n := ctrl.numActiveHeroes(); n != 3 {
		t.Fatalf("divisor de XP = %d, quero 3 (ativos), nao 6 (todos os herois)", n)
	}
	ctrl2 := Control{HeroStates: map[int]HeroState{1: {}, 2: {}}}
	if n := ctrl2.numActiveHeroes(); n != 2 {
		t.Fatalf("fallback sem ActiveHeroCount = %d, quero 2", n)
	}
	var empty Control
	if n := empty.numActiveHeroes(); n != 1 {
		t.Fatalf("sem nada deve ser 1, veio %d", n)
	}
}

func TestActiveHeroLevel(t *testing.T) {
	var s InnerSaveData
	s.HeroSaveDatas = []Hero{{HeroKey: 101, HeroLevel: 1}, {HeroKey: 201, HeroLevel: 32}, {HeroKey: 301, HeroLevel: 30}}
	s.CommonSaveData.ArrangedHeroKey = []int{201, 301}
	if lv := activeHeroLevel(&s); lv != 32 {
		t.Fatalf("nivel ativo = %d, quero 32 (maior entre os arranjados)", lv)
	}
}
