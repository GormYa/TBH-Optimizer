package internal

import (
	"embed"
	"testing"
)

//go:embed testdata/combatdata/*
var combatTestFS embed.FS

func TestInitializeCombatDataCarregaEscalaEBase(t *testing.T) {
	heroBaseStats = map[int]HeroBaseStats{}
	itemBaseStats = map[int][]ItemBaseStat{}
	runeCatalog = map[int]RuneInfo{}
	combatScale = CombatScale{}

	InitializeCombatDataFromFS(combatTestFS)

	if combatScale.CritChance != 100 || combatScale.CritDmg != 1000 {
		t.Fatalf("escala não carregou: %+v", combatScale)
	}
	if got := heroBaseStats[101].CritChance; got != 25.0 {
		t.Fatalf("hero base 101 critChance cru = %v, queria 25", got)
	}
	if len(itemBaseStats[300001]) == 0 {
		t.Fatalf("item 300001 sem base[]")
	}
	if runeCatalog[1].Levels[0].Stat != "AllHeroAttackDamage" {
		t.Fatalf("runa 1 nível 1 stat errado: %+v", runeCatalog[1])
	}
}
