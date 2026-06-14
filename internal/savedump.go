package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

func DumpSaveStructure() error {
	inner, err := DecryptSaveInner()
	if err != nil {
		return err
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(inner, &top); err != nil {
		return err
	}

	const outPath = "save_dump.txt"
	var w io.Writer = os.Stdout
	if f, ferr := os.Create(outPath); ferr == nil {
		defer f.Close()
		w = io.MultiWriter(os.Stdout, f)
	}

	keys := make([]string, 0, len(top))
	for k := range top {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fmt.Fprintf(w, "=== SAVE: %d chaves de topo ===\n", len(top))
	for _, k := range keys {
		fmt.Fprintf(w, "  %-28s %s\n", k, preview(top[k]))
	}

	fmt.Fprintln(w, "\n=== commonSaveData (completo) ===")
	if raw, ok := top["commonSaveData"]; ok {
		printObject(w, raw, "  ")
	}

	fmt.Fprintln(w, "\n=== cubeRecipeSaveDatas (completo) + cubeSaveLevelData ===")
	if raw, ok := top["cubeRecipeSaveDatas"]; ok {
		var pretty bytes.Buffer
		if json.Indent(&pretty, raw, "  ", "  ") == nil {
			fmt.Fprintf(w, "  %s\n", pretty.String())
		} else {
			fmt.Fprintf(w, "  %s\n", string(raw))
		}
	} else {
		fmt.Fprintln(w, "  (ausente no save)")
	}
	if raw, ok := top["cubeSaveLevelData"]; ok {
		fmt.Fprintf(w, "  cubeSaveLevelData = %s\n", string(raw))
	}

	fmt.Fprintln(w, "\n=== Campos com time/stage/wave/clear/elapsed/play (varredura recursiva) ===")
	hunt(w, top, "")

	if save, err := LoadSave(); err == nil {
		hl := activeHeroLevel(save)
		fmt.Fprintf(w, "\n=== Hero level ativo: %d | exp mantida: lvl38=%.0f%% lvl39=%.0f%% lvl40=%.0f%% ===\n",
			hl, expRetention(38, hl)*100, expRetention(39, hl)*100, expRetention(40, hl)*100)
		fmt.Fprintf(w, "=== heroSaveDatas: %d total | arrangedHeroKey (ativos): %v ===\n",
			len(save.HeroSaveDatas), save.CommonSaveData.ArrangedHeroKey)
		for _, h := range save.HeroSaveDatas {
			fmt.Fprintf(w, "    hero %d -> level %d | HeroExp %.0f | ExpForLevelUp(L%d)=%.0f\n",
				h.HeroKey, h.HeroLevel, h.HeroExp, h.HeroLevel, GetXPRequiredForLevel(h.HeroLevel))
		}
	}

	fmt.Fprintf(w, "\n(dump salvo em %s)\n", outPath)
	return nil
}

var huntKW = []string{"time", "stage", "wave", "clear", "elapsed", "play", "tempo", "start", "duration", "second"}

func interestingKey(k string) bool {
	lk := strings.ToLower(k)
	for _, w := range huntKW {
		if strings.Contains(lk, w) {
			return true
		}
	}
	return false
}

// hunt percorre o JSON recursivamente (limitado) e imprime caminhos cuja chave bate
// com palavras de interesse, junto do valor (se for escalar curto).
func hunt(w io.Writer, v any, path string) {
	switch t := v.(type) {
	case map[string]json.RawMessage:
		for k, raw := range t {
			child := decode(raw)
			full := path + "/" + k
			if interestingKey(k) {
				fmt.Fprintf(w, "  %-50s = %s\n", full, preview(raw))
			}
			hunt(w, child, full)
		}
	case map[string]any:
		ks := make([]string, 0, len(t))
		for k := range t {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		for _, k := range ks {
			full := path + "/" + k
			if interestingKey(k) {
				fmt.Fprintf(w, "  %-50s = %s\n", full, scalar(t[k]))
			}
			hunt(w, t[k], full)
		}
	case []any:
		if len(t) > 0 {
			hunt(w, t[0], path+"[0]")
		}
	}
}

func decode(raw json.RawMessage) any {
	var v any
	_ = json.Unmarshal(raw, &v)
	return v
}

func printObject(w io.Writer, raw json.RawMessage, indent string) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		fmt.Fprintf(w, "%s%s\n", indent, preview(raw))
		return
	}
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	for _, k := range ks {
		fmt.Fprintf(w, "%s%-26s %s\n", indent, k, scalar(m[k]))
	}
}

func scalar(v any) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case float64:
		return fmt.Sprintf("%v", t)
	case bool:
		return fmt.Sprintf("%v", t)
	case string:
		if len(t) > 40 {
			return fmt.Sprintf("%q…", t[:40])
		}
		return fmt.Sprintf("%q", t)
	case []any:
		return fmt.Sprintf("[array len=%d]", len(t))
	case map[string]any:
		return fmt.Sprintf("{object %d campos}", len(t))
	default:
		return fmt.Sprintf("%T", t)
	}
}

func preview(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if len(s) > 60 {
		return s[:60] + "…"
	}
	return s
}
