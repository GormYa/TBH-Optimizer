// Command sitegen gera as versões multilíngues da landing estática.
//
// Fonte da verdade: <site>/index.html (pt-BR). Para cada dicionário
// <site>/i18n/site_<code>.json é gerada uma página <out>/<code>/index.html
// com os textos traduzidos por substituição de string (PT -> tradução).
// A raiz <out>/index.html continua pt-BR, mas ganha o cluster hreflang e
// caminhos absolutos (necessários para as subpastas funcionarem).
// Também gera <out>/sitemap.xml com todas as variantes de idioma.
//
// Uso: go run ./cmd/sitegen -site site -out public
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"flag"
)

type langDict struct {
	// Lang vai em <html lang="...">. Default: código do nome do arquivo.
	Lang string `json:"lang"`
	// Hreflang usado nos <link rel="alternate">. Default: Lang.
	Hreflang string `json:"hreflang"`
	// OgLocale vai em og:locale (ex.: en_US).
	OgLocale string `json:"ogLocale"`
	// InLanguage vai no JSON-LD (ex.: en-US). Default: Lang.
	InLanguage string `json:"inLanguage"`
	// Strings mapeia o texto pt-BR exato do index.html para a tradução.
	Strings map[string]string `json:"strings"`

	code string // derivado do nome do arquivo site_<code>.json; vira o path /<code>/
}

func main() {
	siteDir := flag.String("site", "site", "pasta com index.html e i18n/site_*.json")
	outDir := flag.String("out", "public", "pasta de saída")
	baseURL := flag.String("base", "https://tbh-optimizer.pages.dev", "URL pública do site, sem barra final")
	flag.Parse()

	base := strings.TrimSuffix(*baseURL, "/")

	srcBytes, err := os.ReadFile(filepath.Join(*siteDir, "index.html"))
	if err != nil {
		fatal("lendo index.html: %v", err)
	}
	src := string(srcBytes)

	langs, err := loadDicts(filepath.Join(*siteDir, "i18n"))
	if err != nil {
		fatal("carregando dicionários: %v", err)
	}
	if len(langs) == 0 {
		fmt.Println("WARN: nenhum dicionário site_*.json encontrado em", filepath.Join(*siteDir, "i18n"))
	}

	cluster := hreflangCluster(base, langs)

	// Raiz pt-BR: caminhos absolutos + hreflang + seletor. Conteúdo/SEO inalterados.
	root := absolutizePaths(src)
	root = injectAfterCanonical(root, cluster, "pt-BR")
	root = injectLangSelector(root, langs, "pt-BR")
	if err := writeFile(filepath.Join(*outDir, "index.html"), root); err != nil {
		fatal("%v", err)
	}
	fmt.Printf("OK  %s (pt-BR, raiz)\n", filepath.Join(*outDir, "index.html"))

	for _, d := range langs {
		page := absolutizePaths(src)
		page = localizeMeta(page, d, base)
		page = translate(page, d)
		page = injectAfterCanonical(page, cluster, d.code)
		page = injectLangSelector(page, langs, d.code)
		out := filepath.Join(*outDir, d.code, "index.html")
		if err := writeFile(out, page); err != nil {
			fatal("%v", err)
		}
		fmt.Printf("OK  %s (%s)\n", out, d.Lang)
	}

	sm := sitemapXML(base, langs, time.Now().UTC().Format("2006-01-02"))
	if err := writeFile(filepath.Join(*outDir, "sitemap.xml"), sm); err != nil {
		fatal("%v", err)
	}
	fmt.Printf("OK  %s\n", filepath.Join(*outDir, "sitemap.xml"))
}

func loadDicts(dir string) ([]*langDict, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "site_*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var langs []*langDict
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		d := &langDict{}
		if err := json.Unmarshal(b, d); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		d.code = strings.TrimSuffix(strings.TrimPrefix(filepath.Base(p), "site_"), ".json")
		if d.Lang == "" {
			d.Lang = d.code
		}
		if d.Hreflang == "" {
			d.Hreflang = d.Lang
		}
		if d.InLanguage == "" {
			d.InLanguage = d.Lang
		}
		langs = append(langs, d)
	}
	return langs, nil
}

// absolutizePaths troca os caminhos relativos (./x) por absolutos (/x),
// para os mesmos assets servirem a raiz e as subpastas /<code>/.
func absolutizePaths(page string) string {
	page = strings.ReplaceAll(page, `href="./`, `href="/`)
	page = strings.ReplaceAll(page, `src="./`, `src="/`)
	page = strings.ReplaceAll(page, `fetch('./`, `fetch('/`)
	return page
}

// localizeMeta ajusta os metadados estruturais (lang, canonical, OG, JSON-LD)
// que não são texto visível e por isso não vivem no dicionário.
func localizeMeta(page string, d *langDict, base string) string {
	langURL := base + "/" + d.code + "/"
	page = replaceMarker(page, d.code, "html lang",
		`<html lang="pt-BR">`, fmt.Sprintf(`<html lang="%s">`, d.Lang))
	page = replaceMarker(page, d.code, "canonical",
		fmt.Sprintf(`<link rel="canonical" href="%s/">`, base),
		fmt.Sprintf(`<link rel="canonical" href="%s">`, langURL))
	page = replaceMarker(page, d.code, "og:locale",
		`<meta property="og:locale" content="pt_BR">`,
		fmt.Sprintf(`<meta property="og:locale" content="%s">`, d.OgLocale))
	page = replaceMarker(page, d.code, "og:url",
		fmt.Sprintf(`<meta property="og:url" content="%s/">`, base),
		fmt.Sprintf(`<meta property="og:url" content="%s">`, langURL))
	page = replaceMarker(page, d.code, "json-ld url",
		fmt.Sprintf(`"url": "%s/"`, base),
		fmt.Sprintf(`"url": "%s"`, langURL))
	page = replaceMarker(page, d.code, "json-ld inLanguage",
		`"inLanguage": "pt-BR"`,
		fmt.Sprintf(`"inLanguage": "%s"`, d.InLanguage))
	return page
}

// translate aplica o dicionário PT->tradução em uma única passada
// (strings.Replacer não re-traduz texto já substituído). Chaves mais longas
// têm prioridade quando duas casam na mesma posição.
func translate(page string, d *langDict) string {
	keys := make([]string, 0, len(d.Strings))
	for k := range d.Strings {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) > len(keys[j])
		}
		return keys[i] < keys[j]
	})
	pairs := make([]string, 0, 2*len(keys))
	for _, k := range keys {
		if !strings.Contains(page, k) {
			fmt.Printf("WARN [%s]: string PT não encontrada no index.html: %q\n", d.code, snip(k))
			continue
		}
		pairs = append(pairs, k, d.Strings[k])
	}
	return strings.NewReplacer(pairs...).Replace(page)
}

// injectAfterCanonical insere o cluster hreflang logo após a tag canonical.
func injectAfterCanonical(page, cluster, code string) string {
	const marker = `<link rel="canonical"`
	i := strings.Index(page, marker)
	if i < 0 {
		fmt.Printf("WARN [%s]: tag canonical não encontrada; hreflang não injetado\n", code)
		return page
	}
	end := strings.Index(page[i:], ">")
	if end < 0 {
		fmt.Printf("WARN [%s]: tag canonical malformada; hreflang não injetado\n", code)
		return page
	}
	at := i + end + 1
	return page[:at] + "\n" + cluster + page[at:]
}

// nativeNames: rótulo do idioma no próprio idioma, pro seletor da página.
// Fallback: o próprio código. Mesma lista do seletor do painel.
var nativeNames = map[string]string{
	"pt-BR": "Português (Brasil)", "en": "English", "es": "Español", "de": "Deutsch",
	"fr": "Français", "pl": "Polski", "tr": "Türkçe", "ru": "Русский",
	"uk": "Українська", "id": "Bahasa Indonesia", "vi": "Tiếng Việt", "th": "ไทย",
	"ko": "한국어", "ja": "日本語", "zh-Hans": "简体中文", "zh-Hant": "繁體中文",
}

func nativeName(code string) string {
	if n, ok := nativeNames[code]; ok {
		return n
	}
	return code
}

// injectLangSelector insere um <select> de idiomas no nav do header. Navegação
// direta por URL (sem JS além do onchange) pra funcionar em qualquer página.
func injectLangSelector(page string, langs []*langDict, current string) string {
	const marker = `<nav style="display:flex;gap:10px;align-items:center">`
	var b strings.Builder
	b.WriteString(`<select class="lang-sel" aria-label="Idioma / Language" onchange="location.href=this.value" style="font-family:var(--font-d);font-weight:600;font-size:13px;color:var(--txt-dim);background:var(--bg-2);border:1px solid var(--line);border-radius:9px;padding:8px 10px;cursor:pointer">`)
	writeOpt := func(code, href string) {
		sel := ""
		if code == current {
			sel = " selected"
		}
		fmt.Fprintf(&b, `<option value="%s"%s>%s</option>`, href, sel, nativeName(code))
	}
	writeOpt("pt-BR", "/")
	for _, d := range langs {
		writeOpt(d.code, "/"+d.code+"/")
	}
	b.WriteString(`</select>`)
	return replaceMarker(page, current, "nav do header (seletor de idioma)", marker, marker+b.String())
}

func hreflangCluster(base string, langs []*langDict) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\t<link rel=\"alternate\" hreflang=\"pt-BR\" href=\"%s/\">\n", base)
	for _, d := range langs {
		fmt.Fprintf(&b, "\t<link rel=\"alternate\" hreflang=\"%s\" href=\"%s/%s/\">\n", d.Hreflang, base, d.code)
	}
	fmt.Fprintf(&b, "\t<link rel=\"alternate\" hreflang=\"x-default\" href=\"%s/\">", base)
	return b.String()
}

func sitemapXML(base string, langs []*langDict, today string) string {
	var alt strings.Builder
	fmt.Fprintf(&alt, "\t\t<xhtml:link rel=\"alternate\" hreflang=\"pt-BR\" href=\"%s/\"/>\n", base)
	for _, d := range langs {
		fmt.Fprintf(&alt, "\t\t<xhtml:link rel=\"alternate\" hreflang=\"%s\" href=\"%s/%s/\"/>\n", d.Hreflang, base, d.code)
	}
	fmt.Fprintf(&alt, "\t\t<xhtml:link rel=\"alternate\" hreflang=\"x-default\" href=\"%s/\"/>", base)

	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	b.WriteString("<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\" xmlns:xhtml=\"http://www.w3.org/1999/xhtml\">\n")
	writeURL := func(loc, lastmod, freq, prio string, withAlt bool) {
		b.WriteString("\t<url>\n")
		fmt.Fprintf(&b, "\t\t<loc>%s</loc>\n", loc)
		if lastmod != "" {
			fmt.Fprintf(&b, "\t\t<lastmod>%s</lastmod>\n", lastmod)
		}
		fmt.Fprintf(&b, "\t\t<changefreq>%s</changefreq>\n", freq)
		fmt.Fprintf(&b, "\t\t<priority>%s</priority>\n", prio)
		if withAlt {
			b.WriteString(alt.String())
			b.WriteString("\n")
		}
		b.WriteString("\t</url>\n")
	}
	writeURL(base+"/", today, "weekly", "1.0", true)
	for _, d := range langs {
		writeURL(base+"/"+d.code+"/", today, "weekly", "0.9", true)
	}
	writeURL(base+"/privacy.html", "", "yearly", "0.3", false)
	writeURL(base+"/terms.html", "", "yearly", "0.3", false)
	b.WriteString("</urlset>\n")
	return b.String()
}

func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("criando %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("escrevendo %s: %w", path, err)
	}
	return nil
}

func replaceMarker(page, code, what, old, new string) string {
	if !strings.Contains(page, old) {
		fmt.Printf("WARN [%s]: marcador %s não encontrado: %q\n", code, what, snip(old))
		return page
	}
	return strings.ReplaceAll(page, old, new)
}

func snip(s string) string {
	if len(s) > 70 {
		return s[:70] + "…"
	}
	return s
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "sitegen: "+format+"\n", args...)
	os.Exit(1)
}
