package internal

import (
	"encoding/json"
	"math"
	"strconv"
)

// levelExpCurve: XP real por nivel (LevelInfoData.ExpForLevelUp), carregada do embed.
var levelExpCurve map[int]float64

// LoadLevelCurve carrega a curva real de XP por nivel ({"1":30,...}). A fórmula 15*L^4
// não batia com o jogo e gerava picos/zeros de XP no level-up.
func LoadLevelCurve(data []byte) {
	var m map[string]float64
	if err := json.Unmarshal(data, &m); err != nil {
		return
	}
	levelExpCurve = make(map[int]float64, len(m))
	for k, v := range m {
		if lv, err := strconv.Atoi(k); err == nil {
			levelExpCurve[lv] = v
		}
	}
}

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
	if v, ok := levelExpCurve[level]; ok && v > 0 {
		return v
	}
	return 15.0 * math.Pow(float64(level), 4.0)
}

// heroNames: key -> nome do heroi (de heroes.json), pra mensagem do console.
var heroNames map[int]string

// LoadHeroNames carrega {"101":{"name":"Cavaleiro",...}} -> mapa key->nome.
func LoadHeroNames(data []byte) {
	var m map[string]struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return
	}
	heroNames = make(map[int]string, len(m))
	for k, v := range m {
		if key, err := strconv.Atoi(k); err == nil {
			heroNames[key] = v.Name
		}
	}
}

func heroName(key int) string {
	if n, ok := heroNames[key]; ok && n != "" {
		return n
	}
	return "Herói " + strconv.Itoa(key)
}

// HeroLevelUp registra um heroi que subiu de nivel no ciclo (do nivel From p/ To).
type HeroLevelUp struct {
	Key, From, To int
}

// heroLevelUps compara o nivel atual de cada heroi com a baseline do inicio do
// ciclo e devolve quem subiu. Comparar NIVEL (e nao reconstruir o XP) e a fonte
// da verdade: se subiu, a corrida e descartada (o XP do clear fica distorcido
// pelo limiar do nivel, enorme em niveis altos).
func heroLevelUps(heroes []Hero, history map[int]HeroState) []HeroLevelUp {
	var ups []HeroLevelUp
	for _, h := range heroes {
		if old, ok := history[h.HeroKey]; ok && h.HeroLevel > old.Level {
			ups = append(ups, HeroLevelUp{Key: h.HeroKey, From: old.Level, To: h.HeroLevel})
		}
	}
	return ups
}

// computeRoundXp calcula o XP ganho no ciclo SEM mexer na baseline (history).
// A baseline (tempo/ouro/xp) so pode avancar quando um clear de verdade e
// contabilizado; commitHeroStates faz isso. Avancar em saves descartados
// truncava a janela do proximo clear (media um caco da corrida).
func computeRoundXp(heroes []Hero, history map[int]HeroState) float64 {
	xpGainedInTheRound := 0.0
	for _, hero := range heroes {
		oldState, exists := history[hero.HeroKey]
		if !exists || hero.HeroLevel != oldState.Level {
			continue
		}
		if diff := hero.HeroExp - oldState.Xp; diff > 0 {
			xpGainedInTheRound += diff
		}
	}
	return xpGainedInTheRound
}

// commitHeroStates move a baseline de XP/nivel para o estado atual. Chamado so
// quando o relogio do clear avanca (clear contabilizado / fronteira real).
func commitHeroStates(heroes []Hero, history map[int]HeroState) {
	for _, hero := range heroes {
		history[hero.HeroKey] = HeroState{Level: hero.HeroLevel, Xp: hero.HeroExp}
	}
}
