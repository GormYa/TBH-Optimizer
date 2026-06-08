package internal

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// O catalogo (taxas, loot, sprites) vem dos endpoints JSON da taskbarhero.wiki,
// que acompanham os patches do jogo. UpdateChestData regenera tudo localmente.
const wikiOrigin = "https://taskbarhero.wiki"

// ---- formato dos JSONs da wiki ----
type wikiItem struct {
	ID    int               `json:"id"`
	Name  map[string]string `json:"name"`
	Grade string            `json:"grade"`
	Type  string            `json:"type"`
	Icon  string            `json:"icon"`
}
type wikiStage struct {
	Key        int      `json:"key"`
	Act        int      `json:"act"`
	No         int      `json:"no"`
	Difficulty string   `json:"difficulty"`
	Via        string   `json:"via"`
	Rate       *float64 `json:"rate"`
}
type wikiGroup struct {
	ID       int               `json:"id"`
	Name     string            `json:"name"`
	NameI18n map[string]string `json:"name_i18n"`
}
type wikiEntry struct {
	Pct   float64            `json:"pct"`
	Pcts  map[string]float64 `json:"pcts"`
	Group wikiGroup          `json:"group"`
}
type wikiBox struct {
	Table struct {
		Entries []wikiEntry `json:"entries"`
	} `json:"table"`
	Stages []wikiStage `json:"stages"`
}

// ---- formato gerado (servido ao front) ----
type ChestStage struct {
	Key             int     `json:"key"`
	Label           string  `json:"label"`
	Difficulty      string  `json:"difficulty"`
	Via             string  `json:"via"`
	DropRatePercent float64 `json:"dropRatePercent"`
}
type ChestLootGroup struct {
	Weight float64 `json:"weight"`
	Hero   string  `json:"hero"`
	Grade  string  `json:"grade"`
	Items  []int   `json:"items"`
}

// HeroInfo descreve um heroi cuja posse muda a pool de loot (vira toggle no front).
type HeroInfo struct {
	Name string `json:"name"`
	DLC  bool   `json:"dlc"`
}
type ChestEntry struct {
	ID      int              `json:"id"`
	Name    string           `json:"name"`
	Grade   string           `json:"grade"`
	Type    string           `json:"type"`
	IconURL string           `json:"iconUrl"`
	Stages  []ChestStage     `json:"stages"`
	Loot    []ChestLootGroup `json:"loot"`
}
type ItemInfo struct {
	Name  string `json:"name"`
	Grade string `json:"grade"`
	Icon  string `json:"icon"`
}
type ChestDoc struct {
	Schema    map[string]string   `json:"_schema"`
	UpdatedAt string              `json:"updatedAt"`
	Heroes    map[string]HeroInfo `json:"heroes"`
	Chests    []ChestEntry        `json:"chests"`
	ItemsByID map[string]ItemInfo `json:"itemsById"`
}

func pickName(m map[string]string) string {
	if v := m["pt-BR"]; v != "" {
		return v
	}
	if v := m["en-US"]; v != "" {
		return v
	}
	for _, v := range m {
		return v
	}
	return ""
}

func gradeToType(grade string) string {
	switch grade {
	case "COMMON":
		return "Common"
	case "RARE":
		return "Stage Boss"
	case "LEGENDARY":
		return "Act Boss"
	default:
		return grade
	}
}

// localSpritePath converte o icone da wiki ("/game/items/boxes/Item_X.png" ou URL
// completa) no caminho local servido ("sprites/items/boxes/Item_X.png").
func localSpritePath(icon string) string {
	if i := strings.Index(icon, "/game/"); i >= 0 {
		return "sprites/" + icon[i+len("/game/"):]
	}
	return "sprites/" + icon[strings.LastIndex(icon, "/")+1:]
}

// buildChest e a transformacao pura item+box -> entrada de baú.
func buildChest(item wikiItem, box wikiBox) ChestEntry {
	c := ChestEntry{
		ID:      item.ID,
		Name:    pickName(item.Name),
		Grade:   item.Grade,
		Type:    gradeToType(item.Grade),
		IconURL: localSpritePath(item.Icon),
		Stages:  []ChestStage{},
		Loot:    []ChestLootGroup{},
	}
	for _, s := range box.Stages {
		rate := 100.0
		if s.Rate != nil {
			rate = *s.Rate / 10.0
		}
		c.Stages = append(c.Stages, ChestStage{
			Key:             s.Key,
			Label:           fmt.Sprintf("%d-%d", s.Act, s.No),
			Difficulty:      s.Difficulty,
			Via:             s.Via,
			DropRatePercent: rate,
		})
	}
	return c
}

// fetchLootPools puxa do taskbarherowiki.com a lista de itens por grupo (que o
// taskbarhero.wiki resolve so no navegador). Devolve boxId -> grupos de loot.
// boxId = dropKey/10 (ex.: dropKey 9100111 -> box 910011).
func fetchLootPools(client *http.Client) (map[int][]ChestLootGroup, error) {
	req, _ := http.NewRequest("GET", "https://taskbarherowiki.com/chests", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	re := regexp.MustCompile(`(?s)self\.__next_f\.push\(\[1,"(.*?)"\]\)`)
	var sb strings.Builder
	for _, m := range re.FindAllSubmatch(body, -1) {
		var s string
		if json.Unmarshal([]byte(`"`+string(m[1])+`"`), &s) == nil {
			sb.WriteString(s)
		}
	}
	blob := sb.String()
	i := strings.Index(blob, `[{"dropKey"`)
	if i < 0 {
		return nil, fmt.Errorf("dados de loot nao encontrados no taskbarherowiki.com")
	}

	type twPool struct {
		Label   string `json:"label"`
		Entries []struct {
			Probability float64 `json:"probability"`
			Items       []int   `json:"items"`
		} `json:"entries"`
	}
	var tw []struct {
		DropKey int      `json:"dropKey"`
		Pools   []twPool `json:"pools"`
	}
	if err := json.NewDecoder(strings.NewReader(blob[i:])).Decode(&tw); err != nil {
		return nil, err
	}

	out := map[int][]ChestLootGroup{}
	for _, c := range tw {
		boxID := c.DropKey / 10
		base := -1
		for i := range c.Pools {
			if c.Pools[i].Label == "Base" {
				base = i
				break
			}
		}
		if base < 0 && len(c.Pools) > 0 {
			base = 0
		}
		if base < 0 {
			continue
		}
		for _, e := range c.Pools[base].Entries {
			if len(e.Items) == 0 {
				continue
			}
			out[boxID] = append(out[boxID], ChestLootGroup{Weight: e.Probability * 100, Hero: "", Items: e.Items})
		}
	}
	return out, nil
}

// ---- IO ----
func wikiGetJSON(client *http.Client, url string, target any) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0")
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(400 * time.Millisecond)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			lastErr = fmt.Errorf("HTTP %d em %s", resp.StatusCode, url)
			time.Sleep(400 * time.Millisecond)
			continue
		}
		return json.Unmarshal(body, target)
	}
	return lastErr
}

func downloadSprite(client *http.Client, icon, dir string) error {
	dest := filepath.Join(dir, filepath.FromSlash(localSpritePath(icon)))
	if fi, err := os.Stat(dest); err == nil && fi.Size() > 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	req, _ := http.NewRequest("GET", wikiOrigin+icon, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d ao baixar %s", resp.StatusCode, icon)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0644)
}

// UpdateChestData puxa o catalogo da wiki e regenera dir/chest_drops.json + dir/sprites.
// updatedAt e gravado pelo chamador (passado em now) para manter a funcao testavel.
func UpdateChestData(dir, now string) (int, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	var items []wikiItem
	if err := wikiGetJSON(client, wikiOrigin+"/data/items.json", &items); err != nil {
		return 0, fmt.Errorf("items.json: %w", err)
	}

	// Catalogo de itens (id -> nome PT-BR + caminho de icone) p/ resolver o loot.
	type catItem struct{ name, icon, grade string }
	catalog := map[int]catItem{}
	for _, it := range items {
		catalog[it.ID] = catItem{name: pickName(it.Name), icon: it.Icon, grade: it.Grade}
	}

	pools, errPools := fetchLootPools(client)
	if errPools != nil {
		fmt.Println("Aviso: lista de itens por grupo indisponivel:", errPools)
		pools = map[int][]ChestLootGroup{}
	}

	doc := ChestDoc{
		Schema: map[string]string{
			"source":      "taxas/sprites/nomes: taskbarhero.wiki; itens por grupo: taskbarherowiki.com",
			"dropRate":    "stages[].dropRatePercent em %; Act Boss (rate null) = 100% garantido",
			"probability": "loot[].probability = chance do GRUPO; por item ~ probability/len(items)",
		},
		UpdatedAt: now,
		Heroes:    map[string]HeroInfo{},
		ItemsByID: map[string]ItemInfo{},
	}

	referenced := map[int]bool{}
	for _, it := range items {
		if it.Type != "STAGEBOX" {
			continue
		}
		var box wikiBox
		url := fmt.Sprintf("%s/data/boxes/%d.json", wikiOrigin, it.ID)
		if err := wikiGetJSON(client, url, &box); err != nil {
			fmt.Println("Aviso: falha ao baixar", url, "-", err)
			continue
		}
		c := buildChest(it, box)
		if lg, ok := pools[it.ID]; ok {
			c.Loot = lg
		}
		for gi := range c.Loot {
			if len(c.Loot[gi].Items) > 0 {
				c.Loot[gi].Grade = catalog[c.Loot[gi].Items[0]].grade
			}
			for _, id := range c.Loot[gi].Items {
				referenced[id] = true
			}
		}
		doc.Chests = append(doc.Chests, c)
		if err := downloadSprite(client, it.Icon, dir); err != nil {
			fmt.Println("Aviso: sprite", it.Icon, "-", err)
		}
	}

	if len(doc.Chests) == 0 {
		return 0, fmt.Errorf("nenhum baú obtido da wiki")
	}

	for id := range referenced {
		ci, ok := catalog[id]
		if !ok {
			continue
		}
		doc.ItemsByID[fmt.Sprint(id)] = ItemInfo{Name: ci.name, Grade: ci.grade, Icon: localSpritePath(ci.icon)}
		if err := downloadSprite(client, ci.icon, dir); err != nil {
			fmt.Println("Aviso: sprite item", id, "-", err)
		}
	}

	out, err := json.MarshalIndent(doc, "", " ")
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return 0, err
	}
	tmp := filepath.Join(dir, "chest_drops.json.tmp")
	if err := os.WriteFile(tmp, out, 0644); err != nil {
		return 0, err
	}
	return len(doc.Chests), os.Rename(tmp, filepath.Join(dir, "chest_drops.json"))
}
