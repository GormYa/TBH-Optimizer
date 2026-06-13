package internal

import (
	"os"
	"path/filepath"
)

var DataDir = "gamedata"

var (
	HistoryFilePath  = filepath.Join(DataDir, "stage_history.json")
	ChestHistoryPath = filepath.Join(DataDir, "chest_history.json")
	FarmLogPath      = filepath.Join(DataDir, "historico_farm.txt")
)

func ensureDataDir() { _ = os.MkdirAll(DataDir, 0o755) }

// MigrateDataFiles move os arquivos de runtime que versões antigas gravavam na
// pasta do executável para dentro de DataDir, preservando o histórico do usuário
// ao atualizar. Também remove caches de idioma soltos (i18n_names_*.json) que uma
// versão antiga gravava por engano na raiz — são regeneráveis.
func MigrateDataFiles() {
	ensureDataDir()
	for _, name := range []string{"stage_history.json", "chest_history.json", "historico_farm.txt"} {
		dst := filepath.Join(DataDir, name)
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		if _, err := os.Stat(name); err != nil {
			continue
		}
		_ = os.Rename(name, dst)
	}
	if strays, err := filepath.Glob("i18n_names_*.json"); err == nil {
		for _, f := range strays {
			_ = os.Remove(f)
		}
	}
}
