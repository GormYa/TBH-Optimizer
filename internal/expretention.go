package internal

import "math"

func expBands(stageLevel, heroLevel int) (diff, b1, e1, e2 int, factor float64) {
	hero := min(heroLevel, 100)
	diff = stageLevel - hero
	if diff < 0 {
		diff = -diff
	}
	if hero >= stageLevel {
		b1 = 2
		if hero <= 53 {
			e1 = 8
		} else {
			e1 = 9
		}
		factor = 0.5
	} else {
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
	e2 = e1 + int(math.Round(float64(hero)/3.0))
	return
}

func expRetention(stageLevel, heroLevel int) float64 {
	if stageLevel <= 0 || heroLevel <= 0 {
		return 1.0
	}
	diff, b1, e1, e2, factor := expBands(stageLevel, heroLevel)
	const floor = 0.01
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

func expBandReliable(stageLevel, heroLevel int) bool {
	if stageLevel <= 0 || heroLevel <= 0 {
		return true
	}
	diff, _, e1, _, _ := expBands(stageLevel, heroLevel)
	return diff <= e1
}

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

func (ctrl *Control) numActiveHeroes() int {
	if ctrl.ActiveHeroCount > 0 {
		return ctrl.ActiveHeroCount
	}
	if n := len(ctrl.HeroStates); n > 0 {
		return n
	}
	return 1
}

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
