package internal

// Penalidade de XP por diferenca de nivel ("exp mantida").
// E uma PARABOLA ate um piso, ASSIMETRICA por direcao:
//   - over-leveled (heroi >= fase): cai mais rapido, piso 0.5
//   - under-leveled (heroi <  fase): cai mais devagar, piso 0.4
// Dentro de `flat` niveis de diferenca: 100%. De `flat` ate `end`: 1 - (1-piso)*t^2,
// com t = (diff-flat)/(end-flat). A partir de `end`: piso. `flat`/`end` crescem
// devagar com o nivel do heroi (bandas validadas contra a emulacao, erro < 0.06).
func expRetentionBandsOver(hero int) (flat, end int) {
	if hero < 3 {
		flat = hero - 1
	} else {
		flat = 2
	}
	if hero <= 53 {
		end = 8
	} else {
		end = 9
	}
	return
}

func expRetentionBandsUnder(hero int) (flat, end int) {
	switch {
	case hero <= 6:
		flat = 5
	case hero <= 53:
		flat = 6
	default:
		flat = 7
	}
	switch {
	case hero <= 4:
		end = 11
	case hero <= 6:
		end = 12
	case hero <= 27:
		end = 13
	case hero <= 53:
		end = 14
	default:
		end = 15
	}
	return
}

// expRetention devolve a fracao de XP mantida (0..1) numa fase de nivel stageLevel
// para um heroi de nivel heroLevel. 1.0 quando nao ha dado suficiente.
func expRetention(stageLevel, heroLevel int) float64 {
	if stageLevel <= 0 || heroLevel <= 0 {
		return 1.0
	}
	hero := min(heroLevel, 100)
	diff := stageLevel - hero
	if diff < 0 {
		diff = -diff
	}
	var flat, end int
	var floor float64
	if hero >= stageLevel {
		flat, end = expRetentionBandsOver(hero)
		floor = 0.5
	} else {
		flat, end = expRetentionBandsUnder(hero)
		floor = 0.4
	}
	if diff <= flat {
		return 1.0
	}
	if end <= flat || diff >= end {
		return floor
	}
	t := float64(diff-flat) / float64(end-flat)
	return 1.0 - (1.0-floor)*t*t
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
