package internal

import "testing"

// countNewDrops conta itens cujo UniqueId ainda nao foi visto (drops desta janela)
// e agrupa por ItemKey (catalogo). UniqueId 0 (slot vazio) e ignorado.
func TestCountNewDrops(t *testing.T) {
	seen := map[int64]bool{100: true, 200: true}
	items := []Item{
		{ItemKey: 522031, UniqueId: 100},
		{ItemKey: 522031, UniqueId: 300},
		{ItemKey: 410003, UniqueId: 400},
		{ItemKey: 0, UniqueId: 0},
	}

	count, byKey := countNewDrops(seen, items)
	if count != 2 {
		t.Errorf("count = %d, quero 2", count)
	}
	if byKey[522031] != 1 {
		t.Errorf("byKey[522031] = %d, quero 1", byKey[522031])
	}
	if byKey[410003] != 1 {
		t.Errorf("byKey[410003] = %d, quero 1", byKey[410003])
	}
}

// Sem baseline (primeira leitura) nao deve reportar tudo como drop daquela janela
// quem chama trata isso calibrando a baseline antes; aqui so garantimos o calculo.
func TestCountNewDropsNenhumNovo(t *testing.T) {
	seen := map[int64]bool{100: true, 200: true}
	items := []Item{{ItemKey: 1, UniqueId: 100}, {ItemKey: 2, UniqueId: 200}}
	count, byKey := countNewDrops(seen, items)
	if count != 0 || len(byKey) != 0 {
		t.Errorf("nenhum novo esperado, veio count=%d byKey=%v", count, byKey)
	}
}

// snapshotItemIds monta o conjunto de UniqueIds atuais (nova baseline), ignorando 0.
func TestSnapshotItemIds(t *testing.T) {
	items := []Item{{UniqueId: 100}, {UniqueId: 0}, {UniqueId: 200}}
	s := snapshotItemIds(items)
	if len(s) != 2 || !s[100] || !s[200] || s[0] {
		t.Errorf("snapshot inesperado: %v", s)
	}
}
