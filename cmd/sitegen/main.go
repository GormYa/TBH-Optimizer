// Command sitegen gera as versões multilíngues da landing estática.
//
// Fonte da verdade: <site>/index.html e os guias (guide/tutorial/chest-drops), em pt-BR.
// Para cada dicionário <site>/i18n/site_<code>.json é gerada, por página, uma versão
// <out>/<code>/<page> com os textos traduzidos por substituição de string (PT -> tradução).
// As páginas-raiz em <out>/ continuam pt-BR, mas ganham o cluster hreflang, o seletor de
// idioma e caminhos absolutos. Também gera <out>/sitemap.xml com todas as variantes.
//
// O index usa o mapa "strings"; os guias usam "pages": { "<page>": { pt -> tradução } }.
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

// translatedPages: páginas (além do index) que ganham versão por idioma. A chave bate com
// o nome do arquivo e com a chave em langDict.Pages.
var translatedPages = []string{"guide.html", "tutorial.html", "chest-drops.html"}

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
	// Pages: por página (guide.html/...), o mapa pt-BR -> tradução daquele guia.
	Pages map[string]map[string]string `json:"pages"`

	code string // derivado do nome do arquivo site_<code>.json; vira o path /<code>/
}

func main() {
	siteDir := flag.String("site", "site", "pasta com index.html e i18n/site_*.json")
	outDir := flag.String("out", "public", "pasta de saída")
	baseURL := flag.String("base", "https://taskbarhero.fun", "URL pública do site, sem barra final")
	flag.Parse()

	base := strings.TrimSuffix(*baseURL, "/")

	langs, err := loadDicts(filepath.Join(*siteDir, "i18n"))
	if err != nil {
		fatal("carregando dicionários: %v", err)
	}
	if len(langs) == 0 {
		fmt.Println("WARN: nenhum dicionário site_*.json encontrado em", filepath.Join(*siteDir, "i18n"))
	}

	// index.html (suffix "" => URL raiz "/") + cada guia.
	pages := append([]string{""}, translatedPages...)
	for _, page := range pages {
		srcName := page
		if page == "" {
			srcName = "index.html"
		}
		srcBytes, err := os.ReadFile(filepath.Join(*siteDir, srcName))
		if err != nil {
			if page == "" {
				fatal("lendo index.html: %v", err)
			}
			fmt.Printf("WARN: pulando %s: %v\n", srcName, err)
			continue
		}
		src := string(srcBytes)
		cluster := hreflangCluster(base, langs, page)

		// Raiz pt-BR: caminhos absolutos + hreflang + seletor. Conteúdo/SEO inalterados.
		root := absolutizePaths(src)
		root = injectAfterCanonical(root, cluster, "pt-BR")
		root = injectLangSelector(root, langs, "pt-BR", page)
		rootOut := filepath.Join(*outDir, srcName)
		if err := writeFile(rootOut, root); err != nil {
			fatal("%v", err)
		}
		fmt.Printf("OK  %s (pt-BR, raiz)\n", rootOut)

		for _, d := range langs {
			// Traduz ANTES de absolutizar: as chaves PT têm links com "./" (href="./x.html");
			// absolutizar primeiro (./ -> /) quebraria o match. Depois absolutiza tudo (inclusive
			// os links no texto traduzido) e localiza os links internos para /<code>/.
			p := translatePage(src, d, page)
			p = absolutizePaths(p)
			p = localizeMeta(p, d, base, page)
			p = localizeInternalLinks(p, d.code)
			p = injectAfterCanonical(p, cluster, d.code)
			p = injectLangSelector(p, langs, d.code, page)
			out := filepath.Join(*outDir, d.code, srcName)
			if err := writeFile(out, p); err != nil {
				fatal("%v", err)
			}
			fmt.Printf("OK  %s (%s)\n", out, d.Lang)
		}
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

// localizeMeta ajusta os metadados estruturais (lang, canonical, OG, JSON-LD) por página.
// suffix é o caminho da página relativo à raiz ("" para o index, "guide.html" etc.).
func localizeMeta(page string, d *langDict, base, suffix string) string {
	ptURL := base + "/" + suffix
	langURL := base + "/" + d.code + "/" + suffix
	page = replaceMarker(page, d.code, "html lang",
		`<html lang="pt-BR">`, fmt.Sprintf(`<html lang="%s">`, d.Lang))
	page = replaceMarker(page, d.code, "canonical",
		fmt.Sprintf(`<link rel="canonical" href="%s">`, ptURL),
		fmt.Sprintf(`<link rel="canonical" href="%s">`, langURL))
	page = replaceMarker(page, d.code, "og:locale",
		`<meta property="og:locale" content="pt_BR">`,
		fmt.Sprintf(`<meta property="og:locale" content="%s">`, d.OgLocale))
	page = replaceMarker(page, d.code, "og:url",
		fmt.Sprintf(`<meta property="og:url" content="%s">`, ptURL),
		fmt.Sprintf(`<meta property="og:url" content="%s">`, langURL))
	// JSON-LD: troca tanto a URL canônica da página quanto referências de breadcrumb a ela.
	page = strings.ReplaceAll(page, fmt.Sprintf(`"url": "%s"`, ptURL), fmt.Sprintf(`"url": "%s"`, langURL))
	page = strings.ReplaceAll(page, fmt.Sprintf(`"item": "%s"`, ptURL), fmt.Sprintf(`"item": "%s"`, langURL))
	return page
}

// translatePage aplica o dicionário PT->tradução da página em uma única passada. O index
// usa d.Strings; os demais usam d.Pages[suffix]. Chaves mais longas têm prioridade.
func translatePage(page string, d *langDict, suffix string) string {
	dict := d.Strings
	if suffix != "" {
		dict = d.Pages[suffix]
	}
	if len(dict) == 0 {
		if suffix != "" {
			fmt.Printf("WARN [%s]: sem traduções para %s\n", d.code, suffix)
		}
		return page
	}
	keys := make([]string, 0, len(dict))
	for k := range dict {
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
			fmt.Printf("WARN [%s/%s]: string PT não encontrada: %q\n", d.code, suffixOrIndex(suffix), snip(k))
			continue
		}
		pairs = append(pairs, k, dict[k])
	}
	return strings.NewReplacer(pairs...).Replace(page)
}

func suffixOrIndex(s string) string {
	if s == "" {
		return "index.html"
	}
	return s
}

// localizeInternalLinks reescreve, nas páginas de idioma, os links internos para as páginas
// que TÊM versão traduzida (index + guias), apontando para a subpasta /<code>/. Privacy,
// terms e assets (favicon/og) ficam na raiz pt-BR.
func localizeInternalLinks(page, code string) string {
	for _, p := range translatedPages {
		page = strings.ReplaceAll(page, `href="/`+p+`"`, `href="/`+code+`/`+p+`"`)
	}
	page = strings.ReplaceAll(page, `href="/index.html"`, `href="/`+code+`/"`)
	return page
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

// injectLangSelector insere um <select> de idiomas no nav do header. Navega direto por URL
// (sem JS além do onchange) pra mesma página no idioma escolhido. suffix é o caminho da
// página ("" para o index).
func injectLangSelector(page string, langs []*langDict, current, suffix string) string {
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
	writeOpt("pt-BR", "/"+suffix)
	for _, d := range langs {
		writeOpt(d.code, "/"+d.code+"/"+suffix)
	}
	b.WriteString(`</select>`)
	return replaceMarker(page, current, "nav do header (seletor de idioma)", marker, marker+b.String())
}

// hreflangCluster monta os <link rel="alternate"> para uma página (suffix "" = index).
func hreflangCluster(base string, langs []*langDict, suffix string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\t<link rel=\"alternate\" hreflang=\"pt-BR\" href=\"%s/%s\">\n", base, suffix)
	for _, d := range langs {
		fmt.Fprintf(&b, "\t<link rel=\"alternate\" hreflang=\"%s\" href=\"%s/%s/%s\">\n", d.Hreflang, base, d.code, suffix)
	}
	fmt.Fprintf(&b, "\t<link rel=\"alternate\" hreflang=\"x-default\" href=\"%s/%s\">", base, suffix)
	return b.String()
}

var untranslatedSecondary = []struct {
	path, freq, prio string
	stamp            bool
}{
	{"/privacy.html", "yearly", "0.3", false},
	{"/terms.html", "yearly", "0.3", false},
}

func sitemapXML(base string, langs []*langDict, today string) string {
	// bloco de alternates para um dado suffix de página.
	altBlock := func(suffix string) string {
		var alt strings.Builder
		fmt.Fprintf(&alt, "\t\t<xhtml:link rel=\"alternate\" hreflang=\"pt-BR\" href=\"%s/%s\"/>\n", base, suffix)
		for _, d := range langs {
			fmt.Fprintf(&alt, "\t\t<xhtml:link rel=\"alternate\" hreflang=\"%s\" href=\"%s/%s/%s\"/>\n", d.Hreflang, base, d.code, suffix)
		}
		fmt.Fprintf(&alt, "\t\t<xhtml:link rel=\"alternate\" hreflang=\"x-default\" href=\"%s/%s\"/>", base, suffix)
		return alt.String()
	}

	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	b.WriteString("<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\" xmlns:xhtml=\"http://www.w3.org/1999/xhtml\">\n")
	writeURL := func(loc, lastmod, freq, prio, alt string) {
		b.WriteString("\t<url>\n")
		fmt.Fprintf(&b, "\t\t<loc>%s</loc>\n", loc)
		if lastmod != "" {
			fmt.Fprintf(&b, "\t\t<lastmod>%s</lastmod>\n", lastmod)
		}
		fmt.Fprintf(&b, "\t\t<changefreq>%s</changefreq>\n", freq)
		fmt.Fprintf(&b, "\t\t<priority>%s</priority>\n", prio)
		if alt != "" {
			b.WriteString(alt)
			b.WriteString("\n")
		}
		b.WriteString("\t</url>\n")
	}

	// index (raiz + cada idioma) com alternates.
	writeURL(base+"/", today, "weekly", "1.0", altBlock(""))
	for _, d := range langs {
		writeURL(base+"/"+d.code+"/", today, "weekly", "0.9", altBlock(""))
	}
	// guias traduzidos (raiz + cada idioma) com alternates.
	for _, p := range translatedPages {
		writeURL(base+"/"+p, today, "weekly", "0.8", altBlock(p))
		for _, d := range langs {
			writeURL(base+"/"+d.code+"/"+p, today, "weekly", "0.7", altBlock(p))
		}
	}
	// páginas sem tradução (privacy/terms): sem alternates.
	for _, p := range untranslatedSecondary {
		lastmod := ""
		if p.stamp {
			lastmod = today
		}
		writeURL(base+p.path, lastmod, p.freq, p.prio, "")
	}
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
