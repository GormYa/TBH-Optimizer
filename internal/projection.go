package internal

// timePoint e um mapa medido: HP total, numero de ondas e tempo real de clear.
type timePoint struct {
	HP    float64
	Waves float64
	Time  float64
}

// fitTimeModel ajusta tempo = a*HP + b*Waves por minimos quadrados sobre todos os
// pontos. a = segundos por HP (1/DPS efetivo), b = custo fixo por onda. Usar TODOS
// os mapas medidos (em vez de 2 ancoras) evita que a 1-1 (HP ~ 0) distorca o DPS.
// Retorna ok=false se houver menos de 2 pontos, sistema singular, ou a<=0.
func fitTimeModel(pts []timePoint) (a, b float64, ok bool) {
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
	a = (sWW*sHt - sHW*sWt) / det
	b = (sHH*sWt - sHW*sHt) / det
	if a <= 0 {
		return 0, 0, false
	}
	if b < 0 {
		b = 0
	}
	return a, b, true
}

// estimateTime aplica o modelo, com piso de 1s.
func estimateTime(a, b, hp, waves float64) float64 {
	t := a*hp + b*waves
	if t < 1 {
		return 1
	}
	return t
}
