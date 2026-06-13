package internal

import "testing"

func TestResolveHeroEquipment(t *testing.T) {
	save := &InnerSaveData{
		ItemSaveDatas: []Item{
			{ItemKey: 300001, UniqueId: 1001, IsChaotic: true, EnchantData: []ItemEnchant{
				{StatModKey: 5001, Tier: 3, Value: 70, ModType: 1, StatType: 2},
				{StatModKey: 0}, // slot vazio: deve ser ignorado
			}},
			{ItemKey: 310005, UniqueId: 1002},
			{ItemKey: 320009, UniqueId: 9999}, // item solto, não equipado
		},
		HeroSaveDatas: []Hero{
			{HeroKey: 201, HeroLevel: 59, IsUnLock: true, EquippedItemIds: []int64{1001, 0, 1002, 0, 0, 0, 0, 0, 0, 0}},
			{HeroKey: 202, HeroLevel: 40, IsUnLock: true, EquippedItemIds: []int64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
			{HeroKey: 203, HeroLevel: 1, IsUnLock: false, EquippedItemIds: []int64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0}}, // travado: não entra
		},
	}
	save.CommonSaveData.ArrangedHeroKey = []int{201}

	got := resolveHeroEquipment(save)
	if len(got) != 2 {
		t.Fatalf("esperava 2 heróis (201 ativo + 202 destravado), veio %d", len(got))
	}
	// ativo primeiro
	if got[0].HeroKey != 201 || !got[0].Active {
		t.Fatalf("esperava herói 201 ativo primeiro, veio %+v", got[0])
	}
	if got[1].HeroKey != 202 || got[1].Active {
		t.Fatalf("esperava herói 202 inativo em segundo, veio %+v", got[1])
	}
	h := got[0]
	if len(h.Slots) != 10 {
		t.Fatalf("esperava 10 slots, veio %d", len(h.Slots))
	}
	if h.Slots[0].ItemKey != 300001 || !h.Slots[0].IsChaotic {
		t.Fatalf("slot 0 deveria ser o item 300001 caótico, veio %+v", h.Slots[0])
	}
	if len(h.Slots[0].Enchants) != 1 || h.Slots[0].Enchants[0].StatModKey != 5001 {
		t.Fatalf("slot 0 deveria ter 1 enchant rolado (5001), veio %+v", h.Slots[0].Enchants)
	}
	if h.Slots[1].ItemKey != 0 || h.Slots[1].UniqueId != 0 {
		t.Fatalf("slot 1 deveria ser vazio, veio %+v", h.Slots[1])
	}
	if h.Slots[2].ItemKey != 310005 {
		t.Fatalf("slot 2 deveria ser o item 310005, veio %+v", h.Slots[2])
	}
}
