package main

import (
	"embed"
	"fmt"
	"optimizer/internal"
	"os"
	"os/exec"
	"time"
)

//go:embed web
var webFiles embed.FS

//go:embed web/farm_stages.json
var farmStagesData []byte

// gameDataDir guarda o catalogo atualizavel em runtime (chest_drops.json + sprites),
// ao lado do executavel. Servido com prioridade sobre o baseline embarcado.
const gameDataDir = "gamedata"

func main() {
	// `optimizer -update`: regenera o baseline embarcado (web/) a partir da wiki.
	if len(os.Args) > 1 && os.Args[1] == "-update" {
		n, err := internal.UpdateChestData("web", time.Now().Format("2006-01-02 15:04"))
		if err != nil {
			fmt.Println("Atualizacao falhou:", err)
			os.Exit(1)
		}
		fmt.Println("Baseline regenerada:", n, "baús")
		return
	}

	// Remove o optimizer.exe.old deixado por um auto-update anterior.
	internal.CleanupOldBinary()

	farmStages, err := internal.LoadFarmStages(farmStagesData)
	if err != nil {
		fmt.Println("Aviso: farm_stages.json não pôde ser carregado:", err)
	}

	ctrl := internal.Control{
		UseEMA:     true,
		EMAAlpha:   0.2,
		FarmStages: farmStages,
	}

	if err := ctrl.StageHistory.Load(internal.HistoryFilePath); err != nil {
		fmt.Println("Aviso: nao foi possivel carregar o historico persistido:", err)
	}

	go internal.StartMonitoring(&ctrl)

	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = exec.Command("cmd", "/c", "start", "http://localhost:8080").Start()
	}()

	internal.StartWebServer(&ctrl, webFiles, gameDataDir)
}
