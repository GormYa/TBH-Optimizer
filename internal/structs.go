package internal

import (
	"embed"
	"strconv"
	"strings"
	"sync"
)

// FlexFloat aceita número OU string no JSON do save. O jogo às vezes serializa
// campos numéricos (ex.: HeroExp) como string ("1000.5"); um float64 puro
// quebrava o parse ("cannot unmarshal string ... of type float64"). Trata
// "", null e ausência como 0.
type FlexFloat float64

func (f *FlexFloat) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	if strings.Contains(s, ",") {
		if dot := strings.LastIndex(s, "."); dot >= 0 && strings.LastIndex(s, ",") > dot {
			s = strings.ReplaceAll(s, ".", "")
		}
		s = strings.ReplaceAll(s, ",", ".")
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return err
	}
	*f = FlexFloat(v)
	return nil
}

type Currency struct {
	Key      int `json:"Key"`
	Quantity int `json:"Quantity"`
}
type Hero struct {
	HeroKey                    int       `json:"heroKey"`
	HeroLevel                  int       `json:"HeroLevel"`
	HeroExp                    FlexFloat `json:"HeroExp"`
	IsUnLock                   bool      `json:"IsUnLock"`
	AbilityPoint               int       `json:"AbilityPoint"`
	AllocatedHeroAbilityPoint  int       `json:"AllocatedHeroAbilityPoint"`
	EquippedItemIds            []int64   `json:"equippedItemIds"`
	EquippedSkillKey           []int     `json:"equippedSKillKey"`
	UnlockedAttributeGroupKeys []int     `json:"unlockedAttributeGroupKeys"`
}

// ItemEnchant é um mod rolado num item (decoração/gravação/inscrição):
// StatModKey/Value/Tier são o roll; MaterialKey é o material consumido.
type ItemEnchant struct {
	StatModKey  int `json:"StatModKey"`
	Tier        int `json:"Tier"`
	Value       int `json:"Value"`
	RecipeType  int `json:"RecipeType"`
	ModType     int `json:"ModType"`
	MaterialKey int `json:"MaterialKey"`
	StatType    int `json:"StatType"`
}
type Item struct {
	ItemKey                      int           `json:"ItemKey"`
	UniqueId                     int64         `json:"UniqueId"`
	IsChaotic                    bool          `json:"IsChaotic"`
	IsBlocked                    bool          `json:"IsBlocked"`
	EnchantCount                 []int         `json:"EnchantCount"`
	EnchantData                  []ItemEnchant `json:"EnchantData"`
	DecorationAppliedTotalCount  int           `json:"DecorationAppliedTotalCount"`
	EngravingAppliedTotalCount   int           `json:"EngravingAppliedTotalCount"`
	InscriptionAppliedTotalCount int           `json:"InscriptionAppliedTotalCount"`
}
type RuneSave struct {
	RuneKey int `json:"RuneKey"`
	Level   int `json:"Level"`
}

// InventorySlot/StashSlot mapeiam slot -> item (UniqueId, 0 = vazio). Os itens
// em si vivem todos em itemSaveDatas; mochila/armazém/banca só apontam pra lá.
// (No stash ficam os itens guardados pra síntese/criação no Cubo.)
type InventorySlot struct {
	Index            int   `json:"Index"`
	ItemUniqueId     int64 `json:"ItemUniqueId"`
	IsUnlock         bool  `json:"IsUnlock"`
	IsUnlockedByRune bool  `json:"IsUnlockedByRune"`
}
type StashSlot struct {
	Index        int   `json:"Index"`
	ItemUniqueId int64 `json:"ItemUniqueId"`
	IsUnLock     bool  `json:"IsUnLock"`
}

// AttributeSave é um nó alocado da árvore de atributos; o herói vem codificado
// no prefixo do Key (ex.: 201001 -> herói 201).
type AttributeSave struct {
	Key   int `json:"Key"`
	Level int `json:"Level"`
}
type CubeLevel struct {
	Level int     `json:"Level"`
	Exp   float64 `json:"Exp"`
}

type CubeRecipeSave struct {
	CubeRecipeTypeInt  int `json:"CubeRecipeTypeInt"`
	CubeKey            int `json:"CubeKey"`
	MaxUnlockRecipeKey int `json:"MaxUnlockRecipeKey"`
}

// BoxData são os baús não abertos (3 listas paralelas por índice).
// Tipos: 0=NORMAL, 1=BOSS, 2=ACTBOSS.
type BoxData struct {
	BoxTypes    []int   `json:"BoxTypes"`
	BoxUniqueId []int64 `json:"BoxUniqueId"`
	BoxQuantity []int   `json:"BoxQuantity"`
}
type InnerSaveData struct {
	CommonSaveData struct {
		CurrentStageKey   int     `json:"currentStageKey"`
		CurrentStageWave  int     `json:"currentStageWave"`
		PlayTime          float64 `json:"playTime"`
		MaxCompletedStage int     `json:"maxCompletedStage"`
		ArrangedHeroKey   []int   `json:"arrangedHeroKey"`
		ArrangedPetKey    int     `json:"ArrangedPetKey"`
	} `json:"commonSaveData"`
	BoxData             BoxData          `json:"BoxData"`
	CurrenySaveDatas    []Currency       `json:"currenySaveDatas"`
	HeroSaveDatas       []Hero           `json:"heroSaveDatas"`
	AttributeSaveDatas  []AttributeSave  `json:"attributeSaveDatas"`
	ItemSaveDatas       []Item           `json:"itemSaveDatas"`
	RuneSaveDatas       []RuneSave       `json:"RuneSaveData"`
	PetSaveDatas        []PetSave        `json:"PetSaveData"`
	InventorySaveDatas  []InventorySlot  `json:"inventorySaveDatas"`
	StashSaveDatas      []StashSlot      `json:"stashSaveDatas"`
	TradingStashDatas   []StashSlot      `json:"tradingStashSaveDatas"`
	CubeSaveLevelData   CubeLevel        `json:"cubeSaveLevelData"`
	CubeRecipeSaveDatas []CubeRecipeSave `json:"cubeRecipeSaveDatas"`
}
type PetSave struct {
	PetKey   int  `json:"PetKey"`
	IsUnlock bool `json:"IsUnlock"`
}
type OuterSave struct {
	PlayerSaveData struct {
		Type  string `json:"__type"`
		Value string `json:"value"`
	} `json:"PlayerSaveData"`
}
type HeroState struct {
	Level int
	Xp    float64
}
type StageStats struct {
	StageKey          int         `json:"stage_key"`
	TotalRuns         int         `json:"total_runs"`
	ManualTime        float64     `json:"manual_time"`
	AvgTimeSpent      float64     `json:"avg_time_spent"`
	AvgGoldPerRun     float64     `json:"avg_gold_per_run"`
	AvgGoldPerHour    float64     `json:"avg_gold_per_hour"`
	RawGoldPerHour    float64     `json:"raw_gold_per_hour"`
	AvgXpPerRun       float64     `json:"avg_xp_per_run"`
	AvgXpPerHour      float64     `json:"avg_xp_per_hour"`
	RawXpPerHour      float64     `json:"raw_xp_per_hour"`
	AvgItemsPerHour   float64     `json:"avg_items_per_hour"`
	ExpRetained       float64     `json:"exp_retained"`
	MeasuredHeroLevel int         `json:"measured_hero_level"`
	Stale             bool        `json:"stale"`
	ItemCatalog       map[int]int `json:"item_catalog"`
	AccumulatedGold   float64     `json:"-"`
	AccumulatedXp     float64     `json:"-"`
	AccumulatedTime   float64     `json:"-"`
	AccumulatedItems  float64     `json:"-"`
}

type FarmStageInfo struct {
	Key          int     `json:"key"`
	Label        string  `json:"label"`
	TotalHP      float64 `json:"totalHP"`
	ExpectedGold float64 `json:"expectedGold"`
	ExpectedEXP  float64 `json:"expectedEXP"`
	Waves        int     `json:"waves"`
	Level        int     `json:"level"`
	Difficulty   string  `json:"difficulty"`
}

type StageHistoryStore struct {
	mu      sync.RWMutex
	history map[int]*StageStats
}
type Control struct {
	LastPlayTime        float64
	LastGold            int
	LastCurrentStageKey int
	MaxCompletedStage   int
	LastItemIds         map[int64]bool
	HeroStates          map[int]HeroState
	StageHistory        StageHistoryStore
	UseEMA              bool
	EMAAlpha            float64
	FarmStages          map[int]FarmStageInfo
	HeroLevel           int
	ActiveHeroCount     int
	ActiveHeroes        []ActiveHero
	lastMidStage        int
	midWindowStage      int
	midWindowMixed      bool
	midWindowMaxWave    int
	midWindowRestart    bool
	reanchorWave        int
	timeContamStreak    map[int]int
	primeFirstClear     bool
	Gold                int
	RuneLevels          map[int]int
	LastRuneLevels      map[int]int
	OwnedPets           []int
	ActivePet           int
	HeroEquipment       []HeroEquipment
	Inventory           []StoredItem
	CubeLevel           int
	CubeExp             float64
	CubeRecipes         []CubeRecipeSave
	LastBoxQuantity     map[int64]int
	LastBoxTypes        map[int64]int
	ChestHistory        []ChestDropEvent
	WebFiles            embed.FS
	GameDataDir         string
}
type ActiveHero struct {
	Key   int `json:"key"`
	Level int `json:"level"`
}
type AnalyticsReport struct {
	BestGoldStage  int                 `json:"best_gold_stage"`
	BestXpStage    int                 `json:"best_xp_stage"`
	BestComboStage int                 `json:"best_combo_stage"`
	Stages         map[int]*StageStats `json:"stages"`
	Calibrated     bool                `json:"calibrated"`
	EffectiveDPS   float64             `json:"effective_dps"`
	HeroLevel      int                 `json:"hero_level"`
	ActiveHeroes   []ActiveHero        `json:"active_heroes"`
	Gold           int                 `json:"gold"`
	RuneLevels     map[int]int         `json:"rune_levels"`
	OwnedPets      []int               `json:"owned_pets"`
	ActivePet      int                 `json:"active_pet"`
	ChestTracker   ChestTrackerStatus  `json:"chest_tracker"`
	HeroEquipment  []HeroEquipment     `json:"hero_equipment"`
	Inventory      []StoredItem        `json:"inventory"`
	CubeLevel      int                 `json:"cube_level"`
	CubeExp        float64             `json:"cube_exp"`
	CubeRecipes    []CubeRecipeSave    `json:"cube_recipes"`
}
