package main

import (
	"embed"
	"fmt"
	"net/http"
	"optimizer/internal"
	"os"
	"os/exec"
	"time"
)

//go:embed web
var webFiles embed.FS

//go:embed web/farm_stages.json
var farmStagesData []byte

//go:embed web/hero_levels.json
var heroLevelsData []byte

//go:embed web/heroes.json
var heroesData []byte

var gameDataDir = internal.DataDir

func main() {
	if len(os.Args) > 1 && os.Args[1] == "-dump-save" {
		internal.LoadLevelCurve(heroLevelsData)
		internal.LoadHeroNames(heroesData)
		if err := internal.DumpSaveStructure(); err != nil {
			fmt.Println("Dump falhou:", err)
			os.Exit(1)
		}
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "-update" {
		n, err := internal.UpdateChestData("web", time.Now().Format("2006-01-02 15:04"))
		if err != nil {
			fmt.Println("Atualizacao falhou:", err)
			os.Exit(1)
		}
		fmt.Println("Baseline regenerada:", n, "baús")
		return
	}

	internal.CleanupOldBinary()
	internal.MigrateDataFiles()

	relaunch := false
	for _, a := range os.Args[1:] {
		if a == "-no-browser" {
			relaunch = true
		}
	}

	if !relaunch && instanceRunning() {
		openBrowser("http://localhost:8080")
		return
	}

	farmStages, err := internal.LoadFarmStages(farmStagesData)
	if err != nil {
		fmt.Println("Aviso: farm_stages.json não pôde ser carregado:", err)
	}
	internal.LoadLevelCurve(heroLevelsData)
	internal.LoadHeroNames(heroesData)

	ctrl := internal.Control{
		UseEMA:      true,
		EMAAlpha:    0.2,
		FarmStages:  farmStages,
		WebFiles:    webFiles,
		GameDataDir: gameDataDir,
	}

	if err := ctrl.StageHistory.Load(internal.HistoryFilePath); err != nil {
		fmt.Println("Aviso: nao foi possivel carregar o historico persistido:", err)
	}

	go internal.StartMonitoring(&ctrl)

	go func() {
		if relaunch {
			return
		}
		time.Sleep(300 * time.Millisecond)
		openBrowser("http://localhost:8080")
	}()

	internal.StartWebServer(&ctrl, webFiles, gameDataDir)
}

func openBrowser(url string) {
	_ = exec.Command("explorer", url).Start()
}

// instanceRunning diz se ja existe um app nosso respondendo na :8080.
func instanceRunning() bool {
	client := &http.Client{Timeout: 600 * time.Millisecond}
	resp, err := client.Get("http://localhost:8080/api/stats")
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == 200
}
