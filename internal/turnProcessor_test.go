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
	s.HeroSaveDatas = []Hero{{HeroKey: heroKey, HeroLevel: heroLvl, HeroExp: FlexFloat(heroExp)}}
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

	// saves de meio de ciclo viram as waves rodando no PROPRIO sA (auto-avanco real)
	ctrl.midWindowStage = sA

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

func TestReanchorMidCycleDiscardsFirstPartial(t *testing.T) {
	old, _ := os.Getwd()
	if err := os.Chdir(t.TempDir()); err != nil {
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
		LastCurrentStageKey: stage,
		LastPlayTime:        1000,
		LastGold:            500,
		reanchorWave:        5, // re-ancorei no MEIO do ciclo (wave 5) -> 1a volta é parcial
	}
	// 1o fechamento após a re-âncora: parcial (tempo menor que o real), deve ser descartado
	calculateAndLogRound(&ctrl, newClearSave(stage, 1148, 10500, 201, 33, 501000))
	if s, ok := ctrl.StageHistory.Get(stage); ok && s.TotalRuns > 0 {
		t.Fatalf("1a volta após re-âncora no meio do ciclo deveria ser descartada; runs=%d", s.TotalRuns)
	}
	if ctrl.reanchorWave != 0 {
		t.Fatal("reanchorWave deveria ter sido limpo após o descarte")
	}
	// próxima volta completa -> conta normal
	calculateAndLogRound(&ctrl, newClearSave(stage, 1296, 20500, 201, 33, 1001000))
	s, ok := ctrl.StageHistory.Get(stage)
	runs := 0
	if s != nil {
		runs = s.TotalRuns
	}
	if !ok || runs != 1 {
		t.Fatalf("a 2a volta (completa) deveria contar; runs=%d", runs)
	}
}

func TestReanchorWave1AlsoDiscards(t *testing.T) {
	// O ciclo começa na wave 0, então re-ancorar já na wave 1 significa que perdemos a
	// wave 0 -> a 1a volta é parcial e DEVE ser descartada (não é o início real).
	old, _ := os.Getwd()
	if err := os.Chdir(t.TempDir()); err != nil {
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
		LastCurrentStageKey: stage,
		LastPlayTime:        1000,
		LastGold:            500,
		reanchorWave:        1, // peguei o mapa novo na wave 1 -> já passei da wave 0 (início)
	}
	calculateAndLogRound(&ctrl, newClearSave(stage, 1148, 10500, 201, 33, 501000))
	if s, ok := ctrl.StageHistory.Get(stage); ok && s.TotalRuns > 0 {
		t.Fatalf("re-âncora na wave 1 (perdeu a wave 0): a 1a volta deveria ser descartada; runs=%d", s.TotalRuns)
	}
	if ctrl.reanchorWave != 0 {
		t.Fatal("reanchorWave deveria ter sido limpo após o descarte")
	}
	// próxima volta completa -> conta
	calculateAndLogRound(&ctrl, newClearSave(stage, 1296, 20500, 201, 33, 1001000))
	s, ok := ctrl.StageHistory.Get(stage)
	runs := 0
	if s != nil {
		runs = s.TotalRuns
	}
	if !ok || runs != 1 {
		t.Fatalf("a 2a volta (completa) deveria contar; runs=%d", runs)
	}
}

func TestRuneSpendRecoversGold(t *testing.T) {
	old, _ := os.Getwd()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)

	// tabela de custo de runa (normalmente vem de runes.json); restaura no fim
	savedCosts := runeLevelCost
	runeLevelCost = map[int]map[int]float64{10: {1: 200, 2: 300, 3: 500}} // total 0->3 = 1000
	defer func() { runeLevelCost = savedCosts }()

	const stage = 1308
	ctrl := Control{
		UseEMA: true, EMAAlpha: 0.2,
		FarmStages:          map[int]FarmStageInfo{stage: {Key: stage, Label: "3-8", TotalHP: 100000, ExpectedGold: 4000, ExpectedEXP: 60000, Waves: 17, Level: 30}},
		HeroStates:          map[int]HeroState{201: {Level: 33, Xp: 1000}},
		ActiveHeroCount:     1,
		HeroLevel:           33,
		LastCurrentStageKey: stage,
		LastPlayTime:        1000,
		LastGold:            1000,            // tinha 1000 de ouro no início
		LastRuneLevels:      map[int]int{10: 0}, // runa 10 no nível 0
	}
	// Ciclo: ganhou 800 de ouro E subiu a runa 10 pro nível 3 (gastou 1000).
	// Ouro final = 1000 + 800 - 1000 = 800 -> delta = -200 (negativo).
	// recuperado = delta + gasto = -200 + 1000 = 800 (= o ganho real).
	save := newClearSave(stage, 1148, 800, 201, 33, 501000)
	save.RuneSaveDatas = []RuneSave{{RuneKey: 10, Level: 3}}

	calculateAndLogRound(&ctrl, save)

	s, ok := ctrl.StageHistory.Get(stage)
	if !ok || s.TotalRuns != 1 {
		runs := 0
		if s != nil {
			runs = s.TotalRuns
		}
		t.Fatalf("a corrida deveria contar (ouro recuperado), ok=%v runs=%d", ok, runs)
	}
	if s.AvgGoldPerRun != 800 {
		t.Fatalf("esperava ouro real recuperado = 800 (delta -200 + gasto 1000), veio %.0f", s.AvgGoldPerRun)
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

// TROCA MANUAL DISFARCADA DE AUTO-AVANCO: o jogador comeca sA, troca pra sB no meio
// e conclui sB. O save de wave 0 chega identico a um auto-avanco sA->sB, mas os saves
// de meio de ciclo (testemunha) mostraram as waves rodando em sB -- a janela mistura
// tempo dos dois mapas e nao pode ser creditada em sA (nem em sB).
func TestMidWindowWitnessRejectsFakeAutoAdvance(t *testing.T) {
	old, _ := os.Getwd()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)

	const sA, sB = 2206, 2207
	ctrl := Control{
		UseEMA: true, EMAAlpha: 0.2,
		FarmStages: map[int]FarmStageInfo{
			sA: {Key: sA, TotalHP: 100000, ExpectedGold: 4000, ExpectedEXP: 60000, Waves: 17, Level: 30},
			sB: {Key: sB, TotalHP: 110000, ExpectedGold: 4200, ExpectedEXP: 62000, Waves: 17, Level: 31},
		},
		HeroStates:          map[int]HeroState{201: {Level: 33, Xp: 0}},
		ActiveHeroCount:     1,
		HeroLevel:           33,
		LastCurrentStageKey: sA,
		LastPlayTime:        1000,
		LastGold:            500,
	}
	ctrl.midWindowStage = sB // save de meio de ciclo viu as waves rodando em sB

	calculateAndLogRound(&ctrl, newClearSave(sB, 1256, 60000, 201, 33, 4000000))

	if a, ok := ctrl.StageHistory.Get(sA); ok && a.TotalRuns > 0 {
		t.Fatalf("janela mista não deveria creditar sA; runs=%d", a.TotalRuns)
	}
	if b, ok := ctrl.StageHistory.Get(sB); ok && b.TotalRuns > 0 {
		t.Fatalf("janela mista não deveria creditar sB; runs=%d", b.TotalRuns)
	}
	if ctrl.LastPlayTime != 1256 {
		t.Fatalf("descarte deveria re-sincronizar o relógio (1256), foi %.0f", ctrl.LastPlayTime)
	}
	if ctrl.midWindowStage != 0 || ctrl.midWindowMixed {
		t.Fatal("testemunha deveria zerar ao fechar a janela")
	}

	// Janela que viu DOIS mapas no meio (sA -> sB -> volta pra sA e conclui): mesmo
	// com o wave-0 no proprio sA, a mistura descarta.
	ctrl.LastCurrentStageKey = sA
	ctrl.midWindowStage = sA
	ctrl.midWindowMixed = true
	calculateAndLogRound(&ctrl, newClearSave(sA, 1556, 120000, 201, 33, 8000000))
	if a, ok := ctrl.StageHistory.Get(sA); ok && a.TotalRuns > 0 {
		t.Fatalf("janela com mapas misturados não deveria creditar sA; runs=%d", a.TotalRuns)
	}
}

// FANTASMA POS-RESYNC: depois de uma troca manual descartada, um save atrasado chega
// segundos depois com ganhos (recompensa do mapa anterior creditada tarde) numa fase
// SEM historico proprio. Sem media propria, o piso anti-fragmento usa o tempo estimado
// por regressao: 6s nao e uma corrida -- descarta e re-sincroniza (nao soma no proximo
// ciclo, que herdaria xp de outro mapa).
func TestGhostFragmentOnNewStageDiscarded(t *testing.T) {
	old, _ := os.Getwd()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)

	const s1, s2, sNew = 2206, 2208, 2307
	ctrl := Control{
		UseEMA: true, EMAAlpha: 0.2,
		FarmStages: map[int]FarmStageInfo{
			s1:   {Key: s1, TotalHP: 100000, ExpectedGold: 4000, ExpectedEXP: 60000, Waves: 17, Level: 30},
			s2:   {Key: s2, TotalHP: 500000, ExpectedGold: 5000, ExpectedEXP: 70000, Waves: 20, Level: 32},
			sNew: {Key: sNew, TotalHP: 120000, ExpectedGold: 4500, ExpectedEXP: 65000, Waves: 23, Level: 47},
		},
		HeroStates:          map[int]HeroState{201: {Level: 55, Xp: 1000}},
		ActiveHeroCount:     1,
		HeroLevel:           55,
		LastCurrentStageKey: sNew, // o resync da troca manual deixou o baseline aqui
		LastPlayTime:        1000,
		LastGold:            500,
	}
	// outras fases medidas calibram a regressao de tempo (a*HP + b*ondas)
	ctrl.StageHistory.Update(s1, 148, 10000, 500000, true, 0.2, 1, 55, 0, nil)
	ctrl.StageHistory.Update(s2, 262, 12000, 600000, true, 0.2, 1, 55, 0, nil)

	// 6s depois do resync, save em wave 0 com +9240 ouro e +507714 xp -> fantasma.
	calculateAndLogRound(&ctrl, newClearSave(sNew, 1006, 500+9240, 201, 55, 1000+507714))

	if s, ok := ctrl.StageHistory.Get(sNew); ok && s.TotalRuns > 0 {
		t.Fatalf("fantasma de 6s não deveria virar corrida; runs=%d", s.TotalRuns)
	}
	if ctrl.LastPlayTime != 1006 {
		t.Fatalf("fantasma deveria re-sincronizar o relógio (1006), foi %.0f", ctrl.LastPlayTime)
	}
}

// newDeathCtrl monta o cenario do teto anti-morte: uma fase de referencia ja medida
// com carimbo de nivel (multiplicador de xp = 9) e uma fase NOVA sem media propria.
func newDeathCtrl(sRef, sNew int) *Control {
	ctrl := &Control{
		UseEMA: true, EMAAlpha: 0.2,
		FarmStages: map[int]FarmStageInfo{
			sRef: {Key: sRef, TotalHP: 100000, ExpectedGold: 4000, ExpectedEXP: 60000, Waves: 17, Level: 53},
			sNew: {Key: sNew, TotalHP: 120000, ExpectedGold: 4500, ExpectedEXP: 65000, Waves: 23, Level: 54},
		},
		HeroStates:          map[int]HeroState{201: {Level: 55, Xp: 1000}},
		ActiveHeroCount:     1,
		HeroLevel:           55,
		LastCurrentStageKey: sNew,
		LastPlayTime:        1000,
		LastGold:            20000000,
	}
	// referencia: 3 corridas carimbadas no nivel 55, retencao 100% -> multiplicador 9
	for range 3 {
		ctrl.StageHistory.Update(sRef, 250, 10000, 540000, true, 0.2, 1, 55, 0, nil)
	}
	return ctrl
}

// JANELA COM MORTES NUMA FASE NOVA: 2 falhas tardias + 1 clear rendem ~3x o xp de um
// clear, o ouro fica negativo por uma compra de runa no meio (morte nao mexe no ouro)
// e a fase nao tem media propria pra nenhum piso/teto. O clear PROJETADO pelas outras
// fases medidas (esperado x multiplicador x retencao) e o teto que descarta a janela.
func TestDeathWindowOnNewStageRejected(t *testing.T) {
	old, _ := os.Getwd()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)

	const sRef, sNew = 2206, 2309
	ctrl := newDeathCtrl(sRef, sNew)

	// projetado de sNew: 65000 x 9 x ret(54,55)=1.0 = 585000. Janela de 727s com
	// +1.700.000 xp (~2,9x) e ouro -12.471.800 (compra de runa) -> descarta.
	calculateAndLogRound(ctrl, newClearSave(sNew, 1727, 20000000-12471800, 201, 55, 1000+1700000))

	if s, ok := ctrl.StageHistory.Get(sNew); ok && s.TotalRuns > 0 {
		t.Fatalf("janela com mortes não deveria virar corrida; runs=%d", s.TotalRuns)
	}
	if ctrl.LastPlayTime != 1727 {
		t.Fatalf("descarte deveria re-sincronizar o relógio (1727), foi %.0f", ctrl.LastPlayTime)
	}
}

// O teto anti-morte NAO pode rejeitar um clear normal de fase nova: xp ~1x o projetado
// registra de primeira, como sempre.
func TestNormalClearOnNewStagePassesXpCeiling(t *testing.T) {
	old, _ := os.Getwd()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)

	const sRef, sNew = 2206, 2309
	ctrl := newDeathCtrl(sRef, sNew)

	// clear limpo: 280s, +4600 ouro, +610000 xp (~1,04x o projetado de 585000)
	calculateAndLogRound(ctrl, newClearSave(sNew, 1280, 20000000+4600, 201, 55, 1000+610000))

	s, ok := ctrl.StageHistory.Get(sNew)
	if !ok || s.TotalRuns != 1 {
		t.Fatalf("clear normal de fase nova deveria registrar; %v", s)
	}
	if s.AvgXpPerRun != 610000 {
		t.Fatalf("xp registrado = %.0f, esperado 610000", s.AvgXpPerRun)
	}
}

// trackMidWave: waves subindo no mesmo mapa = janela limpa; mapa diferente marca
// janela mista; wave RECUANDO no mesmo mapa marca reinicio (morte/recomeco).
func TestTrackMidWave(t *testing.T) {
	ctrl := &Control{}

	ctrl.trackMidWave(2206, 3)
	ctrl.trackMidWave(2206, 9)
	ctrl.trackMidWave(2206, 15)
	if ctrl.midWindowMixed || ctrl.midWindowRestart {
		t.Fatalf("waves subindo no mesmo mapa não deveriam marcar nada; mixed=%v restart=%v", ctrl.midWindowMixed, ctrl.midWindowRestart)
	}

	ctrl.trackMidWave(2206, 4) // recuou 15 -> 4: a fase reiniciou
	if !ctrl.midWindowRestart {
		t.Fatal("wave recuando no mesmo mapa deveria marcar reinício")
	}

	ctrl2 := &Control{}
	ctrl2.trackMidWave(2206, 10)
	ctrl2.trackMidWave(2207, 2) // outro mapa: mistura, mas nao e reinicio
	if !ctrl2.midWindowMixed {
		t.Fatal("mapa diferente na mesma janela deveria marcar mistura")
	}
	if ctrl2.midWindowRestart {
		t.Fatal("trocar de mapa não é reinício de wave")
	}
}

// MORTE VISTA PELA WAVE: o save de meio de ciclo mostrou wave 20 e depois wave 4 no
// MESMO mapa -> a fase reiniciou (morte). A janela do wave 0 soma as tentativas e
// nao pode entrar na media, mesmo com ouro/xp/tempo dentro das outras travas.
func TestWaveRegressionDiscardsDeathWindow(t *testing.T) {
	old, _ := os.Getwd()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)

	const stage = 2206
	ctrl := Control{
		UseEMA: true, EMAAlpha: 0.2,
		FarmStages:          map[int]FarmStageInfo{stage: {Key: stage, TotalHP: 100000, ExpectedGold: 4000, ExpectedEXP: 60000, Waves: 24, Level: 53}},
		HeroStates:          map[int]HeroState{201: {Level: 55, Xp: 1000}},
		ActiveHeroCount:     1,
		HeroLevel:           55,
		LastCurrentStageKey: stage,
		LastPlayTime:        1000,
		LastGold:            500,
	}
	for range 3 {
		ctrl.StageHistory.Update(stage, 250, 10000, 500000, true, 0.2, 1, 55, 0, nil)
	}

	ctrl.trackMidWave(stage, 20)
	ctrl.trackMidWave(stage, 4) // morreu na 20, recomecou

	// janela de 700s (2 tentativas + clear), ganhos que passariam nas outras travas
	calculateAndLogRound(&ctrl, newClearSave(stage, 1700, 12000, 201, 55, 1000+900000))

	if s, _ := ctrl.StageHistory.Get(stage); s.TotalRuns != 3 {
		t.Fatalf("janela com reinício de wave deveria ser descartada; runs=%d (esperado 3)", s.TotalRuns)
	}
	if ctrl.LastPlayTime != 1700 {
		t.Fatalf("descarte deveria re-sincronizar o relógio (1700), foi %.0f", ctrl.LastPlayTime)
	}
	if ctrl.midWindowRestart || ctrl.midWindowMaxWave != 0 {
		t.Fatal("os marcadores de wave deveriam zerar ao fechar a janela")
	}
}

// COMPRA + MORTE EM FASE ESTABELECIDA: ouro negativo (compra de runa) desliga o
// piso/teto de ouro, mas o xp 3x a media propria entrega a janela com tentativas
// multiplas -> descarta. E o caso do 3-9 numa fase que JA tem historico.
func TestNegativeGoldWithInflatedXpRejected(t *testing.T) {
	old, _ := os.Getwd()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)

	const stage = 2206
	ctrl := Control{
		UseEMA: true, EMAAlpha: 0.2,
		FarmStages:          map[int]FarmStageInfo{stage: {Key: stage, TotalHP: 100000, ExpectedGold: 4000, ExpectedEXP: 60000, Waves: 24, Level: 53}},
		HeroStates:          map[int]HeroState{201: {Level: 55, Xp: 1000}},
		ActiveHeroCount:     1,
		HeroLevel:           55,
		LastCurrentStageKey: stage,
		LastPlayTime:        1000,
		LastGold:            20000000,
	}
	for range 3 {
		ctrl.StageHistory.Update(stage, 250, 10000, 500000, true, 0.2, 1, 55, 0, nil)
	}

	// 700s, ouro -12M (compra), xp 1.5M (3x a média de 500k) -> descarta
	calculateAndLogRound(&ctrl, newClearSave(stage, 1700, 20000000-12000000, 201, 55, 1000+1500000))

	if s, _ := ctrl.StageHistory.Get(stage); s.TotalRuns != 3 {
		t.Fatalf("compra escondendo morte deveria ser descartada; runs=%d (esperado 3)", s.TotalRuns)
	}
	if ctrl.LastPlayTime != 1700 {
		t.Fatalf("descarte deveria re-sincronizar o relógio (1700), foi %.0f", ctrl.LastPlayTime)
	}

	// Compra LEGITIMA (xp normal ~1x) continua contando com o ouro neutralizado.
	calculateAndLogRound(&ctrl, newClearSave(stage, 1955, 8000000-30000, 201, 55, 1000+1500000+510000))

	s, _ := ctrl.StageHistory.Get(stage)
	if s.TotalRuns != 4 {
		t.Fatalf("compra com xp normal deveria contar; runs=%d (esperado 4)", s.TotalRuns)
	}
}

// TENTATIVAS DE BOSS SEM TESTEMUNHA: o jogador tenta o boss do ato 3x entre
// autosaves (nenhum save flagra o outro mapa) e volta a farmar. A janela fecha com
// ganhos de UM clear normal mas tempo somando as tentativas. Caso real: 3-9 com
// média ~260s registrou 432s (+454k ouro, +21,5M xp — ambos ~1x a média).
// Descarta; mas se o tempo alto se repetir em 3 janelas seguidas, é o novo ritmo
// real da fase (time mais fraco) e a 3ª registra.
func TestBossAttemptsInflatedTimeRejected(t *testing.T) {
	old, _ := os.Getwd()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)

	const stage = 2309
	ctrl := Control{
		UseEMA: true, EMAAlpha: 0.2,
		FarmStages:          map[int]FarmStageInfo{stage: {Key: stage, TotalHP: 2000000, ExpectedGold: 50000, ExpectedEXP: 2400000, Waves: 24, Level: 52}},
		HeroStates:          map[int]HeroState{201: {Level: 55, Xp: 1000}},
		ActiveHeroCount:     1,
		HeroLevel:           55,
		LastCurrentStageKey: stage,
		LastPlayTime:        1000,
		LastGold:            500,
	}
	for range 3 {
		ctrl.StageHistory.Update(stage, 260, 450000, 21500000, true, 0.2, 1, 55, 0, nil)
	}

	// 1ª janela contaminada: 432s, ouro e xp de um clear só -> descarta e re-sincroniza
	calculateAndLogRound(&ctrl, newClearSave(stage, 1432, 500+454332, 201, 55, 1000+21508410))
	if s, _ := ctrl.StageHistory.Get(stage); s.TotalRuns != 3 {
		t.Fatalf("janela com tempo de tentativas de boss deveria ser descartada; runs=%d (esperado 3)", s.TotalRuns)
	}
	if ctrl.LastPlayTime != 1432 {
		t.Fatalf("descarte deveria re-sincronizar o relógio (1432), foi %.0f", ctrl.LastPlayTime)
	}

	// 2ª janela igual: ainda descarta
	calculateAndLogRound(&ctrl, newClearSave(stage, 1864, 500+2*454332, 201, 55, 1000+2*21508410))
	if s, _ := ctrl.StageHistory.Get(stage); s.TotalRuns != 3 {
		t.Fatalf("2ª janela contaminada seguida deveria ser descartada; runs=%d (esperado 3)", s.TotalRuns)
	}

	// 3ª seguida: consistente demais pra ser tentativa — é o ritmo novo, registra
	calculateAndLogRound(&ctrl, newClearSave(stage, 2296, 500+3*454332, 201, 55, 1000+3*21508410))
	s, _ := ctrl.StageHistory.Get(stage)
	if s.TotalRuns != 4 {
		t.Fatalf("3ª janela lenta seguida deveria registrar (novo ritmo); runs=%d (esperado 4)", s.TotalRuns)
	}
	if len(ctrl.timeContamStreak) != 0 {
		t.Fatalf("registrar deveria zerar o streak da fase; streak=%v", ctrl.timeContamStreak)
	}

	// Corrida normal (260s, ganhos ~1x) continua registrando como sempre.
	calculateAndLogRound(&ctrl, newClearSave(stage, 2556, 500+3*454332+450000, 201, 55, 1000+3*21508410+21500000))
	if s, _ := ctrl.StageHistory.Get(stage); s.TotalRuns != 5 {
		t.Fatalf("corrida normal após a sequência deveria contar; runs=%d (esperado 5)", s.TotalRuns)
	}
}

// TIME MISTO UPANDO HERÓI NOVO EM MAPA BAIXO: herói 55 + herói 5 farmando a 1-4
// (nível 4). A retenção do 55 é o piso (1%), mas a do herói 5 é 100% — projetar o
// clear pelo nível MÁXIMO do time dava ~30 xp e descartava corrida legítima como
// "mortes/tentativas múltiplas" (caso real do console). A projeção agora usa a
// retenção média do time ativo.
func TestPowerLevelingMixedTeamPassesXpCeiling(t *testing.T) {
	old, _ := os.Getwd()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)

	const sRef, sNew = 2206, 1104
	mixedCtrl := func(activeHeroes []ActiveHero, heroStates map[int]HeroState, n int) *Control {
		ctrl := &Control{
			UseEMA: true, EMAAlpha: 0.2,
			FarmStages: map[int]FarmStageInfo{
				sRef: {Key: sRef, TotalHP: 100000, ExpectedGold: 4000, ExpectedEXP: 60000, Waves: 17, Level: 53},
				sNew: {Key: sNew, TotalHP: 8000, ExpectedGold: 300, ExpectedEXP: 324, Waves: 24, Level: 4},
			},
			HeroStates:          heroStates,
			ActiveHeroCount:     n,
			ActiveHeroes:        activeHeroes,
			HeroLevel:           55,
			LastCurrentStageKey: sNew,
			LastPlayTime:        1000,
			LastGold:            500,
		}
		// referência carimbada no 55 com retenção 100% -> multiplicador ~9
		for range 3 {
			ctrl.StageHistory.Update(sRef, 250, 10000, 540000, true, 0.2, 1, 55, 0, nil)
		}
		return ctrl
	}

	// time [55, 5]: retenção média ~0,505 -> projeção ~1472; clear real de ~1400 xp passa
	ctrl := mixedCtrl(
		[]ActiveHero{{Key: 201, Level: 55}, {Key: 202, Level: 5}},
		map[int]HeroState{201: {Level: 55, Xp: 1000}, 202: {Level: 5, Xp: 100}}, 2)
	save := newClearSave(sNew, 1046, 500+320, 201, 55, 1000+20)
	save.CommonSaveData.ArrangedHeroKey = []int{201, 202}
	save.HeroSaveDatas = append(save.HeroSaveDatas, Hero{HeroKey: 202, HeroLevel: 5, HeroExp: 100 + 1380})
	calculateAndLogRound(ctrl, save)
	if s, _ := ctrl.StageHistory.Get(sNew); s.TotalRuns != 1 {
		t.Fatalf("clear legítimo de power-leveling deveria registrar; runs=%d (esperado 1)", s.TotalRuns)
	}

	// contra-caso: time só de 55 (retenção 1%) com o MESMO xp continua descartado
	ctrl2 := mixedCtrl(
		[]ActiveHero{{Key: 201, Level: 55}, {Key: 203, Level: 55}},
		map[int]HeroState{201: {Level: 55, Xp: 1000}, 203: {Level: 55, Xp: 1000}}, 2)
	save2 := newClearSave(sNew, 1046, 500+320, 201, 55, 1000+700)
	save2.CommonSaveData.ArrangedHeroKey = []int{201, 203}
	save2.HeroSaveDatas = append(save2.HeroSaveDatas, Hero{HeroKey: 203, HeroLevel: 55, HeroExp: 1000 + 700})
	calculateAndLogRound(ctrl2, save2)
	if s, ok := ctrl2.StageHistory.Get(sNew); ok && s.TotalRuns != 0 {
		t.Fatalf("xp 1400 com time todo 55 (projeção ~59) deveria continuar descartado; runs=%d", s.TotalRuns)
	}
}
