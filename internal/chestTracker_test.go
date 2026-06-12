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
	if statusCap != "cooldown" {
		t.Fatalf("expected status 'cooldown', got %s", statusCap)
	}
}
