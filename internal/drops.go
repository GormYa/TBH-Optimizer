package internal

// countNewDrops compara os itens atuais com os UniqueIds ja vistos (baseline) e
// devolve quantos sao novos (drops desta janela) e a contagem por ItemKey (catalogo).
// E robusto a venda/sintese: remover um item nao cria UniqueId novo, entao so
// dropagem real conta. UniqueId 0 (slot vazio) e ignorado.
func countNewDrops(seen map[int64]bool, items []Item) (int, map[int]int) {
	count := 0
	byKey := make(map[int]int)
	for _, it := range items {
		if it.UniqueId == 0 {
			continue
		}
		if !seen[it.UniqueId] {
			count++
			byKey[it.ItemKey]++
		}
	}
	return count, byKey
}

// snapshotItemIds monta o conjunto de UniqueIds atuais para virar a nova baseline.
func snapshotItemIds(items []Item) map[int64]bool {
	s := make(map[int64]bool, len(items))
	for _, it := range items {
		if it.UniqueId != 0 {
			s[it.UniqueId] = true
		}
	}
	return s
}
