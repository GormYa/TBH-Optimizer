package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Os dicionários de UI do painel precisam ter TODOS o mesmo conjunto de chaves do
// en-US (referência). Esse teste é o guardião do fluxo de tradução: adicionou string
// nova na UI -> adiciona em todos os ui_*.json, senão o CI quebra aqui.
func TestUIDictsMesmasChaves(t *testing.T) {
	dir := filepath.Join("..", "web", "i18n")
	ref := loadDict(t, filepath.Join(dir, "ui_en-US.json"))

	paths, _ := filepath.Glob(filepath.Join(dir, "ui_*.json"))
	if len(paths) < 15 {
		t.Fatalf("esperava >=15 dicionários de UI, achei %d", len(paths))
	}
	for _, p := range paths {
		d := loadDict(t, p)
		for k := range ref {
			if v, ok := d[k]; !ok {
				t.Errorf("%s: falta a chave %q", filepath.Base(p), k)
			} else if v == "" {
				t.Errorf("%s: chave %q com valor vazio", filepath.Base(p), k)
			}
		}
		for k := range d {
			if _, ok := ref[k]; !ok {
				t.Errorf("%s: chave extra %q (não existe no en-US)", filepath.Base(p), k)
			}
		}
	}
}

// Mesmo contrato pros dicionários da landing (site_*.json): as chaves "strings"
// devem ser as mesmas do site_en.json em todos os idiomas.
func TestSiteDictsMesmasChaves(t *testing.T) {
	dir := filepath.Join("..", "site", "i18n")
	type siteDict struct {
		Strings map[string]string `json:"strings"`
	}
	load := func(p string) map[string]string {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		var d siteDict
		if err := json.Unmarshal(b, &d); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		return d.Strings
	}
	ref := load(filepath.Join(dir, "site_en.json"))
	paths, _ := filepath.Glob(filepath.Join(dir, "site_*.json"))
	if len(paths) < 15 {
		t.Fatalf("esperava >=15 dicionários da landing, achei %d", len(paths))
	}
	for _, p := range paths {
		d := load(p)
		for k := range ref {
			if d[k] == "" {
				t.Errorf("%s: falta/vazia a chave %q", filepath.Base(p), k)
			}
		}
		for k := range d {
			if _, ok := ref[k]; !ok {
				t.Errorf("%s: chave extra %q", filepath.Base(p), k)
			}
		}
	}
}

func loadDict(t *testing.T, path string) map[string]string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	var d map[string]string
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return d
}
