package internal

// Penalidade de XP por diferenca de nivel ("exp mantida"): o jogo reduz o XP quando a
// fase esta longe do nivel do heroi -- tanto ACIMA (fase dificil demais) quanto ABAIXO
// (fase facil demais). Mecanica escondida (sem dado/identificador nos arquivos); a curva
// foi derivada da tabela do taskbarhero.wiki: 100% dentro de +-6 niveis do heroi, depois
// -4% por nivel de distancia, em qualquer direcao.
const (
	expPenaltyThreshold = 6
	expPenaltyPerLevel  = 0.04
	expRetentionFloor   = 0.0
)

// expRetention devolve a fracao de XP mantida (0..1) numa fase de nivel stageLevel
// para um heroi de nivel heroLevel. 1.0 quando nao ha dado suficiente.
func expRetention(stageLevel, heroLevel int) float64 {
	if stageLevel <= 0 || heroLevel <= 0 {
		return 1.0
	}
	diff := stageLevel - heroLevel
	if diff < 0 {
		diff = -diff
	}
	over := diff - expPenaltyThreshold
	if over <= 0 {
		return 1.0
	}
	r := 1.0 - expPenaltyPerLevel*float64(over)
	if r < expRetentionFloor {
		return expRetentionFloor
	}
	return r
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
