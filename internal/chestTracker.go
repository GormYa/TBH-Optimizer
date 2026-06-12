package internal

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows/registry"
)

type ChestDropEvent struct {
	ItemDefID   int       `json:"item_def_id"`
	Timestamp   time.Time `json:"timestamp"`
	BoxUniqueId int64     `json:"box_unique_id,omitempty"`
}

type ChestCooldownConfig struct {
	ItemDefID        int
	DropInterval     int
	DropMaxPerWindow int
	DropWindow       int
	UseDropWindow    bool
}

type SteamCachedItem struct {
	AppID            interface{} `json:"appid"`
	ItemID           interface{} `json:"itemid"`
	ItemDefID        interface{} `json:"itemdefid"`
	DropInterval     interface{} `json:"drop_interval"`
	DropMaxPerWindow interface{} `json:"drop_max_per_window"`
	DropWindow       interface{} `json:"drop_window"`
	UseDropWindow    interface{} `json:"use_drop_window"`
	Exchange         interface{} `json:"exchange"`
}

type ChestFamilyStatus struct {
	ItemDefID         int    `json:"item_def_id"`
	Name              string `json:"name"`
	Rarity            string `json:"rarity"`
	IconURL           string `json:"icon_url"`
	DroppedInWindow   int    `json:"dropped_in_window"`
	MaxPerWindow      int    `json:"max_per_window"`
	WindowSeconds     int    `json:"window_seconds"`
	WindowRemaining   int    `json:"window_remaining_seconds"`
	CooldownRemaining int    `json:"cooldown_remaining_seconds"`
	OptimalStage      int    `json:"optimal_stage"`
	OptimalStageName  string `json:"optimal_stage_name"`
	Status            string `json:"status"`
}

type ChestTrackerStatus struct {
	Families   []ChestFamilyStatus `json:"families"`
	SuggestMap int                 `json:"suggest_map"`
}

type stageAndType struct {
	StageKey int
	BoxType  int
}

var stageAndTypeToBoxId = make(map[stageAndType]int)
var chestNames = make(map[int]string)
var chestIconURLs = make(map[int]string)

var OptimalNormalChests = []struct {
	ItemDefID int
	StageKey  int
}{
	{910011, 1101},
	{910051, 1104},
	{910101, 1108},
	{910151, 1203},
	{910201, 1208},
	{910301, 1308},
	{910401, 2109},
	{910501, 2305},
	{910651, 3205},
	{910801, 4103},
}

func getSteamInstallPath() string {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Valve\Steam`, registry.QUERY_VALUE)
	if err == nil {
		defer k.Close()
		val, _, err := k.GetStringValue("SteamPath")
		if err == nil && val != "" {
			return filepath.Clean(val)
		}
	}
	k2, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Valve\Steam`, registry.QUERY_VALUE)
	if err == nil {
		defer k2.Close()
		val, _, err := k2.GetStringValue("InstallPath")
		if err == nil && val != "" {
			return filepath.Clean(val)
		}
	}
	return ""
}

func getSteamUserdataDirs() []string {
	var paths []string
	var bases []string
	if installPath := getSteamInstallPath(); installPath != "" {
		bases = append(bases, filepath.Join(installPath, "userdata"))
	}
	bases = append(bases, `C:\Program Files (x86)\Steam\userdata`, `C:\Program Files\Steam\userdata`)
	seen := make(map[string]bool)
	for _, base := range bases {
		base = filepath.Clean(base)
		if seen[base] {
			continue
		}
		seen[base] = true
		if fi, err := os.Stat(base); err == nil && fi.IsDir() {
			entries, err := os.ReadDir(base)
			if err == nil {
				for _, entry := range entries {
					if entry.IsDir() {
						if _, err := strconv.Atoi(entry.Name()); err == nil {
							paths = append(paths, filepath.Join(base, entry.Name()))
						}
					}
				}
			}
		}
	}
	return paths
}

func valToString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return fmt.Sprintf("%.0f", val)
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func valToInt(v interface{}) int {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return int(val)
	case string:
		i, _ := strconv.Atoi(val)
		return i
	case int:
		return val
	case int64:
		return int(val)
	default:
		return 0
	}
}

func ScanSteamInventoryCache() (map[int64]int, map[int]ChestCooldownConfig) {
	itemMap := make(map[int64]int)
	cooldownMap := make(map[int]ChestCooldownConfig)
	userdataDirs := getSteamUserdataDirs()
	for _, uDir := range userdataDirs {
		cacheDir := filepath.Join(uDir, "inventorymsgcache")
		files, err := os.ReadDir(cacheDir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".cachedmsg") {
				continue
			}
			fPath := filepath.Join(cacheDir, f.Name())
			data, err := os.ReadFile(fPath)
			if err != nil {
				continue
			}
			startIdx := strings.Index(string(data), "[")
			if startIdx < 0 {
				continue
			}
			jsonBytes := data[startIdx:]
			for len(jsonBytes) > 0 && (jsonBytes[len(jsonBytes)-1] == 0 || jsonBytes[len(jsonBytes)-1] == ' ' || jsonBytes[len(jsonBytes)-1] == '\r' || jsonBytes[len(jsonBytes)-1] == '\n' || jsonBytes[len(jsonBytes)-1] == '\t') {
				jsonBytes = jsonBytes[:len(jsonBytes)-1]
			}
			if len(jsonBytes) == 0 || jsonBytes[len(jsonBytes)-1] != ']' {
				continue
			}
			var items []SteamCachedItem
			if err := json.Unmarshal(jsonBytes, &items); err != nil {
				continue
			}
			for _, item := range items {
				appIdStr := valToString(item.AppID)
				if appIdStr != "3678970" {
					continue
				}
				itemDefId := valToInt(item.ItemDefID)
				itemIdStr := valToString(item.ItemID)
				if itemIdStr != "" && itemDefId != 0 {
					itemId, err := strconv.ParseInt(itemIdStr, 10, 64)
					if err == nil {
						itemMap[itemId] = itemDefId
					}
				}
				if itemDefId != 0 && (item.DropInterval != nil || item.DropWindow != nil) {
					cfg := ChestCooldownConfig{
						ItemDefID:        itemDefId,
						DropInterval:     valToInt(item.DropInterval),
						DropMaxPerWindow: valToInt(item.DropMaxPerWindow),
						DropWindow:       valToInt(item.DropWindow),
						UseDropWindow:    true,
					}
					if item.UseDropWindow != nil {
						if b, ok := item.UseDropWindow.(bool); ok {
							cfg.UseDropWindow = b
						} else if s := valToString(item.UseDropWindow); s == "true" {
							cfg.UseDropWindow = true
						}
					}
					if cfg.DropMaxPerWindow == 0 {
						cfg.DropMaxPerWindow = 3
					}
					cooldownMap[itemDefId] = cfg
				}
			}
		}
	}
	return itemMap, cooldownMap
}

var steamScanMu sync.Mutex
var steamScanAt time.Time
var steamScanItems map[int64]int
var steamScanCfgs map[int]ChestCooldownConfig

func scanSteamCached() (map[int64]int, map[int]ChestCooldownConfig) {
	steamScanMu.Lock()
	defer steamScanMu.Unlock()
	if steamScanItems != nil && time.Since(steamScanAt) < 5*time.Minute {
		return steamScanItems, steamScanCfgs
	}
	steamScanItems, steamScanCfgs = ScanSteamInventoryCache()
	steamScanAt = time.Now()
	return steamScanItems, steamScanCfgs
}

var chestHistMu sync.Mutex

func InitializeChestLookup(webFiles embed.FS, gameDataDir string) {
	var data []byte
	var err error
	localPath := filepath.Join(gameDataDir, "chest_drops.json")
	if data, err = os.ReadFile(localPath); err != nil {
		data, err = webFiles.ReadFile("web/chest_drops.json")
		if err != nil {
			return
		}
	}
	var doc ChestDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return
	}
	for _, c := range doc.Chests {
		chestNames[c.ID] = c.Name
		chestIconURLs[c.ID] = c.IconURL
		bType := -1
		if c.Grade == "COMMON" {
			bType = 0
		} else if c.Grade == "RARE" {
			bType = 1
		} else if c.Grade == "LEGENDARY" {
			bType = 2
		}
		if bType != -1 {
			for _, st := range c.Stages {
				stageAndTypeToBoxId[stageAndType{StageKey: st.Key, BoxType: bType}] = c.ID
			}
		}
	}
}

const ChestHistoryPath = "chest_history.json"

func LoadChestHistory() ([]ChestDropEvent, error) {
	data, err := os.ReadFile(ChestHistoryPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var history []ChestDropEvent
	if err := json.Unmarshal(data, &history); err != nil {
		return nil, err
	}
	return history, nil
}

func SaveChestHistory(history []ChestDropEvent) error {
	// Reter mais que o maior ciclo conhecido: 920001–006 têm cooldown de 3600 min (60h)
	// e janela de 10800 min (7,5 dias) — 48h apagaria o drop antes do cooldown acabar.
	cutoff := time.Now().Add(-8 * 24 * time.Hour)
	var filtered []ChestDropEvent
	for _, ev := range history {
		if ev.Timestamp.After(cutoff) {
			filtered = append(filtered, ev)
		}
	}
	data, err := json.MarshalIndent(filtered, "", "  ")
	if err != nil {
		return err
	}
	tmp := ChestHistoryPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, ChestHistoryPath)
}

func GetChestCooldownConfig(itemDefID int, steamConfigs map[int]ChestCooldownConfig) ChestCooldownConfig {
	if cfg, ok := steamConfigs[itemDefID]; ok {
		return cfg
	}
	cfg := ChestCooldownConfig{
		ItemDefID:        itemDefID,
		DropMaxPerWindow: 3,
		UseDropWindow:    true,
	}
	if itemDefID >= 910011 && itemDefID <= 910801 {
		cfg.DropInterval = 11
		cfg.DropWindow = 33
	} else if itemDefID >= 920000 && itemDefID < 930000 {
		if itemDefID >= 920001 && itemDefID <= 920006 {
			cfg.DropInterval = 3600
			cfg.DropWindow = 10800
		} else if itemDefID >= 920020 && itemDefID <= 920059 {
			cfg.DropInterval = 300
			cfg.DropWindow = 900
		} else {
			cfg.DropInterval = 9
			cfg.DropWindow = 27
		}
	} else {
		cfg.DropInterval = 0
		cfg.DropMaxPerWindow = 0
		cfg.DropWindow = 0
		cfg.UseDropWindow = false
	}
	return cfg
}

func GetBoxItemDefID(boxType int, boxUniqueId int64, currentStageKey int, itemIdMap map[int64]int) int {
	if defId, ok := itemIdMap[boxUniqueId]; ok && defId != 0 {
		return defId
	}
	if defId, ok := stageAndTypeToBoxId[stageAndType{StageKey: currentStageKey, BoxType: boxType}]; ok && defId != 0 {
		return defId
	}
	if boxType == 0 {
		return 910011
	} else if boxType == 1 {
		// 920011 e não 920001: a família 920001–006 é a exceção de 3600 min — atribuir
		// um drop desconhecido a ela mostraria 60h de cooldown em vez dos 9 min normais.
		return 920011
	} else if boxType == 2 {
		return 930101
	}
	return 0
}

func GetChestName(defId int) string {
	if name, ok := chestNames[defId]; ok && name != "" {
		return name
	}
	if defId >= 910000 && defId < 920000 {
		return fmt.Sprintf("Caixa de Monstro (%d)", defId)
	} else if defId >= 920000 && defId < 930000 {
		return fmt.Sprintf("Caixa de Chefe de Fase (%d)", defId)
	} else if defId >= 930000 && defId < 940000 {
		return fmt.Sprintf("Caixa de Chefe de Ato (%d)", defId)
	}
	return fmt.Sprintf("Baú %d", defId)
}

func ProcessChestDrops(ctrl *Control, currentSave *InnerSaveData) {
	if ctrl.LastBoxQuantity == nil {
		ctrl.LastBoxQuantity = make(map[int64]int)
		ctrl.LastBoxTypes = make(map[int64]int)
		for i, uniqueId := range currentSave.BoxData.BoxUniqueId {
			if uniqueId != 0 {
				ctrl.LastBoxQuantity[uniqueId] = currentSave.BoxData.BoxQuantity[i]
				ctrl.LastBoxTypes[uniqueId] = currentSave.BoxData.BoxTypes[i]
			}
		}
		return
	}
	itemIdMap, _ := scanSteamCached()

	// Calculate total quantity by box type in the last baseline
	oldTotalByType := make(map[int]int)
	for uniqueId, qty := range ctrl.LastBoxQuantity {
		bType := ctrl.LastBoxTypes[uniqueId]
		oldTotalByType[bType] += qty
	}

	// Calculate total quantity by box type in the current save
	newTotalByType := make(map[int]int)
	for i, uniqueId := range currentSave.BoxData.BoxUniqueId {
		if uniqueId != 0 {
			bType := currentSave.BoxData.BoxTypes[i]
			qty := currentSave.BoxData.BoxQuantity[i]
			newTotalByType[bType] += qty
		}
	}

	type dropToRecord struct {
		defId int
		uid   int64
	}
	var newDrops []dropToRecord

	// Check if the total count of any box type increased to avoid false drops on ID regeneration
	for bType, newQty := range newTotalByType {
		oldQty := oldTotalByType[bType]
		if newQty > oldQty {
			diff := newQty - oldQty
			
			// Find the new unique IDs of this box type in the current save
			var newUniqueIds []int64
			for i, uniqueId := range currentSave.BoxData.BoxUniqueId {
				if uniqueId != 0 && currentSave.BoxData.BoxTypes[i] == bType {
					if _, exists := ctrl.LastBoxQuantity[uniqueId]; !exists {
						newUniqueIds = append(newUniqueIds, uniqueId)
					}
				}
			}
			
			// Associate the drops to these new unique IDs (up to diff drops)
			for i := 0; i < diff; i++ {
				var uniqueId int64 = 0
				if i < len(newUniqueIds) {
					uniqueId = newUniqueIds[i]
				}
				
				defId := 0
				if uniqueId != 0 {
					defId = GetBoxItemDefID(bType, uniqueId, currentSave.CommonSaveData.CurrentStageKey, itemIdMap)
				}
				if defId == 0 {
					defId = GetBoxItemDefID(bType, 0, currentSave.CommonSaveData.CurrentStageKey, itemIdMap)
				}
				
				if defId != 0 {
					newDrops = append(newDrops, dropToRecord{defId: defId, uid: uniqueId})
				}
			}
		}
	}

	// Update baseline
	ctrl.LastBoxQuantity = make(map[int64]int)
	ctrl.LastBoxTypes = make(map[int64]int)
	for i, uniqueId := range currentSave.BoxData.BoxUniqueId {
		if uniqueId != 0 {
			ctrl.LastBoxQuantity[uniqueId] = currentSave.BoxData.BoxQuantity[i]
			ctrl.LastBoxTypes[uniqueId] = currentSave.BoxData.BoxTypes[i]
		}
	}

	if len(newDrops) > 0 {
		now := time.Now()
		chestHistMu.Lock()
		for _, dr := range newDrops {
			ctrl.ChestHistory = append(ctrl.ChestHistory, ChestDropEvent{
				ItemDefID:   dr.defId,
				Timestamp:   now,
				BoxUniqueId: dr.uid,
			})
			name := GetChestName(dr.defId)
			if dr.uid != 0 {
				Logf("run", "BAÚ DETECTADO: %s dropado! (ID: %d)", name, dr.uid)
			} else {
				Logf("run", "BAÚ DETECTADO: %s dropado!", name)
			}
		}
		history := append([]ChestDropEvent(nil), ctrl.ChestHistory...)
		chestHistMu.Unlock()
		_ = SaveChestHistory(history)
	}
}

func CalculateCooldowns(itemDefId int, history []ChestDropEvent, cfg ChestCooldownConfig) (droppedInWindow int, windowRemaining int, cooldownRemaining int, status string) {
	if cfg.DropInterval == 0 && cfg.DropWindow == 0 {
		return 0, 0, 0, "available"
	}
	now := time.Now()
	intervalDur := time.Duration(cfg.DropInterval) * time.Minute
	windowDur := time.Duration(cfg.DropWindow) * time.Minute
	var myDrops []time.Time
	for _, ev := range history {
		if ev.ItemDefID == itemDefId {
			myDrops = append(myDrops, ev.Timestamp)
		}
	}
	if len(myDrops) == 0 {
		return 0, 0, 0, "available"
	}
	lastDrop := myDrops[len(myDrops)-1]
	timeSinceLast := now.Sub(lastDrop)
	if timeSinceLast < intervalDur {
		cooldownRemaining = int((intervalDur - timeSinceLast).Seconds())
	}
	
	// Sliding rolling window: count how many drops happened in the last cfg.DropWindow minutes
	var activeDrops []time.Time
	for _, t := range myDrops {
		if now.Sub(t) < windowDur {
			activeDrops = append(activeDrops, t)
		}
	}
	droppedInWindow = len(activeDrops)

	// If there are active drops, the window remaining is when the oldest active drop expires
	if len(activeDrops) > 0 {
		oldestActive := activeDrops[0]
		timeSinceOldest := now.Sub(oldestActive)
		if timeSinceOldest < windowDur {
			windowRemaining = int((windowDur - timeSinceOldest).Seconds())
		}
	}

	status = "available"
	if cooldownRemaining > 0 {
		status = "cooldown"
	}
	return droppedInWindow, windowRemaining, cooldownRemaining, status
}

func GetChestTrackerStatus(ctrl *Control) ChestTrackerStatus {
	itemIdMap, steamConfigs := scanSteamCached()
	chestHistMu.Lock()
	historyUpdated := false
	for i := range ctrl.ChestHistory {
		ev := &ctrl.ChestHistory[i]
		if ev.BoxUniqueId != 0 {
			if correctDefID, ok := itemIdMap[ev.BoxUniqueId]; ok && correctDefID != 0 && correctDefID != ev.ItemDefID {
				Logf("run", "Auto-corrigindo ItemDefID do baú único %d de %d para %d no histórico", ev.BoxUniqueId, ev.ItemDefID, correctDefID)
				ev.ItemDefID = correctDefID
				historyUpdated = true
			}
		}
	}
	if historyUpdated {
		historyCopy := append([]ChestDropEvent(nil), ctrl.ChestHistory...)
		go func() {
			_ = SaveChestHistory(historyCopy)
		}()
	}
	history := append([]ChestDropEvent(nil), ctrl.ChestHistory...)
	chestHistMu.Unlock()
	var families []ChestFamilyStatus
	for _, opt := range OptimalNormalChests {
		cfg := GetChestCooldownConfig(opt.ItemDefID, steamConfigs)
		dropped, winRem, coolRem, status := CalculateCooldowns(opt.ItemDefID, history, cfg)
		name := GetChestName(opt.ItemDefID)
		icon := ""
		if url, ok := chestIconURLs[opt.ItemDefID]; ok {
			icon = url
		}
		families = append(families, ChestFamilyStatus{
			ItemDefID:         opt.ItemDefID,
			Name:              name,
			Rarity:            "common",
			IconURL:           icon,
			DroppedInWindow:   dropped,
			MaxPerWindow:      cfg.DropMaxPerWindow,
			WindowSeconds:     cfg.DropWindow * 60,
			WindowRemaining:   winRem,
			CooldownRemaining: coolRem,
			OptimalStage:      opt.StageKey,
			OptimalStageName:  ctrl.stageDisplay(opt.StageKey),
			Status:            status,
		})
	}
	// Find all boss chest defIds and their optimal (lowest) stages dynamically
	allBossChests := make(map[int]int)
	for key, val := range stageAndTypeToBoxId {
		if key.BoxType == 1 && val >= 920000 && val < 930000 {
			oldStage, ok := allBossChests[val]
			if !ok || key.StageKey < oldStage {
				allBossChests[val] = key.StageKey
			}
		}
	}

	for defId, optStage := range allBossChests {
		cfg := GetChestCooldownConfig(defId, steamConfigs)
		dropped, winRem, coolRem, status := CalculateCooldowns(defId, history, cfg)
		
		// Only list the boss chest if the player has unlocked its optimal stage
		if optStage > ctrl.MaxCompletedStage && optStage != 1101 {
			continue
		}
		
		name := GetChestName(defId)
		icon := ""
		if url, ok := chestIconURLs[defId]; ok {
			icon = url
		}
		families = append(families, ChestFamilyStatus{
			ItemDefID:         defId,
			Name:              name,
			Rarity:            "rare",
			IconURL:           icon,
			DroppedInWindow:   dropped,
			MaxPerWindow:      cfg.DropMaxPerWindow,
			WindowSeconds:     cfg.DropWindow * 60,
			WindowRemaining:   winRem,
			CooldownRemaining: coolRem,
			OptimalStage:      optStage,
			OptimalStageName:  ctrl.stageDisplay(optStage),
			Status:            status,
		})
	}

	suggestMap := 0
	bestStageKey := 0
	var availableStages []ChestFamilyStatus
	var cooldownStages []ChestFamilyStatus
	var cappedStages []ChestFamilyStatus
	for _, fam := range families {
		// Prioritize blue (boss) chests over common chests for farm suggestion
		if fam.Rarity != "rare" {
			continue
		}
		if fam.OptimalStage > ctrl.MaxCompletedStage && fam.OptimalStage != 1101 {
			continue
		}
		if fam.Status == "available" {
			availableStages = append(availableStages, fam)
		} else if fam.Status == "cooldown" {
			cooldownStages = append(cooldownStages, fam)
		} else {
			cappedStages = append(cappedStages, fam)
		}
	}
	if len(availableStages) > 0 {
		for _, fam := range availableStages {
			if fam.OptimalStage > bestStageKey {
				bestStageKey = fam.OptimalStage
				suggestMap = fam.OptimalStage
			}
		}
	} else if len(cooldownStages) > 0 {
		minCool := 9999999
		for _, fam := range cooldownStages {
			if fam.CooldownRemaining < minCool {
				minCool = fam.CooldownRemaining
				suggestMap = fam.OptimalStage
			}
		}
	} else if len(cappedStages) > 0 {
		minWin := 9999999
		for _, fam := range cappedStages {
			if fam.WindowRemaining < minWin {
				minWin = fam.WindowRemaining
				suggestMap = fam.OptimalStage
			}
		}
	}
	return ChestTrackerStatus{
		Families:   families,
		SuggestMap: suggestMap,
	}
}
