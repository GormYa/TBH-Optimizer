package internal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Version e a versao embarcada deste binario. O CI deve bumpar isto a cada release
// e publicar o MESMO numero no version.json do CDN.
var Version = "1.0.0"

// updateBaseURL: raiz publica do CDN com version.json + optimizer.exe (+ dados).
// Troque pela sua URL (GitHub Releases publico, Cloudflare Pages, Vercel...).
// Enquanto contiver "EXAMPLE" o auto-update fica DESLIGADO (nao quebra nada).
const updateBaseURL = "https://tbh-optimizer.pages.dev/"

// ReleaseInfo e o conteudo do version.json publicado no CDN.
type ReleaseInfo struct {
	Version string `json:"version"`
	Exe     string `json:"exe,omitempty"`
	SHA256  string `json:"sha256,omitempty"`
	Notes   string `json:"notes,omitempty"`
}

// UpdateEnabled diz se o updateBaseURL ja foi configurado.
func UpdateEnabled() bool { return !strings.Contains(updateBaseURL, "EXAMPLE") }

// parseVer quebra "MAJOR.MINOR.PATCH" em inteiros (tolera prefixo "v" e campos faltando).
func parseVer(s string) [3]int {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	var out [3]int
	for i, p := range strings.SplitN(s, ".", 3) {
		n, _ := strconv.Atoi(strings.TrimSpace(p))
		out[i] = n
	}
	return out
}

// isNewer compara versoes semanticas: remote > local?
func isNewer(remote, local string) bool {
	r, l := parseVer(remote), parseVer(local)
	for i := 0; i < 3; i++ {
		if r[i] != l[i] {
			return r[i] > l[i]
		}
	}
	return false
}

// CheckForUpdate baixa o version.json e diz se ha versao mais nova que a embarcada.
func CheckForUpdate(client *http.Client) (rel *ReleaseInfo, hasUpdate bool, err error) {
	if !UpdateEnabled() {
		return nil, false, fmt.Errorf("auto-update desativado: configure updateBaseURL")
	}
	body, err := httpGet(client, updateBaseURL+"version.json")
	if err != nil {
		return nil, false, err
	}
	var r ReleaseInfo
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, false, fmt.Errorf("version.json invalido: %w", err)
	}
	return &r, isNewer(r.Version, Version), nil
}

// ApplySelfUpdate baixa o exe novo, valida o SHA-256 e troca o binario em uso.
// No Windows nao da pra sobrescrever um exe rodando, mas da pra renomea-lo:
// renomeia o atual -> .old, instala o novo no lugar. O .old e apagado no proximo boot.
// NAO relança sozinho; chame RelaunchAndExit depois de responder ao cliente.
func ApplySelfUpdate(client *http.Client, rel *ReleaseInfo) error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
		exePath = resolved
	}

	name := rel.Exe
	if name == "" {
		name = "optimizer.exe"
	}
	data, err := httpGet(client, updateBaseURL+name)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return fmt.Errorf("download vazio")
	}
	if rel.SHA256 != "" {
		sum := sha256.Sum256(data)
		if !strings.EqualFold(hex.EncodeToString(sum[:]), strings.TrimSpace(rel.SHA256)) {
			return fmt.Errorf("SHA-256 nao confere (download corrompido ou adulterado)")
		}
	}

	newPath := exePath + ".new"
	oldPath := exePath + ".old"
	if err := os.WriteFile(newPath, data, 0o755); err != nil {
		return err
	}
	_ = os.Remove(oldPath)
	if err := os.Rename(exePath, oldPath); err != nil {
		_ = os.Remove(newPath)
		return fmt.Errorf("nao consegui mover o exe atual: %w", err)
	}
	if err := os.Rename(newPath, exePath); err != nil {
		_ = os.Rename(oldPath, exePath) // rollback
		return fmt.Errorf("nao consegui instalar o exe novo: %w", err)
	}
	return nil
}

// RelaunchAndExit abre o exe novo (apos uma pausa que libera a :8080) e encerra este.
func RelaunchAndExit() {
	exePath, err := os.Executable()
	if err == nil {
		// cmd espera 1s (este processo sai antes) e reabre o exe no lugar.
		_ = exec.Command("cmd", "/c",
			"timeout /t 1 /nobreak >nul & start \"\" \""+exePath+"\"").Start()
	}
	go func() { time.Sleep(300 * time.Millisecond); os.Exit(0) }()
}

// CleanupOldBinary apaga o .old deixado por um update anterior (chame no boot).
func CleanupOldBinary() {
	if exe, err := os.Executable(); err == nil {
		_ = os.Remove(exe + ".old")
	}
}

func httpGet(client *http.Client, url string) ([]byte, error) {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "tbh-optimizer/"+Version)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d em %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}
