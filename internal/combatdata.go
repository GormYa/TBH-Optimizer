package internal

import (
	"embed"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
)

// statTypeNames: índice = ItemEnchant.StatType (enum do jogo). ESPELHA o STAT_TYPE
// do web/index.html — mantê-los em sincronia ao adicionar stats.
var statTypeNames = []string{
	"NONE", "AttackDamage", "AttackSpeed", "CriticalChance", "CriticalDamage", "MaxHp", "Armor",
	"MovementSpeed", "AreaOfEffect", "BaseAttackCountReduction", "CooldownReduction", "SkillRangeExpansion",
	"FireResistance", "ColdResistance", "LightningResistance", "ChaosResistance", "DodgeChance", "BlockChance",
	"MaxDodgeChance", "MaxBlockChance", "Multistrike", "HpLeech", "ProjectileCount", "HpRegenPerSec",
	"PhysicalDamagePercent", "FireDamagePercent", "ColdDamagePercent", "LightningDamagePercent", "ChaosDamagePercent",
	"MaxFireResistance", "MaxColdResistance", "MaxLightningResistance", "MaxChaosResistance", "AddHpPerHit",
	"DamageReduction", "PhysicalDamageReduction", "FireDamageReduction", "ColdDamageReduction", "LightningDamageReduction",
	"ChaosDamageReduction", "DamageAbsorption", "DamageAddition", "PhysicalDamageAddition", "FireDamageAddition",
	"ColdDamageAddition", "LightningDamageAddition", "ChaosDamageAddition", "IncreaseExpAmount", "AdditionalExp",
	"CastSpeed", "SkillHealIncrease", "SkillDurationIncrease", "AllElementalResistance", "IncreaseProjectileDamage",
	"IncreaseMeleeDamage", "IncreaseAreaOfEffectDamage", "IncreaseSummonDamage", "IncreaseProjectileSpeed",
	"AddHpPerKill", "AddAllSkillLevel", "ElementalBlockChance", "ElementalDodgeChance", "MaxElementalBlockChance",
	"MaxElementalDodgeChance",
}

func statTypeName(i int) string {
	if i >= 0 && i < len(statTypeNames) {
		return statTypeNames[i]
	}
	return ""
}

// CombatScale: divisores p/ levar valores crus ao espaço da fórmula. Default = valores
// confirmados na Fase A2.5 (também servem de fallback se combat_model.json sumir).
type CombatScale struct {
	CritChance            float64
	CritDmg               float64
	AtkSpeed              float64
	Atk                   float64
	EnchantPercentDivisor float64
}

var combatScale = CombatScale{CritChance: 100, CritDmg: 1000, AtkSpeed: 1, Atk: 1, EnchantPercentDivisor: 1000}
var combatModelLoaded = false

type HeroBaseStats struct {
	ATK        float64 `json:"atk"`
	AtkSpeed   float64 `json:"atkSpeed"`
	CritChance float64 `json:"critChance"`
	CritDmg    float64 `json:"critDmg"`
	MaxHp      float64 `json:"maxHp"`
	Armor      float64 `json:"armor"`
}

var heroBaseStats = map[int]HeroBaseStats{}

type ItemBaseStat struct {
	Stat  string  `json:"stat"`
	Mod   string  `json:"mod"`
	Value float64 `json:"value"`
}

type ItemMeta struct {
	Grade string `json:"grade"`
	Part  string `json:"part"`
	Gear  string `json:"gear"`
	Level int    `json:"level"`
	Drops bool   `json:"drops"`
}

var itemBaseStats = map[int][]ItemBaseStat{}
var itemMeta = map[int]ItemMeta{}

// itemsByPart indexa os ItemKeys por 'part' (weapon/armor/accessory/...) pra o C2 varrer
// só os candidatos do mesmo slot em vez de todo o catálogo (~5760 itens) a cada /api/stats.
var itemsByPart = map[string][]int{}

type RuneLevel struct {
	Level int     `json:"level"`
	Cost  float64 `json:"cost"`
	Stat  string  `json:"stat"`
	Value float64 `json:"value"`
}
type RuneInfo struct {
	Key      int         `json:"key"`
	Name     string      `json:"name"`
	MaxLevel int         `json:"maxLevel"`
	PrevReq  string      `json:"prevReq"`
	Levels   []RuneLevel `json:"levels"`
}

var runeCatalog = map[int]RuneInfo{}

// nonzero devolve v se != 0, senão def (p/ campos opcionais do JSON).
func nonzero(v, def float64) float64 {
	if v != 0 {
		return v
	}
	return def
}

// InitializeCombatData: gamedata atualizável primeiro, embed como fallback. Chamado uma
// vez no startup do monitor. Cada arquivo é independente — falta de um não impede os outros.
func InitializeCombatData(webFiles embed.FS, gameDataDir string) {
	read := func(name string) ([]byte, error) {
		if gameDataDir != "" {
			if data, err := os.ReadFile(filepath.Join(gameDataDir, name)); err == nil {
				return data, nil
			}
		}
		return webFiles.ReadFile("web/" + name)
	}
	loadAll(read)
}

// InitializeCombatDataFromFS: variante p/ testes — lê só de um FS embutido.
// O FS embutido via //go:embed testdata/combatdata/* tem raiz no pacote, então
// os arquivos ficam em testdata/combatdata/web/<name>. Fazemos sub-FS para
// normalizar o prefixo igual ao InitializeCombatData (que usa "web/<name>").
func InitializeCombatDataFromFS(testFS fs.FS) {
	sub, err := fs.Sub(testFS, "testdata/combatdata")
	if err != nil {
		// fallback: tenta direto (útil se caller já passou um sub-FS)
		loadAll(func(name string) ([]byte, error) { return fs.ReadFile(testFS, "web/"+name) })
		return
	}
	loadAll(func(name string) ([]byte, error) { return fs.ReadFile(sub, "web/"+name) })
}

func loadAll(read func(string) ([]byte, error)) {
	if data, err := read("combat_model.json"); err == nil {
		var doc struct {
			Scale struct {
				CritChance            float64 `json:"critChance"`
				CritDmg               float64 `json:"critDmg"`
				AtkSpeed              float64 `json:"atkSpeed"`
				Atk                   float64 `json:"atk"`
				EnchantPercentDivisor float64 `json:"enchantPercentDivisor"`
			} `json:"scale"`
		}
		if json.Unmarshal(data, &doc) == nil && doc.Scale.CritChance > 0 {
			combatScale = CombatScale{
				CritChance:            doc.Scale.CritChance,
				CritDmg:               nonzero(doc.Scale.CritDmg, 1000),
				AtkSpeed:              nonzero(doc.Scale.AtkSpeed, 1),
				Atk:                   nonzero(doc.Scale.Atk, 1),
				EnchantPercentDivisor: nonzero(doc.Scale.EnchantPercentDivisor, 1000),
			}
			combatModelLoaded = true
		}
	}
	if data, err := read("hero_base.json"); err == nil {
		var doc struct {
			Heroes map[string]HeroBaseStats `json:"heroes"`
		}
		if json.Unmarshal(data, &doc) == nil {
			for ks, v := range doc.Heroes {
				if k, e := strconv.Atoi(ks); e == nil {
					heroBaseStats[k] = v
				}
			}
		}
	}
	if data, err := read("items.json"); err == nil {
		var doc struct {
			Items map[string]struct {
				Grade string         `json:"grade"`
				Part  string         `json:"part"`
				Gear  string         `json:"gear"`
				Level int            `json:"level"`
				Drops bool           `json:"drops"`
				Base  []ItemBaseStat `json:"base"`
			} `json:"items"`
		}
		if json.Unmarshal(data, &doc) == nil {
			for ks, v := range doc.Items {
				if k, e := strconv.Atoi(ks); e == nil {
					itemBaseStats[k] = v.Base
					itemMeta[k] = ItemMeta{Grade: v.Grade, Part: v.Part, Gear: v.Gear, Level: v.Level, Drops: v.Drops}
					itemsByPart[v.Part] = append(itemsByPart[v.Part], k)
				}
			}
		}
	}
	if data, err := read("runes.json"); err == nil {
		var doc map[string]RuneInfo
		if json.Unmarshal(data, &doc) == nil {
			for ks, v := range doc {
				if k, e := strconv.Atoi(ks); e == nil {
					runeCatalog[k] = v
				}
			}
		}
	}
}
