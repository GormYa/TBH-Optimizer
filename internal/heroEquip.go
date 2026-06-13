package internal

// EquippedSlot é um dos 10 slots posicionais de um herói. ItemKey 0 = slot vazio.
type EquippedSlot struct {
	SlotIndex int           `json:"slot_index"`
	ItemKey   int           `json:"item_key"`
	UniqueId  int64         `json:"unique_id"`
	IsChaotic bool          `json:"is_chaotic"`
	Enchants  []ItemEnchant `json:"enchants"`
}

// HeroEquipment é um herói com seus slots equipados resolvidos.
type HeroEquipment struct {
	HeroKey int            `json:"hero_key"`
	Level   int            `json:"level"`
	Active  bool           `json:"active"`
	Slots   []EquippedSlot `json:"slots"`
}

// resolveHeroEquipment monta a lista de heróis destravados com os itens equipados
// resolvidos. Ordem: time ativo primeiro (na ordem do arrangedHeroKey), depois o
// resto do roster. Itens cujo UniqueId não existe em itemSaveDatas viram slot vazio.
func resolveHeroEquipment(save *InnerSaveData) []HeroEquipment {
	itemByUID := make(map[int64]*Item, len(save.ItemSaveDatas))
	for i := range save.ItemSaveDatas {
		it := &save.ItemSaveDatas[i]
		itemByUID[it.UniqueId] = it
	}

	active := make(map[int]bool, len(save.CommonSaveData.ArrangedHeroKey))
	for _, hk := range save.CommonSaveData.ArrangedHeroKey {
		active[hk] = true
	}

	resolve := func(h *Hero) HeroEquipment {
		he := HeroEquipment{
			HeroKey: h.HeroKey,
			Level:   h.HeroLevel,
			Active:  active[h.HeroKey],
			Slots:   make([]EquippedSlot, 0, len(h.EquippedItemIds)),
		}
		for slot, uid := range h.EquippedItemIds {
			es := EquippedSlot{SlotIndex: slot, UniqueId: uid}
			if uid != 0 {
				if it, ok := itemByUID[uid]; ok {
					es.ItemKey = it.ItemKey
					es.IsChaotic = it.IsChaotic
					for _, e := range it.EnchantData {
						if e.StatModKey != 0 {
							es.Enchants = append(es.Enchants, e)
						}
					}
				}
			}
			he.Slots = append(he.Slots, es)
		}
		return he
	}

	heroByKey := make(map[int]*Hero, len(save.HeroSaveDatas))
	for i := range save.HeroSaveDatas {
		heroByKey[save.HeroSaveDatas[i].HeroKey] = &save.HeroSaveDatas[i]
	}

	out := make([]HeroEquipment, 0, len(save.HeroSaveDatas))
	seen := make(map[int]bool)
	for _, hk := range save.CommonSaveData.ArrangedHeroKey {
		if h, ok := heroByKey[hk]; ok && !seen[hk] {
			out = append(out, resolve(h))
			seen[hk] = true
		}
	}
	for i := range save.HeroSaveDatas {
		h := &save.HeroSaveDatas[i]
		if h.IsUnLock && !seen[h.HeroKey] {
			out = append(out, resolve(h))
			seen[h.HeroKey] = true
		}
	}
	return out
}
