package internal

type StoredItem struct {
	Location  string `json:"location"`
	SlotIndex int    `json:"slot_index"`
	ItemKey   int    `json:"item_key"`
	UniqueId  int64  `json:"unique_id"`
	IsChaotic bool   `json:"is_chaotic"`
	IsBlocked bool   `json:"is_blocked"`
	Enchants  int    `json:"enchants"`
}

func resolveInventory(save *InnerSaveData) []StoredItem {
	itemByUID := make(map[int64]*Item, len(save.ItemSaveDatas))
	for i := range save.ItemSaveDatas {
		it := &save.ItemSaveDatas[i]
		itemByUID[it.UniqueId] = it
	}

	out := make([]StoredItem, 0, len(save.InventorySaveDatas)+len(save.StashSaveDatas)+len(save.TradingStashDatas))

	add := func(loc string, slot int, uid int64) {
		if uid == 0 {
			return
		}
		it, ok := itemByUID[uid]
		if !ok {
			return
		}
		ench := 0
		for _, e := range it.EnchantData {
			if e.StatModKey != 0 {
				ench++
			}
		}
		out = append(out, StoredItem{
			Location:  loc,
			SlotIndex: slot,
			ItemKey:   it.ItemKey,
			UniqueId:  uid,
			IsChaotic: it.IsChaotic,
			IsBlocked: it.IsBlocked,
			Enchants:  ench,
		})
	}

	for _, s := range save.InventorySaveDatas {
		add("bag", s.Index, s.ItemUniqueId)
	}
	for _, s := range save.StashSaveDatas {
		add("stash", s.Index, s.ItemUniqueId)
	}
	for _, s := range save.TradingStashDatas {
		add("trade", s.Index, s.ItemUniqueId)
	}
	return out
}
