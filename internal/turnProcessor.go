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
	ctrl.OwnedPets = ownedPets(currentSave)
	ctrl.ActivePet = currentSave.CommonSaveData.ArrangedPetKey
	ctrl.primeFirstClear = currentSave.CommonSaveData.CurrentStageWave != 0

	InitializeChestLookup(ctrl.WebFiles, ctrl.GameDataDir)
	if hist, err := LoadChestHistory(); err == nil {
		ctrl.ChestHistory = hist
	}
	ctrl.LastBoxQuantity = make(map[int64]int)
	ctrl.LastBoxTypes = make(map[int64]int)
	for i, uniqueId := range currentSave.BoxData.BoxUniqueId {
		if uniqueId != 0 {
			ctrl.LastBoxQuantity[uniqueId] = currentSave.BoxData.BoxQuantity[i]
			ctrl.LastBoxTypes[uniqueId] = currentSave.BoxData.BoxTypes[i]
		}
	}

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
	ctrl.OwnedPets = ownedPets(currentSave)
	ctrl.ActivePet = currentSave.CommonSaveData.ArrangedPetKey
	ProcessChestDrops(ctrl, currentSave)
	if currentSave.CommonSaveData.CurrentStageWave != 0 {
		stg := currentSave.CommonSaveData.CurrentStageKey
		ctrl.trackMidWave(stg, currentSave.CommonSaveData.CurrentStageWave)
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

func (ctrl *Control) trackMidWave(stg, wave int) {
	if ctrl.midWindowStage == stg {
		if wave < ctrl.midWindowMaxWave {
			ctrl.midWindowRestart = true
		}
	} else {
		if ctrl.midWindowStage != 0 {
			ctrl.midWindowMixed = true
		}
		ctrl.midWindowStage = stg
		ctrl.midWindowMaxWave = 0
	}
	if wave > ctrl.midWindowMaxWave {
		ctrl.midWindowMaxWave = wave
	}
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

func isValidRound(timeSpent float64, goldGain int, xpGain float64) bool {
	if timeSpent < 3 || xpGain < 0 {
		return false
	}
	return xpGain > 0 || goldGain > 0
}

const timeOutlierFactor = 3.0

// Janela com ganhos de UM clear mas tempo bem acima da própria média = o tempo
// extra foi gasto em OUTRO mapa sem nenhum save testemunhar (tentativas curtas de
// boss entre autosaves). Acima de timeContamFactor x a média própria descarta;
// na timeContamAccept-ésima janela seguida assim, aceita — repetição consistente
// não é tentativa de boss, é o ritmo real da fase que mudou (time mais fraco).
const timeContamFactor = 1.4

const timeContamAccept = 3

const goldFloorFactor = 0.5

const goldCeilFactor = 3.0

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

// estimateClearXp e o analogo do ouro pro XP: media propria do clear (>=3 corridas).
// Serve pra distinguir venda de item (ouro sobe sozinho) de evolucao de poder (ouro
// E XP sobem juntos) quando o ouro estoura o teto da media.
func (ctrl *Control) estimateClearXp(stage int) float64 {
	if s, ok := ctrl.StageHistory.Get(stage); ok && s.TotalRuns >= 3 && s.AvgXpPerRun > 0 {
		return s.AvgXpPerRun
	}
	return 0
}

const xpCeilFactor = 2.0

// projectedClearXp projeta o XP de UM clear da fase: esperado dos dados do jogo x
// multiplicador medido (mediana das fases com carimbo de nivel) x retencao MEDIA
// do time ativo.
func (ctrl *Control) projectedClearXp(stage int) float64 {
	info, ok := ctrl.FarmStages[stage]
	if !ok || info.ExpectedEXP <= 0 {
		return 0
	}
	xm, measured := xpMultiplierMeasured(ctrl.StageHistory.AllStats(), ctrl.FarmStages)
	if !measured {
		return 0
	}
	return info.ExpectedEXP * xm * ctrl.meanActiveRetention(info.Level)
}

// meanActiveRetention é a média das retenções de XP dos heróis ATIVOS. A retenção
// é por herói: num time misto (upando herói novo em mapa baixo) o herói baixo
// retém ~100% enquanto os altos ficam no piso de 1% — projetar pelo nível máximo
// do time subestima o clear em dezenas de vezes e descarta corrida legítima.
func (ctrl *Control) meanActiveRetention(stageLevel int) float64 {
	if len(ctrl.ActiveHeroes) == 0 {
		return expRetention(stageLevel, ctrl.HeroLevel)
	}
	sum := 0.0
	for _, h := range ctrl.ActiveHeroes {
		sum += expRetention(stageLevel, h.Level)
	}
	return sum / float64(len(ctrl.ActiveHeroes))
}

func estimatedRunTime(ownAvg float64, ownRuns int, stageHP, refHP, refAvg float64) float64 {
	if ownRuns >= 3 && ownAvg > 0 {
		return ownAvg
	}
	if stageHP > 0 && refHP > 0 && refAvg > 0 {
		return refAvg * (stageHP / refHP)
	}
	return 0
}

func isTimeTrustworthy(timeSpent, estTime, factor float64, stageChanged bool) bool {
	if estTime > 0 {
		return timeSpent <= factor*estTime
	}
	return !stageChanged
}

func (ctrl *Control) estimateStageTime(stage int) float64 {
	if s, ok := ctrl.StageHistory.Get(stage); ok && s.TotalRuns >= 3 && s.AvgTimeSpent > 0 {
		return s.AvgTimeSpent
	}
	info, ok := ctrl.FarmStages[stage]
	if !ok || info.TotalHP <= 0 {
		return 0
	}
	var points []timePoint
	for _, st := range ctrl.StageHistory.AllStats() {
		if (st.TotalRuns == 0 && st.ManualTime == 0) || st.AvgTimeSpent <= 0 {
			continue
		}
		fi, ok := ctrl.FarmStages[st.StageKey]
		if !ok || fi.TotalHP <= 0 {
			continue
		}
		points = append(points, timePoint{HP: fi.TotalHP, Waves: float64(fi.Waves), Time: st.AvgTimeSpent})
	}
	dps, overhead, ok := effectiveDPS(points)
	if !ok {
		return 0
	}
	return estimateTimeDPS(dps, overhead, info.TotalHP, float64(info.Waves))
}

// recordedAvgTime devolve a média de tempo já medida da fase (a partir de 1
// corrida). Usada só como piso anti-fragmento, não para estimar fases novas.
func (ctrl *Control) recordedAvgTime(stage int) float64 {
	if s, ok := ctrl.StageHistory.Get(stage); ok && s.TotalRuns >= 1 {
		return s.AvgTimeSpent
	}
	return 0
}

func (ctrl *Control) nextStageKey(stage int) int {
	best := 0
	for k := range ctrl.FarmStages {
		if k > stage && (best == 0 || k < best) {
			best = k
		}
	}
	return best
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
	clearedStage := ctrl.LastCurrentStageKey
	currStage := currentSave.CommonSaveData.CurrentStageKey

	timeSpent := currentSave.CommonSaveData.PlayTime - ctrl.LastPlayTime
	goldGain := ExtractGold(currentSave.CurrenySaveDatas) - ctrl.LastGold
	levelUps := heroLevelUps(currentSave.HeroSaveDatas, ctrl.HeroStates)
	xpGain := computeRoundXp(currentSave.HeroSaveDatas, ctrl.HeroStates)

	cleanWindow := currStage == clearedStage || currStage == ctrl.nextStageKey(clearedStage)
	estTime := ctrl.estimateStageTime(clearedStage)
	dropCount, dropsByKey := countNewDrops(ctrl.LastItemIds, currentSave.ItemSaveDatas)

	defer func() {
		ctrl.LastCurrentStageKey = currStage
		ctrl.MaxCompletedStage = currentSave.CommonSaveData.MaxCompletedStage
		ctrl.LastItemIds = snapshotItemIds(currentSave.ItemSaveDatas)
	}()

	advanceClock := func() {
		ctrl.LastPlayTime = currentSave.CommonSaveData.PlayTime
		ctrl.LastGold = ExtractGold(currentSave.CurrenySaveDatas)
		commitHeroStates(currentSave.HeroSaveDatas, ctrl.HeroStates)
		ctrl.midWindowStage = 0
		ctrl.midWindowMixed = false
		ctrl.midWindowMaxWave = 0
		ctrl.midWindowRestart = false
	}

	if !isValidRound(timeSpent, goldGain, xpGain) {
		Logf("info", "Fase %s: save em wave 0 sem ganho NOVO de ouro/xp — ignorado (normalmente um 2º save logo após um clear que já foi contado).", ctrl.stageDisplay(clearedStage))
		return
	}

	if ctrl.primeFirstClear {
		ctrl.primeFirstClear = false
		advanceClock()
		Logf("info", "Fase %s: o monitor começou no meio do ciclo — esta primeira janela é parcial (%.0fs) e foi descartada. Conto a partir do próximo clear completo.", ctrl.stageDisplay(clearedStage), timeSpent)
		return
	}

	if !cleanWindow {
		advanceClock()
		Logf("reject", "Fase %s descartada: a fase pulou pra %s (troca manual, não auto-avanço) — a janela mistura tempo de mapas diferentes. Re-sincronizando.", ctrl.stageDisplay(clearedStage), ctrl.stageDisplay(currStage))
		return
	}

	if ctrl.midWindowMixed || (ctrl.midWindowStage != 0 && ctrl.midWindowStage != clearedStage) {
		witness := ctrl.midWindowStage
		advanceClock()
		Logf("reject", "Fase %s descartada: durante o ciclo o jogo esteve em %s — troca manual de mapa no meio da janela (não auto-avanço). Re-sincronizando.", ctrl.stageDisplay(clearedStage), ctrl.stageDisplay(witness))
		return
	}

	if ctrl.midWindowRestart {
		advanceClock()
		Logf("reject", "Fase %s descartada: a wave recuou no meio da janela — a fase reiniciou (morte/recomeço), o tempo soma tentativas e não um clear só. Re-sincronizando.", ctrl.stageDisplay(clearedStage))
		return
	}

	if avg := ctrl.recordedAvgTime(clearedStage); avg > 0 && timeSpent < avg/timeOutlierFactor {
		Logf("reject", "Fase %s descartada: %.0fs curto demais para um clear (média ~%.0fs) — fragmento de save, não um ciclo completo. Junto com o próximo ciclo.", ctrl.stageDisplay(clearedStage), timeSpent, avg)
		return
	} else if avg <= 0 && estTime > 0 && timeSpent < estTime/timeOutlierFactor {
		advanceClock()
		Logf("reject", "Fase %s descartada: %.0fs curto demais para um clear (estimado ~%.0fs) — sobra de save fora de ciclo, não uma corrida. Re-sincronizando.", ctrl.stageDisplay(clearedStage), timeSpent, estTime)
		return
	}

	if !isTimeTrustworthy(timeSpent, estTime, timeOutlierFactor, false) {
		advanceClock()
		Logf("reject", "Fase %s descartada: tempo %.0fs muito acima do esperado (~%.0fs). Provável ociosidade/decisão no meio do ciclo.", ctrl.stageDisplay(clearedStage), timeSpent, estTime)
		return
	}

	if avg := ctrl.recordedAvgTime(clearedStage); avg > 0 && timeSpent > timeContamFactor*avg {
		if xpAvg := ctrl.estimateClearXp(clearedStage); xpAvg > 0 && xpGain < timeContamFactor*xpAvg {
			if ctrl.timeContamStreak == nil {
				ctrl.timeContamStreak = make(map[int]int)
			}
			ctrl.timeContamStreak[clearedStage]++
			if ctrl.timeContamStreak[clearedStage] < timeContamAccept {
				advanceClock()
				Logf("reject", "Fase %s descartada: %.0fs com ganhos de UM clear só (média ~%.0fs) — o tempo extra foi gasto em outro mapa (tentativas de boss) sem nenhum save flagrar. Re-sincronizando.", ctrl.stageDisplay(clearedStage), timeSpent, avg)
				return
			}
			Logf("info", "Fase %s: tempo %.0fs acima da média (~%.0fs) pela %dª janela seguida — consistente demais pra ser tentativa de boss; aceito como o novo ritmo real da fase.", ctrl.stageDisplay(clearedStage), timeSpent, avg, ctrl.timeContamStreak[clearedStage])
		}
	}

	if len(levelUps) > 0 {
		advanceClock()
		Logf("reject", "Fase %s descartada: %s — o XP de um clear com level-up fica distorcido pelo limiar do nível. A próxima corrida já conta normal.", ctrl.stageDisplay(clearedStage), describeLevelUps(levelUps))
		return
	}

	if ctrl.estimateClearXp(clearedStage) == 0 {
		if xpEst := ctrl.projectedClearXp(clearedStage); xpEst > 0 && xpGain > xpCeilFactor*xpEst {
			advanceClock()
			Logf("reject", "Fase %s descartada: +%.0f xp é %.1fx o esperado de UM clear (~%.0f xp, não segundos) — a janela contém mortes/tentativas múltiplas, não uma corrida só.", ctrl.stageDisplay(clearedStage), xpGain, xpGain/xpEst, xpEst)
			return
		}
	}

	if currStage != clearedStage {
		Logf("info", "Fase %s: auto-avanço pro %s — o tempo deste ciclo é do %s, creditando nele (não no próximo mapa).", ctrl.stageDisplay(clearedStage), ctrl.stageDisplay(currStage), ctrl.stageDisplay(clearedStage))
	}

	goldRec := float64(goldGain)
	if goldGain < 0 {
		if xpFloor := ctrl.estimateClearXp(clearedStage); xpFloor > 0 && xpGain > xpCeilFactor*xpFloor {
			advanceClock()
			Logf("reject", "Fase %s descartada: ouro negativo (compra no meio) E +%.0f xp (%.1fx a média de um clear) — janela com mortes/tentativas múltiplas escondida pela compra. Re-sincronizando.", ctrl.stageDisplay(clearedStage), xpGain, xpGain/xpFloor)
			return
		}
		if s, ok := ctrl.StageHistory.Get(clearedStage); ok {
			goldRec = s.AvgGoldPerRun
		} else {
			goldRec = 0
		}
		Logf("info", "Fase %s: ouro caiu %d no ciclo — provável compra de runa/item. Conto tempo/xp; o ouro não distorce a média.", ctrl.stageDisplay(clearedStage), goldGain)
	} else if floor := ctrl.estimateClearGold(clearedStage); floor > 0 {
		if float64(goldGain) < goldFloorFactor*floor {
			Logf("reject", "Fase %s descartada: ouro %.0f baixo demais para um clear (média ~%.0f). Provável morte/clear parcial.", ctrl.stageDisplay(clearedStage), float64(goldGain), floor)
			return
		}
		if float64(goldGain) > goldCeilFactor*floor {
			xpFloor := ctrl.estimateClearXp(clearedStage)
			powerJump := xpFloor > 0 && xpGain >= goldCeilFactor*xpFloor
			if !powerJump {
				advanceClock()
				Logf("reject", "Fase %s descartada: ouro %.0f MUITO acima da média (~%.0f) sem XP equivalente — provável venda de itens/bônus, não ouro de farm. Não inflo a média.", ctrl.stageDisplay(clearedStage), float64(goldGain), floor)
				return
			}
			Logf("info", "Fase %s: ouro %.0f bem acima da média (~%.0f), mas o XP subiu junto — evolução de poder, conto como clear real.", ctrl.stageDisplay(clearedStage), float64(goldGain), floor)
		}
	}

	advanceClock()

	xpPerHour := (xpGain / float64(ctrl.numActiveHeroes()) / timeSpent) * 3600
	goldPerHour := (goldRec / timeSpent) * 3600

	saveStageLog(clearedStage, timeSpent, goldPerHour, xpPerHour, ctrl.HeroLevel)

	ctrl.StageHistory.Update(
		clearedStage,
		timeSpent,
		goldRec,
		xpGain,
		ctrl.UseEMA,
		ctrl.EMAAlpha,
		ctrl.numActiveHeroes(),
		ctrl.HeroLevel,
		dropCount,
		dropsByKey,
	)

	delete(ctrl.timeContamStreak, clearedStage)

	if err := ctrl.StageHistory.Save(HistoryFilePath); err != nil {
		fmt.Println("Aviso: falha ao persistir o historico:", err)
	}

	s, exists := ctrl.StageHistory.Get(clearedStage)
	if exists {
		nh := ctrl.numActiveHeroes()
		Logf("run", "Fase %s REGISTRADA: %.0fs · +%.0f ouro · +%.0f xp (%.0f/herói ÷%d) · total de corridas: %d",
			ctrl.stageDisplay(clearedStage), timeSpent, goldRec, xpGain, xpGain/float64(nh), nh, s.TotalRuns)
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

func saveStageLog(stageKey int, timeSpent float64, goldGain float64, xpGain float64, heroLevel int) {
	file, err := os.OpenFile("historico_farm.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("Erro ao abrir o bloco de notas:", err)
		return
	}
	defer file.Close()

	logLine := fmt.Sprintf("[%s] Estágio Concluído: %d | Tempo Gasto: %.2fs | Ouro/h: %.0f | XP/h: %.0f | Nível: %d\n",
		time.Now().Format("2006-01-02 15:04:05"), stageKey, timeSpent, goldGain, xpGain, heroLevel)
	_, err = file.WriteString(logLine)
	if err != nil {
		fmt.Println("Erro ao descarregar os dados no bloco de notas:", err)
	}
}
