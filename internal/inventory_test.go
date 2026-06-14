package internal

import "testing"

func TestResolveInventory(t *testing.T) {
	save := &InnerSaveData{
		ItemSaveDatas: []Item{
			{ItemKey: 300001, UniqueId: 1001, IsChaotic: true, EnchantData: []ItemEnchant{
				{StatModKey: 5001}, {StatModKey: 0}, // 1 enchant rolado
			}},
			{ItemKey: 310005, UniqueId: 1002, IsBlocked: true},
			{ItemKey: 320009, UniqueId: 1003},
		},
		InventorySaveDatas: []InventorySlot{
			{Index: 0, ItemUniqueId: 1001},
			{Index: 1, ItemUniqueId: 0},    // slot vazio -> ignorar
			{Index: 2, ItemUniqueId: 9999}, // uid inexistente -> ignorar
		},
		StashSaveDatas:    []StashSlot{{Index: 0, ItemUniqueId: 1002}},
		TradingStashDatas: []StashSlot{{Index: 0, ItemUniqueId: 1003}},
	}

	got := resolveInventory(save)
	if len(got) != 3 {
		t.Fatalf("esperava 3 itens guardados (vazio e dangling ignorados), veio %d: %+v", len(got), got)
	}

	byLoc := map[string]StoredItem{}
	for _, s := range got {
		byLoc[s.Location] = s
	}
	bag, ok := byLoc["bag"]
	if !ok || bag.ItemKey != 300001 || !bag.IsChaotic || bag.Enchants != 1 {
		t.Fatalf("mochila deveria ter o item caótico 300001 com 1 enchant, veio %+v", bag)
	}
	if s, ok := byLoc["stash"]; !ok || s.ItemKey != 310005 || !s.IsBlocked {
		t.Fatalf("armazém deveria ter o item 310005 travado, veio %+v", s)
	}
	if s, ok := byLoc["trade"]; !ok || s.ItemKey != 320009 {
		t.Fatalf("banca deveria ter o item 320009, veio %+v", s)
	}
}
