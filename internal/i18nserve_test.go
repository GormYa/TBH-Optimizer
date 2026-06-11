package internal

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Em desenvolvimento (rodando do repo), o pack vem do site/i18n local sem rede.
func TestI18nPackLocalFile(t *testing.T) {
	old, _ := os.Getwd()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)

	if err := os.MkdirAll(filepath.Join("site", "i18n"), 0755); err != nil {
		t.Fatal(err)
	}
	want := `{"items":{"1":"Sword"}}`
	if err := os.WriteFile(filepath.Join("site", "i18n", "names_en-US.json"), []byte(want), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := i18nPack(&http.Client{Timeout: time.Nanosecond}, "en-US")
	if err != nil || string(got) != want {
		t.Fatalf("pack local deveria ser servido sem rede; got=%q err=%v", got, err)
	}
}

// Sem repo local e sem rede, o ultimo download cacheado em disco segura o offline.
func TestI18nPackCacheFallback(t *testing.T) {
	old, _ := os.Getwd()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)

	want := `{"items":{"2":"Bow"}}`
	if err := os.WriteFile("i18n_names_de-DE.json", []byte(want), 0644); err != nil {
		t.Fatal(err)
	}

	// timeout de 1ns força falha imediata do CDN -> tem que cair pro cache
	got, err := i18nPack(&http.Client{Timeout: time.Nanosecond}, "de-DE")
	if err != nil || string(got) != want {
		t.Fatalf("cache em disco deveria segurar o offline; got=%q err=%v", got, err)
	}
}

// Todos os 16 idiomas do jogo estao na whitelist do endpoint.
func TestI18nLangsCompletos(t *testing.T) {
	for _, c := range []string{"pt-BR", "en-US", "es-ES", "de-DE", "fr-FR", "pl-PL",
		"tr-TR", "ru-RU", "uk-UA", "id-ID", "vi-VN", "th-TH", "ko-KR", "ja-JP",
		"zh-Hans", "zh-Hant"} {
		if !i18nLangs[c] {
			t.Errorf("idioma %s fora da whitelist", c)
		}
	}
	if i18nLangs["xx-XX"] {
		t.Error("idioma inexistente não deveria passar")
	}
}
