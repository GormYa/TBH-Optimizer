package internal

import "math"

func CalibrateHeroStates(heroes []Hero) map[int]HeroState {
	heroesXp := make(map[int]HeroState)
	for _, hero := range heroes {
		heroesXp[hero.HeroKey] = HeroState{Level: hero.HeroLevel, Xp: hero.HeroExp}
	}
	return heroesXp
}

func GetXPRequiredForLevel(level int) float64 {
	if level <= 0 {
		return 0
	}
	lvlFloat := float64(level)
	return 15.0 * math.Pow(lvlFloat, 4.0)
}

func ProcessRoundXp(heroes []Hero, history map[int]HeroState) float64 {
	xpGainedInTheRound := 0.0
	for _, hero := range heroes {
		oldState, exists := history[hero.HeroKey]
		var xpDifference float64
		if !exists {
			xpDifference = 0
		} else {
			if hero.HeroLevel == oldState.Level {
				xpDifference = hero.HeroExp - oldState.Xp
				if xpDifference < 0 {
					xpDifference = 0
				}
			} else if hero.HeroLevel > oldState.Level {
				xpDifference = GetXPRequiredForLevel(oldState.Level) - oldState.Xp
				for l := oldState.Level + 1; l < hero.HeroLevel; l++ {
					xpDifference += GetXPRequiredForLevel(l)
				}
				xpDifference += hero.HeroExp
				if xpDifference < 0 {
					xpDifference = 0
				}
			} else {
				xpDifference = 0
			}
		}

		xpGainedInTheRound += xpDifference

		history[hero.HeroKey] = HeroState{Level: hero.HeroLevel, Xp: hero.HeroExp}
	}
	return xpGainedInTheRound
}

