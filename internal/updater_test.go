package internal

import "testing"

func f64ptr(v float64) *float64 { return &v }

// buildChest transforma o item (catalogo) + o box JSON da wiki numa entrada de
// baú pronta: tipo a partir do grade, taxa por fase (rate/10; null = 100% garantido),
// loot por grupo (probabilidade "base" + nome PT-BR), e icone como caminho local.
func TestBuildChest(t *testing.T) {
	item := wikiItem{
		ID:    910011,
		Name:  map[string]string{"pt-BR": "Caixa de Monstro 1", "en-US": "Normal Monster Box 1"},
		Grade: "COMMON",
		Type:  "STAGEBOX",
		Icon:  "/game/items/boxes/Item_910011.png",
	}
	var box wikiBox
	box.Stages = []wikiStage{
		{Key: 1101, Act: 1, No: 1, Difficulty: "NORMAL", Via: "monster", Rate: f64ptr(160)},
		{Key: 1305, Act: 3, No: 5, Difficulty: "NORMAL", Via: "boss", Rate: nil}, // garantido
	}
	c := buildChest(item, box)

	if c.ID != 910011 || c.Name != "Caixa de Monstro 1" || c.Type != "Common" {
		t.Errorf("cabecalho errado: %+v", c)
	}
	if c.IconURL != "sprites/items/boxes/Item_910011.png" {
		t.Errorf("iconUrl = %q", c.IconURL)
	}
	if len(c.Stages) != 2 {
		t.Fatalf("stages = %d, quero 2", len(c.Stages))
	}
	if c.Stages[0].DropRatePercent != 16.0 || c.Stages[0].Label != "1-1" {
		t.Errorf("stage0 = %+v (quero 16%% e '1-1')", c.Stages[0])
	}
	if c.Stages[1].DropRatePercent != 100.0 {
		t.Errorf("stage garantido (rate null) deveria ser 100%%, veio %v", c.Stages[1].DropRatePercent)
	}
	if c.Loot == nil {
		t.Error("Loot deveria iniciar como slice vazio, nao nil")
	}
}

func TestLocalSpritePath(t *testing.T) {
	if got := localSpritePath("/game/items/boxes/Item_910011.png"); got != "sprites/items/boxes/Item_910011.png" {
		t.Errorf("got %q", got)
	}
	if got := localSpritePath("https://x/game/gear/armor/A.png"); got != "sprites/gear/armor/A.png" {
		t.Errorf("got %q", got)
	}
}
