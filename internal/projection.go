package internal

import "sort"

// median devolve a mediana da amostra (robusta a outliers, ao contrario da media).
// Ordena uma copia para nao mexer no slice do chamador.
func median(xs []float64) float64 {
	n := len(xs)
	if n == 0 {
		return 0
	}
	cp := make([]float64, n)
	copy(cp, xs)
	sort.Float64s(cp)
	if n%2 == 1 {
		return cp[n/2]
	}
	return (cp[n/2-1] + cp[n/2]) / 2
}

// timePoint e um mapa medido: HP total, numero de ondas e tempo real de clear.
type timePoint struct {
	HP    float64
	Waves float64
	Time  float64
}

// effectiveDPS deduz o DPS do jogador ajustando tempo = HP/DPS + b*ondas por minimos
// quadrados sobre as fases medidas. DPS = 1/a (HP por segundo), b = custo fixo por onda.
// Testei a versao "robusta" por mediana de inclinacoes (Theil-Sen) com os dados reais e
// ela errou MAIS (27% vs 20%): o tempo de clear nao e limpo o bastante em HP pra mediana
// ganhar do ajuste global. ok=false com <2 pontos, sistema singular ou DPS<=0.
func effectiveDPS(pts []timePoint) (dps, overheadPerWave float64, ok bool) {
	if len(pts) < 2 {
		return 0, 0, false
	}
	var sHH, sHW, sWW, sHt, sWt float64
	for _, p := range pts {
		sHH += p.HP * p.HP
		sHW += p.HP * p.Waves
		sWW += p.Waves * p.Waves
		sHt += p.HP * p.Time
		sWt += p.Waves * p.Time
	}
	det := sHH*sWW - sHW*sHW
	if det == 0 {
		return 0, 0, false
	}
	a := (sWW*sHt - sHW*sWt) / det
	b := (sHH*sWt - sHW*sHt) / det
	if a <= 0 {
		return 0, 0, false
	}
	if b < 0 {
		b = 0
	}
	return 1 / a, b, true
}

// estimateTimeDPS aplica o DPS: tempo = HP/DPS + ondas*custo_por_onda, piso de 1s.
func estimateTimeDPS(dps, overheadPerWave, hp, waves float64) float64 {
	if dps <= 0 {
		return 0
	}
	t := hp/dps + overheadPerWave*waves
	if t < 1 {
		return 1
	}
	return t
}
