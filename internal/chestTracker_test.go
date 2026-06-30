package internal

import (
	"testing"
	"time"
)

func TestGetChestCooldownConfig(t *testing.T) {
	configs := map[int]ChestCooldownConfig{
		910011: {ItemDefID: 910011, DropInterval: 15, DropMaxPerWindow: 2, DropWindow: 45, UseDropWindow: true},
	}
	cfg := GetChestCooldownConfig(910011, configs)
	if cfg.DropInterval != 15 || cfg.DropMaxPerWindow != 2 {
		t.Fatalf("expected custom config, got %+v", cfg)
	}

	cfgDefault := GetChestCooldownConfig(910011, nil)
	if cfgDefault.DropInterval != 11 || cfgDefault.DropWindow != 33 {
		t.Fatalf("expected default config, got %+v", cfgDefault)
	}

	cfgBoss := GetChestCooldownConfig(920002, nil)
	if cfgBoss.DropInterval != 3600 {
		t.Fatalf("expected boss 3600min config, got %+v", cfgBoss)
	}
}

func TestCalculateCooldowns(t *testing.T) {
	cfg := ChestCooldownConfig{
		ItemDefID:        910011,
		DropInterval:     11,
		DropMaxPerWindow: 3,
		DropWindow:       33,
		UseDropWindow:    true,
	}
	now := time.Now()
	history := []ChestDropEvent{
		{ItemDefID: 910011, Timestamp: now.Add(-60 * time.Minute)},
		{ItemDefID: 910011, Timestamp: now.Add(-20 * time.Minute)},
		{ItemDefID: 910011, Timestamp: now.Add(-5 * time.Minute)},
	}
	dropped, _, coolRem, status := CalculateCooldowns(910011, history, cfg)
	if dropped != 2 {
		t.Fatalf("expected 2 drops in window, got %d", dropped)
	}
	if coolRem <= 0 {
		t.Fatalf("expected individual cooldown remaining, got %d", coolRem)
	}
	if status != "cooldown" {
		t.Fatalf("expected status 'cooldown', got %s", status)
	}

	historyCap := []ChestDropEvent{
		{ItemDefID: 910011, Timestamp: now.Add(-25 * time.Minute)},
		{ItemDefID: 910011, Timestamp: now.Add(-14 * time.Minute)},
		{ItemDefID: 910011, Timestamp: now.Add(-2 * time.Minute)},
	}
	droppedCap, winRemCap, _, statusCap := CalculateCooldowns(910011, historyCap, cfg)
	if droppedCap != 3 {
		t.Fatalf("expected 3 drops in window, got %d", droppedCap)
	}
	if winRemCap <= 0 {
		t.Fatalf("expected window remaining, got %d", winRemCap)
	}
	if statusCap != "capped" {
		t.Fatalf("expected status 'capped', got %s", statusCap)
	}
}

// v1.00.12: o cooldown real (server-side) e menor que o config stale (11). Quando o
// historico mostra um gap real menor, CalculateCooldowns deve usar o gap empirico.
func TestEmpiricalIntervalShortensCooldown(t *testing.T) {
	cfg := ChestCooldownConfig{ItemDefID: 910011, DropInterval: 11, DropMaxPerWindow: 3, DropWindow: 33, UseDropWindow: true}
	now := time.Now()
	history := []ChestDropEvent{
		{ItemDefID: 910011, Timestamp: now.Add(-5 * time.Minute)},
		{ItemDefID: 910011, Timestamp: now.Add(-1 * time.Minute)}, // gap real = 4 min
	}
	if emp := empiricalIntervalMin(910011, history); emp != 4 {
		t.Fatalf("intervalo empirico esperado 4min, got %d", emp)
	}
	_, _, coolRem, status := CalculateCooldowns(910011, history, cfg)
	if status != "cooldown" || coolRem <= 0 {
		t.Fatalf("esperava cooldown ativo, got status=%s coolRem=%d", status, coolRem)
	}
	// intervalo efetivo 4min, ultimo drop ha 1min => ~3min. Com o 11 stale seria ~10min.
	if coolRem > 4*60 {
		t.Fatalf("cooldown nao encurtou com intervalo empirico: %ds", coolRem)
	}
}

// A taxa observada deve medir a cadência entre drops e IGNORAR buracos longos (baú cheio
// que para de dropar, offline, AFK) -- senão a técnica de stacking derruba o cálculo.
func TestObservedDropRatesIgnoresFullSlotIdle(t *testing.T) {
	base := time.Now().Add(-12 * time.Hour)
	at := func(min float64) time.Time { return base.Add(time.Duration(min*60) * time.Second) }
	var hist []ChestDropEvent
	// 5 drops a cada 6 min (4 gaps ativos)
	for i := 0; i < 5; i++ {
		hist = append(hist, ChestDropEvent{ItemDefID: 910011, Timestamp: at(float64(i) * 6)})
	}
	// buraco de 180 min (baús cheios / offline) e mais 4 drops a cada 6 min
	for i := 0; i < 4; i++ {
		hist = append(hist, ChestDropEvent{ItemDefID: 910011, Timestamp: at(24 + 180 + float64(i)*6)})
	}
	r := observedDropRates(hist)[910011]
	// mediana dos gaps ativos = 6 min -> 10/h. O buraco de 180 min NÃO pode puxar pra baixo.
	if r < 8 || r > 12 {
		t.Fatalf("esperava ~10/h (cadência de 6min), got %.2f -- buraco de baú cheio derrubou", r)
	}
}

// v1.00.12: deteccao exata pelo ledger BoxBucketGetBoxList (id novo = 1 aquisicao).
func TestDropsFromBucketLedger(t *testing.T) {
	ctrl := &Control{LastBoxBucketGet: map[string]bool{"100": true}}
	save := &InnerSaveData{
		BoxBucketGetBoxList: []string{"100", "102", "104"}, // 102 e 104 sao novos
		BoxData: BoxData{
			BoxTypes:    []int{0, 0},
			BoxUniqueId: []int64{102, 104},
			BoxQuantity: []int{1, 1},
		},
	}
	save.CommonSaveData.CurrentStageKey = 1101

	drops := dropsFromBucketLedger(ctrl, save, map[int64]int{102: 910051})
	if len(drops) != 2 {
		t.Fatalf("esperava 2 drops novos, got %d", len(drops))
	}
	got := map[int64]int{}
	for _, d := range drops {
		got[d.uid] = d.defId
	}
	if got[102] != 910051 {
		t.Fatalf("uid 102 deveria mapear 910051 via itemIdMap, got %d", got[102])
	}
	if got[104] == 0 {
		t.Fatalf("uid 104 deveria ter defId de fallback, got 0")
	}
	if !ctrl.LastBoxBucketGet["102"] || !ctrl.LastBoxBucketGet["104"] {
		t.Fatalf("ledger 'visto' nao foi atualizado")
	}
	// id ja visto nao reconta
	if again := dropsFromBucketLedger(ctrl, save, nil); len(again) != 0 {
		t.Fatalf("nenhum drop novo esperado na 2a passada, got %d", len(again))
	}
}
