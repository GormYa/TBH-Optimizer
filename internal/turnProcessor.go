package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// saveDebounce agrupa o burst de eventos que um unico save atomico do ES3 gera.
const saveDebounce = 250 * time.Millisecond

func StartMonitoring(ctrl *Control) {
	archivePath, watcher, err := setupWatcher(ctrl)
	if err != nil {
		fmt.Println("Falha na inicialização do monitoramento:", err)
		return
	}
	defer watcher.Close()

	runDebouncedMonitor(watcher.Events, watcher.Errors, archivePath, saveDebounce, func() {
		processSaveChange(ctrl)
	})
}

func setupWatcher(ctrl *Control) (string, *fsnotify.Watcher, error) {
	currentSave, err := LoadSave()
	if err != nil {
		return "", nil, err
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", nil, err
	}

	ctrl.LastPlayTime = currentSave.CommonSaveData.PlayTime
	ctrl.LastCurrentStageKey = currentSave.CommonSaveData.CurrentStageKey
	ctrl.LastGold = ExtractGold(currentSave.CurrenySaveDatas)
	ctrl.HeroStates = CalibrateHeroStates(currentSave.HeroSaveDatas)
	ctrl.MaxCompletedStage = currentSave.CommonSaveData.MaxCompletedStage
	ctrl.LastItemIds = snapshotItemIds(currentSave.ItemSaveDatas)

	archivePath := homeDir + `\AppData\LocalLow\TesseractStudio\TaskbarHero\SaveFile_Live.es3`
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return "", nil, err
	}

	err = watcher.Add(filepath.Dir(archivePath))
	if err != nil {
		watcher.Close()
		return "", nil, err
	}

	fmt.Printf("Calibrado! Fase Atual: %d | Ouro Inicial: %d | Heróis Calibrados: %d\n",
		ctrl.LastCurrentStageKey, ctrl.LastGold, len(ctrl.HeroStates))

	return archivePath, watcher, nil
}

// isRelevantSaveEvent filtra o ruido do diretorio (Player.log, .tmp, .bak, .vdf):
// so interessa escrita ou criacao do proprio arquivo de save.
func isRelevantSaveEvent(event fsnotify.Event, targetPath string) bool {
	if filepath.Base(event.Name) != filepath.Base(targetPath) {
		return false
	}
	return event.Has(fsnotify.Write) || event.Has(fsnotify.Create)
}

// runDebouncedMonitor e o loop event-driven. Cada save atomico do ES3 dispara um
// burst de eventos no arquivo alvo; em vez de processar a cada evento, reiniciamos
// um timer curto e so processamos quando o burst estabiliza -> 1 corrida = 1 processamento.
func runDebouncedMonitor(events <-chan fsnotify.Event, errs <-chan error, targetPath string, debounce time.Duration, onSave func()) {
	timer := time.NewTimer(debounce)
	if !timer.Stop() {
		<-timer.C
	}
	for {
		select {
		case event, ok := <-events:
			if !ok {
				timer.Stop()
				return
			}
			if isRelevantSaveEvent(event, targetPath) {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(debounce)
			}
		case err, ok := <-errs:
			if !ok {
				timer.Stop()
				return
			}
			fmt.Println("Erro no canal do watcher:", err)
		case <-timer.C:
			onSave()
		}
	}
}

func processSaveChange(ctrl *Control) {
	fmt.Println("Modificação detectada via evento do Sistema Operacional!")

	currentSave, err := loadSaveWithRetry()
	if err != nil {
		fmt.Println("Não foi possível ler o arquivo após 5 tentativas:", err)
		return
	}

	if currentSave.CommonSaveData.CurrentStageWave != 0 {
		return
	}
	if currentSave.CommonSaveData.PlayTime == ctrl.LastPlayTime {
		return
	}

	calculateAndLogRound(ctrl, currentSave)
}

func loadSaveWithRetry() (*InnerSaveData, error) {
	var currentSave *InnerSaveData
	var err error
	for i := 0; i < 10; i++ {
		currentSave, err = LoadSave()
		if err == nil {
			return currentSave, nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	fmt.Println("Falha definitiva ao ler o save após 10 tentativas:", err)
	return nil, err
}

// isValidRound faz as checagens de sanidade da janela medida: tempo minimo,
// ganhos nao-negativos E ganho real (>0). Uma conclusao de fase sempre concede
// ouro/xp; um save em wave 0 SEM conclusao (autosave ocioso, tela de selecao ao
// abrir o app) tem ganho zero -> nao e corrida, nao deve inflar a contagem.
func isValidRound(timeSpent float64, goldGain int, xpGain float64) bool {
	if timeSpent < 3 || goldGain < 0 || xpGain < 0 {
		return false
	}
	return goldGain > 0 || xpGain > 0
}

// timeOutlierFactor: descarta a corrida se o tempo medido passar deste fator
// vezes o tempo estimado da fase (pega o tempo inflado da troca manual de mapa).
const timeOutlierFactor = 3.0

// goldFloorFactor: descarta a corrida se o ouro ficar abaixo deste fator vezes a
// media da fase. Pega MORTE / clear parcial (o ganho parcial fica, mas e bem
// menor que um clear real). So aplica quando a fase ja tem historico.
const goldFloorFactor = 0.5

// expectedClearGold estima o ouro de um clear real da fase: media propria
// (>=3 corridas) ou, no cold start, ExpectedGold da fase x multiplicador de ouro
// do jogador. 0 quando nao ha base alguma (sem piso).
func expectedClearGold(ownAvg float64, ownRuns int, stageExpectedGold, goldMult float64) float64 {
	if ownRuns >= 3 && ownAvg > 0 {
		return ownAvg
	}
	if stageExpectedGold > 0 && goldMult > 0 {
		return stageExpectedGold * goldMult
	}
	return 0
}

// goldMultiplier mede o multiplicador de ouro do jogador (ouro real / ouro base)
// a partir das fases ja medidas. 0 se nao houver base.
func (ctrl *Control) goldMultiplier() float64 {
	var sum float64
	var n int
	for _, st := range ctrl.StageHistory.AllStats() {
		if st.TotalRuns < 3 || st.AvgGoldPerRun <= 0 {
			continue
		}
		if info, ok := ctrl.FarmStages[st.StageKey]; ok && info.ExpectedGold > 0 {
			sum += st.AvgGoldPerRun / info.ExpectedGold
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// estimateClearGold devolve o ouro esperado de um clear da fase (piso anti-morte),
// funcionando ja no cold start via ExpectedGold x multiplicador.
func (ctrl *Control) estimateClearGold(stage int) float64 {
	var ownAvg float64
	var ownRuns int
	if s, ok := ctrl.StageHistory.Get(stage); ok {
		ownAvg = s.AvgGoldPerRun
		ownRuns = s.TotalRuns
	}
	var stageExp float64
	if info, ok := ctrl.FarmStages[stage]; ok {
		stageExp = info.ExpectedGold
	}
	return expectedClearGold(ownAvg, ownRuns, stageExp, ctrl.goldMultiplier())
}

// estimatedRunTime estima quanto uma fase deveria levar (segundos). Prioriza o
// historico proprio da fase (>=3 corridas); senao extrapola por HP a partir de
// uma fase de referencia ja medida (tempo ~ proporcional ao HP total). Retorna 0
// quando nao ha base de comparacao alguma.
func estimatedRunTime(ownAvg float64, ownRuns int, stageHP, refHP, refAvg float64) float64 {
	if ownRuns >= 3 && ownAvg > 0 {
		return ownAvg
	}
	if stageHP > 0 && refHP > 0 && refAvg > 0 {
		return refAvg * (stageHP / refHP)
	}
	return 0
}

// isTimeTrustworthy decide se o tempo da janela e confiavel. Com estimativa
// disponivel, confia enquanto timeSpent <= fator*estimativa: o auto-avanco (tempo
// normal) passa e a troca manual de mapa (tempo inflado pela decisao/ocioso) cai.
// Sem estimativa, so confia se a fase NAO mudou -- a 1a corrida apos uma troca
// carrega o tempo da transicao e e descartada.
func isTimeTrustworthy(timeSpent, estTime, factor float64, stageChanged bool) bool {
	if estTime > 0 {
		return timeSpent <= factor*estTime
	}
	return !stageChanged
}

// estimateStageTime monta os insumos (historico proprio e fase de referencia) e
// delega para estimatedRunTime.
func (ctrl *Control) estimateStageTime(stage int) float64 {
	var ownAvg float64
	var ownRuns int
	if s, ok := ctrl.StageHistory.Get(stage); ok {
		ownAvg = s.AvgTimeSpent
		ownRuns = s.TotalRuns
	}

	var stageHP float64
	if info, ok := ctrl.FarmStages[stage]; ok {
		stageHP = info.TotalHP
	}

	refKey, _, refAvg := ctrl.StageHistory.MostRunStage()
	var refHP float64
	if info, ok := ctrl.FarmStages[refKey]; ok {
		refHP = info.TotalHP
	}

	return estimatedRunTime(ownAvg, ownRuns, stageHP, refHP, refAvg)
}

func calculateAndLogRound(ctrl *Control, currentSave *InnerSaveData) {
	stage := currentSave.CommonSaveData.CurrentStageKey
	timeSpent := currentSave.CommonSaveData.PlayTime - ctrl.LastPlayTime
	goldGain := ExtractGold(currentSave.CurrenySaveDatas) - ctrl.LastGold
	xpGain := ProcessRoundXp(currentSave.HeroSaveDatas, ctrl.HeroStates)

	averageXpPerHero := xpGain / float64(len(ctrl.HeroStates))

	stageChanged := stage != ctrl.LastCurrentStageKey
	estTime := ctrl.estimateStageTime(stage)
	dropCount, dropsByKey := countNewDrops(ctrl.LastItemIds, currentSave.ItemSaveDatas)

	defer func() {
		ctrl.LastPlayTime = currentSave.CommonSaveData.PlayTime
		ctrl.LastCurrentStageKey = currentSave.CommonSaveData.CurrentStageKey
		ctrl.LastGold = ExtractGold(currentSave.CurrenySaveDatas)
		ctrl.MaxCompletedStage = currentSave.CommonSaveData.MaxCompletedStage
		ctrl.LastItemIds = snapshotItemIds(currentSave.ItemSaveDatas)
	}()

	if !isValidRound(timeSpent, goldGain, xpGain) {
		return
	}

	// So registramos corridas em fase ESTAVEL (mesma fase do inicio ao fim da janela).
	// Qualquer mudanca de fase -- troca manual, auto-avanco ou morte que muda de mapa --
	// e ambigua sobre qual fase foi realmente jogada. Descartamos e re-baselizamos (defer);
	// a proxima corrida, ja estavel na fase nova, e registrada corretamente.
	if stageChanged {
		return
	}

	// Tempo inflado (ex.: morreu no meio e completou depois): destoa do esperado -> descarta.
	if !isTimeTrustworthy(timeSpent, estTime, timeOutlierFactor, false) {
		return
	}

	// Morte / clear parcial: ouro bem abaixo do esperado de um clear (histórico ou,
	// no cold start, ExpectedGold x multiplicador do jogador) -> descarta.
	if floor := ctrl.estimateClearGold(stage); floor > 0 && float64(goldGain) < goldFloorFactor*floor {
		return
	}

	xpPerHour := (averageXpPerHero / timeSpent) * 3600
	goldPerHour := (float64(goldGain) / timeSpent) * 3600

	saveStageLog(stage, timeSpent, goldPerHour, xpPerHour)

	ctrl.StageHistory.Update(
		stage,
		timeSpent,
		float64(goldGain),
		xpGain,
		ctrl.UseEMA,
		ctrl.EMAAlpha,
		len(ctrl.HeroStates),
		dropCount,
		dropsByKey,
	)

	if err := ctrl.StageHistory.Save(HistoryFilePath); err != nil {
		fmt.Println("Aviso: falha ao persistir o historico:", err)
	}

	s, exists := ctrl.StageHistory.Get(stage)
	if exists {
		fmt.Printf("Média do Mapa %d -> Corridas: %d | Avg Ouro/h: %.0f | Avg XP/h: %.0f\n",
			stage, s.TotalRuns, s.AvgGoldPerHour, s.AvgXpPerHour)
	}
}

func saveStageLog(stageKey int, timeSpent float64, goldGain float64, xpGain float64) {
	file, err := os.OpenFile("historico_farm.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("Erro ao abrir o bloco de notas:", err)
		return
	}
	defer file.Close()

	logLine := fmt.Sprintf("Estágio Concluído: %d | Tempo Gasto: %.2fs | Ouro/h: %.0f | XP/h: %.0f\n",
		stageKey, timeSpent, goldGain, xpGain)
	_, err = file.WriteString(logLine)
	if err != nil {
		fmt.Println("Erro ao descarregar os dados no bloco de notas:", err)
	}
}
