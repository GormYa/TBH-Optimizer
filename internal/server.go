package internal

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func StartWebServer(ctrl *Control, webFiles embed.FS, dataDir string) {
	http.HandleFunc("/api/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method != "GET" {
			http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
			return
		}

		report := ctrl.GenerateReportWithEstimates()

		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(report)
		if err != nil {
			return
		}
	})

	http.HandleFunc("/api/stats/reset", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method != "POST" {
			http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
			return
		}

		// Body opcional {stage_key}: se vier, limpa só aquela fase; senão, tudo.
		var req struct {
			StageKey int `json:"stage_key"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.StageKey > 0 {
			ctrl.StageHistory.Remove(req.StageKey)
		} else {
			ctrl.StageHistory.Clear()
			_ = os.Truncate("historico_farm.txt", 0)
		}
		_ = ctrl.StageHistory.Save(HistoryFilePath)

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"success"}`))
	})

	http.HandleFunc("/api/stats/add", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method != "POST" {
			http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			StageKey  int         `json:"stage_key"`
			TimeSpent interface{} `json:"time_spent"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		goldGain := 0.0
		xpGain := 0.0
		if ctrl.FarmStages != nil {
			if info, exists := ctrl.FarmStages[req.StageKey]; exists {
				goldGain = info.ExpectedGold
				xpGain = info.ExpectedEXP
			}
		}

		timeSpentSeconds := ParseTimeSpent(req.TimeSpent)

		ctrl.StageHistory.Update(
			req.StageKey,
			timeSpentSeconds,
			goldGain,
			xpGain,
			ctrl.UseEMA,
			ctrl.EMAAlpha,
			len(ctrl.HeroStates),
			0,
			nil,
		)
		_ = ctrl.StageHistory.Save(HistoryFilePath)

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"success"}`))
	})

	http.HandleFunc("/api/chests/update", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method != "POST" {
			http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
			return
		}
		n, err := UpdateChestData(dataDir, time.Now().Format("2006-01-02 15:04"))
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			fmt.Fprintf(w, `{"status":"error","message":%q}`, err.Error())
			return
		}
		fmt.Fprintf(w, `{"status":"success","chests":%d}`, n)
	})

	http.HandleFunc("/api/update/check", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		out := map[string]any{"current": Version, "enabled": UpdateEnabled()}
		if UpdateEnabled() {
			rel, has, err := CheckForUpdate(&http.Client{Timeout: 20 * time.Second})
			if err != nil {
				out["error"] = err.Error()
			} else {
				out["has_update"] = has
				out["latest"] = rel.Version
				out["notes"] = rel.Notes
			}
		}
		_ = json.NewEncoder(w).Encode(out)
	})

	http.HandleFunc("/api/update/apply", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method != "POST" {
			http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		client := &http.Client{Timeout: 60 * time.Second}
		rel, has, err := CheckForUpdate(client)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			fmt.Fprintf(w, `{"status":"error","message":%q}`, err.Error())
			return
		}
		if !has {
			fmt.Fprintf(w, `{"status":"uptodate","current":%q}`, Version)
			return
		}
		if err := ApplySelfUpdate(client, rel); err != nil {
			w.WriteHeader(http.StatusBadGateway)
			fmt.Fprintf(w, `{"status":"error","message":%q}`, err.Error())
			return
		}
		fmt.Fprintf(w, `{"status":"success","version":%q,"message":"Atualizado. Reabrindo o app..."}`, rel.Version)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		RelaunchAndExit()
	})

	webSubFS, err := fs.Sub(webFiles, "web")
	if err != nil {
		fmt.Println("Erro ao carregar arquivos estáticos do frontend:", err)
		return
	}
	embedded := http.FileServer(http.FS(webSubFS))

	// chest_drops.json e /sprites/ tem prioridade no disco (dataDir, atualizavel em
	// runtime); o resto (e o fallback) vem do baseline embarcado.
	diskOrEmbed := func(w http.ResponseWriter, r *http.Request) {
		rel := strings.TrimPrefix(r.URL.Path, "/")
		diskPath := filepath.Join(dataDir, filepath.FromSlash(rel))
		if fi, statErr := os.Stat(diskPath); statErr == nil && !fi.IsDir() {
			http.ServeFile(w, r, diskPath)
			return
		}
		embedded.ServeHTTP(w, r)
	}
	http.HandleFunc("/chest_drops.json", diskOrEmbed)
	http.HandleFunc("/sprites/", diskOrEmbed)
	http.Handle("/", embedded)

	fmt.Println("Servidor HTTP iniciado em http://localhost:8080")
	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		fmt.Println("Erro ao iniciar o servidor HTTP:", err)
	}
}

func ParseTimeSpent(val interface{}) float64 {
	if val == nil {
		return 0.0
	}
	switch v := val.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		s := strings.TrimSpace(v)
		if strings.Contains(s, ":") {
			parts := strings.Split(s, ":")
			if len(parts) == 2 {
				m, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
				sec, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
				return float64(m)*60.0 + sec
			}
		}
		f, _ := strconv.ParseFloat(s, 64)
		return f
	}
	return 0.0
}
