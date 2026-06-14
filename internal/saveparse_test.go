package internal

import (
	"encoding/json"
	"testing"
)

// TestHeroExpAceitaStringOuNumero: o jogo às vezes serializa HeroExp como string
// ("1000.5") em vez de número, o que quebrava o parse com "cannot unmarshal string
// ... of type float64". FlexFloat deve aceitar ambas as formas (e "" / null = 0).
func TestHeroExpAceitaStringOuNumero(t *testing.T) {
	cases := map[string]float64{
		`{"HeroExp": 1000.5}`:   1000.5,
		`{"HeroExp": "1000.5"}`: 1000.5,
		`{"HeroExp": "1000,5"}`: 1000.5,
		`{"HeroExp": "1.234,5"}`: 1234.5,
		`{"HeroExp": "1234567"}`: 1234567,
		`{"HeroExp": "0"}`:      0,
		`{"HeroExp": ""}`:       0,
		`{"HeroExp": null}`:     0,
		`{}`:                    0,
	}
	for raw, want := range cases {
		var h Hero
		if err := json.Unmarshal([]byte(raw), &h); err != nil {
			t.Fatalf("%s: erro inesperado: %v", raw, err)
		}
		if float64(h.HeroExp) != want {
			t.Errorf("%s: HeroExp = %v, esperado %v", raw, float64(h.HeroExp), want)
		}
	}
}

// O save real (PlayerSaveData) traz equipados/enchants/atributos/cubo/baús —
// campos novos do roadmap. Fixture com a forma exata vista no save de verdade.
func TestInnerSaveDataParseiaCamposNovos(t *testing.T) {
	raw := `{
		"commonSaveData": {"currentStageKey": 2309, "currentStageWave": 0, "playTime": 1234.5,
			"maxCompletedStage": 2309, "arrangedHeroKey": [201, 401], "ArrangedPetKey": 3},
		"BoxData": {"BoxTypes": [0, 1], "BoxUniqueId": [502860785626520512, 502860785626520513], "BoxQuantity": [11, 2]},
		"currenySaveDatas": [{"Key": 100001, "Quantity": 500}],
		"heroSaveDatas": [{
			"heroKey": 401, "HeroLevel": 55, "HeroExp": 1000.5, "IsUnLock": true,
			"AbilityPoint": 12, "AllocatedHeroAbilityPoint": 9,
			"equippedItemIds": [515245594311006810, 0, 0, 0, 0, 0, 0, 0, 0, 0],
			"equippedSKillKey": [40101, 40102, 0],
			"unlockedAttributeGroupKeys": [10002, 10003]
		}],
		"attributeSaveDatas": [{"Key": 201001, "Level": 3}, {"Key": 201002, "Level": 10}],
		"itemSaveDatas": [{
			"ItemKey": 315071, "UniqueId": 515245594311006810, "IsChaotic": false,
			"EnchantCount": [2, 1, 0],
			"EnchantData": [
				{"StatModKey": 102401, "Tier": 2, "Value": 200, "RecipeType": 3, "ModType": 0, "MaterialKey": 110005, "StatType": 24},
				{"StatModKey": 102401, "Tier": 2, "Value": 300, "RecipeType": 3, "ModType": 0, "MaterialKey": 110005, "StatType": 24},
				{"StatModKey": 102401, "Tier": 3, "Value": 350, "RecipeType": 4, "ModType": 0, "MaterialKey": 121002, "StatType": 24},
				{"StatModKey": 0, "Tier": 0, "Value": 0, "RecipeType": 0, "ModType": 0, "MaterialKey": 0, "StatType": 0},
				{"StatModKey": 0, "Tier": 0, "Value": 0, "RecipeType": 0, "ModType": 0, "MaterialKey": 0, "StatType": 0},
				{"StatModKey": 0, "Tier": 0, "Value": 0, "RecipeType": 0, "ModType": 0, "MaterialKey": 0, "StatType": 0}
			],
			"DecorationAppliedTotalCount": 2, "EngravingAppliedTotalCount": 1, "InscriptionAppliedTotalCount": 0
		}],
		"RuneSaveData": [{"RuneKey": 1, "Level": 2}],
		"PetSaveData": [{"PetKey": 1, "IsUnlock": true}],
		"inventorySaveDatas": [{"Index": 0, "ItemUniqueId": 515245594311006810, "IsUnlock": true, "IsUnlockedByRune": false}],
		"stashSaveDatas": [{"Index": 0, "ItemUniqueId": 515245594311006811, "IsUnLock": true}, {"Index": 1, "ItemUniqueId": 0, "IsUnLock": true}],
		"tradingStashSaveDatas": [{"Index": 0, "ItemUniqueId": 0, "IsUnLock": false}],
		"cubeSaveLevelData": {"Level": 36, "Exp": 207314.266},
		"cubeRecipeSaveDatas": [
			{"CubeRecipeTypeInt": 1, "CubeKey": 100001, "MaxUnlockRecipeKey": 100003},
			{"CubeRecipeTypeInt": 0, "CubeKey": 200001, "MaxUnlockRecipeKey": 200001},
			{"CubeRecipeTypeInt": 2, "CubeKey": 600001, "MaxUnlockRecipeKey": 0}
		]
	}`

	var s InnerSaveData
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	h := s.HeroSaveDatas[0]
	if h.EquippedItemIds[0] != 515245594311006810 {
		t.Fatalf("equippedItemIds[0] = %d", h.EquippedItemIds[0])
	}
	if len(h.EquippedItemIds) != 10 || h.AbilityPoint != 12 || !h.IsUnLock {
		t.Fatalf("hero mal parseado: %+v", h)
	}

	it := s.ItemSaveDatas[0]
	if it.UniqueId != h.EquippedItemIds[0] {
		t.Fatal("o item equipado deveria cruzar com o UniqueId do inventário")
	}
	if it.EnchantCount[0] != 2 || it.EnchantData[2].Value != 350 || it.EnchantData[2].MaterialKey != 121002 {
		t.Fatalf("enchants mal parseados: %+v", it)
	}

	if len(s.AttributeSaveDatas) != 2 || s.AttributeSaveDatas[1].Level != 10 {
		t.Fatalf("atributos mal parseados: %+v", s.AttributeSaveDatas)
	}
	if s.CubeSaveLevelData.Level != 36 {
		t.Fatalf("cubo mal parseado: %+v", s.CubeSaveLevelData)
	}
	if s.BoxData.BoxQuantity[0] != 11 || s.BoxData.BoxTypes[1] != 1 {
		t.Fatalf("BoxData mal parseado: %+v", s.BoxData)
	}
	if s.StashSaveDatas[0].ItemUniqueId != 515245594311006811 || !s.StashSaveDatas[0].IsUnLock {
		t.Fatalf("stash mal parseado: %+v", s.StashSaveDatas)
	}
	if s.InventorySaveDatas[0].ItemUniqueId != 515245594311006810 || !s.InventorySaveDatas[0].IsUnlock {
		t.Fatalf("inventário mal parseado: %+v", s.InventorySaveDatas)
	}
	if s.TradingStashDatas[0].IsUnLock {
		t.Fatalf("banca mal parseada: %+v", s.TradingStashDatas)
	}
	if len(s.CubeRecipeSaveDatas) != 3 || s.CubeRecipeSaveDatas[0].CubeKey != 100001 ||
		s.CubeRecipeSaveDatas[0].MaxUnlockRecipeKey != 100003 || s.CubeRecipeSaveDatas[2].MaxUnlockRecipeKey != 0 {
		t.Fatalf("receitas do cubo mal parseadas: %+v", s.CubeRecipeSaveDatas)
	}
}
