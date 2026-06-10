package internal

import "math"

// expRetention devolve a fracao de XP mantida (0..1) numa fase de nivel stageLevel para
// um heroi de nivel heroLevel. Formula REAL do GameAssembly (IL2CPP, classe `vy`, metodo
// `jwe`), extraida emulando a funcao com Unicorn na faixa inteira de diferenca de nivel
// (ver memoria il2cpp-exp-rate-decompile e tools/_il2cpp/emu*.py). Validada contra a
// emulacao em 13.680 pontos: erro medio 0.0003, maximo 0.055 (so em heroi ~6).
//
// Tres fases, assimetricas por direcao (over: heroi>=fase / under: heroi<fase):
//   - diff <= b1: 100%.
//   - b1 < diff <= e1: parabola 1 -> factor (0.5 over / 0.4 under).
//   - e1 < diff < e2: decaimento GEOMETRICO de factor ate o piso 0.01 (o `pow` do jogo).
//   - diff >= e2: piso 0.01.
// e2 = e1 + round(heroi/3): a cauda alonga com o nivel. O piso real e 1% -- a versao
// antiga errava travando em 50%/40% (so tinha amostrado a faixa antes da cauda).
func expRetention(stageLevel, heroLevel int) float64 {
	if stageLevel <= 0 || heroLevel <= 0 {
		return 1.0
	}
	hero := min(heroLevel, 100)
	diff := stageLevel - hero
	if diff < 0 {
		diff = -diff
	}
	const floor = 0.01
	var b1, e1 int
	var factor float64
	if hero >= stageLevel { // over-leveled
		b1 = 2
		if hero <= 53 {
			e1 = 8
		} else {
			e1 = 9
		}
		factor = 0.5
	} else { // under-leveled
		switch {
		case hero <= 5:
			b1 = 5
		case hero <= 53:
			b1 = 6
		default:
			b1 = 7
		}
		switch {
		case hero <= 4:
			e1 = 11
		case hero <= 6:
			e1 = 12
		case hero <= 27:
			e1 = 13
		case hero <= 53:
			e1 = 14
		default:
			e1 = 15
		}
		factor = 0.4
	}
	e2 := e1 + int(math.Round(float64(hero)/3.0))
	if diff <= b1 {
		return 1.0
	}
	if diff <= e1 {
		t := float64(diff-b1) / float64(e1-b1)
		return 1.0 - (1.0-factor)*t*t
	}
	if diff < e2 {
		return factor * math.Pow(floor/factor, float64(diff-e1)/float64(e2-e1))
	}
	return floor
}

// runeLevels devolve runeKey -> nivel a partir do save (runas que o jogador tem).
func ownedPets(save *InnerSaveData) []int {
	out := make([]int, 0, len(save.PetSaveDatas))
	for _, p := range save.PetSaveDatas {
		if p.IsUnlock {
			out = append(out, p.PetKey)
		}
	}
	return out
}

func runeLevels(save *InnerSaveData) map[int]int {
	out := make(map[int]int, len(save.RuneSaveDatas))
	for _, r := range save.RuneSaveDatas {
		out[r.RuneKey] = r.Level
	}
	return out
}

// activeHeroes devolve os herois do time ativo (arrangedHeroKey) com seus niveis,
// na ordem do save. Usado pra exibir o time no painel.
func activeHeroes(save *InnerSaveData) []ActiveHero {
	levelByHero := make(map[int]int, len(save.HeroSaveDatas))
	for _, h := range save.HeroSaveDatas {
		levelByHero[h.HeroKey] = h.HeroLevel
	}
	out := make([]ActiveHero, 0, len(save.CommonSaveData.ArrangedHeroKey))
	for _, hk := range save.CommonSaveData.ArrangedHeroKey {
		out = append(out, ActiveHero{Key: hk, Level: levelByHero[hk]})
	}
	return out
}

// numActiveHeroes e o tamanho do time ativo (arrangedHeroKey) e o divisor correto do
// XP: o jogo da XP por heroi ativo, entao XP/hora por heroi = XP total do round /
// herois ativos. Herois travados (nao no time) nao ganham XP e nao devem diluir.
func (ctrl *Control) numActiveHeroes() int {
	if ctrl.ActiveHeroCount > 0 {
		return ctrl.ActiveHeroCount
	}
	if n := len(ctrl.HeroStates); n > 0 {
		return n
	}
	return 1
}

// activeHeroLevel devolve o maior nivel entre os herois ativos (arrangedHeroKey) do
// save. Fallback: maior nivel entre todos os herois. 0 se nao houver nenhum.
func activeHeroLevel(save *InnerSaveData) int {
	levelByHero := make(map[int]int, len(save.HeroSaveDatas))
	for _, h := range save.HeroSaveDatas {
		levelByHero[h.HeroKey] = h.HeroLevel
	}
	best := 0
	for _, hk := range save.CommonSaveData.ArrangedHeroKey {
		if lv := levelByHero[hk]; lv > best {
			best = lv
		}
	}
	if best == 0 {
		for _, h := range save.HeroSaveDatas {
			if h.HeroLevel > best {
				best = h.HeroLevel
			}
		}
	}
	return best
}
