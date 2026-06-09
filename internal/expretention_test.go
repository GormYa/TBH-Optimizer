package internal

import "testing"

// Valida a curva de exp mantida contra a tabela do wiki (heroi nivel 32):
// lvl<=38 = 100%, lvl39 = 96%, lvl40 = 92% (-4% por nivel acima de heroi+6).
func TestExpRetention(t *testing.T) {
	cases := []struct {
		stage, hero int
		want        float64
	}{
		// over-level (fase acima do heroi)
		{30, 32, 1.00}, {36, 32, 1.00}, {38, 32, 1.00},
		{39, 32, 0.96}, {40, 32, 0.92}, {44, 32, 0.76},
		// under-level (fase abaixo do heroi) -- simetrico
		{26, 32, 1.00}, {20, 32, 0.76}, {10, 32, 0.36},
		// guardas
		{0, 32, 1.00}, {40, 0, 1.00},
	}
	for _, c := range cases {
		got := expRetention(c.stage, c.hero)
		if d := got - c.want; d > 1e-9 || d < -1e-9 {
			t.Errorf("expRetention(%d,%d)=%.3f, quero %.3f", c.stage, c.hero, got, c.want)
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
