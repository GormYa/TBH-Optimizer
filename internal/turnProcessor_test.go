package internal

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

const testSavePath = `C:\Users\x\AppData\LocalLow\TesseractStudio\TaskbarHero\SaveFile_Live.es3`

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
		{"ouro negativo (anomalia)", 60, -1, 500, false},
		{"xp negativo (anomalia)", 60, 1000, -1, false},
	}
	for _, c := range cases {
		if got := isValidRound(c.timeSpent, c.goldGain, c.xpGain); got != c.want {
			t.Errorf("%s: isValidRound = %v, quero %v", c.desc, got, c.want)
		}
	}
}

// expectedClearGold estima o ouro de um clear real da fase: media propria
// (>=3 corridas) ou, no cold start, ExpectedGold da fase x multiplicador de ouro
// do jogador (medido em outras fases). 0 quando nao ha base -> sem piso.
func TestExpectedClearGold(t *testing.T) {
	cases := []struct {
		desc     string
		ownAvg   float64
		ownRuns  int
		stageExp float64
		goldMult float64
		want     float64
	}{
		{"usa media propria com historico", 3000, 5, 2593, 1.2, 3000},
		{"cold start: ExpectedGold x multiplicador", 0, 0, 2593, 1.2, 2593 * 1.2},
		{"poucas corridas usa estimativa", 999, 2, 2000, 1.5, 3000},
		{"sem base alguma -> 0 (sem piso)", 0, 0, 0, 0, 0},
		{"sem multiplicador -> 0", 0, 0, 2000, 0, 0},
	}
	for _, c := range cases {
		if got := expectedClearGold(c.ownAvg, c.ownRuns, c.stageExp, c.goldMult); got != c.want {
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
