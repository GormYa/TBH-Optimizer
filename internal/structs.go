package internal

import "sync"

type Currency struct {
	Key      int `json:"Key"`
	Quantity int `json:"Quantity"`
}
type Hero struct {
	HeroKey   int     `json:"heroKey"`
	HeroLevel int     `json:"HeroLevel"`
	HeroExp   float64 `json:"HeroExp"`
}
type Item struct {
	ItemKey  int   `json:"ItemKey"`
	UniqueId int64 `json:"UniqueId"`
}
type InnerSaveData struct {
	CommonSaveData struct {
		CurrentStageKey   int     `json:"currentStageKey"`
		CurrentStageWave  int     `json:"currentStageWave"`
		PlayTime          float64 `json:"playTime"`
		MaxCompletedStage int     `json:"maxCompletedStage"`
	} `json:"commonSaveData"`
	CurrenySaveDatas []Currency `json:"currenySaveDatas"`
	HeroSaveDatas    []Hero     `json:"heroSaveDatas"`
	ItemSaveDatas    []Item     `json:"itemSaveDatas"`
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
	StageKey         int         `json:"stage_key"`
	TotalRuns        int         `json:"total_runs"`
	ManualTime       float64     `json:"manual_time"`
	AvgTimeSpent     float64     `json:"avg_time_spent"`
	AvgGoldPerRun    float64     `json:"avg_gold_per_run"`
	AvgGoldPerHour   float64     `json:"avg_gold_per_hour"`
	RawGoldPerHour   float64     `json:"raw_gold_per_hour"`
	AvgXpPerRun      float64     `json:"avg_xp_per_run"`
	AvgXpPerHour     float64     `json:"avg_xp_per_hour"`
	RawXpPerHour     float64     `json:"raw_xp_per_hour"`
	AvgItemsPerHour  float64     `json:"avg_items_per_hour"`
	ItemCatalog      map[int]int `json:"item_catalog"`
	AccumulatedGold  float64     `json:"-"`
	AccumulatedXp    float64     `json:"-"`
	AccumulatedTime  float64     `json:"-"`
	AccumulatedItems float64     `json:"-"`
}

type FarmStageInfo struct {
	Key          int     `json:"key"`
	Label        string  `json:"label"`
	TotalHP      float64 `json:"totalHP"`
	ExpectedGold float64 `json:"expectedGold"`
	ExpectedEXP  float64 `json:"expectedEXP"`
	Waves        int     `json:"waves"`
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
}
type AnalyticsReport struct {
	BestGoldStage int                 `json:"best_gold_stage"`
	BestXpStage   int                 `json:"best_xp_stage"`
	Stages        map[int]*StageStats `json:"stages"`
	Calibrated    bool                `json:"calibrated"`
}
