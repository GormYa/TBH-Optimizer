package internal

import (
	"net/http"
	"os"
	"path/filepath"
)

var i18nLangs = map[string]bool{
	"pt-BR": true, "en-US": true, "es-ES": true, "de-DE": true,
	"fr-FR": true, "pl-PL": true, "tr-TR": true, "ru-RU": true,
	"uk-UA": true, "id-ID": true, "vi-VN": true, "th-TH": true,
	"ko-KR": true, "ja-JP": true, "zh-Hans": true, "zh-Hant": true,
}

func i18nPack(client *http.Client, dataDir, code string) ([]byte, error) {
	name := "names_" + code + ".json"
	if b, err := os.ReadFile(filepath.Join("site", "i18n", name)); err == nil {
		return b, nil
	}
	cache := filepath.Join(dataDir, "i18n", name)
	b, err := httpGet(client, updateBaseURL+"i18n/"+name)
	if err == nil {
		if mkErr := os.MkdirAll(filepath.Dir(cache), 0755); mkErr == nil {
			_ = os.WriteFile(cache, b, 0644)
		}
		return b, nil
	}
	if cached, cerr := os.ReadFile(cache); cerr == nil {
		return cached, nil
	}
	return nil, err
}
