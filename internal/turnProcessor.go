package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	ctrl.HeroLevel = activeHeroLevel(currentSave)
	ctrl.ActiveHeroCount = len(currentSave.CommonSaveData.ArrangedHeroKey)
	ctrl.ActiveHeroes = activeHeroes(currentSave)
	ctrl.Gold = ctrl.LastGold
	ctrl.RuneLevels = runeLevels(currentSave)

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

	totalWaves := 0
	if info, ok := ctrl.FarmStages[ctrl.LastCurrentStageKey]; ok {
		totalWaves = info.Waves
	}
	Logf("info", "Monitoramento iniciado · Fase atual: %s · wave %d/%d · heróis ativos: %d (nível %d). A corrida é registrada quando a wave volta a 0 (fim do ciclo).",
		ctrl.stageDisplay(ctrl.LastCurrentStageKey), currentSave.CommonSaveData.CurrentStageWave, totalWaves, ctrl.ActiveHeroCount, ctrl.HeroLevel)

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
	currentSave, err := loadSaveWithRetry()
	if err != nil {
		Logf("reject", "Não consegui ler o save (várias tentativas). Tento de novo no próximo evento.")
		return
	}

	if currentSave.CommonSaveData.PlayTime == ctrl.LastPlayTime {
		return
	}
	ctrl.Gold = ExtractGold(currentSave.CurrenySaveDatas)
	ctrl.RuneLevels = runeLevels(currentSave)
	if currentSave.CommonSaveData.CurrentStageWave != 0 {
		stg := currentSave.CommonSaveData.CurrentStageKey
		if stg != ctrl.lastMidStage {
			ctrl.lastMidStage = stg
			total := 0
			if info, ok := ctrl.FarmStages[stg]; ok {
				total = info.Waves
			}
			Logf("info", "Fazendo %s · wave %d/%d — a corrida vai valer quando a wave voltar a 0 (fim do ciclo).",
				ctrl.stageDisplay(stg), currentSave.CommonSaveData.CurrentStageWave, total)
		}
		return
	}

	ctrl.HeroLevel = activeHeroLevel(currentSave)
	ctrl.ActiveHeroCount = len(currentSave.CommonSaveData.ArrangedHeroKey)
	ctrl.ActiveHeroes = activeHeroes(currentSave)
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
	if timeSpent < 3 || xpGain < 0 {
		return false
	}
	return xpGain > 0 || goldGain > 0
}

// timeOutlierFactor: descarta a corrida se o tempo medido passar deste fator
// vezes o tempo estimado da fase (pega o tempo inflado da troca manual de mapa).
const timeOutlierFactor = 3.0

// goldFloorFactor: descarta a corrida se o ouro ficar abaixo deste fator vezes a
// media da fase. Pega MORTE / clear parcial (o ganho parcial fica, mas e bem
// menor que um clear real). So aplica quando a fase ja tem historico.
const goldFloorFactor = 0.5

// goldCeilFactor: descarta a corrida se o ouro passar deste fator vezes a media da
// fase. Pega ganho de ouro que NAO veio do farm (venda de itens, baus grandes, bonus)
// e que senao inflaria a media/h (ex.: um pico de 11M puxando a media movel pra 3M).
const goldCeilFactor = 3.0

// expectedClearGold devolve o ouro esperado de um clear SÓ a partir da média própria
// da fase (>=3 corridas). Sem histórico próprio -> 0 (sem piso). O piso de cold-start
// via ExpectedGold x multiplicador errava feio entre dificuldades (ex.: esperar 1,5M
// numa Nightmare 1-5 que dá 41k) e descartava clears reais. Igual à trava de tempo:
// não julgamos uma fase nova com extrapolação de outras.
func expectedClearGold(ownAvg float64, ownRuns int) float64 {
	if ownRuns >= 3 && ownAvg > 0 {
		return ownAvg
	}
	return 0
}

// estimateClearGold devolve o piso de ouro anti-morte da fase: só a média própria
// medida (>=3 corridas). Cold-start não tem piso (ver expectedClearGold).
func (ctrl *Control) estimateClearGold(stage int) float64 {
	if s, ok := ctrl.StageHistory.Get(stage); ok {
		return expectedClearGold(s.AvgGoldPerRun, s.TotalRuns)
	}
	return 0
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
	if s, ok := ctrl.StageHistory.Get(stage); ok && s.TotalRuns >= 3 && s.AvgTimeSpent > 0 {
		return s.AvgTimeSpent
	}

	return 0
}

// recordedAvgTime devolve a média de tempo já medida da fase (a partir de 1
// corrida). Usada só como piso anti-fragmento, não para estimar fases novas.
func (ctrl *Control) recordedAvgTime(stage int) float64 {
	if s, ok := ctrl.StageHistory.Get(stage); ok && s.TotalRuns >= 1 {
		return s.AvgTimeSpent
	}
	return 0
}

// stageDisplay devolve "Nightmare 1-5" (dificuldade + label) pro console.
func (ctrl *Control) stageDisplay(stage int) string {
	if info, ok := ctrl.FarmStages[stage]; ok && info.Label != "" {
		if d := difficultyLabel(info.Difficulty); d != "" {
			return d + " " + info.Label
		}
		return info.Label
	}
	return fmt.Sprintf("%d", stage)
}

func difficultyLabel(d string) string {
	switch d {
	case "NORMAL":
		return "Normal"
	case "NIGHTMARE":
		return "Nightmare"
	case "HELL":
		return "Hell"
	case "TORMENT":
		return "Torment"
	default:
		return ""
	}
}

func calculateAndLogRound(ctrl *Control, currentSave *InnerSaveData) {
	stage := currentSave.CommonSaveData.CurrentStageKey
	timeSpent := currentSave.CommonSaveData.PlayTime - ctrl.LastPlayTime
	goldGain := ExtractGold(currentSave.CurrenySaveDatas) - ctrl.LastGold
	levelUps := heroLevelUps(currentSave.HeroSaveDatas, ctrl.HeroStates)
	xpGain := computeRoundXp(currentSave.HeroSaveDatas, ctrl.HeroStates)

	stageChanged := stage != ctrl.LastCurrentStageKey
	estTime := ctrl.estimateStageTime(stage)
	dropCount, dropsByKey := countNewDrops(ctrl.LastItemIds, currentSave.ItemSaveDatas)

	defer func() {
		ctrl.LastCurrentStageKey = currentSave.CommonSaveData.CurrentStageKey
		ctrl.MaxCompletedStage = currentSave.CommonSaveData.MaxCompletedStage
		ctrl.LastItemIds = snapshotItemIds(currentSave.ItemSaveDatas)
	}()

	advanceClock := func() {
		ctrl.LastPlayTime = currentSave.CommonSaveData.PlayTime
		ctrl.LastGold = ExtractGold(currentSave.CurrenySaveDatas)
		commitHeroStates(currentSave.HeroSaveDatas, ctrl.HeroStates)
	}

	if !isValidRound(timeSpent, goldGain, xpGain) {
		Logf("info", "Fase %s: save em wave 0 sem ganho NOVO de ouro/xp — ignorado (normalmente um 2º save logo após um clear que já foi contado).", ctrl.stageDisplay(stage))
		return
	}

	if stageChanged {
		advanceClock()
		Logf("reject", "Fase %s descartada: a fase mudou no meio da janela (auto-avanço ou morte que trocou de mapa). Só conto ciclo estável na mesma fase.", ctrl.stageDisplay(stage))
		return
	}

	if avg := ctrl.recordedAvgTime(stage); avg > 0 && timeSpent < avg/timeOutlierFactor {
		Logf("reject", "Fase %s descartada: %.0fs curto demais para um clear (média ~%.0fs) — fragmento de save, não um ciclo completo. Junto com o próximo ciclo.", ctrl.stageDisplay(stage), timeSpent, avg)
		return
	}

	if !isTimeTrustworthy(timeSpent, estTime, timeOutlierFactor, false) {
		advanceClock()
		Logf("reject", "Fase %s descartada: tempo %.0fs muito acima do esperado (~%.0fs). Provável ociosidade/decisão no meio do ciclo.", ctrl.stageDisplay(stage), timeSpent, estTime)
		return
	}

	if len(levelUps) > 0 {
		advanceClock()
		Logf("reject", "Fase %s descartada: %s — o XP de um clear com level-up fica distorcido pelo limiar do nível. A próxima corrida já conta normal.", ctrl.stageDisplay(stage), describeLevelUps(levelUps))
		return
	}

	// goldRec = o ouro que ENTRA na média. Eventos fora do farm distorcem o delta:
	//   - ouro caiu (goldGain<0): compra de runa/item no meio -> corrida real (deu xp),
	//     conto tempo/xp mas neutralizo o ouro (uso a média atual, blend ~no-op).
	//   - ouro baixo demais (positivo): morte/clear parcial -> descarta.
	//   - ouro alto demais: venda/bônus -> descarta.
	goldRec := float64(goldGain)
	if goldGain < 0 {
		if s, ok := ctrl.StageHistory.Get(stage); ok {
			goldRec = s.AvgGoldPerRun
		} else {
			goldRec = 0
		}
		Logf("info", "Fase %s: ouro caiu %d no ciclo — provável compra de runa/item. Conto tempo/xp; o ouro não distorce a média.", ctrl.stageDisplay(stage), goldGain)
	} else if floor := ctrl.estimateClearGold(stage); floor > 0 {
		if float64(goldGain) < goldFloorFactor*floor {
			Logf("reject", "Fase %s descartada: ouro %.0f baixo demais para um clear (média ~%.0f). Provável morte/clear parcial.", ctrl.stageDisplay(stage), float64(goldGain), floor)
			return
		}
		if float64(goldGain) > goldCeilFactor*floor {
			advanceClock()
			Logf("reject", "Fase %s descartada: ouro %.0f MUITO acima da média (~%.0f) — provável venda de itens/bônus, não ouro de farm. Não inflo a média.", ctrl.stageDisplay(stage), float64(goldGain), floor)
			return
		}
	}

	advanceClock()

	xpPerHour := (xpGain / float64(ctrl.numActiveHeroes()) / timeSpent) * 3600
	goldPerHour := (goldRec / timeSpent) * 3600

	saveStageLog(stage, timeSpent, goldPerHour, xpPerHour)

	ctrl.StageHistory.Update(
		stage,
		timeSpent,
		goldRec,
		xpGain,
		ctrl.UseEMA,
		ctrl.EMAAlpha,
		ctrl.numActiveHeroes(),
		dropCount,
		dropsByKey,
	)

	if err := ctrl.StageHistory.Save(HistoryFilePath); err != nil {
		fmt.Println("Aviso: falha ao persistir o historico:", err)
	}

	s, exists := ctrl.StageHistory.Get(stage)
	if exists {
		Logf("run", "Fase %s REGISTRADA: %.0fs · +%.0f ouro · +%.0f xp · total de corridas: %d",
			ctrl.stageDisplay(stage), timeSpent, goldRec, xpGain, s.TotalRuns)
	}
}

// describeLevelUps monta a frase do console listando quem subiu e de qual
// nivel para qual (ex.: "o heroi Cavaleiro (nivel 39 -> 40) subiu de nivel").
func describeLevelUps(ups []HeroLevelUp) string {
	parts := make([]string, len(ups))
	for i, u := range ups {
		parts[i] = fmt.Sprintf("%s (nível %d → %d)", heroName(u.Key), u.From, u.To)
	}
	joined := strings.Join(parts, ", ")
	if len(ups) == 1 {
		return "o herói " + joined + " subiu de nível"
	}
	return "os heróis " + joined + " subiram de nível"
}

func saveStageLog(stageKey int, timeSpent float64, goldGain float64, xpGain float64) {
	file, err := os.OpenFile("historico_farm.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("Erro ao abrir o bloco de notas:", err)
		return
	}
	defer file.Close()

	logLine := fmt.Sprintf("[%s] Estágio Concluído: %d | Tempo Gasto: %.2fs | Ouro/h: %.0f | XP/h: %.0f\n",
		time.Now().Format("2006-01-02 15:04:05"), stageKey, timeSpent, goldGain, xpGain)
	_, err = file.WriteString(logLine)
	if err != nil {
		fmt.Println("Erro ao descarregar os dados no bloco de notas:", err)
	}
}
