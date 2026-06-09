package internal

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// DumpSaveStructure decripta o save e imprime a ESTRUTURA (chaves + tipos + previews
// curtos), destacando campos que possam ser cronometro de estagio (time/wave/stage/
// clear/elapsed/play). Nao despeja valores longos: serve pra mapear o schema do save
// sem vazar dados. Use:  optimizer -dump-save
func DumpSaveStructure() error {
	inner, err := DecryptSaveInner()
	if err != nil {
		return err
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(inner, &top); err != nil {
		return err
	}

	keys := make([]string, 0, len(top))
	for k := range top {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fmt.Printf("=== SAVE: %d chaves de topo ===\n", len(top))
	for _, k := range keys {
		fmt.Printf("  %-28s %s\n", k, preview(top[k]))
	}

	fmt.Println("\n=== commonSaveData (completo) ===")
	if raw, ok := top["commonSaveData"]; ok {
		printObject(raw, "  ")
	}

	fmt.Println("\n=== Campos com time/stage/wave/clear/elapsed/play (varredura recursiva) ===")
	hunt(top, "")

	if save, err := LoadSave(); err == nil {
		hl := activeHeroLevel(save)
		fmt.Printf("\n=== Hero level ativo: %d | exp mantida: lvl38=%.0f%% lvl39=%.0f%% lvl40=%.0f%% ===\n",
			hl, expRetention(38, hl)*100, expRetention(39, hl)*100, expRetention(40, hl)*100)
		fmt.Printf("=== heroSaveDatas: %d total | arrangedHeroKey (ativos): %v ===\n",
			len(save.HeroSaveDatas), save.CommonSaveData.ArrangedHeroKey)
		for _, h := range save.HeroSaveDatas {
			fmt.Printf("    hero %d -> level %d\n", h.HeroKey, h.HeroLevel)
		}
	}
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
func hunt(v any, path string) {
	switch t := v.(type) {
	case map[string]json.RawMessage:
		for k, raw := range t {
			child := decode(raw)
			full := path + "/" + k
			if interestingKey(k) {
				fmt.Printf("  %-50s = %s\n", full, preview(raw))
			}
			hunt(child, full)
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
				fmt.Printf("  %-50s = %s\n", full, scalar(t[k]))
			}
			hunt(t[k], full)
		}
	case []any:
		if len(t) > 0 {
			hunt(t[0], path+"[0]")
		}
	}
}

func decode(raw json.RawMessage) any {
	var v any
	_ = json.Unmarshal(raw, &v)
	return v
}

func printObject(raw json.RawMessage, indent string) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		fmt.Printf("%s%s\n", indent, preview(raw))
		return
	}
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	for _, k := range ks {
		fmt.Printf("%s%-26s %s\n", indent, k, scalar(m[k]))
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
