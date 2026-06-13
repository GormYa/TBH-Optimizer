package internal

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
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
			_ = os.Truncate(FarmLogPath, 0)
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
				gMult, xMult := yieldMultipliers(ctrl.StageHistory.AllStats(), ctrl.FarmStages)
				goldGain = info.ExpectedGold * gMult
				xpGain = info.ExpectedEXP * xMult * expRetention(info.Level, ctrl.HeroLevel)
			}
		}

		timeSpentSeconds := ParseTimeSpent(req.TimeSpent)

		ctrl.StageHistory.SetManualTime(
			req.StageKey,
			timeSpentSeconds,
			goldGain,
			xpGain,
			ctrl.numActiveHeroes(),
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
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"bundled","message":"O catálogo de baús já vem embutido no app (extração local). Ele é atualizado junto com as atualizações do programa."}`)
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

	http.HandleFunc("/api/i18n/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		code := strings.TrimPrefix(r.URL.Path, "/api/i18n/")
		if !i18nLangs[code] {
			http.Error(w, "idioma desconhecido", http.StatusNotFound)
			return
		}
		data, err := i18nPack(&http.Client{Timeout: 15 * time.Second}, dataDir, code)
		if err != nil {
			http.Error(w, "pack de idioma indisponível (offline e sem cache): "+err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "max-age=3600")
		_, _ = w.Write(data)
	})

	http.HandleFunc("/api/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"version":%q}`, Version)
	})

	http.HandleFunc("/api/logs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")
		since, _ := strconv.Atoi(r.URL.Query().Get("since"))
		_ = json.NewEncoder(w).Encode(map[string]any{"logs": LogsSince(since)})
	})

	http.HandleFunc("/api/quit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://localhost:8080" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"bye"}`))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		go func() { time.Sleep(200 * time.Millisecond); os.Exit(0) }()
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

	http.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		embedded.ServeHTTP(w, r)
	}))

	fmt.Println("Servidor HTTP iniciado em http://localhost:8080")
	for i := 0; i < 30; i++ {
		err = http.ListenAndServe(":8080", nil)
		if err == nil {
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	fmt.Println("Erro ao iniciar o servidor HTTP:", err)
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
