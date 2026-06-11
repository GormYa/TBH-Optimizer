package internal

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

const testSavePath = `C:\Users\x\AppData\LocalLow\TesseractStudio\TaskbarHero\SaveFile_Live.es3`

// newClearSave monta um save representando um clear estavel da fase (wave 0), com
// ganho de ouro e xp em relacao a baseline do ctrl.
func newClearSave(stage int, playTime float64, gold int, heroKey int, heroLvl int, heroExp float64) *InnerSaveData {
	s := &InnerSaveData{}
	s.CommonSaveData.CurrentStageKey = stage
	s.CommonSaveData.CurrentStageWave = 0
	s.CommonSaveData.PlayTime = playTime
	s.CommonSaveData.ArrangedHeroKey = []int{heroKey}
	s.CurrenySaveDatas = []Currency{{Key: 100001, Quantity: gold}}
	s.HeroSaveDatas = []Hero{{HeroKey: heroKey, HeroLevel: heroLvl, HeroExp: heroExp}}
	return s
}

// PROVA do fix: uma fase NOVA (sem historico proprio) registra o clear real. O bug era
// a trava de tempo usar um modelo estimado que subestimava a fase e descartava tudo.
func TestNewStageRecordsRound(t *testing.T) {
	old, _ := os.Getwd()
	if err := os.Chdir(t.TempDir()); err != nil { // isola stage_history.json/historico_farm.txt
		t.Fatal(err)
	}
	defer os.Chdir(old)

	const stage = 1308
	ctrl := Control{
		UseEMA: true, EMAAlpha: 0.2,
		FarmStages:          map[int]FarmStageInfo{stage: {Key: stage, Label: "3-8", TotalHP: 100000, ExpectedGold: 4000, ExpectedEXP: 60000, Waves: 17, Level: 30}},
		HeroStates:          map[int]HeroState{201: {Level: 33, Xp: 1000}},
		ActiveHeroCount:     1,
		HeroLevel:           33,
		LastCurrentStageKey: stage, // mesma fase no fim -> ciclo estavel
		LastPlayTime:        1000,
		LastGold:            500,
	}
	// clear de 148s, +10000 ouro, +500000 xp
	calculateAndLogRound(&ctrl, newClearSave(stage, 1148, 10500, 201, 33, 501000))

	s, ok := ctrl.StageHistory.Get(stage)
	runs := 0
	if s != nil {
		runs = s.TotalRuns
	}
	if !ok || runs != 1 {
		t.Fatalf("fase NOVA (sem histórico) deveria registrar o clear; ok=%v runs=%d", ok, runs)
	}
	if s.AvgTimeSpent != 148 {
		t.Fatalf("tempo registrado = %.0fs, esperado 148s", s.AvgTimeSpent)
	}
}

// A trava de tempo AINDA protege: numa fase com historico proprio (>=3 corridas), um
// clear com tempo absurdamente inflado (morte/ociosidade) e descartado.
func TestEstablishedStageRejectsInflatedTime(t *testing.T) {
	old, _ := os.Getwd()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)

	const stage = 1308
	ctrl := Control{
		UseEMA: true, EMAAlpha: 0.2,
		FarmStages:          map[int]FarmStageInfo{stage: {Key: stage, Label: "3-8", TotalHP: 100000, ExpectedGold: 4000, ExpectedEXP: 60000, Waves: 17, Level: 30}},
		HeroStates:          map[int]HeroState{201: {Level: 33, Xp: 0}},
		ActiveHeroCount:     1,
		HeroLevel:           33,
		LastCurrentStageKey: stage,
	}
	// 3 corridas de ~148s pra estabelecer a media propria
	for i := 0; i < 3; i++ {
		ctrl.StageHistory.Update(stage, 148, 10000, 500000, true, 0.2, 1, 33, 0, nil)
	}
	ctrl.LastPlayTime = 1000
	ctrl.LastGold = 500

	// clear com 600s (>3x148): deve ser DESCARTADO. Ouro normal pra isolar a trava de tempo.
	calculateAndLogRound(&ctrl, newClearSave(stage, 1600, 10500, 201, 33, 500000))

	if s, _ := ctrl.StageHistory.Get(stage); s.TotalRuns != 3 {
		t.Fatalf("corrida com tempo inflado deveria ser descartada; TotalRuns=%d (esperado 3)", s.TotalRuns)
	}
}

// Regressao: uma fase de BAIXO HP (1-1) nao pode estimar tempo ~0s quando so ha
// fases de ALTO HP medidas. O modelo antigo (tempo ~ HP) dava ~0,02s pra 1-1 e
// fazia clears reais de ~38s serem descartados como outliers de tempo inflado.
func TestLowHPStageNotRejected(t *testing.T) {
	ctrl := Control{
		UseEMA:   true,
		EMAAlpha: 0.2,
		FarmStages: map[int]FarmStageInfo{
			1101: {Key: 1101, TotalHP: 560, Waves: 10},
			1102: {Key: 1102, TotalHP: 2040, Waves: 11},
			1305: {Key: 1305, TotalHP: 2912165, Waves: 17},
		},
	}
	ctrl.StageHistory.Update(1102, 60, 0, 0, true, 0.2, 1, 30, 0, nil)
	ctrl.StageHistory.Update(1305, 250, 0, 0, true, 0.2, 1, 30, 0, nil)

	est := ctrl.estimateStageTime(1101)
	if est > 0 && est < 5 {
		t.Fatalf("1-1 estimado em %.2fs (~0): clears reais seriam descartados", est)
	}
	if est > 0 && !isTimeTrustworthy(38, est, timeOutlierFactor, false) {
		t.Fatalf("clear real de 38s do 1-1 rejeitado (est=%.2fs)", est)
	}
}

// AUTO-AVANCO: ao concluir sA o jogo auto-avanca e o save de wave 0 ja vem com sB (o
// PROXIMO mapa). O tempo desse ciclo e do sA (o que rodou), entao tem que ser creditado
// no sA -- nao no sB. E conta de primeira (sem precisar concluir 2x).
func TestAutoAdvanceCreditsPreviousStage(t *testing.T) {
	old, _ := os.Getwd()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)

	const sA, sB = 2206, 2207 // contiguos: sB e o proximo de sA
	ctrl := Control{
		UseEMA: true, EMAAlpha: 0.2,
		FarmStages: map[int]FarmStageInfo{
			sA: {Key: sA, TotalHP: 100000, ExpectedGold: 4000, ExpectedEXP: 60000, Waves: 17, Level: 30},
			sB: {Key: sB, TotalHP: 110000, ExpectedGold: 4200, ExpectedEXP: 62000, Waves: 17, Level: 31},
		},
		HeroStates:          map[int]HeroState{201: {Level: 33, Xp: 0}},
		ActiveHeroCount:     1,
		HeroLevel:           33,
		LastCurrentStageKey: sA, // estavamos rodando sA
		LastPlayTime:        1000,
		LastGold:            500,
	}

	// save de wave 0 ja com sB (auto-avancou), ciclo de 267s
	calculateAndLogRound(&ctrl, newClearSave(sB, 1267, 60000, 201, 33, 4000000))

	a, _ := ctrl.StageHistory.Get(sA)
	if a == nil || a.TotalRuns != 1 {
		t.Fatalf("o clear deveria ser creditado em sA (o que rodou); a=%v", a)
	}
	if a.AvgTimeSpent != 267 {
		t.Fatalf("tempo de sA = %.0fs, esperado 267 (o ciclo que rodou)", a.AvgTimeSpent)
	}
	if b, ok := ctrl.StageHistory.Get(sB); ok && b.TotalRuns > 0 {
		t.Fatalf("sB (próximo mapa) não deveria ter corrida ainda; runs=%d", b.TotalRuns)
	}
}

// PULO MANUAL: se o destino nao e nem o mesmo mapa nem o proximo na ordem, a janela
// mistura tempo de mapas diferentes (+ menu) e e descartada -- nao credita ninguem.
func TestManualJumpDiscarded(t *testing.T) {
	old, _ := os.Getwd()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)

	const sA, sNext, sJump = 2206, 2207, 2209
	ctrl := Control{
		UseEMA: true, EMAAlpha: 0.2,
		FarmStages: map[int]FarmStageInfo{
			sA:    {Key: sA, TotalHP: 100000, ExpectedGold: 4000, ExpectedEXP: 60000, Waves: 17, Level: 30},
			sNext: {Key: sNext, TotalHP: 110000, ExpectedGold: 4200, ExpectedEXP: 62000, Waves: 17, Level: 31},
			sJump: {Key: sJump, TotalHP: 130000, ExpectedGold: 4800, ExpectedEXP: 68000, Waves: 17, Level: 33},
		},
		HeroStates:          map[int]HeroState{201: {Level: 33, Xp: 0}},
		ActiveHeroCount:     1,
		HeroLevel:           33,
		LastCurrentStageKey: sA, // rodavamos sA; o proximo seria sNext, mas pulou pra sJump
		LastPlayTime:        1000,
		LastGold:            500,
	}

	calculateAndLogRound(&ctrl, newClearSave(sJump, 1300, 60000, 201, 33, 4000000))
	if a, ok := ctrl.StageHistory.Get(sA); ok && a.TotalRuns > 0 {
		t.Fatalf("pulo manual não deveria creditar em sA; runs=%d", a.TotalRuns)
	}
	if j, ok := ctrl.StageHistory.Get(sJump); ok && j.TotalRuns > 0 {
		t.Fatalf("pulo manual não deveria creditar no destino; runs=%d", j.TotalRuns)
	}
}

// INICIO NO MEIO DO CICLO: quando o monitor abre com wave != 0, a 1a janela mede so
// um pedaco do ciclo (ex.: 97s onde o clear real e ~255s). Deve ser DESCARTADA, e o
// proximo clear completo conta.
func TestStartMidCycleDiscardsFirstClear(t *testing.T) {
	old, _ := os.Getwd()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)

	const stage = 2209
	ctrl := Control{
		UseEMA: true, EMAAlpha: 0.2,
		FarmStages:          map[int]FarmStageInfo{stage: {Key: stage, Label: "2-9", TotalHP: 27000000, ExpectedGold: 50000, ExpectedEXP: 940000, Waves: 23, Level: 45}},
		HeroStates:          map[int]HeroState{201: {Level: 52, Xp: 0}},
		ActiveHeroCount:     3,
		HeroLevel:           52,
		LastCurrentStageKey: stage,
		LastPlayTime:        1000,
		LastGold:            500,
		primeFirstClear:     true, // monitor comecou no meio do ciclo
	}
	// "clear" parcial de 97s (meio ciclo) -> descartado
	calculateAndLogRound(&ctrl, newClearSave(stage, 1097, 52000, 201, 52, 1987378))
	if s, ok := ctrl.StageHistory.Get(stage); ok && s.TotalRuns > 0 {
		t.Fatalf("1a janela parcial deveria ser descartada; runs=%d", s.TotalRuns)
	}
	if ctrl.primeFirstClear {
		t.Fatal("primeFirstClear deveria ter sido limpo")
	}
	// proximo clear completo (255s) -> conta
	calculateAndLogRound(&ctrl, newClearSave(stage, 1352, 147000, 201, 52, 5587378))
	s, ok := ctrl.StageHistory.Get(stage)
	runs := 0
	if s != nil {
		runs = s.TotalRuns
	}
	if !ok || runs != 1 {
		t.Fatalf("o 2o clear (completo) deveria contar; runs=%d", runs)
	}
}

func dirEvent(name string, op fsnotify.Op) fsnotify.Event {
	return fsnotify.Event{Name: filepath.Join(filepath.Dir(testSavePath), name), Op: op}
}

func TestIsRelevantSaveEvent(t *testing.T) {
	cases := []struct {
		desc  string
		event fsnotify.Event
		want  bool
	}{
		{"write no save real", dirEvent("SaveFile_Live.es3", fsnotify.Write), true},
		{"create do save (replace atomico)", dirEvent("SaveFile_Live.es3", fsnotify.Create), true},
		{"Player.log e ruido", dirEvent("Player.log", fsnotify.Write), false},
		{"arquivo temporario do ES3", dirEvent("SaveFile_Live.es3.tmp", fsnotify.Write), false},
		{"backup rotacionado do ES3", dirEvent("SaveFile_Live_1.es3.bak", fsnotify.Create), false},
		{"steam autocloud", dirEvent("steam_autocloud.vdf", fsnotify.Write), false},
		{"chmod no save nao conta", dirEvent("SaveFile_Live.es3", fsnotify.Chmod), false},
	}
	for _, c := range cases {
		if got := isRelevantSaveEvent(c.event, testSavePath); got != c.want {
			t.Errorf("%s: isRelevantSaveEvent = %v, quero %v", c.desc, got, c.want)
		}
	}
}

// Um unico save atomico do ES3 dispara um burst de eventos no arquivo alvo.
// O coalescer deve traduzir esse burst em UMA unica chamada de processamento.
func TestRunDebouncedMonitorCoalesceBurst(t *testing.T) {
	events := make(chan fsnotify.Event, 16)
	errs := make(chan error)
	fired := make(chan struct{}, 16)

	go runDebouncedMonitor(events, errs, testSavePath, 30*time.Millisecond, func() {
		fired <- struct{}{}
	})

	events <- dirEvent("SaveFile_Live.es3.tmp", fsnotify.Write)
	events <- dirEvent("SaveFile_Live_1.es3.bak", fsnotify.Create)
	events <- dirEvent("SaveFile_Live.es3", fsnotify.Create)
	events <- dirEvent("SaveFile_Live.es3", fsnotify.Write)
	events <- dirEvent("SaveFile_Live.es3", fsnotify.Write)

	select {
	case <-fired:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("processamento nunca disparou apos o burst de save")
	}

	select {
	case <-fired:
		t.Fatal("save processado mais de uma vez (falso positivo do burst)")
	case <-time.After(120 * time.Millisecond):
	}
}

// Uma corrida so e valida se a fase nao mudou durante a janela de medicao.
// Trocar de mapa (mesmo sem concluir) contamina tempo/ouro/xp com dados de
// fases diferentes e deve ser descartado.
func TestIsValidRound(t *testing.T) {
	cases := []struct {
		desc      string
		timeSpent float64
		goldGain  int
		xpGain    float64
		want      bool
	}{
		{"corrida normal", 60, 1000, 500, true},
		{"mapa rapido (1102, ~5s) e valido", 5, 1000, 500, true},
		{"so ouro (herois no max, xp 0)", 60, 1000, 0, true},
		{"ganho zero (save ocioso / selecao) -> invalido", 60, 0, 0, false},
		{"tempo curto demais (ruido <3s)", 2, 1000, 500, false},
		{"ouro negativo COM xp (compra de runa no meio) -> valido", 60, -50000, 500, true},
		{"ouro negativo SEM xp (so compra, sem clear) -> invalido", 60, -50000, 0, false},
		{"xp negativo (anomalia)", 60, 1000, -1, false},
	}
	for _, c := range cases {
		if got := isValidRound(c.timeSpent, c.goldGain, c.xpGain); got != c.want {
			t.Errorf("%s: isValidRound = %v, quero %v", c.desc, got, c.want)
		}
	}
}

// expectedClearGold: piso de ouro SÓ pela média própria (>=3 corridas). Cold-start e
// poucas corridas -> 0 (sem piso), pra não descartar clear de fase nova com palpite.
func TestExpectedClearGold(t *testing.T) {
	cases := []struct {
		desc    string
		ownAvg  float64
		ownRuns int
		want    float64
	}{
		{"usa media propria com historico", 3000, 5, 3000},
		{"exatamente 3 corridas usa a media", 2500, 3, 2500},
		{"cold start sem historico -> 0 (sem piso)", 0, 0, 0},
		{"poucas corridas (<3) -> 0 (sem piso)", 999, 2, 0},
	}
	for _, c := range cases {
		if got := expectedClearGold(c.ownAvg, c.ownRuns); got != c.want {
			t.Errorf("%s: expectedClearGold = %v, quero %v", c.desc, got, c.want)
		}
	}
}

// estimatedRunTime estima quanto uma fase deveria levar. Prioriza o historico
// proprio (>=3 corridas); senao extrapola por HP a partir de uma fase de
// referencia ja medida (tempo ~ proporcional ao HP total). 0 quando nao ha base.
func TestEstimatedRunTime(t *testing.T) {
	cases := []struct {
		desc    string
		ownAvg  float64
		ownRuns int
		stageHP float64
		refHP   float64
		refAvg  float64
		want    float64
	}{
		{"usa media propria quando ha >=3 corridas", 40, 5, 1400, 2800, 140, 40},
		{"extrapola por HP na primeira visita", 0, 0, 1400, 2800, 140, 70},
		{"ignora media propria com poucas corridas", 999, 2, 1400, 2800, 140, 70},
		{"sem referencia retorna 0", 0, 0, 1400, 0, 0, 0},
	}
	for _, c := range cases {
		if got := estimatedRunTime(c.ownAvg, c.ownRuns, c.stageHP, c.refHP, c.refAvg); got != c.want {
			t.Errorf("%s: estimatedRunTime = %v, quero %v", c.desc, got, c.want)
		}
	}
}

// isTimeTrustworthy decide se o tempo da janela e confiavel. Com estimativa,
// confia enquanto timeSpent <= fator*estimativa (auto-avanco passa, troca manual
// inflada cai). Sem estimativa, so confia se a fase NAO mudou.
func TestIsTimeTrustworthy(t *testing.T) {
	const f = 3.0
	cases := []struct {
		desc      string
		timeSpent float64
		estTime   float64
		changed   bool
		want      bool
	}{
		{"tempo normal dentro da estimativa", 40, 30, true, true},
		{"troca manual com tempo inflado (1101: 136 vs ~28)", 136, 28, true, false},
		{"auto-avanco com tempo normal apesar de mudar fase", 140, 135, true, true},
		{"sem estimativa e mesma fase confia", 40, 0, false, true},
		{"sem estimativa e trocou de fase desconfia", 136, 0, true, false},
	}
	for _, c := range cases {
		if got := isTimeTrustworthy(c.timeSpent, c.estTime, f, c.changed); got != c.want {
			t.Errorf("%s: isTimeTrustworthy = %v, quero %v", c.desc, got, c.want)
		}
	}
}

// Ruido constante (Player.log etc.) jamais pode disparar processamento.
func TestRunDebouncedMonitorIgnoresNoise(t *testing.T) {
	events := make(chan fsnotify.Event, 16)
	errs := make(chan error)
	fired := make(chan struct{}, 16)

	go runDebouncedMonitor(events, errs, testSavePath, 30*time.Millisecond, func() {
		fired <- struct{}{}
	})

	events <- dirEvent("Player.log", fsnotify.Write)
	events <- dirEvent("Player.log", fsnotify.Write)
	events <- dirEvent("SaveFile_Live.es3.tmp", fsnotify.Write)

	select {
	case <-fired:
		t.Fatal("ruido disparou processamento indevidamente")
	case <-time.After(150 * time.Millisecond):
	}
}

// Corrida em que um herói SOBE DE NÍVEL é descartada (o XP do ciclo fica distorcido
// pelo limiar do nível). Detectado pelo nível, não pelo XP.
func TestLevelUpDiscardsRound(t *testing.T) {
	old, _ := os.Getwd()
	os.Chdir(t.TempDir())
	defer os.Chdir(old)
	const stage = 2206
	ctrl := Control{
		UseEMA: true, EMAAlpha: 0.2, HeroLevel: 39, ActiveHeroCount: 1,
		FarmStages:          map[int]FarmStageInfo{stage: {Key: stage, Label: "2-6", Difficulty: "NIGHTMARE", TotalHP: 1000000, ExpectedGold: 36802, ExpectedEXP: 699465, Waves: 17, Level: 39}},
		HeroStates:          map[int]HeroState{201: {Level: 39, Xp: 0}},
		LastCurrentStageKey: stage, LastPlayTime: 1000, LastGold: 500,
	}
	// clear estável, ouro normal, mas o herói sobe L39->L40 -> deve DESCARTAR.
	calculateAndLogRound(&ctrl, newClearSave(stage, 1300, 100500, 201, 40, 3000000))
	if s, _ := ctrl.StageHistory.Get(stage); s != nil && s.TotalRuns != 0 {
		t.Fatalf("corrida com level-up deveria ser descartada; TotalRuns=%d (esperado 0)", s.TotalRuns)
	}

	// A PRÓXIMA corrida (sem level-up) conta normal — a baseline já avançou pro L40.
	ctrl.LastPlayTime = 1300
	ctrl.LastGold = 100500
	calculateAndLogRound(&ctrl, newClearSave(stage, 1600, 200000, 201, 40, 6000000))
	if s, _ := ctrl.StageHistory.Get(stage); s == nil || s.TotalRuns != 1 {
		t.Fatalf("corrida seguinte (sem level-up) deveria contar; %v", s)
	}
}

// PROVA do fix do caco de 7s: um save ignorado (sem ganho novo) NAO move o baseline,
// um fragmento curto demais e descartado, e o proximo clear real e medido da janela
// inteira (do ultimo clear bom), nao do fragmento.
func TestFragmentRejectedAndBaselineKept(t *testing.T) {
	old, _ := os.Getwd()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)

	const stage = 2106
	ctrl := Control{
		UseEMA: true, EMAAlpha: 0.2,
		FarmStages:          map[int]FarmStageInfo{stage: {Key: stage, Label: "2-6", TotalHP: 500000, ExpectedGold: 50000, ExpectedEXP: 3000000, Waves: 20, Level: 40}},
		HeroStates:          map[int]HeroState{201: {Level: 40, Xp: 1000}},
		ActiveHeroCount:     1,
		HeroLevel:           40,
		LastCurrentStageKey: stage,
		LastPlayTime:        1000,
		LastGold:            500,
	}

	// 1) clear real de 262s -> registra, baseline avanca para 1262.
	calculateAndLogRound(&ctrl, newClearSave(stage, 1262, 99429, 201, 40, 5461397))
	if ctrl.LastPlayTime != 1262 {
		t.Fatalf("clear real deveria mover o baseline para 1262, foi %.0f", ctrl.LastPlayTime)
	}

	// 2) save sem ganho novo (duplicado/ocioso) -> ignorado, baseline NAO anda.
	calculateAndLogRound(&ctrl, newClearSave(stage, 1269, 99429, 201, 40, 5461397))
	if ctrl.LastPlayTime != 1262 {
		t.Fatalf("save ignorado moveu o baseline para %.0f (era pra ficar 1262)", ctrl.LastPlayTime)
	}

	// 3) fragmento curto demais (14s vs media 262s) -> descartado, baseline mantido.
	calculateAndLogRound(&ctrl, newClearSave(stage, 1276, 99429+2443, 201, 40, 5461397+144233))
	s, _ := ctrl.StageHistory.Get(stage)
	if s.TotalRuns != 1 {
		t.Fatalf("fragmento de 14s foi registrado (runs=%d, esperado 1)", s.TotalRuns)
	}
	if ctrl.LastPlayTime != 1262 {
		t.Fatalf("fragmento moveu o baseline para %.0f (era pra ficar 1262)", ctrl.LastPlayTime)
	}

	// 4) clear de verdade medido do baseline mantido (1524-1262=262s) -> registra run 2.
	calculateAndLogRound(&ctrl, newClearSave(stage, 1524, 99429+98929, 201, 40, 5461397+5460397))
	s, _ = ctrl.StageHistory.Get(stage)
	if s.TotalRuns != 2 {
		t.Fatalf("clear real apos fragmento nao registrou (runs=%d, esperado 2)", s.TotalRuns)
	}
}
