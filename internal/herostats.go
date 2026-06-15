package internal

import (
	"embed"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
)

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

func nonzero(v, def float64) float64 {
	if v != 0 {
		return v
	}
	return def
}

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

const (
	fAtk = iota
	fAtkSpeed
	fCritChance
	fCritDmg
	fMaxHp
	fArmor
	fIgnore
)

const (
	modFlat           = 0
	modAdditive       = 1
	modMultiplicative = 2
)

const (
	runePercentDivisor = 100.0
	gearPercentDivisor = 100.0
)

type statContribution struct {
	field int
	mod   int
	value float64
	slot  int
}

func modName(s string) int {
	switch s {
	case "ADDITIVE", "PERCENT":
		return modAdditive
	case "MULTIPLICATIVE", "MULT":
		return modMultiplicative
	default:
		return modFlat
	}
}

func routeStat(stat string, mod int, raw, percentDiv float64) (field int, modOut int, value float64, ok bool) {
	switch stat {
	case "AttackDamage", "AllHeroAttackDamage":
		if mod == modAdditive {
			return fAtk, modAdditive, raw / percentDiv, true
		}
		return fAtk, modFlat, raw / combatScale.Atk, true
	case "AttackDamagePercent", "AllHeroAttackDamagePercent", "PhysicalDamagePercent", "IncreaseMeleeDamage":
		return fAtk, modAdditive, raw / percentDiv, true
	case "AttackSpeed", "AllHeroAttackSpeed":
		if mod == modAdditive {
			return fAtkSpeed, modAdditive, raw / percentDiv, true
		}
		return fAtkSpeed, modFlat, raw / combatScale.AtkSpeed, true
	case "CriticalChance":
		return fCritChance, modFlat, raw / combatScale.CritChance, true
	case "CriticalDamage":
		return fCritDmg, modFlat, raw / combatScale.CritDmg, true
	case "MaxHp", "AllHeroMaxHp":
		if mod == modAdditive {
			return fMaxHp, modAdditive, raw / percentDiv, true
		}
		return fMaxHp, modFlat, raw, true
	case "MaxHpPercent", "AllHeroMaxHpPercent":
		return fMaxHp, modAdditive, raw / percentDiv, true
	case "Armor", "AllHeroArmor":
		if mod == modAdditive {
			return fArmor, modAdditive, raw / percentDiv, true
		}
		return fArmor, modFlat, raw, true
	case "ArmorPercent", "AllHeroArmorPercent":
		return fArmor, modAdditive, raw / percentDiv, true
	}
	return fIgnore, 0, 0, false
}

type statAccum struct {
	flat, pct, mult float64
}

func newAccum(base float64) statAccum { return statAccum{flat: base, pct: 0, mult: 1} }
func (a *statAccum) apply(mod int, v float64) {
	switch mod {
	case modAdditive:
		a.pct += v
	case modMultiplicative:
		a.mult *= (1 + v)
	default:
		a.flat += v
	}
}
func (a statAccum) value() float64 { return a.flat * (1 + a.pct) * a.mult }

func itemContributions(itemKey int, enchants []ItemEnchant) []statContribution {
	var out []statContribution
	for _, b := range itemBaseStats[itemKey] {
		if f, m, v, ok := routeStat(b.Stat, modName(b.Mod), b.Value, gearPercentDivisor); ok {
			out = append(out, statContribution{field: f, mod: m, value: v, slot: -1})
		}
	}
	for _, e := range enchants {
		name := statTypeName(e.StatType)
		if f, m, v, ok := routeStat(name, e.ModType, float64(e.Value), combatScale.EnchantPercentDivisor); ok {
			out = append(out, statContribution{field: f, mod: m, value: v, slot: -1})
		}
	}
	return out
}

func heroContributions(he HeroEquipment) []statContribution {
	var out []statContribution
	for _, s := range he.Slots {
		cs := itemContributions(s.ItemKey, s.Enchants)
		for i := range cs {
			cs[i].slot = s.SlotIndex
		}
		out = append(out, cs...)
	}
	return out
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func resolveRuneContributions(runeLevels map[int]int) []statContribution {
	var out []statContribution
	for key, lvl := range runeLevels {
		if lvl <= 0 {
			continue
		}
		info, ok := runeCatalog[key]
		if !ok || lvl > len(info.Levels) {
			continue
		}
		lv := info.Levels[lvl-1]
		if f, m, v, ok := routeStat(lv.Stat, modAdditiveIfPercent(lv.Stat), lv.Value, runePercentDivisor); ok {
			out = append(out, statContribution{field: f, mod: m, value: v, slot: -1})
		}
	}
	return out
}

func modAdditiveIfPercent(stat string) int {
	switch stat {
	case "AllHeroAttackDamagePercent", "AllHeroArmorPercent":
		return modAdditive
	}
	return modFlat
}

type HeroStats struct {
	Key        int
	ATK        float64
	AtkSpeed   float64
	CritChance float64
	CritDmg    float64
}

func fieldValues(base HeroBaseStats, contribs []statContribution) [fIgnore]float64 {
	accs := [fIgnore]statAccum{
		fAtk:        newAccum(base.ATK),
		fAtkSpeed:   newAccum(base.AtkSpeed),
		fCritChance: newAccum(base.CritChance / combatScale.CritChance),
		fCritDmg:    newAccum(base.CritDmg / combatScale.CritDmg),
		fMaxHp:      newAccum(base.MaxHp),
		fArmor:      newAccum(base.Armor),
	}
	for _, c := range contribs {
		if c.field >= fAtk && c.field < fIgnore {
			accs[c.field].apply(c.mod, c.value)
		}
	}
	var out [fIgnore]float64
	for i := range fIgnore {
		out[i] = accs[i].value()
	}
	return out
}

func statsDominate(cand, cur [fIgnore]float64) bool {
	const eps = 1e-9
	strictly := false
	for i := range fIgnore {
		if cand[i] < cur[i]-eps {
			return false
		}
		if cand[i] > cur[i]+eps {
			strictly = true
		}
	}
	return strictly
}

func aggregateHeroStats(he HeroEquipment, extra []statContribution) HeroStats {
	contribs := append(heroContributions(he), extra...)
	v := fieldValues(heroBaseStats[he.HeroKey], contribs)
	return HeroStats{Key: he.HeroKey, ATK: v[fAtk], AtkSpeed: v[fAtkSpeed],
		CritChance: v[fCritChance], CritDmg: v[fCritDmg]}
}

func damageWeight(h HeroStats) float64 {
	cb := h.CritDmg - 1
	if cb < 0 {
		cb = 0
	}
	return h.ATK * h.AtkSpeed * (1 + clamp01(h.CritChance)*cb)
}
