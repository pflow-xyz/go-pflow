// Package catacombs implements a roguelike dungeon crawler using Petri nets.
package catacombs

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/pflow-xyz/go-pflow/petri"
)

// ActionType represents player actions
type ActionType string

const (
	ActionMoveUp    ActionType = "move_up"
	ActionMoveDown  ActionType = "move_down"
	ActionMoveLeft  ActionType = "move_left"
	ActionMoveRight ActionType = "move_right"
	ActionInteract  ActionType = "interact"
	ActionAttack    ActionType = "attack"
	ActionUseItem   ActionType = "use_item"
	ActionOpenDoor  ActionType = "open_door"
	ActionTalk      ActionType = "talk"
	ActionWait      ActionType = "wait"
	ActionDescend   ActionType = "descend"
	ActionAscend    ActionType = "ascend"
	// Combat actions
	ActionEndTurn     ActionType = "end_turn"
	ActionAimedShot   ActionType = "aimed_shot"
	ActionCombatMove  ActionType = "combat_move"
	ActionFlee        ActionType = "flee"
)

// BodyPart represents targetable body parts (Fallout 2 style)
type BodyPart int

const (
	BodyTorso BodyPart = iota
	BodyHead
	BodyLeftArm
	BodyRightArm
	BodyLeftLeg
	BodyRightLeg
	BodyEyes
	BodyGroin
)

// BodyPartInfo contains targeting data for each body part
var BodyPartInfo = map[BodyPart]struct {
	Name       string
	HitPenalty int  // Penalty to hit chance
	DamageMult float64 // Damage multiplier
	CritMult   float64 // Critical chance multiplier
	CanCripple bool
}{
	BodyTorso:    {"Torso", 0, 1.0, 1.0, false},
	BodyHead:     {"Head", 40, 1.2, 3.0, true},
	BodyLeftArm:  {"Left Arm", 30, 0.8, 1.5, true},
	BodyRightArm: {"Right Arm", 30, 0.8, 1.5, true},
	BodyLeftLeg:  {"Left Leg", 20, 0.8, 1.5, true},
	BodyRightLeg: {"Right Leg", 20, 0.8, 1.5, true},
	BodyEyes:     {"Eyes", 60, 1.0, 4.0, true},
	BodyGroin:    {"Groin", 30, 1.0, 2.5, true},
}

// CombatState tracks turn-based combat
type CombatState struct {
	Active        bool           `json:"active"`
	PlayerTurn    bool           `json:"player_turn"`
	CurrentAP     int            `json:"current_ap"`
	MaxAP         int            `json:"max_ap"`
	SelectedEnemy string         `json:"selected_enemy,omitempty"`
	TargetPart    BodyPart       `json:"target_part"`
	TurnOrder     []string       `json:"turn_order"` // IDs in turn order
	TurnIndex     int            `json:"turn_index"`
	Combatants    []string       `json:"combatants"` // Enemy IDs in combat
	RoundNumber   int            `json:"round_number"`
	CombatLog     []string       `json:"combat_log"`
}

// CombatResult contains outcome of an attack
type CombatResult struct {
	Hit        bool     `json:"hit"`
	Damage     int      `json:"damage"`
	Critical   bool     `json:"critical"`
	CritEffect string   `json:"crit_effect,omitempty"`
	Miss       bool     `json:"miss"`
	Message    string   `json:"message"`
}

// AP costs for actions
const (
	APCostMove       = 1
	APCostAttack     = 4
	APCostAimedShot  = 6
	APCostUseItem    = 2
	APCostReload     = 2
	BaseAP           = 10
)

// Enemy represents a hostile creature
type Enemy struct {
	ID           string         `json:"id"`
	Type         EnemyType      `json:"type"`
	Name         string         `json:"name"`
	X            int            `json:"x"`
	Y            int            `json:"y"`
	Health       int            `json:"health"`
	MaxHealth    int            `json:"max_health"`
	Damage       int            `json:"damage"`
	XP           int            `json:"xp"`
	State        EnemyState     `json:"state"`
	AlertDist    int            `json:"alert_dist"`
	// Combat stats
	AP           int            `json:"ap"`
	MaxAP        int            `json:"max_ap"`
	Accuracy     int            `json:"accuracy"`    // Base hit chance
	CrippledParts map[BodyPart]bool `json:"crippled_parts,omitempty"`
}

// EnemyType categorizes enemies
type EnemyType int

const (
	EnemySkeleton EnemyType = iota
	EnemyZombie
	EnemyGhost
	EnemySpider
	EnemyBat
	EnemyRat
	EnemyOrc
	EnemyTroll
	EnemyLich
)

// EnemyState tracks enemy behavior
type EnemyState int

const (
	StateIdle EnemyState = iota
	StatePatrol
	StateAlert
	StateChasing
	StateAttacking
	StateFleeing
	StateDead
)

// Player represents the player character
type Player struct {
	X          int             `json:"x"`
	Y          int             `json:"y"`
	FacingX    int             `json:"facing_x"` // Direction player is facing (-1, 0, or 1)
	FacingY    int             `json:"facing_y"` // Direction player is facing (-1, 0, or 1)
	Health     int             `json:"health"`
	MaxHealth  int             `json:"max_health"`
	Mana       int             `json:"mana"`
	MaxMana    int             `json:"max_mana"`
	Gold       int             `json:"gold"`
	XP         int             `json:"xp"`
	Level      int             `json:"level"`
	Attack     int             `json:"attack"`
	Defense    int             `json:"defense"`
	Inventory  []Item          `json:"inventory"`
	Keys       map[string]bool `json:"keys"`
	ActiveQuests []string      `json:"active_quests"`
	// Combat stats (SPECIAL-like)
	Strength   int             `json:"strength"`   // Melee damage, carry weight
	Perception int             `json:"perception"` // Ranged accuracy, spotting
	Agility    int             `json:"agility"`    // AP, dodge chance
	Luck       int             `json:"luck"`       // Critical chance
	Accuracy   int             `json:"accuracy"`   // Base hit chance (derived from weapon skill + perception)
	CritChance int             `json:"crit_chance"` // Base critical chance
	CrippledParts map[BodyPart]bool `json:"crippled_parts,omitempty"`
}

// AIState tracks AI player behavior
type AIState struct {
	Enabled       bool              `json:"enabled"`
	Mode          string            `json:"mode"`          // "explore", "combat", "interact", "heal", "find_key"
	Target        string            `json:"target"`        // Current target (enemy ID, NPC ID, or coordinates)
	Path          [][2]int          `json:"path"`          // Planned path
	ThinkDelay    int               `json:"think_delay"`   // Ticks between actions
	LastAction    string            `json:"last_action"`
	ActionCount   int               `json:"action_count"`
	GoalsComplete map[string]bool   `json:"goals_complete"` // Track demo goals
	StuckCounter  int               `json:"-"`              // How many times we've been in same spot
	LastX         int               `json:"-"`              // Last position X
	LastY         int               `json:"-"`              // Last position Y
	LockedDoors   [][2]int          `json:"-"`              // Known locked door positions
	AvoidDoors    map[[2]int]bool   `json:"-"`              // Doors to avoid (no key yet)
	RecentPos     [][2]int          `json:"-"`              // Recent positions for oscillation detection
	TargetTicks   int               `json:"-"`              // Ticks spent pursuing current target
	FleeAttempts  int               `json:"-"`              // Failed flee attempts this combat (reset on combat end)
	CommitToFight bool              `json:"-"`              // After too many flee failures, commit to fighting
	FleeFrom      [2]int            `json:"-"`              // Position we fled from (to move away after flee)
	FleeTicks     int               `json:"-"`              // Ticks since we fled (to know when to stop running)
	// pflow-based AI components (nil if using legacy mode)
	StateMachine    *AIStateMachine   `json:"-"` // Formal state machine for mode transitions
	CombatEvaluator *CombatEvaluator  `json:"-"` // ODE-based combat decision evaluator
	Brain           *AIBrain          `json:"-"` // Petri net-based AI brain with memory and goals

	// Cache instrumentation
	CacheStatsInterval int `json:"-"` // Log cache stats every N ticks (0 = disabled)
	LastCacheLog       int `json:"-"` // Last tick we logged cache stats
}

// Game represents the complete game state
type Game struct {
	Dungeon       *Dungeon          `json:"dungeon"`
	Player        Player            `json:"player"`
	Enemies       []*Enemy          `json:"enemies"`
	NPCs          []*NPC            `json:"npcs"`
	Quests        map[string]*Quest `json:"quests"`
	Items         []*GroundItem     `json:"items"`
	Level         int               `json:"level"`
	Turn          int               `json:"turn"`
	Message       string            `json:"message"`
	MessageLog    []string          `json:"message_log"`
	GameOver      bool              `json:"game_over"`
	Victory       bool              `json:"victory"`
	InDialogue    bool              `json:"in_dialogue"`
	DialogueNPC   string            `json:"dialogue_npc,omitempty"`
	DialogueNode  string            `json:"dialogue_node,omitempty"`
	InShop        bool              `json:"in_shop"`
	Combat        CombatState       `json:"combat"`
	AI            AIState           `json:"ai"`
	Seed          int64             `json:"seed"`
	AISeed        int64             `json:"ai_seed"`
	InfiniteMode  bool              `json:"infinite_mode"`
	rng           *rand.Rand
	aiRng         *rand.Rand // Separate RNG for AI decisions
	net           *petri.PetriNet
}

// GroundItem represents an item on the floor
type GroundItem struct {
	Item Item `json:"item"`
	X    int  `json:"x"`
	Y    int  `json:"y"`
}

// GameState is the serialized state sent to clients
type GameState struct {
	MapWidth      int               `json:"map_width"`
	MapHeight     int               `json:"map_height"`
	Tiles         [][]int           `json:"tiles"`
	Player        Player            `json:"player"`
	Enemies       []EnemyView       `json:"enemies"`
	NPCs          []NPCView         `json:"npcs"`
	Items         []ItemView        `json:"items"`
	Level         int               `json:"level"`
	Turn          int               `json:"turn"`
	Message       string            `json:"message"`
	MessageLog    []string          `json:"message_log"`
	GameOver      bool              `json:"game_over"`
	Victory       bool              `json:"victory"`
	InDialogue    bool              `json:"in_dialogue"`
	DialogueNPC   string            `json:"dialogue_npc,omitempty"`
	DialogueNode  string            `json:"dialogue_node,omitempty"`
	DialogueData  *DialogueView     `json:"dialogue_data,omitempty"`
	InShop        bool              `json:"in_shop"`
	ShopItems     []Item            `json:"shop_items,omitempty"`
	VisibleTiles  [][]bool          `json:"visible_tiles"`
	ExploredTiles [][]bool          `json:"explored_tiles"`
	Seed          int64             `json:"seed"`
	AISeed        int64             `json:"ai_seed"`
	InfiniteMode  bool              `json:"infinite_mode"`
	// Combat state
	Combat        *CombatView       `json:"combat,omitempty"`
	// AI state
	AI            *AIState          `json:"ai,omitempty"`
}

// CombatView is the client-visible combat state
type CombatView struct {
	Active        bool              `json:"active"`
	PlayerTurn    bool              `json:"player_turn"`
	CurrentAP     int               `json:"current_ap"`
	MaxAP         int               `json:"max_ap"`
	SelectedEnemy string            `json:"selected_enemy,omitempty"`
	TargetPart    int               `json:"target_part"`
	TargetPartName string           `json:"target_part_name"`
	HitChance     int               `json:"hit_chance"`
	Combatants    []CombatantView   `json:"combatants"`
	RoundNumber   int               `json:"round_number"`
	CombatLog     []string          `json:"combat_log"`
	AvailableActions []CombatAction `json:"available_actions"`
}

// CombatantView shows a combatant's status
type CombatantView struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	IsPlayer      bool     `json:"is_player"`
	Health        int      `json:"health"`
	MaxHealth     int      `json:"max_health"`
	AP            int      `json:"ap"`
	MaxAP         int      `json:"max_ap"`
	IsTurn        bool     `json:"is_turn"`
	CrippledParts []string `json:"crippled_parts,omitempty"`
}

// CombatAction represents an available combat action
type CombatAction struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	APCost   int    `json:"ap_cost"`
	Enabled  bool   `json:"enabled"`
	HitChance int   `json:"hit_chance,omitempty"`
}

// EnemyView is the client-visible enemy data
type EnemyView struct {
	ID        string `json:"id"`
	Type      int    `json:"type"`
	Name      string `json:"name"`
	X         int    `json:"x"`
	Y         int    `json:"y"`
	Health    int    `json:"health"`
	MaxHealth int    `json:"max_health"`
	State     int    `json:"state"`
}

// NPCView is the client-visible NPC data
type NPCView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type int    `json:"type"`
	X    int    `json:"x"`
	Y    int    `json:"y"`
	Met  bool   `json:"met"`
}

// ItemView is the client-visible item data
type ItemView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	X    int    `json:"x"`
	Y    int    `json:"y"`
}

// DialogueView contains current dialogue state
type DialogueView struct {
	Speaker string          `json:"speaker"`
	Text    string          `json:"text"`
	Choices []DialogueChoice `json:"choices,omitempty"`
}

// EnemyTemplates provides enemy configurations
var EnemyTemplates = map[EnemyType]struct {
	Name      string
	Health    int
	Damage    int
	XP        int
	AlertDist int
	AP        int
	Accuracy  int
}{
	EnemySkeleton: {"Skeleton", 20, 5, 10, 5, 6, 50},
	EnemyZombie:   {"Zombie", 30, 8, 15, 4, 4, 40},
	EnemyGhost:    {"Ghost", 15, 10, 20, 7, 8, 60},
	EnemySpider:   {"Giant Spider", 12, 6, 8, 6, 10, 55},
	EnemyBat:      {"Bat", 8, 3, 5, 8, 12, 45},
	EnemyRat:      {"Giant Rat", 10, 4, 5, 5, 8, 50},
	EnemyOrc:      {"Orc", 40, 12, 25, 6, 8, 55},
	EnemyTroll:    {"Troll", 60, 15, 40, 5, 6, 50},
	EnemyLich:     {"Lich King", 200, 30, 500, 10, 12, 75},
}

// NewGame creates a new game with default parameters
func NewGame() *Game {
	return NewGameWithParams(DefaultParams())
}

// NewGameWithParams creates a new game with custom parameters
func NewGameWithParams(params DungeonParams) *Game {
	seed := params.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(seed))

	// AI seed defaults to map seed if not specified
	aiSeed := params.AISeed
	if aiSeed == 0 {
		aiSeed = seed
	}
	aiRng := rand.New(rand.NewSource(aiSeed))

	g := &Game{
		Player: Player{
			Health:    100,
			MaxHealth: 100,
			Mana:      50,
			MaxMana:   50,
			Gold:      20,
			XP:        0,
			Level:     1,
			Attack:    10,
			Defense:   5,
			Inventory: []Item{},
			Keys:      make(map[string]bool),
			ActiveQuests: []string{},
			// SPECIAL-like stats
			Strength:   5,
			Perception: 5,
			Agility:    5,
			Luck:       5,
			Accuracy:   65, // Base 65% hit chance
			CritChance: 5,  // Base 5% crit chance
			CrippledParts: make(map[BodyPart]bool),
		},
		Enemies:    make([]*Enemy, 0),
		NPCs:       make([]*NPC, 0),
		Quests:     make(map[string]*Quest),
		Items:      make([]*GroundItem, 0),
		Level:      1,
		Turn:       0,
		MessageLog: make([]string, 0),
		Seed:       seed,
		AISeed:     aiSeed,
		rng:        rng,
		aiRng:      aiRng,
	}

	// Generate dungeon with reachability validation
	// Try up to 10 times with different seeds to find a valid dungeon
	maxAttempts := 10
	for attempt := 0; attempt < maxAttempts; attempt++ {
		// Generate dungeon
		g.Dungeon = GenerateDungeon(params, 1)
		g.Player.X = g.Dungeon.SpawnX
		g.Player.Y = g.Dungeon.SpawnY
		g.Player.FacingX = 0
		g.Player.FacingY = 1

		// Clear previous items/NPCs/enemies for regeneration attempts
		g.Enemies = make([]*Enemy, 0)
		g.NPCs = make([]*NPC, 0)
		g.Items = make([]*GroundItem, 0)

		// Populate
		g.populateEnemies(params)
		g.populateNPCs(params)
		g.populateItems(params)

		// Validate dungeon reachability using Petri net analysis
		keyLocations := g.getKeyLocations()
		valid, reason := ValidateDungeon(g.Dungeon, keyLocations)

		if valid {
			break // Dungeon is valid
		}

		// If last attempt, force fix the dungeon
		if attempt == maxAttempts-1 {
			g.fixUnreachableDungeon(reason)
			break
		}

		// Try with a different seed
		params.Seed = seed + int64(attempt+1)
		g.rng = rand.New(rand.NewSource(params.Seed))
	}

	// Build Petri net for resource tracking
	g.buildPetriNet()

	g.addMessage("You descend into the dark catacombs...")

	return g
}

// getKeyLocations returns positions of all keys on the ground
func (g *Game) getKeyLocations() [][2]int {
	var keys [][2]int
	for _, item := range g.Items {
		if item.Item.Type == ItemKey {
			keys = append(keys, [2]int{item.X, item.Y})
		}
	}
	return keys
}

// getItemLocationsWithValues returns item positions with their values for path scoring.
// Higher value items (keys, potions) get higher scores to encourage collection.
func (g *Game) getItemLocationsWithValues() map[[2]int]float64 {
	items := make(map[[2]int]float64)
	for _, item := range g.Items {
		pos := [2]int{item.X, item.Y}
		// Assign values based on item type
		var value float64
		switch item.Item.Type {
		case ItemKey:
			value = 50.0 // Keys are very valuable
		case ItemPotion:
			value = 20.0 // Potions are valuable for survival
		case ItemWeapon, ItemArmor:
			value = 10.0 // Equipment is useful
		case ItemScroll:
			value = 8.0
		case ItemGold:
			value = float64(item.Item.Value) * 0.1 // Gold scales with amount
		case ItemQuest:
			value = 25.0 // Quest items are important
		default:
			value = 5.0
		}
		items[pos] = value
	}
	return items
}

// getChestLocations returns positions of all unopened chests
func (g *Game) getChestLocations() [][2]int {
	var chests [][2]int
	for y := 0; y < g.Dungeon.Height; y++ {
		for x := 0; x < g.Dungeon.Width; x++ {
			if g.Dungeon.Tiles[y][x] == TileChest {
				chests = append(chests, [2]int{x, y})
			}
		}
	}
	return chests
}

// fixUnreachableDungeon attempts to make the dungeon playable
func (g *Game) fixUnreachableDungeon(reason string) {
	// If stairs are unreachable, remove all locked doors as a fallback
	for y := 0; y < g.Dungeon.Height; y++ {
		for x := 0; x < g.Dungeon.Width; x++ {
			if g.Dungeon.Tiles[y][x] == TileLockedDoor {
				g.Dungeon.Tiles[y][x] = TileDoor // Convert to regular door
			}
		}
	}
}

func (g *Game) populateEnemies(params DungeonParams) {
	enemyCount := int(float64(len(g.Dungeon.Rooms)) * params.EnemyDensity * 3)

	for i := 0; i < enemyCount; i++ {
		// Pick a random room (not start room)
		if len(g.Dungeon.Rooms) < 2 {
			continue
		}
		roomIdx := 1 + g.rng.Intn(len(g.Dungeon.Rooms)-1)
		room := g.Dungeon.Rooms[roomIdx]

		// Random position in room
		x := room.X + 1 + g.rng.Intn(room.Width-2)
		y := room.Y + 1 + g.rng.Intn(room.Height-2)

		// Check tile is walkable (not wall)
		if g.Dungeon.Tiles[y][x] == TileWall {
			continue
		}

		// Check not occupied
		if g.getEnemyAt(x, y) != nil || g.getNPCAt(x, y) != nil {
			continue
		}

		// Enemy type based on difficulty and room type
		var enemyType EnemyType
		if room.Type == RoomBoss {
			if g.Level >= 5 {
				enemyType = EnemyLich
			} else {
				enemyType = EnemyTroll
			}
		} else {
			// Weighted random by difficulty
			types := []EnemyType{EnemyRat, EnemyBat, EnemySkeleton, EnemyZombie, EnemySpider}
			if params.Difficulty > 3 {
				types = append(types, EnemyOrc)
			}
			if params.Difficulty > 5 {
				types = append(types, EnemyGhost, EnemyTroll)
			}
			enemyType = types[g.rng.Intn(len(types))]
		}

		template := EnemyTemplates[enemyType]
		// Scale by difficulty
		healthMod := 1.0 + float64(params.Difficulty-1)*0.1
		damageMod := 1.0 + float64(params.Difficulty-1)*0.1

		enemy := &Enemy{
			ID:            fmt.Sprintf("enemy_%d", i),
			Type:          enemyType,
			Name:          template.Name,
			X:             x,
			Y:             y,
			Health:        int(float64(template.Health) * healthMod),
			MaxHealth:     int(float64(template.Health) * healthMod),
			Damage:        int(float64(template.Damage) * damageMod),
			XP:            template.XP,
			State:         StateIdle,
			AlertDist:     template.AlertDist,
			AP:            template.AP,
			MaxAP:         template.AP,
			Accuracy:      template.Accuracy,
			CrippledParts: make(map[BodyPart]bool),
		}
		g.Enemies = append(g.Enemies, enemy)
	}
}

func (g *Game) populateNPCs(params DungeonParams) {
	npcCount := params.NPCCount
	if npcCount == 0 {
		npcCount = 3
	}

	// Guaranteed NPCs: merchant, healer, quest giver
	npcTypes := []NPCType{NPCMerchant, NPCHealer, NPCQuestGiver}
	for i := 0; i < npcCount && i < len(npcTypes); i++ {
		npcTypes = append(npcTypes, NPCType(g.rng.Intn(6)))
	}

	for i, npcType := range npcTypes {
		// Find a safe room (not start, not boss, not exit)
		var room *Room
		for attempts := 0; attempts < 50; attempts++ {
			idx := g.rng.Intn(len(g.Dungeon.Rooms))
			r := g.Dungeon.Rooms[idx]
			if r.Type != RoomStart && r.Type != RoomExit && r.Type != RoomBoss {
				room = r
				break
			}
		}
		if room == nil && len(g.Dungeon.Rooms) > 1 {
			room = g.Dungeon.Rooms[1]
		}
		if room == nil {
			continue
		}

		// Position in room
		x := room.X + room.Width/2
		y := room.Y + room.Height/2

		// Offset to avoid overlap (limit attempts to prevent infinite loop)
		for attempts := 0; attempts < 20 && (g.getEnemyAt(x, y) != nil || g.getNPCAt(x, y) != nil); attempts++ {
			x = room.X + 1 + g.rng.Intn(max(1, room.Width-2))
			y = room.Y + 1 + g.rng.Intn(max(1, room.Height-2))
		}

		npc := GenerateNPC(g.rng, npcType, fmt.Sprintf("npc_%d", i), x, y)
		g.NPCs = append(g.NPCs, npc)

		// Create quest for quest givers
		if npcType == NPCQuestGiver {
			quest := &Quest{
				ID:          fmt.Sprintf("quest_%d", i),
				Name:        "Clear the Skeletons",
				Description: "Defeat 5 skeletons in the eastern chambers",
				GiverID:     npc.ID,
				Status:      QuestNotStarted,
				Objective:   "kill",
				Target:      "Skeleton",
				Required:    5,
				Progress:    0,
				RewardGold:  50,
				RewardXP:    100,
			}
			g.Quests[quest.ID] = quest
			npc.QuestID = quest.ID
		}
	}
}

func (g *Game) populateItems(params DungeonParams) {
	// Add items to treasure rooms and scattered around
	itemPool := []Item{
		{ID: "health_potion", Name: "Health Potion", Type: ItemPotion, Value: 25, Effect: 30, Description: "Restores 30 health"},
		{ID: "gold_pile", Name: "Gold Coins", Type: ItemGold, Value: 15, Effect: 15, Description: "15 gold"},
		{ID: "rusty_key", Name: "Rusty Key", Type: ItemKey, Value: 0, Effect: 0, Description: "Opens locked doors"},
	}

	for _, room := range g.Dungeon.Rooms {
		var itemChance float64
		switch room.Type {
		case RoomTreasure:
			itemChance = 1.0
		case RoomShrine:
			itemChance = 0.5
		default:
			itemChance = params.LootDensity * 0.3
		}

		if g.rng.Float64() < itemChance {
			item := itemPool[g.rng.Intn(len(itemPool))]
			x := room.X + 1 + g.rng.Intn(room.Width-2)
			y := room.Y + 1 + g.rng.Intn(room.Height-2)

			// Don't place items at spawn position
			if x == g.Dungeon.SpawnX && y == g.Dungeon.SpawnY {
				continue
			}

			g.Items = append(g.Items, &GroundItem{
				Item: item,
				X:    x,
				Y:    y,
			})
		}
	}

	// Ensure a key exists if there are locked doors blocking stairs
	g.ensureKeyForLockedDoors()
}

// ensureKeyForLockedDoors checks if locked doors block the path to stairs
// and places a key in a reachable location if needed
func (g *Game) ensureKeyForLockedDoors() {
	// Check if there are any locked doors
	hasLockedDoor := false
	for y := 0; y < g.Dungeon.Height; y++ {
		for x := 0; x < g.Dungeon.Width; x++ {
			if g.Dungeon.Tiles[y][x] == TileLockedDoor {
				hasLockedDoor = true
				break
			}
		}
		if hasLockedDoor {
			break
		}
	}

	if !hasLockedDoor {
		return
	}

	// Find all tiles reachable from spawn without going through locked doors
	reachable := g.findReachableFromSpawn()

	// Check if an accessible key already exists on the ground
	// A key is only accessible if:
	// 1. There's no enemy or NPC standing on it
	// 2. It's in a reachable location (not behind locked door)
	for _, item := range g.Items {
		if item.Item.Type == ItemKey {
			keyReachable := reachable[[2]int{item.X, item.Y}]
			notBlocked := g.getNPCAt(item.X, item.Y) == nil && g.getEnemyAt(item.X, item.Y) == nil
			if keyReachable && notBlocked {
				return // Accessible key already exists
			}
			// Key exists but is blocked or unreachable - remove it and place a new one
			g.removeItemAt(item.X, item.Y)
			break
		}
	}

	// Check if stairs are reachable without a key
	stairsReachable := false
	for y := 0; y < g.Dungeon.Height; y++ {
		for x := 0; x < g.Dungeon.Width; x++ {
			if g.Dungeon.Tiles[y][x] == TileStairsDown {
				if reachable[[2]int{x, y}] {
					stairsReachable = true
				}
			}
		}
	}

	if stairsReachable {
		return // Stairs are reachable without key, no need to place one
	}

	// Place a key in a reachable room (not the spawn room for more interest)
	keyItem := Item{ID: "rusty_key", Name: "Rusty Key", Type: ItemKey, Value: 0, Effect: 0, Description: "Opens locked doors"}

	// Find a good location - prefer treasure rooms, then any room
	var keyX, keyY int
	placed := false

	// Helper to find an unoccupied spot in a room
	findUnoccupiedSpot := func(room *Room) (int, int, bool) {
		// Try room center first
		cx := room.X + room.Width/2
		cy := room.Y + room.Height/2
		if reachable[[2]int{cx, cy}] && g.getNPCAt(cx, cy) == nil && g.getEnemyAt(cx, cy) == nil {
			return cx, cy, true
		}
		// Try other positions in the room
		for dy := 1; dy < room.Height-1; dy++ {
			for dx := 1; dx < room.Width-1; dx++ {
				x := room.X + dx
				y := room.Y + dy
				if reachable[[2]int{x, y}] && g.getNPCAt(x, y) == nil && g.getEnemyAt(x, y) == nil {
					return x, y, true
				}
			}
		}
		return 0, 0, false
	}

	// First try treasure rooms
	for _, room := range g.Dungeon.Rooms {
		if room.Type == RoomTreasure {
			if x, y, found := findUnoccupiedSpot(room); found {
				keyX, keyY = x, y
				placed = true
				break
			}
		}
	}

	// If no treasure room works, try any reachable room (skip spawn room)
	if !placed {
		for i, room := range g.Dungeon.Rooms {
			if i == 0 {
				continue // Skip spawn room
			}
			if x, y, found := findUnoccupiedSpot(room); found {
				keyX, keyY = x, y
				placed = true
				break
			}
		}
	}

	// Last resort: place in spawn room
	if !placed {
		if len(g.Dungeon.Rooms) > 0 {
			room := g.Dungeon.Rooms[0]
			if x, y, found := findUnoccupiedSpot(room); found {
				keyX, keyY = x, y
				placed = true
			} else {
				// Truly last resort - place anywhere reachable
				keyX = room.X + 1 + g.rng.Intn(room.Width-2)
				keyY = room.Y + 1 + g.rng.Intn(room.Height-2)
				placed = true
			}
		}
	}

	if placed {
		// Always try to give key to an NPC instead of placing on ground
		// Only non-healer NPCs can hold keys (healers are too helpful to make mandatory)
		var keyHolderNPC *NPC
		// Find a suitable NPC in a reachable location
		for _, npc := range g.NPCs {
			// Skip healers - they're too important for survival
			if npc.Type == NPCHealer {
				continue
			}
			// NPC must be reachable
			if reachable[[2]int{npc.X, npc.Y}] {
				keyHolderNPC = npc
				break
			}
		}

		// If no suitable NPC exists in reachable area, spawn a key keeper
		if keyHolderNPC == nil {
			// Find a reachable spot in any room (prefer non-spawn room)
			var keyKeeperX, keyKeeperY int
			foundSpot := false

			// First try non-spawn rooms
			for i, room := range g.Dungeon.Rooms {
				if i == 0 {
					continue // Skip spawn room first
				}
				// Check if room center is reachable
				cx := room.X + room.Width/2
				cy := room.Y + room.Height/2
				if reachable[[2]int{cx, cy}] && g.getNPCAt(cx, cy) == nil && g.getEnemyAt(cx, cy) == nil {
					keyKeeperX, keyKeeperY = cx, cy
					foundSpot = true
					break
				}
			}

			// Fallback to spawn room if needed
			if !foundSpot && len(g.Dungeon.Rooms) > 0 {
				room := g.Dungeon.Rooms[0]
				// Try various positions in spawn room
				for dy := 1; dy < room.Height-1 && !foundSpot; dy++ {
					for dx := 1; dx < room.Width-1 && !foundSpot; dx++ {
						x := room.X + dx
						y := room.Y + dy
						// Skip player spawn position
						if x == g.Dungeon.SpawnX && y == g.Dungeon.SpawnY {
							continue
						}
						if reachable[[2]int{x, y}] && g.getNPCAt(x, y) == nil && g.getEnemyAt(x, y) == nil {
							keyKeeperX, keyKeeperY = x, y
							foundSpot = true
						}
					}
				}
			}

			if foundSpot {
				keyKeeperID := fmt.Sprintf("key_keeper_%d", g.rng.Int())
				keyKeeper := GenerateNPC(g.rng, NPCWanderer, keyKeeperID, keyKeeperX, keyKeeperY)
				keyKeeper.Name = "Key Keeper"
				g.NPCs = append(g.NPCs, keyKeeper)
				keyHolderNPC = keyKeeper
			}
		}

		if keyHolderNPC != nil {
			// Give key to NPC
			keyHolderNPC.HasKey = true
		} else {
			// Place key on ground as fallback
			g.Items = append(g.Items, &GroundItem{
				Item: keyItem,
				X:    keyX,
				Y:    keyY,
			})
		}
	}
}

// findReachableFromSpawn returns all positions reachable from spawn without using locked doors
func (g *Game) findReachableFromSpawn() map[[2]int]bool {
	reachable := make(map[[2]int]bool)
	queue := [][2]int{{g.Dungeon.SpawnX, g.Dungeon.SpawnY}}
	reachable[[2]int{g.Dungeon.SpawnX, g.Dungeon.SpawnY}] = true

	dirs := [][2]int{{0, 1}, {0, -1}, {1, 0}, {-1, 0}}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, d := range dirs {
			nx, ny := current[0]+d[0], current[1]+d[1]
			pos := [2]int{nx, ny}

			if reachable[pos] {
				continue
			}
			if nx < 0 || ny < 0 || nx >= g.Dungeon.Width || ny >= g.Dungeon.Height {
				continue
			}

			tile := g.Dungeon.Tiles[ny][nx]
			// Can walk on floor, door, stairs, but NOT locked door, wall, void
			if tile == TileFloor || tile == TileDoor || tile == TileStairsDown ||
				tile == TileStairsUp || tile == TileChest || tile == TileAltar {
				reachable[pos] = true
				queue = append(queue, pos)
			}
		}
	}

	return reachable
}

func (g *Game) buildPetriNet() {
	builder := petri.Build().
		Place("health", float64(g.Player.Health)).
		Place("max_health", float64(g.Player.MaxHealth)).
		Place("mana", float64(g.Player.Mana)).
		Place("gold", float64(g.Player.Gold)).
		Place("xp", float64(g.Player.XP)).
		Place("level", float64(g.Player.Level)).
		Place("enemies_killed", 0).
		Place("turns", 0).
		Transition("take_damage").
		Transition("heal").
		Transition("gain_gold").
		Transition("gain_xp").
		Transition("kill_enemy").
		Transition("advance_turn")

	g.net = builder.Done()
}

// ProcessAction handles a player action
func (g *Game) ProcessAction(action ActionType) error {
	if g.GameOver {
		return fmt.Errorf("game is over")
	}

	if g.InDialogue {
		return fmt.Errorf("in dialogue - use dialogue actions")
	}

	g.Message = ""
	g.Turn++

	switch action {
	case ActionMoveUp:
		g.tryMove(0, -1)
	case ActionMoveDown:
		g.tryMove(0, 1)
	case ActionMoveLeft:
		g.tryMove(-1, 0)
	case ActionMoveRight:
		g.tryMove(1, 0)
	case ActionInteract:
		g.interact()
	case ActionAttack:
		g.attack()
	case ActionTalk:
		g.talk()
	case ActionWait:
		g.addMessage("You wait...")
	case ActionDescend:
		g.tryDescend()
	case ActionAscend:
		g.tryAscend()
	}

	// Enemy turns
	g.updateEnemies()

	// Check death
	if g.Player.Health <= 0 {
		g.GameOver = true
		g.addMessage("You have died in the Catacombs...")
	}

	return nil
}

func (g *Game) tryMove(dx, dy int) {
	// Always update facing direction when trying to move
	g.Player.FacingX = dx
	g.Player.FacingY = dy

	newX := g.Player.X + dx
	newY := g.Player.Y + dy

	// Bounds check
	if newX < 0 || newX >= g.Dungeon.Width || newY < 0 || newY >= g.Dungeon.Height {
		g.addMessage("You can't go that way.")
		return
	}

	tile := g.Dungeon.Tiles[newY][newX]

	switch tile {
	case TileWall, TileVoid:
		g.addMessage("You can't walk through walls.")
		return
	case TileLockedDoor:
		if g.Player.Keys["rusty_key"] {
			g.Dungeon.Tiles[newY][newX] = TileDoor
			g.addMessage("You unlock the door with your key.")
		} else {
			g.addMessage("The door is locked. You need a key.")
			return
		}
	case TileLava:
		g.Player.Health -= 20
		g.addMessage("The lava burns! (-20 HP)")
	case TileWater:
		g.addMessage("You wade through the water.")
	}

	// Check for enemy collision
	if enemy := g.getEnemyAt(newX, newY); enemy != nil && enemy.State != StateDead {
		g.addMessage(fmt.Sprintf("A %s blocks your path!", enemy.Name))
		return
	}

	// Move
	g.Player.X = newX
	g.Player.Y = newY

	// Pick up items
	g.pickupItems()
}

func (g *Game) pickupItems() {
	remaining := make([]*GroundItem, 0)
	for _, item := range g.Items {
		if item.X == g.Player.X && item.Y == g.Player.Y {
			if item.Item.Type == ItemGold {
				g.Player.Gold += item.Item.Effect
				g.addMessage(fmt.Sprintf("You pick up %d gold.", item.Item.Effect))
			} else if item.Item.Type == ItemKey {
				g.Player.Keys[item.Item.ID] = true
				g.addMessage(fmt.Sprintf("You pick up %s.", item.Item.Name))
				// Clear AI door avoidance since we now have a key
				if g.AI.Enabled {
					g.AI.AvoidDoors = nil
				}
			} else {
				g.Player.Inventory = append(g.Player.Inventory, item.Item)
				g.addMessage(fmt.Sprintf("You pick up %s.", item.Item.Name))
			}
		} else {
			remaining = append(remaining, item)
		}
	}
	g.Items = remaining
}

func (g *Game) interact() {
	// Check adjacent tiles for interactable objects
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			x, y := g.Player.X+dx, g.Player.Y+dy
			if x < 0 || x >= g.Dungeon.Width || y < 0 || y >= g.Dungeon.Height {
				continue
			}

			tile := g.Dungeon.Tiles[y][x]
			switch tile {
			case TileDoor:
				g.Dungeon.Tiles[y][x] = TileFloor
				g.addMessage("You open the door.")
				return
			case TileChest:
				g.openChest(x, y)
				return
			case TileAltar:
				g.useAltar(x, y)
				return
			}
		}
	}
	g.addMessage("Nothing to interact with.")
}

func (g *Game) openChest(x, y int) {
	// Random loot
	gold := 10 + g.rng.Intn(40)
	g.Player.Gold += gold
	g.Dungeon.Tiles[y][x] = TileFloor
	g.addMessage(fmt.Sprintf("You open the chest and find %d gold!", gold))

	// Chance for item
	if g.rng.Float64() < 0.5 {
		item := Item{ID: "health_potion", Name: "Health Potion", Type: ItemPotion, Value: 25, Effect: 30}
		g.Player.Inventory = append(g.Player.Inventory, item)
		g.addMessage("You also find a Health Potion!")
	}
}

// dropEnemyLoot spawns loot at the enemy's location when killed
func (g *Game) dropEnemyLoot(enemy *Enemy) {
	// Base drop chance scales with enemy XP value
	dropChance := 0.25 + float64(enemy.XP)*0.01
	if dropChance > 0.6 {
		dropChance = 0.6
	}

	if g.rng.Float64() < dropChance {
		// Decide what to drop
		roll := g.rng.Float64()
		if roll < 0.5 {
			// Drop gold (50% of drops)
			gold := 5 + g.rng.Intn(enemy.XP+5)
			g.Items = append(g.Items, &GroundItem{
				Item: Item{ID: "gold_pile", Name: "Gold Coins", Type: ItemGold, Value: gold, Effect: gold},
				X:    enemy.X,
				Y:    enemy.Y,
			})
		} else {
			// Drop health potion (50% of drops)
			g.Items = append(g.Items, &GroundItem{
				Item: Item{ID: "health_potion", Name: "Health Potion", Type: ItemPotion, Value: 25, Effect: 30},
				X:    enemy.X,
				Y:    enemy.Y,
			})
		}
	}
}

func (g *Game) useAltar(x, y int) {
	// Heal at altar
	heal := 30
	g.Player.Health += heal
	if g.Player.Health > g.Player.MaxHealth {
		g.Player.Health = g.Player.MaxHealth
	}
	g.Player.Mana = g.Player.MaxMana
	g.addMessage("The altar's light heals your wounds and restores your spirit.")
}

func (g *Game) attack() {
	// Find adjacent enemy
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			x, y := g.Player.X+dx, g.Player.Y+dy
			if enemy := g.getEnemyAt(x, y); enemy != nil && enemy.State != StateDead {
				g.attackEnemy(enemy)
				return
			}
		}
	}
	g.addMessage("Nothing to attack.")
}

func (g *Game) attackEnemy(enemy *Enemy) {
	// Calculate damage
	damage := g.Player.Attack + g.rng.Intn(5) - 2
	if damage < 1 {
		damage = 1
	}

	enemy.Health -= damage
	enemy.State = StateChasing
	g.addMessage(fmt.Sprintf("You hit the %s for %d damage!", enemy.Name, damage))

	if enemy.Health <= 0 {
		enemy.State = StateDead
		g.Player.XP += enemy.XP
		g.addMessage(fmt.Sprintf("You defeated the %s! (+%d XP)", enemy.Name, enemy.XP))

		// Clear stuck flag for this enemy now that they're dead
		delete(g.AI.GoalsComplete, "stuck_enemy_"+enemy.ID)

		// Check quest progress
		for _, questID := range g.Player.ActiveQuests {
			quest := g.Quests[questID]
			if quest != nil && quest.Objective == "kill" && quest.Target == enemy.Name {
				quest.Progress++
				if quest.Progress >= quest.Required {
					quest.Status = QuestComplete
					g.addMessage(fmt.Sprintf("Quest '%s' completed! Return to the quest giver.", quest.Name))
				}
			}
		}

		// Check level up
		g.checkLevelUp()
	}
}

func (g *Game) checkLevelUp() {
	xpNeeded := g.Player.Level * 100
	if g.Player.XP >= xpNeeded {
		g.Player.Level++
		g.Player.XP -= xpNeeded
		g.Player.MaxHealth += 10
		g.Player.Health = g.Player.MaxHealth
		g.Player.Attack += 2
		g.Player.Defense += 1
		g.addMessage(fmt.Sprintf("LEVEL UP! You are now level %d!", g.Player.Level))
	}
}

// ============================================================================
// COMBAT SYSTEM (Fallout 2 style)
// ============================================================================

// InitiateCombat starts turn-based combat with nearby enemies
func (g *Game) InitiateCombat() {
	if g.Combat.Active {
		return
	}

	// Find all enemies within alert distance
	combatants := make([]string, 0)
	for _, enemy := range g.Enemies {
		if enemy.State == StateDead {
			continue
		}
		dx := g.Player.X - enemy.X
		dy := g.Player.Y - enemy.Y
		dist := math.Sqrt(float64(dx*dx + dy*dy))
		if dist <= float64(enemy.AlertDist) {
			combatants = append(combatants, enemy.ID)
			enemy.State = StateChasing
		}
	}

	if len(combatants) == 0 {
		g.addMessage("No enemies nearby to fight.")
		return
	}

	// Calculate max AP based on agility
	maxAP := BaseAP + g.Player.Agility

	g.Combat = CombatState{
		Active:        true,
		PlayerTurn:    true,
		CurrentAP:     maxAP,
		MaxAP:         maxAP,
		TargetPart:    BodyTorso,
		TurnOrder:     append([]string{"player"}, combatants...),
		TurnIndex:     0,
		Combatants:    combatants,
		RoundNumber:   1,
		CombatLog:     make([]string, 0),
	}

	// Select first visible enemy
	if len(combatants) > 0 {
		g.Combat.SelectedEnemy = combatants[0]
	}

	g.addCombatLog("=== COMBAT INITIATED ===")
	g.addCombatLog(fmt.Sprintf("Round %d - Your turn. AP: %d", g.Combat.RoundNumber, g.Combat.CurrentAP))
	g.addMessage("Combat begins! You have initiative.")
}

// EndCombat finishes turn-based combat
func (g *Game) EndCombat() {
	g.Combat = CombatState{}
	g.addMessage("Combat ended.")
	// Reset flee tracking for next combat
	g.AI.FleeAttempts = 0
	g.AI.CommitToFight = false
}

// ProcessCombatAction handles combat-specific actions
func (g *Game) ProcessCombatAction(action ActionType, params map[string]interface{}) error {
	if !g.Combat.Active {
		return fmt.Errorf("not in combat")
	}

	if !g.Combat.PlayerTurn {
		return fmt.Errorf("not your turn")
	}

	switch action {
	case ActionAttack:
		return g.combatAttack(false)
	case ActionAimedShot:
		return g.combatAttack(true)
	case ActionCombatMove:
		dx, dy := 0, 0
		if d, ok := params["dx"].(float64); ok {
			dx = int(d)
		}
		if d, ok := params["dy"].(float64); ok {
			dy = int(d)
		}
		return g.combatMove(dx, dy)
	case ActionEndTurn:
		return g.endPlayerTurn()
	case ActionFlee:
		return g.attemptFlee()
	case ActionUseItem:
		idx := 0
		if i, ok := params["index"].(float64); ok {
			idx = int(i)
		}
		return g.combatUseItem(idx)
	default:
		return fmt.Errorf("unknown combat action")
	}
}

// SetTargetPart changes the aimed body part
func (g *Game) SetTargetPart(part BodyPart) {
	if part >= 0 && part <= BodyGroin {
		g.Combat.TargetPart = part
	}
}

// SetTargetEnemy changes the selected enemy
func (g *Game) SetTargetEnemy(enemyID string) {
	for _, id := range g.Combat.Combatants {
		if id == enemyID {
			g.Combat.SelectedEnemy = enemyID
			return
		}
	}
}

// combatAttack performs an attack in combat
func (g *Game) combatAttack(aimed bool) error {
	apCost := APCostAttack
	if aimed {
		apCost = APCostAimedShot
	}

	if g.Combat.CurrentAP < apCost {
		return fmt.Errorf("not enough AP (need %d, have %d)", apCost, g.Combat.CurrentAP)
	}

	enemy := g.getEnemyByID(g.Combat.SelectedEnemy)
	if enemy == nil || enemy.State == StateDead {
		return fmt.Errorf("no valid target")
	}

	// Calculate distance
	dx := g.Player.X - enemy.X
	dy := g.Player.Y - enemy.Y
	dist := math.Sqrt(float64(dx*dx + dy*dy))

	// Check range (melee = 1.5, later can add ranged weapons)
	if dist > 1.5 {
		return fmt.Errorf("target is out of range (distance: %.1f)", dist)
	}

	g.Combat.CurrentAP -= apCost

	// Calculate hit chance
	targetPart := BodyTorso
	if aimed {
		targetPart = g.Combat.TargetPart
	}
	result := g.calculateAttack(enemy, targetPart, dist)

	// Apply result
	if result.Hit {
		enemy.Health -= result.Damage
		if result.Critical {
			g.addCombatLog(fmt.Sprintf("CRITICAL HIT on %s's %s! %d damage! %s",
				enemy.Name, BodyPartInfo[targetPart].Name, result.Damage, result.CritEffect))
		} else {
			g.addCombatLog(fmt.Sprintf("Hit %s's %s for %d damage.",
				enemy.Name, BodyPartInfo[targetPart].Name, result.Damage))
		}

		if enemy.Health <= 0 {
			enemy.State = StateDead
			g.Player.XP += enemy.XP
			g.addCombatLog(fmt.Sprintf("%s is killed! (+%d XP)", enemy.Name, enemy.XP))
			// Clear stuck flag for this enemy now that they're dead
			delete(g.AI.GoalsComplete, "stuck_enemy_"+enemy.ID)
			g.dropEnemyLoot(enemy)
			g.removeCombatant(enemy.ID)
			g.checkLevelUp()
			g.checkCombatEnd()
		}
	} else {
		g.addCombatLog(fmt.Sprintf("Missed %s!", enemy.Name))
	}

	return nil
}

// calculateAttack computes hit/damage/crit for an attack
func (g *Game) calculateAttack(enemy *Enemy, targetPart BodyPart, distance float64) CombatResult {
	partInfo := BodyPartInfo[targetPart]

	// Base hit chance
	hitChance := g.Player.Accuracy

	// Apply perception bonus
	hitChance += g.Player.Perception * 2

	// Apply body part penalty
	hitChance -= partInfo.HitPenalty

	// Distance penalty (melee has small penalty at range 1)
	hitChance -= int(distance * 2)

	// Crippled arm penalty
	if g.Player.CrippledParts[BodyRightArm] {
		hitChance -= 20
	}

	// Clamp hit chance
	if hitChance < 5 {
		hitChance = 5 // Minimum 5% chance
	}
	if hitChance > 95 {
		hitChance = 95 // Maximum 95%
	}

	// Roll to hit
	roll := g.rng.Intn(100)
	hit := roll < hitChance

	if !hit {
		return CombatResult{Miss: true, Message: "Miss!"}
	}

	// Calculate damage
	baseDamage := g.Player.Attack + g.Player.Strength
	damage := baseDamage + g.rng.Intn(5) - 2
	damage = int(float64(damage) * partInfo.DamageMult)

	// Apply enemy defense
	damage -= enemy.Damage / 5 // Enemies use damage as proxy for toughness
	if damage < 1 {
		damage = 1
	}

	// Check for critical hit
	critChance := g.Player.CritChance + g.Player.Luck
	critChance = int(float64(critChance) * partInfo.CritMult)
	critRoll := g.rng.Intn(100)
	isCrit := critRoll < critChance

	var critEffect string
	if isCrit {
		damage = int(float64(damage) * 2.0)

		// Apply crippling effects for aimed shots
		if partInfo.CanCripple && g.rng.Intn(100) < 50 {
			enemy.CrippledParts[targetPart] = true
			switch targetPart {
			case BodyHead:
				critEffect = "Knocked unconscious!"
				enemy.AP = 0
			case BodyLeftArm, BodyRightArm:
				critEffect = "Arm crippled! Reduced accuracy."
				enemy.Accuracy -= 20
			case BodyLeftLeg, BodyRightLeg:
				critEffect = "Leg crippled! Reduced movement."
				enemy.MaxAP -= 2
			case BodyEyes:
				critEffect = "Blinded! Severely reduced accuracy."
				enemy.Accuracy -= 40
			case BodyGroin:
				critEffect = "Critical groin hit! Stunned."
				enemy.AP = 0
			}
		}
	}

	return CombatResult{
		Hit:        true,
		Damage:     damage,
		Critical:   isCrit,
		CritEffect: critEffect,
	}
}

// combatMove moves the player during combat
func (g *Game) combatMove(dx, dy int) error {
	if g.Combat.CurrentAP < APCostMove {
		return fmt.Errorf("not enough AP to move")
	}

	newX := g.Player.X + dx
	newY := g.Player.Y + dy

	// Check if can move there
	if !g.canMoveTo(newX, newY) {
		return fmt.Errorf("can't move there")
	}

	if g.getEnemyAt(newX, newY) != nil {
		return fmt.Errorf("space occupied by enemy")
	}

	g.Player.X = newX
	g.Player.Y = newY
	g.Combat.CurrentAP -= APCostMove

	g.addCombatLog(fmt.Sprintf("Moved. AP: %d remaining.", g.Combat.CurrentAP))

	return nil
}

// combatUseItem uses an item during combat
func (g *Game) combatUseItem(idx int) error {
	if g.Combat.CurrentAP < APCostUseItem {
		return fmt.Errorf("not enough AP to use item")
	}

	if idx < 0 || idx >= len(g.Player.Inventory) {
		return fmt.Errorf("invalid item index")
	}

	item := g.Player.Inventory[idx]

	switch item.Type {
	case ItemPotion:
		g.Player.Health += item.Effect
		if g.Player.Health > g.Player.MaxHealth {
			g.Player.Health = g.Player.MaxHealth
		}
		g.addCombatLog(fmt.Sprintf("Used %s. Healed %d HP.", item.Name, item.Effect))
		g.Player.Inventory = append(g.Player.Inventory[:idx], g.Player.Inventory[idx+1:]...)
		g.Combat.CurrentAP -= APCostUseItem
	default:
		return fmt.Errorf("can't use that item in combat")
	}

	return nil
}

// endPlayerTurn ends the player's combat turn
func (g *Game) endPlayerTurn() error {
	g.addCombatLog("You end your turn.")
	g.Combat.PlayerTurn = false

	// Process enemy turns
	g.processEnemyTurns()

	// Start new round
	g.Combat.RoundNumber++
	g.Combat.TurnIndex = 0
	g.Combat.PlayerTurn = true
	g.Combat.CurrentAP = g.Combat.MaxAP

	// Restore enemy AP
	for _, enemyID := range g.Combat.Combatants {
		if enemy := g.getEnemyByID(enemyID); enemy != nil {
			enemy.AP = enemy.MaxAP
		}
	}

	g.addCombatLog(fmt.Sprintf("=== Round %d - Your turn. AP: %d ===", g.Combat.RoundNumber, g.Combat.CurrentAP))

	return nil
}

// processEnemyTurns handles all enemy actions
func (g *Game) processEnemyTurns() {
	for _, enemyID := range g.Combat.Combatants {
		enemy := g.getEnemyByID(enemyID)
		if enemy == nil || enemy.State == StateDead {
			continue
		}

		g.addCombatLog(fmt.Sprintf("--- %s's turn (AP: %d) ---", enemy.Name, enemy.AP))

		// Simple AI: move towards player and attack if possible
		for enemy.AP > 0 {
			dx := g.Player.X - enemy.X
			dy := g.Player.Y - enemy.Y
			dist := math.Sqrt(float64(dx*dx + dy*dy))

			// If adjacent, attack
			if dist <= 1.5 && enemy.AP >= APCostAttack {
				result := g.calculateEnemyAttack(enemy)
				enemy.AP -= APCostAttack

				if result.Hit {
					g.Player.Health -= result.Damage
					if result.Critical {
						g.addCombatLog(fmt.Sprintf("%s lands a CRITICAL HIT for %d damage! %s",
							enemy.Name, result.Damage, result.CritEffect))
					} else {
						g.addCombatLog(fmt.Sprintf("%s hits you for %d damage.", enemy.Name, result.Damage))
					}

					if g.Player.Health <= 0 {
						g.GameOver = true
						g.addCombatLog("You have been slain!")
						return
					}
				} else {
					g.addCombatLog(fmt.Sprintf("%s misses.", enemy.Name))
				}
			} else if enemy.AP >= APCostMove {
				// Move towards player
				moveX, moveY := 0, 0
				if dx > 0 {
					moveX = 1
				} else if dx < 0 {
					moveX = -1
				}
				if dy > 0 {
					moveY = 1
				} else if dy < 0 {
					moveY = -1
				}

				newX := enemy.X + moveX
				newY := enemy.Y + moveY

				if g.canMoveTo(newX, newY) && g.getEnemyAt(newX, newY) == nil &&
					!(newX == g.Player.X && newY == g.Player.Y) {
					enemy.X = newX
					enemy.Y = newY
					enemy.AP -= APCostMove
				} else {
					// Can't move, end turn
					break
				}
			} else {
				break
			}
		}
	}
}

// calculateEnemyAttack computes enemy attack result
func (g *Game) calculateEnemyAttack(enemy *Enemy) CombatResult {
	hitChance := enemy.Accuracy

	// Player agility gives dodge bonus
	hitChance -= g.Player.Agility * 2

	// Crippled parts affect enemy accuracy
	if enemy.CrippledParts[BodyEyes] {
		hitChance -= 40
	}
	if enemy.CrippledParts[BodyRightArm] || enemy.CrippledParts[BodyLeftArm] {
		hitChance -= 20
	}

	if hitChance < 5 {
		hitChance = 5
	}
	if hitChance > 95 {
		hitChance = 95
	}

	roll := g.rng.Intn(100)
	if roll >= hitChance {
		return CombatResult{Miss: true}
	}

	damage := enemy.Damage - g.Player.Defense + g.rng.Intn(5) - 2
	if damage < 1 {
		damage = 1
	}

	// 10% crit chance for enemies
	isCrit := g.rng.Intn(100) < 10
	var critEffect string
	if isCrit {
		damage *= 2
		// Random crippling effect on player
		if g.rng.Intn(100) < 30 {
			parts := []BodyPart{BodyLeftArm, BodyRightArm, BodyLeftLeg, BodyRightLeg}
			part := parts[g.rng.Intn(len(parts))]
			if !g.Player.CrippledParts[part] {
				g.Player.CrippledParts[part] = true
				critEffect = fmt.Sprintf("Your %s is crippled!", BodyPartInfo[part].Name)
			}
		}
	}

	return CombatResult{
		Hit:        true,
		Damage:     damage,
		Critical:   isCrit,
		CritEffect: critEffect,
	}
}

// attemptFlee tries to escape combat. Returns error only if flee is impossible (not enough AP).
// Use FleeSucceeded() to check if the flee actually worked.
func (g *Game) attemptFlee() error {
	if g.Combat.CurrentAP < APCostMove*2 {
		return fmt.Errorf("not enough AP to flee")
	}

	// 50% base chance + 5% per agility
	fleeChance := 50 + g.Player.Agility*5

	// Crippled legs reduce flee chance
	if g.Player.CrippledParts[BodyLeftLeg] || g.Player.CrippledParts[BodyRightLeg] {
		fleeChance -= 25
	}

	roll := g.rng.Intn(100)
	if roll < fleeChance {
		g.addCombatLog("You successfully flee from combat!")
		// Remember where we fled from so we can move away
		g.AI.FleeFrom = [2]int{g.Player.X, g.Player.Y}
		g.AI.FleeTicks = 5 // Run away for 5 ticks
		g.EndCombat()
		return nil
	}

	g.Combat.CurrentAP -= APCostMove * 2
	g.AI.FleeAttempts++
	g.addCombatLog("Failed to flee! You stumble and lose AP.")

	// After 3 failed flee attempts, AI commits to fighting
	if g.AI.FleeAttempts >= 3 {
		g.AI.CommitToFight = true
		g.addCombatLog("No escape! You must fight!")
	}

	return nil
}

// removeCombatant removes an enemy from combat
func (g *Game) removeCombatant(enemyID string) {
	newCombatants := make([]string, 0)
	for _, id := range g.Combat.Combatants {
		if id != enemyID {
			newCombatants = append(newCombatants, id)
		}
	}
	g.Combat.Combatants = newCombatants

	// Update selected enemy if needed
	if g.Combat.SelectedEnemy == enemyID {
		if len(newCombatants) > 0 {
			g.Combat.SelectedEnemy = newCombatants[0]
		} else {
			g.Combat.SelectedEnemy = ""
		}
	}
}

// checkCombatEnd ends combat if no enemies remain
func (g *Game) checkCombatEnd() {
	if len(g.Combat.Combatants) == 0 {
		g.addCombatLog("=== VICTORY! All enemies defeated! ===")
		g.EndCombat()
	}
}

// getEnemyByID finds an enemy by ID
func (g *Game) getEnemyByID(id string) *Enemy {
	for _, e := range g.Enemies {
		if e.ID == id {
			return e
		}
	}
	return nil
}

// CalculateHitChance returns the hit chance for the current target/part
func (g *Game) CalculateHitChance() int {
	if !g.Combat.Active || g.Combat.SelectedEnemy == "" {
		return 0
	}

	enemy := g.getEnemyByID(g.Combat.SelectedEnemy)
	if enemy == nil || enemy.State == StateDead {
		return 0
	}

	dx := g.Player.X - enemy.X
	dy := g.Player.Y - enemy.Y
	dist := math.Sqrt(float64(dx*dx + dy*dy))

	partInfo := BodyPartInfo[g.Combat.TargetPart]
	hitChance := g.Player.Accuracy + g.Player.Perception*2
	hitChance -= partInfo.HitPenalty
	hitChance -= int(dist * 2)

	if g.Player.CrippledParts[BodyRightArm] {
		hitChance -= 20
	}

	if hitChance < 5 {
		hitChance = 5
	}
	if hitChance > 95 {
		hitChance = 95
	}

	return hitChance
}

// addCombatLog adds a message to the combat log
func (g *Game) addCombatLog(msg string) {
	g.Combat.CombatLog = append(g.Combat.CombatLog, msg)
	if len(g.Combat.CombatLog) > 50 {
		g.Combat.CombatLog = g.Combat.CombatLog[1:]
	}
	g.addMessage(msg)
}

// ============================================================================
// END COMBAT SYSTEM
// ============================================================================

func (g *Game) talk() {
	// Find adjacent NPC
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			x, y := g.Player.X+dx, g.Player.Y+dy
			if npc := g.getNPCAt(x, y); npc != nil {
				g.startDialogue(npc)
				return
			}
		}
	}
	g.addMessage("No one to talk to.")
}

func (g *Game) startDialogue(npc *NPC) {
	g.InDialogue = true
	g.DialogueNPC = npc.ID
	// If NPC has a key, start with the key dialogue instead of normal dialogue
	if npc.HasKey {
		g.DialogueNode = "has_key"
	} else {
		g.DialogueNode = "start"
	}
	npc.Met = true
	g.addMessage(fmt.Sprintf("You approach %s.", npc.Name))
}

// ProcessDialogueChoice handles dialogue selection
func (g *Game) ProcessDialogueChoice(choiceIdx int) error {
	if !g.InDialogue {
		return fmt.Errorf("not in dialogue")
	}

	npc := g.getNPCByID(g.DialogueNPC)
	if npc == nil {
		g.InDialogue = false
		return fmt.Errorf("NPC not found")
	}

	dialogue := GetDialogue(npc)
	var currentNode *DialogueNode
	for i := range dialogue {
		if dialogue[i].ID == g.DialogueNode {
			currentNode = &dialogue[i]
			break
		}
	}

	if currentNode == nil {
		g.InDialogue = false
		return fmt.Errorf("dialogue node not found")
	}

	// Check for auto-continue
	if currentNode.NextID != "" && len(currentNode.Choices) == 0 {
		g.DialogueNode = currentNode.NextID
		return nil
	}

	// Process choice
	if choiceIdx < 0 || choiceIdx >= len(currentNode.Choices) {
		return fmt.Errorf("invalid choice")
	}

	choice := currentNode.Choices[choiceIdx]

	// Check condition
	if choice.Condition != nil {
		if choice.Condition.RequireGold > g.Player.Gold {
			return fmt.Errorf("not enough gold")
		}
	}

	// Apply effect
	if choice.Effect != nil {
		if choice.Effect.AddGold > 0 {
			g.Player.Gold += choice.Effect.AddGold
		}
		if choice.Effect.RemoveGold > 0 {
			g.Player.Gold -= choice.Effect.RemoveGold
		}
		if choice.Effect.Heal > 0 {
			g.Player.Health += choice.Effect.Heal
			if g.Player.Health > g.Player.MaxHealth {
				g.Player.Health = g.Player.MaxHealth
			}
		}
		if choice.Effect.StartQuest != "" {
			if quest, ok := g.Quests[choice.Effect.StartQuest]; ok {
				quest.Status = QuestActive
				g.Player.ActiveQuests = append(g.Player.ActiveQuests, quest.ID)
				g.addMessage(fmt.Sprintf("Quest started: %s", quest.Name))
			}
		}
	}

	// Navigate to next node
	if choice.NextID == "" || choice.NextID == "end" {
		g.InDialogue = false
		g.DialogueNPC = ""
		g.DialogueNode = ""
	} else {
		g.DialogueNode = choice.NextID
	}

	// Check for special actions
	for i := range dialogue {
		if dialogue[i].ID == g.DialogueNode {
			if dialogue[i].Action == ActionEnd {
				g.InDialogue = false
				g.DialogueNPC = ""
				g.DialogueNode = ""
			} else if dialogue[i].Action == ActionShop {
				g.InShop = true
			} else if dialogue[i].Action == ActionGiveKey {
				// Transfer key from NPC to player
				if npc.HasKey {
					npc.HasKey = false
					g.Player.Keys["rusty_key"] = true
					g.addMessage(fmt.Sprintf("%s hands you a rusty key!", npc.Name))
				}
			}
			break
		}
	}

	return nil
}

// BuyItem purchases an item from the current shop NPC by item ID
func (g *Game) BuyItem(itemID string) error {
	if !g.InShop {
		return fmt.Errorf("not in a shop")
	}

	npc := g.getNPCByID(g.DialogueNPC)
	if npc == nil {
		return fmt.Errorf("no shop NPC found")
	}

	// Find item in NPC inventory
	var itemIdx = -1
	var item Item
	for i, invItem := range npc.Inventory {
		if invItem.ID == itemID {
			itemIdx = i
			item = invItem
			break
		}
	}

	if itemIdx == -1 {
		return fmt.Errorf("item not found in shop")
	}

	// Check if player can afford it
	if g.Player.Gold < item.Value {
		g.addMessage(fmt.Sprintf("You can't afford %s (costs %d gold).", item.Name, item.Value))
		return fmt.Errorf("not enough gold")
	}

	// Complete purchase
	g.Player.Gold -= item.Value
	g.Player.Inventory = append(g.Player.Inventory, item)
	npc.Gold += item.Value

	// Remove from NPC inventory
	npc.Inventory = append(npc.Inventory[:itemIdx], npc.Inventory[itemIdx+1:]...)

	g.addMessage(fmt.Sprintf("You bought %s for %d gold.", item.Name, item.Value))
	return nil
}

// CloseShop exits the shop interface and ends dialogue
func (g *Game) CloseShop() {
	g.InShop = false
	g.InDialogue = false
	g.DialogueNPC = ""
	g.DialogueNode = ""
}

func (g *Game) tryDescend() {
	tile := g.Dungeon.Tiles[g.Player.Y][g.Player.X]
	if tile != TileStairsDown {
		g.addMessage("There are no stairs down here.")
		return
	}

	g.Level++
	params := g.Dungeon.Params
	params.Difficulty = g.Level
	params.Seed = g.rng.Int63()
	g.Dungeon = GenerateDungeon(params, g.Level)
	g.Player.X = g.Dungeon.SpawnX
	g.Player.Y = g.Dungeon.SpawnY

	// Repopulate
	g.Enemies = make([]*Enemy, 0)
	g.NPCs = make([]*NPC, 0)
	g.Items = make([]*GroundItem, 0)
	g.populateEnemies(params)
	g.populateNPCs(params)
	g.populateItems(params)

	g.addMessage(fmt.Sprintf("You descend to level %d of the Catacombs...", g.Level))

	// Victory check (skipped in infinite mode)
	if !g.InfiniteMode && g.Level >= 10 {
		g.Victory = true
		g.GameOver = true
		g.addMessage("You have reached the bottom of the Catacombs and found the ancient treasure! VICTORY!")
	}
}

func (g *Game) tryAscend() {
	tile := g.Dungeon.Tiles[g.Player.Y][g.Player.X]
	if tile != TileStairsUp {
		g.addMessage("There are no stairs up here.")
		return
	}

	if g.Level == 1 {
		g.addMessage("You escape the Catacombs... but without the treasure.")
		g.GameOver = true
	} else {
		g.Level--
		g.addMessage("You ascend... (level generation on ascent not implemented)")
	}
}

func (g *Game) updateEnemies() {
	for _, enemy := range g.Enemies {
		if enemy.State == StateDead {
			continue
		}

		// Calculate distance to player
		dx := g.Player.X - enemy.X
		dy := g.Player.Y - enemy.Y
		dist := math.Sqrt(float64(dx*dx + dy*dy))

		// State machine
		switch enemy.State {
		case StateIdle:
			if dist <= float64(enemy.AlertDist) {
				enemy.State = StateAlert
			}
		case StateAlert:
			if dist <= 1.5 {
				enemy.State = StateAttacking
			} else if dist <= float64(enemy.AlertDist) {
				enemy.State = StateChasing
			} else {
				enemy.State = StateIdle
			}
		case StateChasing:
			if dist <= 1.5 {
				enemy.State = StateAttacking
			} else {
				g.moveEnemyToward(enemy, g.Player.X, g.Player.Y)
			}
		case StateAttacking:
			if dist > 1.5 {
				enemy.State = StateChasing
			} else {
				g.enemyAttack(enemy)
			}
		}
	}
}

func (g *Game) moveEnemyToward(enemy *Enemy, targetX, targetY int) {
	dx := 0
	dy := 0

	if targetX > enemy.X {
		dx = 1
	} else if targetX < enemy.X {
		dx = -1
	}

	if targetY > enemy.Y {
		dy = 1
	} else if targetY < enemy.Y {
		dy = -1
	}

	// Try to move
	newX := enemy.X + dx
	newY := enemy.Y + dy

	if g.canMoveTo(newX, newY) && g.getEnemyAt(newX, newY) == nil {
		enemy.X = newX
		enemy.Y = newY
	} else if dx != 0 && g.canMoveTo(enemy.X+dx, enemy.Y) && g.getEnemyAt(enemy.X+dx, enemy.Y) == nil {
		enemy.X += dx
	} else if dy != 0 && g.canMoveTo(enemy.X, enemy.Y+dy) && g.getEnemyAt(enemy.X, enemy.Y+dy) == nil {
		enemy.Y += dy
	}
}

func (g *Game) canMoveTo(x, y int) bool {
	if x < 0 || x >= g.Dungeon.Width || y < 0 || y >= g.Dungeon.Height {
		return false
	}
	tile := g.Dungeon.Tiles[y][x]
	if !(tile == TileFloor || tile == TileDoor || tile == TileStairsUp || tile == TileStairsDown || tile == TileWater || tile == TileLava) {
		return false
	}
	// Check for enemy blocking the way
	if enemy := g.getEnemyAt(x, y); enemy != nil && enemy.State != StateDead {
		return false
	}
	return true
}

func (g *Game) enemyAttack(enemy *Enemy) {
	damage := enemy.Damage - g.Player.Defense + g.rng.Intn(5) - 2
	if damage < 1 {
		damage = 1
	}

	g.Player.Health -= damage
	g.addMessage(fmt.Sprintf("The %s attacks you for %d damage!", enemy.Name, damage))
}

func (g *Game) getEnemyAt(x, y int) *Enemy {
	for _, enemy := range g.Enemies {
		if enemy.X == x && enemy.Y == y && enemy.State != StateDead {
			return enemy
		}
	}
	return nil
}

func (g *Game) getNPCAt(x, y int) *NPC {
	for _, npc := range g.NPCs {
		if npc.X == x && npc.Y == y {
			return npc
		}
	}
	return nil
}

func (g *Game) getNPCByID(id string) *NPC {
	for _, npc := range g.NPCs {
		if npc.ID == id {
			return npc
		}
	}
	return nil
}

// removeItemAt removes an item at the given position
func (g *Game) removeItemAt(x, y int) {
	for i, item := range g.Items {
		if item.X == x && item.Y == y {
			g.Items = append(g.Items[:i], g.Items[i+1:]...)
			return
		}
	}
}

func (g *Game) addMessage(msg string) {
	g.Message = msg
	g.MessageLog = append(g.MessageLog, msg)
	if len(g.MessageLog) > 100 {
		g.MessageLog = g.MessageLog[1:]
	}
}

// GetState returns the current game state for the client
func (g *Game) GetState() GameState {
	// Convert tiles
	tiles := make([][]int, g.Dungeon.Height)
	for y := 0; y < g.Dungeon.Height; y++ {
		tiles[y] = make([]int, g.Dungeon.Width)
		for x := 0; x < g.Dungeon.Width; x++ {
			tiles[y][x] = int(g.Dungeon.Tiles[y][x])
		}
	}

	// Convert enemies
	enemies := make([]EnemyView, 0)
	for _, e := range g.Enemies {
		enemies = append(enemies, EnemyView{
			ID:        e.ID,
			Type:      int(e.Type),
			Name:      e.Name,
			X:         e.X,
			Y:         e.Y,
			Health:    e.Health,
			MaxHealth: e.MaxHealth,
			State:     int(e.State),
		})
	}

	// Convert NPCs
	npcs := make([]NPCView, 0)
	for _, n := range g.NPCs {
		npcs = append(npcs, NPCView{
			ID:   n.ID,
			Name: n.Name,
			Type: int(n.Type),
			X:    n.X,
			Y:    n.Y,
			Met:  n.Met,
		})
	}

	// Convert items
	items := make([]ItemView, 0)
	for _, i := range g.Items {
		items = append(items, ItemView{
			ID:   i.Item.ID,
			Name: i.Item.Name,
			X:    i.X,
			Y:    i.Y,
		})
	}

	// FOV calculation (simple)
	visible := g.calculateFOV()

	state := GameState{
		MapWidth:      g.Dungeon.Width,
		MapHeight:     g.Dungeon.Height,
		Tiles:         tiles,
		Player:        g.Player,
		Enemies:       enemies,
		NPCs:          npcs,
		Items:         items,
		Level:         g.Level,
		Turn:          g.Turn,
		Message:       g.Message,
		MessageLog:    g.MessageLog,
		GameOver:      g.GameOver,
		Victory:       g.Victory,
		InDialogue:    g.InDialogue,
		DialogueNPC:   g.DialogueNPC,
		DialogueNode:  g.DialogueNode,
		InShop:        g.InShop,
		VisibleTiles:  visible,
		ExploredTiles: visible, // Simplified - real game would track explored
		Seed:          g.Seed,
		AISeed:        g.AISeed,
		InfiniteMode:  g.InfiniteMode,
	}

	// Add dialogue data if in dialogue
	if g.InDialogue {
		npc := g.getNPCByID(g.DialogueNPC)
		if npc != nil {
			dialogue := GetDialogue(npc)
			for i := range dialogue {
				if dialogue[i].ID == g.DialogueNode {
					state.DialogueData = &DialogueView{
						Speaker: dialogue[i].Speaker,
						Text:    dialogue[i].Text,
						Choices: dialogue[i].Choices,
					}
					break
				}
			}
		}
	}

	// Add shop items if in shop
	if g.InShop && g.DialogueNPC != "" {
		npc := g.getNPCByID(g.DialogueNPC)
		if npc != nil {
			state.ShopItems = npc.Inventory
		}
	}

	// Add combat view if in combat
	if g.Combat.Active {
		state.Combat = g.buildCombatView()
	}

	// Add AI state if enabled
	if g.AI.Enabled {
		state.AI = &g.AI
	}

	return state
}

// buildCombatView creates the client-visible combat state
func (g *Game) buildCombatView() *CombatView {
	view := &CombatView{
		Active:         g.Combat.Active,
		PlayerTurn:     g.Combat.PlayerTurn,
		CurrentAP:      g.Combat.CurrentAP,
		MaxAP:          g.Combat.MaxAP,
		SelectedEnemy:  g.Combat.SelectedEnemy,
		TargetPart:     int(g.Combat.TargetPart),
		TargetPartName: BodyPartInfo[g.Combat.TargetPart].Name,
		HitChance:      g.CalculateHitChance(),
		RoundNumber:    g.Combat.RoundNumber,
		CombatLog:      g.Combat.CombatLog,
		Combatants:     make([]CombatantView, 0),
		AvailableActions: make([]CombatAction, 0),
	}

	// Add player as combatant
	playerCrippled := make([]string, 0)
	for part, crippled := range g.Player.CrippledParts {
		if crippled {
			playerCrippled = append(playerCrippled, BodyPartInfo[part].Name)
		}
	}
	view.Combatants = append(view.Combatants, CombatantView{
		ID:            "player",
		Name:          "You",
		IsPlayer:      true,
		Health:        g.Player.Health,
		MaxHealth:     g.Player.MaxHealth,
		AP:            g.Combat.CurrentAP,
		MaxAP:         g.Combat.MaxAP,
		IsTurn:        g.Combat.PlayerTurn,
		CrippledParts: playerCrippled,
	})

	// Add enemies as combatants
	for _, enemyID := range g.Combat.Combatants {
		enemy := g.getEnemyByID(enemyID)
		if enemy == nil || enemy.State == StateDead {
			continue
		}
		enemyCrippled := make([]string, 0)
		for part, crippled := range enemy.CrippledParts {
			if crippled {
				enemyCrippled = append(enemyCrippled, BodyPartInfo[part].Name)
			}
		}
		view.Combatants = append(view.Combatants, CombatantView{
			ID:            enemy.ID,
			Name:          enemy.Name,
			IsPlayer:      false,
			Health:        enemy.Health,
			MaxHealth:     enemy.MaxHealth,
			AP:            enemy.AP,
			MaxAP:         enemy.MaxAP,
			IsTurn:        !g.Combat.PlayerTurn,
			CrippledParts: enemyCrippled,
		})
	}

	// Build available actions
	if g.Combat.PlayerTurn {
		// Quick attack (torso)
		view.AvailableActions = append(view.AvailableActions, CombatAction{
			ID:        "attack",
			Name:      "Attack",
			APCost:    APCostAttack,
			Enabled:   g.Combat.CurrentAP >= APCostAttack,
			HitChance: g.CalculateHitChance(),
		})

		// Aimed shot
		view.AvailableActions = append(view.AvailableActions, CombatAction{
			ID:        "aimed_shot",
			Name:      fmt.Sprintf("Aimed: %s", BodyPartInfo[g.Combat.TargetPart].Name),
			APCost:    APCostAimedShot,
			Enabled:   g.Combat.CurrentAP >= APCostAimedShot,
			HitChance: g.CalculateHitChance(),
		})

		// Move
		view.AvailableActions = append(view.AvailableActions, CombatAction{
			ID:      "move",
			Name:    "Move",
			APCost:  APCostMove,
			Enabled: g.Combat.CurrentAP >= APCostMove,
		})

		// Use item
		view.AvailableActions = append(view.AvailableActions, CombatAction{
			ID:      "use_item",
			Name:    "Use Item",
			APCost:  APCostUseItem,
			Enabled: g.Combat.CurrentAP >= APCostUseItem && len(g.Player.Inventory) > 0,
		})

		// Flee
		view.AvailableActions = append(view.AvailableActions, CombatAction{
			ID:      "flee",
			Name:    "Flee",
			APCost:  APCostMove * 2,
			Enabled: g.Combat.CurrentAP >= APCostMove*2,
		})

		// End turn
		view.AvailableActions = append(view.AvailableActions, CombatAction{
			ID:      "end_turn",
			Name:    "End Turn",
			APCost:  0,
			Enabled: true,
		})
	}

	return view
}

func (g *Game) calculateFOV() [][]bool {
	visible := make([][]bool, g.Dungeon.Height)
	for y := 0; y < g.Dungeon.Height; y++ {
		visible[y] = make([]bool, g.Dungeon.Width)
	}

	// Simple radius-based FOV
	viewRadius := 8
	for dy := -viewRadius; dy <= viewRadius; dy++ {
		for dx := -viewRadius; dx <= viewRadius; dx++ {
			if dx*dx+dy*dy > viewRadius*viewRadius {
				continue
			}
			x := g.Player.X + dx
			y := g.Player.Y + dy
			if x >= 0 && x < g.Dungeon.Width && y >= 0 && y < g.Dungeon.Height {
				// Simple line-of-sight check
				if g.hasLineOfSight(g.Player.X, g.Player.Y, x, y) {
					visible[y][x] = true
				}
			}
		}
	}

	return visible
}

func (g *Game) hasLineOfSight(x0, y0, x1, y1 int) bool {
	dx := abs(x1 - x0)
	dy := abs(y1 - y0)
	sx := 1
	if x0 > x1 {
		sx = -1
	}
	sy := 1
	if y0 > y1 {
		sy = -1
	}
	err := dx - dy

	for {
		if x0 == x1 && y0 == y1 {
			return true
		}

		tile := g.Dungeon.Tiles[y0][x0]
		if tile == TileWall || tile == TileVoid {
			return false
		}

		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x0 += sx
		}
		if e2 < dx {
			err += dx
			y0 += sy
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// GetAvailableActions returns valid actions for the current state
func (g *Game) GetAvailableActions() []string {
	if g.GameOver {
		return []string{}
	}

	if g.InDialogue {
		return []string{"dialogue_choice"}
	}

	actions := []string{
		string(ActionMoveUp),
		string(ActionMoveDown),
		string(ActionMoveLeft),
		string(ActionMoveRight),
		string(ActionWait),
	}

	// Check for interactions
	hasAdjacentEnemy := false
	hasAdjacentNPC := false

	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			x, y := g.Player.X+dx, g.Player.Y+dy
			if g.getEnemyAt(x, y) != nil {
				hasAdjacentEnemy = true
			}
			if g.getNPCAt(x, y) != nil {
				hasAdjacentNPC = true
			}
		}
	}

	if hasAdjacentEnemy {
		actions = append(actions, string(ActionAttack))
	}
	if hasAdjacentNPC {
		actions = append(actions, string(ActionTalk))
	}

	actions = append(actions, string(ActionInteract))

	tile := g.Dungeon.Tiles[g.Player.Y][g.Player.X]
	if tile == TileStairsDown {
		actions = append(actions, string(ActionDescend))
	}
	if tile == TileStairsUp {
		actions = append(actions, string(ActionAscend))
	}

	return actions
}

// Reset restarts the game
func (g *Game) Reset() {
	*g = *NewGame()
}

// UseItem uses an item from inventory
func (g *Game) UseItem(itemIdx int) error {
	if itemIdx < 0 || itemIdx >= len(g.Player.Inventory) {
		return fmt.Errorf("invalid item index")
	}

	item := g.Player.Inventory[itemIdx]

	switch item.Type {
	case ItemPotion:
		g.Player.Health += item.Effect
		if g.Player.Health > g.Player.MaxHealth {
			g.Player.Health = g.Player.MaxHealth
		}
		g.addMessage(fmt.Sprintf("You drink the %s. (+%d HP)", item.Name, item.Effect))
		// Remove item
		g.Player.Inventory = append(g.Player.Inventory[:itemIdx], g.Player.Inventory[itemIdx+1:]...)
	default:
		return fmt.Errorf("cannot use that item")
	}

	return nil
}

// ToASCII renders the current view as ASCII
func (g *Game) ToASCII() string {
	visible := g.calculateFOV()
	result := ""

	for y := 0; y < g.Dungeon.Height; y++ {
		for x := 0; x < g.Dungeon.Width; x++ {
			if !visible[y][x] {
				result += " "
				continue
			}

			// Check for entities
			if x == g.Player.X && y == g.Player.Y {
				result += "@"
				continue
			}

			if enemy := g.getEnemyAt(x, y); enemy != nil && enemy.State != StateDead {
				result += string(EnemyToRune(enemy))
				continue
			}

			if npc := g.getNPCAt(x, y); npc != nil {
				result += string(NPCToRune(npc))
				continue
			}

			// Check for items
			for _, item := range g.Items {
				if item.X == x && item.Y == y {
					result += string(ItemToRune(item.Item))
					goto nextTile
				}
			}

			result += string(TileToRune(g.Dungeon.Tiles[y][x]))
		nextTile:
		}
		result += "\n"
	}
	return result
}

// EnemyToRune returns ASCII representation of an enemy
func EnemyToRune(e *Enemy) rune {
	switch e.Type {
	case EnemySkeleton:
		return 's'
	case EnemyZombie:
		return 'z'
	case EnemyGhost:
		return 'g'
	case EnemySpider:
		return 'S'
	case EnemyBat:
		return 'b'
	case EnemyRat:
		return 'r'
	case EnemyOrc:
		return 'O'
	case EnemyTroll:
		return 'T'
	case EnemyLich:
		return 'L'
	default:
		return 'e'
	}
}

// ItemToRune returns ASCII representation of an item
func ItemToRune(i Item) rune {
	switch i.Type {
	case ItemPotion:
		return '!'
	case ItemGold:
		return '*'
	case ItemKey:
		return 'k'
	case ItemWeapon:
		return '/'
	case ItemArmor:
		return '['
	case ItemScroll:
		return '?'
	default:
		return 'i'
	}
}

// ============================================================================
// AI PLAYER MODE
// ============================================================================

// NewDemoGame creates a game in AI demo mode with a curated level
func NewDemoGame() *Game {
	params := DemoParams()
	g := NewGameWithParams(params)

	// Enable AI mode
	g.AI = AIState{
		Enabled:       true,
		Mode:          "explore",
		ThinkDelay:    0,
		GoalsComplete: make(map[string]bool),
	}

	g.addMessage("AI Demo Mode - Watch the game play itself!")
	return g
}

// DemoParams returns parameters for a demo level that showcases all features
func DemoParams() DungeonParams {
	return DungeonParams{
		Width:        30,
		Height:       25,
		RoomCount:    5,
		MinRoomSize:  5,
		MaxRoomSize:  8,
		EnemyDensity: 0.4,
		LootDensity:  0.5,
		NPCCount:     3,
		Seed:         42, // Fixed seed for reproducible demo
		Difficulty:   3,
	}
}

// EnableAI turns on AI player mode
func (g *Game) EnableAI() {
	g.AI = AIState{
		Enabled:         true,
		Mode:            "explore",
		ThinkDelay:      0,
		GoalsComplete:   make(map[string]bool),
		StateMachine:    NewAIStateMachine(),
		CombatEvaluator: NewCombatEvaluator(),
		Brain:           NewAIBrain(),
	}
	g.addMessage("AI Mode enabled - watching the adventure unfold...")
}

// DisableAI turns off AI player mode
func (g *Game) DisableAI() {
	g.AI.Enabled = false
	g.addMessage("AI Mode disabled - you're in control!")
}

// EnableInfiniteMode turns on infinite mode (no level cap)
func (g *Game) EnableInfiniteMode() {
	g.InfiniteMode = true
	g.addMessage("Infinite Mode enabled - descend forever!")
}

// NewInfiniteGame creates a new game with infinite mode enabled
func NewInfiniteGame() *Game {
	g := NewGame()
	g.EnableInfiniteMode()
	return g
}

// NewInfiniteGameWithSeed creates a new infinite mode game with a specific seed
func NewInfiniteGameWithSeed(seed int64) *Game {
	params := DefaultParams()
	params.Seed = seed
	g := NewGameWithParams(params)
	g.EnableInfiniteMode()
	return g
}

// AITick performs one AI decision/action cycle
// Returns the action taken (or empty string if waiting)
// By default, uses the Petri net brain for ODE-based decision making.
func (g *Game) AITick() ActionType {
	if !g.AI.Enabled || g.GameOver {
		return ""
	}

	g.AI.ActionCount++

	// Log cache stats periodically if instrumentation is enabled
	if g.AI.CacheStatsInterval > 0 && g.AI.ActionCount-g.AI.LastCacheLog >= g.AI.CacheStatsInterval {
		g.LogCacheStats()
		g.AI.LastCacheLog = g.AI.ActionCount
	}

	// Periodically reset NPC conversation cooldowns to allow re-talking (every 50 ticks)
	// But only for NPCs that are far away (> 10 tiles) to avoid endless talk loops
	if g.AI.ActionCount%50 == 0 {
		for key := range g.AI.GoalsComplete {
			if len(key) > 5 && key[:5] == "talk_" {
				// Extract NPC ID and check distance
				npcID := key[5:]
				npc := g.getNPCByID(npcID)
				if npc == nil {
					delete(g.AI.GoalsComplete, key)
				} else {
					dist := abs(g.Player.X-npc.X) + abs(g.Player.Y-npc.Y)
					if dist > 10 {
						delete(g.AI.GoalsComplete, key)
					}
				}
			}
		}
	}

	// Track last position (used for other stuck detection)
	g.AI.LastX = g.Player.X
	g.AI.LastY = g.Player.Y

	// Use brain-based AI by default if available
	if g.AI.Brain != nil {
		return g.aiTickWithBrain()
	}

	// Fall back to legacy state machine AI
	return g.aiTickLegacy()
}

// aiTickWithBrain performs AI decision-making using the Petri net brain.
// This uses ODE simulation via the hypothesis package to evaluate actions.
func (g *Game) aiTickWithBrain() ActionType {
	brain := g.AI.Brain

	// Update memory with current game state
	brain.UpdateMemory(g, g.AI.ActionCount)

	// Handle special states first
	if g.InShop {
		return g.aiHandleShop()
	}
	if g.InDialogue {
		return g.aiHandleDialogue()
	}

	// Priority: If on exit stairs, always descend
	if g.Player.X == g.Dungeon.ExitX && g.Player.Y == g.Dungeon.ExitY {
		g.ProcessAction(ActionDescend)
		g.AI.LastAction = "descend_exit"
		brain.Goals.LevelsComplete++
		return ActionDescend
	}

	// Handle active combat
	if g.Combat.Active {
		return g.aiHandleCombat()
	}

	// If we just fled, keep running away for a few ticks
	if g.AI.FleeTicks > 0 {
		g.AI.FleeTicks--

		// Try to find the best escape direction (away from most enemies)
		escapeAction := g.aiFindEscapeRoute()
		if escapeAction != "" {
			g.AI.LastAction = "flee_running"
			return escapeAction
		}

		// Try to move away from where we fled from
		action := g.aiMoveAwayFrom(g.AI.FleeFrom[0], g.AI.FleeFrom[1])
		if action != "" {
			g.AI.LastAction = "flee_running"
			return action
		}

		// Can't move - check if we're truly cornered
		adjacentCount := g.countAdjacentEnemies()
		if adjacentCount >= 2 && g.AI.FleeTicks == 0 {
			// Tried to flee, can't escape, must fight
			g.AI.CommitToFight = true
			g.addMessage("Cornered! Must fight!")
		} else if adjacentCount == 0 {
			// No enemies adjacent, we're safe - stop fleeing
			g.AI.FleeTicks = 0
		}
		// If 1 adjacent enemy and still have flee ticks, keep trying
	}

	// HIGH PRIORITY: Check for adjacent NPCs (distance 0-1) before enemy checks
	// When we're literally standing on or next to an NPC, we should talk to them
	// This enables NPC interactions during normal exploration
	adjacentNPC := g.findNearestNPC(2)
	if adjacentNPC != nil && !g.AI.GoalsComplete["talk_"+adjacentNPC.ID] && !g.AI.GoalsComplete["unreachable_npc_"+adjacentNPC.ID] {
		npcDist := abs(g.Player.X-adjacentNPC.X) + abs(g.Player.Y-adjacentNPC.Y)
		// If NPC is at distance 0 or 1 (adjacent/same tile), talk to them
		if npcDist <= 1 {
			g.AI.Mode = "interact"
			g.AI.Target = adjacentNPC.ID
			g.AI.TargetTicks = 0
			g.AI.LastAction = "talk_adjacent_npc"
			return g.aiInteract()
		}
	}

	// Check for adjacent enemies - but consider threat assessment first
	if enemy := g.findAdjacentEnemy(); enemy != nil {
		adjacentCount := g.countAdjacentEnemies()
		healthPct := float64(g.Player.Health) / float64(g.Player.MaxHealth)

		// Critical survival check: low HP + multiple enemies = try to escape first
		if healthPct < 0.4 && adjacentCount >= 2 && !g.AI.CommitToFight {
			// Desperately try to find an escape route
			escapeAction := g.aiFindEscapeRoute()
			if escapeAction != "" {
				g.AI.LastAction = "desperate_escape"
				g.AI.FleeTicks = 3
				g.AI.FleeFrom = [2]int{g.Player.X, g.Player.Y}
				return escapeAction
			}
			// No escape - must fight
			g.AI.CommitToFight = true
		}

		// Use turn-based combat so the UI shows the combat panel
		g.AI.Target = enemy.ID
		g.InitiateCombat()
		g.AI.LastAction = "attack_adjacent"
		brain.SetTarget("enemy", enemy.X, enemy.Y, g.AI.ActionCount,
			float64(abs(g.Player.X-enemy.X)+abs(g.Player.Y-enemy.Y)))
		// Don't attack yet - let aiHandleCombat take over on next tick
		return ActionAttack
	}

	// Handle oscillation detection - check if stuck in small area
	if brain.IsOscillating() {
		g.AI.StuckCounter++
		// If stuck for too long, try to break out by exploring a new direction
		if g.AI.StuckCounter > 4 {
			g.AI.StuckCounter = 0
			g.AI.LastAction = "unstuck_explore"
			// Find a direction we haven't visited recently
			return g.aiBreakOscillation(brain)
		}
	} else {
		// Only reset when clearly not oscillating
		g.AI.StuckCounter = 0
	}

	// Use brain to evaluate best action
	bestAction, reason := brain.EvaluateActions(g)
	g.AI.LastAction = reason

	// Set target based on brain decision (to avoid oscillation)
	g.setTargetFromBrainDecision(brain, reason)

	// Convert evaluated action to actual game action
	return g.executeBrainAction(bestAction, brain)
}

// aiTickLegacy performs AI decision-making using the legacy state machine.
func (g *Game) aiTickLegacy() ActionType {
	// Handle shop with AI (shop is entered via dialogue, check BEFORE dialogue)
	// This is checked first because InShop and InDialogue can both be true
	if g.InShop {
		return g.aiHandleShop()
	}

	// Handle dialogue with AI
	if g.InDialogue {
		return g.aiHandleDialogue()
	}

	// Priority: If on exit stairs, always descend (escape from any situation!)
	// This prevents getting stuck in flee loops at the exit
	if g.Player.X == g.Dungeon.ExitX && g.Player.Y == g.Dungeon.ExitY {
		g.ProcessAction(ActionDescend)
		g.AI.LastAction = "descend_escape"
		g.AI.Mode = "explore"
		return ActionDescend
	}

	// Handle combat with AI
	if g.Combat.Active {
		return g.aiHandleCombat()
	}

	// If very stuck, force a random walk to break out
	if g.AI.StuckCounter > 5 {
		g.AI.StuckCounter = 0
		g.AI.Mode = "wander"
		return g.aiRandomWalk()
	}

	// Decide AI mode based on current state
	g.aiDecideMode()

	// Execute based on mode
	switch g.AI.Mode {
	case "heal":
		return g.aiHeal()
	case "combat":
		if action := g.aiEngageCombat(); action != "" {
			return action
		}
		// Combat target invalid/dead, fall through to explore
		return g.aiExplore()
	case "interact":
		return g.aiInteract()
	case "loot":
		return g.aiLoot()
	case "find_key":
		return g.aiFindKeyAction()
	case "explore":
		return g.aiExplore()
	case "wander":
		return g.aiRandomWalk()
	default:
		return g.aiExplore()
	}
}

// executeBrainAction converts a brain-evaluated action into actual game moves.
func (g *Game) executeBrainAction(action ActionType, brain *AIBrain) ActionType {
	switch action {
	case ActionDescend:
		if g.Player.X == g.Dungeon.ExitX && g.Player.Y == g.Dungeon.ExitY {
			g.ProcessAction(ActionDescend)
			brain.Goals.LevelsComplete++
			return ActionDescend
		}
		// Not at exit, move toward it
		return g.aiMoveToward(g.Dungeon.ExitX, g.Dungeon.ExitY)

	case ActionAttack:
		// Find nearest enemy to attack - must be truly adjacent (not through wall)
		enemy := g.findAdjacentEnemy()
		if enemy != nil {
			return g.aiAttackDirection(enemy.X, enemy.Y)
		}
		// No adjacent enemy - try to path toward nearest reachable enemy
		enemy = g.findNearestReachableEnemy(8)
		if enemy != nil {
			action := g.aiFindPathBFS(enemy.X, enemy.Y)
			if action != "" {
				g.ProcessAction(action)
				return action
			}
		}
		// Can't reach any enemy, fall back to exploration
		return g.aiMoveToward(g.Dungeon.ExitX, g.Dungeon.ExitY)

	case ActionUseItem:
		// Use healing potion
		for i, item := range g.Player.Inventory {
			if item.Type == ItemPotion {
				g.UseItem(i)
				return ActionUseItem
			}
		}
		return ActionWait

	case ActionTalk:
		// Find nearest NPC
		npc := g.findNearestNPC(2)
		if npc != nil {
			// Move to NPC if not adjacent
			dist := abs(g.Player.X-npc.X) + abs(g.Player.Y-npc.Y)
			if dist > 1 {
				return g.aiMoveToward(npc.X, npc.Y)
			}
			// Talk to adjacent NPC
			g.ProcessAction(ActionTalk)
			return ActionTalk
		}
		return ActionWait

	case ActionMoveUp, ActionMoveDown, ActionMoveLeft, ActionMoveRight:
		// Direct movement - use Petri net pathfinding for proper locked door handling
		state := brain.GetState()

		// Check if we're in find_key mode (LastAction contains the reason)
		if strings.Contains(g.AI.LastAction, "find_key") {
			return g.aiFindKeyAction()
		}

		// CRITICAL: Proactively find potions when inventory is low
		// This is the #1 priority fix for seed 87 (0 potions)
		potionCount := g.countPotions()
		if potionCount < 2 && state["dist_to_potion"] < 1000 {
			// Look for nearby potion with wider radius when desperate
			radius := 12
			if potionCount == 0 {
				radius = 20 // Desperate search
			}
			potion := g.findNearestPotion(radius)
			if potion != nil {
				action := g.aiMoveTowardPetri(potion.X, potion.Y)
				if action != "" && action != ActionWait {
					return action
				}
			}
		}

		// Priority: exit if path exists - use Petri net pathfinding
		// Petri net handles locked doors correctly
		if state["path_exists"] > 0 {
			action := g.aiMoveTowardPetri(g.Dungeon.ExitX, g.Dungeon.ExitY)
			if action != "" && action != ActionWait {
				return action
			}
		}

		// Priority: nearby chest (high value loot!) - but only if reachable
		if strings.Contains(g.AI.LastAction, "loot_chest") || state["dist_to_chest"] < 15 {
			chestX, chestY := g.findNearestReachableChest()
			if chestX >= 0 {
				action := g.aiMoveTowardPetri(chestX, chestY)
				if action != "" && action != ActionWait {
					return action
				}
			}
		}

		// Priority: nearby item (might be a key!)
		if state["dist_to_item"] < 10 {
			item := g.findNearestItem(10)
			if item != nil {
				action := g.aiMoveTowardPetri(item.X, item.Y)
				if action != "" && action != ActionWait {
					return action
				}
			}
		}

		// Priority: enemy if healthy
		healthPct := state["health"] / state["max_health"]
		if state["dist_to_enemy"] < 8 && healthPct > 0.4 {
			enemy := g.findNearestEnemy(8)
			if enemy != nil {
				// Use regular BFS for enemies since they're usually on floor tiles
				return g.aiMoveToward(enemy.X, enemy.Y)
			}
		}

		// If path doesn't exist, explore to find key
		if state["path_exists"] == 0 {
			return g.aiFindKeyAction()
		}

		// Default: explore toward exit using Petri net
		return g.aiMoveTowardPetri(g.Dungeon.ExitX, g.Dungeon.ExitY)

	case ActionWait:
		return ActionWait

	default:
		// Unknown action, fall back to exploration
		return g.aiExplore()
	}
}

// setTargetFromBrainDecision sets the AI target based on the brain's decision.
// This helps the AI commit to a target instead of oscillating between goals.
func (g *Game) setTargetFromBrainDecision(brain *AIBrain, reason string) {
	tick := g.AI.ActionCount

	// Parse the reason to determine target type
	// Format: "action_type (score: X)"
	switch {
	case strings.Contains(reason, "move_to_exit"):
		dist := float64(abs(g.Player.X-g.Dungeon.ExitX) + abs(g.Player.Y-g.Dungeon.ExitY))
		if brain.ShouldSwitchTarget(dist, tick, brain.canReachExit(g)) {
			brain.SetTarget("exit", g.Dungeon.ExitX, g.Dungeon.ExitY, tick, dist)
		}

	case strings.Contains(reason, "loot_chest"):
		chestX, chestY := g.findNearestChest()
		if chestX >= 0 {
			dist := float64(abs(g.Player.X-chestX) + abs(g.Player.Y-chestY))
			if brain.ShouldSwitchTarget(dist, tick, true) {
				brain.SetTarget("chest", chestX, chestY, tick, dist)
			}
		}

	case strings.Contains(reason, "move_to_item"):
		item := g.findNearestItem(10)
		if item != nil {
			dist := float64(abs(g.Player.X-item.X) + abs(g.Player.Y-item.Y))
			if brain.ShouldSwitchTarget(dist, tick, true) {
				brain.SetTarget("item", item.X, item.Y, tick, dist)
			}
		}

	case strings.Contains(reason, "find_key"):
		// For keys, set target to the key location if known
		for _, item := range g.Items {
			if item.Item.Type == ItemKey {
				dist := float64(abs(g.Player.X-item.X) + abs(g.Player.Y-item.Y))
				if brain.ShouldSwitchTarget(dist, tick, true) {
					brain.SetTarget("key", item.X, item.Y, tick, dist)
				}
				break
			}
		}

	case strings.Contains(reason, "attack_enemy") || strings.Contains(reason, "approach_enemy"):
		enemy := g.findNearestEnemy(8)
		if enemy != nil {
			dist := float64(abs(g.Player.X-enemy.X) + abs(g.Player.Y-enemy.Y))
			if brain.ShouldSwitchTarget(dist, tick, true) {
				brain.SetTarget("enemy", enemy.X, enemy.Y, tick, dist)
			}
		}

	case strings.Contains(reason, "descend_exit"):
		// Clear target when descending
		brain.ClearTarget()
	}
}

// aiAttackDirection returns the attack action for a target position.
// It uses ActionAttack which automatically attacks the nearest adjacent enemy.
func (g *Game) aiAttackDirection(targetX, targetY int) ActionType {
	// ActionAttack will attack any adjacent enemy, which is what we want.
	// The targetX, targetY are informational - attack() finds adjacent enemies.
	g.ProcessAction(ActionAttack)
	return ActionAttack
}

// aiMoveToward moves one step toward a target using BFS pathfinding.
func (g *Game) aiMoveToward(targetX, targetY int) ActionType {
	// Use existing BFS pathfinding
	action := g.aiFindPathBFS(targetX, targetY)
	if action != "" {
		g.ProcessAction(action)
		return action
	}
	// No path found, try random walk
	return g.aiRandomWalk()
}

// aiDecideMode chooses what the AI should focus on
func (g *Game) aiDecideMode() {
	// Use state machine if available, otherwise fall back to legacy logic
	if g.AI.StateMachine != nil {
		g.aiDecideModeStateMachine()
		return
	}
	g.aiDecideModeLegacy()
}

// aiSelectBestTarget uses ODE evaluation to pick the optimal enemy target.
// Returns the enemy ID and score, or empty string if no valid targets.
func (g *Game) aiSelectBestTarget(maxDist int) (string, float64) {
	threats := g.gatherNearbyThreats(maxDist)
	if len(threats) == 0 {
		return "", 0
	}

	// If we have a combat evaluator, use ODE to select best target
	if g.AI.CombatEvaluator != nil {
		return g.AI.CombatEvaluator.SelectBestTarget(
			g.Player.Health, g.Player.MaxHealth,
			g.Player.Attack+g.Player.Strength, g.Player.Defense,
			g.hasHealingPotion(), 30, threats,
		)
	}

	// Fallback: pick closest enemy
	bestID := threats[0].ID
	bestDist := threats[0].Distance
	for _, t := range threats[1:] {
		if t.Distance < bestDist {
			bestDist = t.Distance
			bestID = t.ID
		}
	}
	return bestID, float64(100 - bestDist*10)
}

// aiDecideModeStateMachine uses the pflow state machine for mode transitions.
// It fires events based on game state and lets the state machine handle transitions.
func (g *Game) aiDecideModeStateMachine() {
	sm := g.AI.StateMachine

	// Helper to check if an enemy is marked as unreachable
	isUnreachable := func(enemy *Enemy) bool {
		if enemy == nil {
			return true
		}
		stuckKey := "stuck_enemy_" + enemy.ID
		return g.AI.GoalsComplete[stuckKey]
	}

	// PRIORITY: Preserve interact mode if we're actively talking to an NPC
	// This must happen BEFORE enemy/health checks to prevent overwriting interact mode
	if g.AI.Mode == "interact" && g.AI.Target != "" {
		npc := g.getNPCByID(g.AI.Target)
		if npc != nil && !g.AI.GoalsComplete["talk_"+npc.ID] && !g.AI.GoalsComplete["unreachable_npc_"+npc.ID] {
			// Still pursuing NPC - don't let enemy checks overwrite unless enemy is adjacent
			adjacentEnemy := g.findNearestEnemy(1)
			if adjacentEnemy == nil || (abs(g.Player.X-adjacentEnemy.X)+abs(g.Player.Y-adjacentEnemy.Y)) > 1 {
				// No truly adjacent enemy, persist interact mode
				if g.AI.TargetTicks > 25 || g.aiIsOscillating() {
					// Give up on this NPC
					g.AI.GoalsComplete["unreachable_npc_"+npc.ID] = true
					g.AI.Mode = "explore"
					g.AI.Target = ""
					g.AI.TargetTicks = 0
				} else {
					g.AI.TargetTicks++
					return // Persist interact mode
				}
			}
		}
	}

	// HIGH PRIORITY: Check for ADJACENT NPCs (distance 0-1) before enemy checks
	// When we're literally standing on or next to an NPC, we should talk to them
	// even if there's an adjacent enemy - the NPC interaction takes priority
	adjacentNPC := g.findNearestNPC(2)
	if adjacentNPC != nil && !g.AI.GoalsComplete["talk_"+adjacentNPC.ID] && !g.AI.GoalsComplete["unreachable_npc_"+adjacentNPC.ID] {
		npcDist := abs(g.Player.X-adjacentNPC.X) + abs(g.Player.Y-adjacentNPC.Y)
		// If NPC is at distance 0 or 1 (adjacent/same tile), always talk - highest priority
		if npcDist <= 1 {
			g.AI.Mode = "interact"
			g.AI.Target = adjacentNPC.ID
			g.AI.TargetTicks = 0
			return
		}
		// If NPC is at distance 2, only pursue if no truly adjacent enemy
		adjacentEnemy := g.findNearestEnemy(1)
		hasAdjacentEnemy := adjacentEnemy != nil && (abs(g.Player.X-adjacentEnemy.X)+abs(g.Player.Y-adjacentEnemy.Y)) <= 1
		if !hasAdjacentEnemy {
			g.AI.Mode = "interact"
			g.AI.Target = adjacentNPC.ID
			g.AI.TargetTicks = 0
			return
		}
	}

	// Detect game state and fire appropriate events

	// Check for adjacent enemies - highest priority
	adjacentEnemy := g.findNearestEnemy(1)
	if adjacentEnemy != nil {
		dx := abs(g.Player.X - adjacentEnemy.X)
		dy := abs(g.Player.Y - adjacentEnemy.Y)
		// If enemy is truly adjacent (distance 1), we can attack them directly
		// Always engage truly adjacent enemies, clearing any stuck flag
		if dx+dy == 1 {
			delete(g.AI.GoalsComplete, "stuck_enemy_"+adjacentEnemy.ID)
			sm.SendEvent(EventEnemyVisible)
			g.AI.Mode = sm.Mode()
			// Use ODE-based target selection if multiple adjacent enemies
			if bestID, _ := g.aiSelectBestTarget(1); bestID != "" {
				g.AI.Target = bestID
			} else {
				g.AI.Target = adjacentEnemy.ID
			}
			return
		}
		// For non-adjacent nearby enemies, only engage if not marked unreachable
		if !isUnreachable(adjacentEnemy) {
			sm.SendEvent(EventEnemyVisible)
			g.AI.Mode = sm.Mode()
			g.AI.Target = adjacentEnemy.ID
			return
		}
	}

	// Check for health-based transitions
	healthPct := float64(g.Player.Health) / float64(g.Player.MaxHealth)
	if healthPct < 0.33 {
		if g.hasHealingItem() {
			// Fire health_low event - may trigger recovery phase
			sm.SetCanFlee(true)
			sm.SendEvent(EventHealthLow)
			g.AI.Mode = sm.Mode()
			if g.AI.Mode == "heal" {
				return
			}
		}
	}

	// Count nearby threats for tactical decisions
	nearbyEnemyCount := g.countEnemiesInRange(5)

	// Check for aggressive enemies - use ODE to select best target
	nearbyEnemy := g.findNearestEnemy(5)
	if nearbyEnemy != nil && nearbyEnemy.State == StateChasing && !isUnreachable(nearbyEnemy) {
		// Threat assessment: if low HP and multiple enemies, consider avoiding
		if healthPct < 0.5 && nearbyEnemyCount >= 2 && !g.AI.CommitToFight {
			// Try to find a safer path - avoid engaging multiple enemies
			saferPath := g.aiFindSaferPath(nearbyEnemy.X, nearbyEnemy.Y)
			if saferPath != "" {
				g.AI.LastAction = "avoiding_multi_threat"
				g.AI.Mode = "explore"
				return
			}
		}

		sm.SendEvent(EventEnemyVisible)
		g.AI.Mode = sm.Mode()
		// Use ODE-based target selection to pick optimal target among nearby enemies
		if bestID, _ := g.aiSelectBestTarget(5); bestID != "" {
			g.AI.Target = bestID
		} else {
			g.AI.Target = nearbyEnemy.ID
		}
		return
	}

	// Check for loot
	if g.AI.Mode == "loot" {
		if g.aiIsOscillating() || g.AI.TargetTicks > 15 {
			sm.SendEvent(EventLootCollected)
			g.AI.Mode = sm.Mode()
			g.AI.Target = ""
			g.AI.TargetTicks = 0
		} else {
			g.AI.TargetTicks++
			nearbyItem := g.findNearestItem(5)
			if nearbyItem != nil {
				return
			}
			sm.SendEvent(EventLootCollected)
			g.AI.Mode = sm.Mode()
		}
	}

	// Persist interact mode while seeking NPC (like loot mode above)
	if g.AI.Mode == "interact" && g.AI.Target != "" {
		npc := g.getNPCByID(g.AI.Target)
		if npc != nil && !g.AI.GoalsComplete["talk_"+npc.ID] {
			if g.aiIsOscillating() || g.AI.TargetTicks > 30 {
				// Can't reach NPC, mark as unreachable and move on
				g.AI.GoalsComplete["unreachable_npc_"+npc.ID] = true
				sm.SendEvent(EventDialogueEnded)
				g.AI.Mode = sm.Mode()
				g.AI.Target = ""
				g.AI.TargetTicks = 0
			} else {
				g.AI.TargetTicks++
				return // Preserve interact mode, let movement logic handle it
			}
		} else {
			// NPC gone or already talked, end interaction
			sm.SendEvent(EventDialogueEnded)
			g.AI.Mode = sm.Mode()
			g.AI.Target = ""
			g.AI.TargetTicks = 0
		}
	}

	// Check for potions - prioritize based on health and inventory
	potionCount := g.countPotions()

	// Urgently need potions if health is low
	if healthPct < 0.7 {
		potion := g.findNearestPotion(10) // Expand search range when hurt
		if potion != nil {
			sm.SendEvent(EventLootNearby)
			g.AI.Mode = sm.Mode()
			g.AI.TargetTicks = 0
			return
		}
	}

	// Proactively gather potions if we have few
	// Critical: Always search for potions regardless of level - seed 87 had 0 potions!
	if potionCount < 2 {
		// Use larger search radius when inventory is empty
		searchRadius := 8
		if potionCount == 0 {
			searchRadius = 12 // Desperate search when no potions
		}

		// FIRST: Check if there's a merchant/healer nearby selling potions
		// This is more reliable than finding random potion drops!
		potionSeller := g.findNearestPotionSeller(searchRadius)
		if potionSeller != nil && !g.AI.GoalsComplete["talk_"+potionSeller.ID] {
			// Directly set interact mode - don't rely on state machine event
			// since EventNPCAdjacent only fires from exploration phase
			g.AI.Mode = "interact"
			g.AI.Target = potionSeller.ID
			g.AI.TargetTicks = 0
			return
		}

		// THEN: Look for potion items on the ground
		potion := g.findNearestPotion(searchRadius)
		if potion != nil {
			sm.SendEvent(EventLootNearby)
			g.AI.Mode = sm.Mode()
			g.AI.TargetTicks = 0
			return
		}
	}

	// Check for nearby NPCs BEFORE generic items - NPCs get priority when passing by
	// This ensures we talk to NPCs we walk past instead of ignoring them for loot
	nearbyNPC := g.findNearestNPC(3)
	if nearbyNPC != nil && !g.AI.GoalsComplete["talk_"+nearbyNPC.ID] && !g.AI.GoalsComplete["unreachable_npc_"+nearbyNPC.ID] {
		// Directly set interact mode - don't rely on state machine event
		g.AI.Mode = "interact"
		g.AI.Target = nearbyNPC.ID
		g.AI.TargetTicks = 0
		return
	}

	// Check for nearby items
	nearbyItem := g.findNearestItem(3)
	if nearbyItem != nil {
		sm.SendEvent(EventLootNearby)
		g.AI.Mode = sm.Mode()
		g.AI.TargetTicks = 0
		return
	}

	// Check for keys needed
	if g.AI.Mode == "find_key" && g.AI.Target != "" {
		if g.aiIsOscillating() || g.AI.TargetTicks > 60 {
			g.AI.GoalsComplete["unreachable_key_"+g.AI.Target] = true
			sm.SendEvent(EventKeyFound) // Gives up, effectively "found" or gave up
			g.AI.Mode = sm.Mode()
			g.AI.Target = ""
			g.AI.TargetTicks = 0
		} else {
			g.AI.TargetTicks++
			return
		}
	}

	if g.aiNeedsKey() {
		keyItem := g.aiFindKey()
		if keyItem != nil {
			keyTarget := fmt.Sprintf("%d,%d", keyItem.X, keyItem.Y)
			if !g.AI.GoalsComplete["unreachable_key_"+keyTarget] {
				sm.SendEvent(EventKeyNeeded)
				g.AI.Mode = sm.Mode()
				g.AI.Target = keyTarget
				g.AI.TargetTicks = 0
				return
			}
		}
	}

	// Check for NPC interaction
	distToExit := abs(g.Player.X-g.Dungeon.ExitX) + abs(g.Player.Y-g.Dungeon.ExitY)
	if distToExit <= 3 {
		sm.SendEvent(EventDialogueEnded) // Prioritize exit over NPCs
		g.AI.Mode = sm.Mode()
	} else if g.AI.Mode == "interact" && g.AI.Target != "" {
		currentNPC := g.getNPCByID(g.AI.Target)
		if currentNPC == nil {
			// Target NPC doesn't exist anymore - clear target and switch to explore
			sm.SendEvent(EventDialogueEnded)
			g.AI.Mode = sm.Mode()
			g.AI.Target = ""
			g.AI.TargetTicks = 0
		} else if !g.AI.GoalsComplete["talk_"+currentNPC.ID] && !g.AI.GoalsComplete["unreachable_npc_"+currentNPC.ID] {
			if g.aiIsOscillating() || g.AI.TargetTicks > 25 {
				g.AI.GoalsComplete["unreachable_npc_"+currentNPC.ID] = true
				sm.SendEvent(EventDialogueEnded)
				g.AI.Mode = sm.Mode()
				g.AI.Target = ""
				g.AI.TargetTicks = 0
			} else {
				g.AI.TargetTicks++
				return
			}
		} else {
			// Already talked to this NPC or marked unreachable - clear and explore
			sm.SendEvent(EventDialogueEnded)
			g.AI.Mode = sm.Mode()
			g.AI.Target = ""
			g.AI.TargetTicks = 0
		}
	}

	// Only seek NPCs when far from exit (>8 tiles) to prioritize level completion
	if distToExit > 8 {
		nearbyNPC := g.findNearestNPC(8)
		if nearbyNPC != nil && !g.AI.GoalsComplete["talk_"+nearbyNPC.ID] && !g.AI.GoalsComplete["unreachable_npc_"+nearbyNPC.ID] {
			sm.SendEvent(EventNPCAdjacent)
			g.AI.Mode = sm.Mode()
			g.AI.Target = nearbyNPC.ID
			g.AI.TargetTicks = 0
			return
		}
	}

	// Check for visible enemies
	if g.AI.Mode == "combat" && g.AI.Target != "" {
		if g.aiIsOscillating() || g.AI.TargetTicks > 30 {
			g.AI.GoalsComplete["stuck_enemy_"+g.AI.Target] = true
			sm.SendEvent(EventEnemyDead) // Effectively gives up on this enemy
			g.AI.Mode = sm.Mode()
			g.AI.Target = ""
			g.AI.TargetTicks = 0
		} else {
			g.AI.TargetTicks++
			return
		}
	}

	visibleEnemy := g.findNearestEnemy(5)
	if visibleEnemy != nil && !isUnreachable(visibleEnemy) {
		sm.SendEvent(EventEnemyVisible)
		g.AI.Mode = sm.Mode()
		g.AI.Target = visibleEnemy.ID
		g.AI.TargetTicks = 0
		return
	}

	// Handle loot oscillation
	if g.AI.Mode == "loot" && g.aiIsOscillating() {
		sm.SendEvent(EventLootCollected)
		g.AI.Mode = sm.Mode()
		g.AI.Target = ""
	}

	// Only switch to wander if we're really stuck for a long time
	// The oscillation detection is too aggressive for normal exploration
	// Rely on StuckCounter which is set when aiExplore fails to make progress
	if g.AI.Mode == "explore" && g.AI.StuckCounter > 10 {
		sm.SendEvent(EventDeadEnd)
		g.AI.Target = ""
		g.AI.StuckCounter = 0 // Reset so we don't immediately switch back
	}

	// Ensure mode is synced from state machine
	g.AI.Mode = sm.Mode()

	// Safety check: If state machine is in interact mode but no valid target, transition out
	if g.AI.Mode == "interact" && g.AI.Target == "" {
		sm.SendEvent(EventDialogueEnded)
		g.AI.Mode = sm.Mode()
	}

	// Safety check: If state machine is in combat mode but no valid target, transition out
	if g.AI.Mode == "combat" && g.AI.Target == "" {
		sm.SendEvent(EventEnemyDead) // No target = no enemy to fight
		g.AI.Mode = sm.Mode()
	}

	if g.AI.Mode == "explore" || g.AI.Mode == "wander" {
		g.AI.Target = ""
	}
}

// aiDecideModeLegacy is the original priority-based mode decision logic.
// Kept for fallback when state machine is not initialized.
func (g *Game) aiDecideModeLegacy() {
	// Helper to check if an enemy is marked as unreachable
	isUnreachable := func(enemy *Enemy) bool {
		if enemy == nil {
			return true
		}
		stuckKey := "stuck_enemy_" + enemy.ID
		return g.AI.GoalsComplete[stuckKey]
	}

	// Priority 0: Fight adjacent enemies (in combat or attacking us)
	adjacentEnemy := g.findNearestEnemy(1)
	if adjacentEnemy != nil {
		dx := abs(g.Player.X - adjacentEnemy.X)
		dy := abs(g.Player.Y - adjacentEnemy.Y)
		// If enemy is truly adjacent (distance 1), we can attack them directly
		// Always engage truly adjacent enemies, clearing any stuck flag
		if dx+dy == 1 {
			delete(g.AI.GoalsComplete, "stuck_enemy_"+adjacentEnemy.ID)
			g.AI.Mode = "combat"
			// Use ODE-based target selection if multiple adjacent enemies
			if bestID, _ := g.aiSelectBestTarget(1); bestID != "" {
				g.AI.Target = bestID
			} else {
				g.AI.Target = adjacentEnemy.ID
			}
			return
		}
		// For non-adjacent nearby enemies, only engage if not marked unreachable
		if !isUnreachable(adjacentEnemy) {
			g.AI.Mode = "combat"
			g.AI.Target = adjacentEnemy.ID
			return
		}
	}

	// Priority 1: Heal if health is low
	if g.Player.Health < g.Player.MaxHealth/3 {
		if g.hasHealingItem() {
			g.AI.Mode = "heal"
			return
		}
	}

	// Priority 2: Fight nearby aggressive enemies (within 5 tiles, chasing us)
	nearbyEnemy := g.findNearestEnemy(5)
	if nearbyEnemy != nil && nearbyEnemy.State == StateChasing && !isUnreachable(nearbyEnemy) {
		g.AI.Mode = "combat"
		// Use ODE-based target selection to pick optimal target
		if bestID, _ := g.aiSelectBestTarget(5); bestID != "" {
			g.AI.Target = bestID
		} else {
			g.AI.Target = nearbyEnemy.ID
		}
		return
	}

	// Priority 3: Pick up nearby items (prioritize potions if health < 70%)
	// Only pick up items that are close (3 tiles) to avoid wandering off
	// Exception: potions when health is low (use 5 tiles)
	// If already looting, check for oscillation or timeout
	if g.AI.Mode == "loot" {
		if g.aiIsOscillating() || g.AI.TargetTicks > 15 {
			g.AI.Mode = "explore"
			g.AI.Target = ""
			g.AI.TargetTicks = 0
			// Fall through to explore
		} else {
			g.AI.TargetTicks++
			// Check if item still exists and is reachable
			nearbyItem := g.findNearestItem(5)
			if nearbyItem != nil {
				return // Keep looting
			}
			// Item gone, switch to explore
			g.AI.Mode = "explore"
		}
	}
	// Check for potions first if health is low
	if g.Player.Health < (g.Player.MaxHealth*7)/10 {
		potion := g.findNearestPotion(5)
		if potion != nil {
			g.AI.Mode = "loot"
			g.AI.TargetTicks = 0
			return
		}
	}
	// Proactively gather potions if inventory is low (critical fix for seed 87)
	potionCount := g.countPotions()
	if potionCount < 2 {
		searchRadius := 8
		if potionCount == 0 {
			searchRadius = 12 // Desperate search when no potions
		}
		potion := g.findNearestPotion(searchRadius)
		if potion != nil {
			g.AI.Mode = "loot"
			g.AI.TargetTicks = 0
			return
		}
	}
	// Otherwise only pick up items that are very close
	nearbyItem := g.findNearestItem(3)
	if nearbyItem != nil {
		g.AI.Mode = "loot"
		g.AI.TargetTicks = 0
		return
	}

	// Priority 3.5: If we found locked doors and don't have a key, look for keys
	// If already in find_key mode, check for oscillation or timeout
	// Keys are critical so give more time (60 ticks) before giving up
	if g.AI.Mode == "find_key" && g.AI.Target != "" {
		if g.aiIsOscillating() || g.AI.TargetTicks > 60 {
			// Mark this key location as unreachable
			g.AI.GoalsComplete["unreachable_key_"+g.AI.Target] = true
			g.AI.Mode = "explore"
			g.AI.Target = ""
			g.AI.TargetTicks = 0
			// Fall through to explore
		} else {
			g.AI.TargetTicks++
			return // Keep trying for the key
		}
	}
	if g.aiNeedsKey() {
		keyItem := g.aiFindKey()
		if keyItem != nil {
			keyTarget := fmt.Sprintf("%d,%d", keyItem.X, keyItem.Y)
			if !g.AI.GoalsComplete["unreachable_key_"+keyTarget] {
				g.AI.Mode = "find_key"
				g.AI.Target = keyTarget
				g.AI.TargetTicks = 0
				return
			}
		}
	}

	// Priority 4: Talk to nearby NPCs we haven't talked to (within 8 tiles)
	// SKIP if we're close to the exit (8 tiles) - prioritize descending!
	distToExit := abs(g.Player.X-g.Dungeon.ExitX) + abs(g.Player.Y-g.Dungeon.ExitY)
	if distToExit <= 8 {
		// Close to exit - skip NPC interaction, go straight for stairs
		g.AI.Mode = "explore"
		// Don't return - fall through to explore
	} else if g.AI.Mode == "interact" && g.AI.Target != "" {
		currentNPC := g.getNPCByID(g.AI.Target)
		if currentNPC != nil && !g.AI.GoalsComplete["talk_"+currentNPC.ID] && !g.AI.GoalsComplete["unreachable_npc_"+currentNPC.ID] {
			// Check if we're stuck trying to reach this NPC (oscillating or taking too long)
			if g.aiIsOscillating() || g.AI.TargetTicks > 25 {
				// Mark as unreachable permanently on this level (separate from talk_ which resets)
				g.AI.GoalsComplete["unreachable_npc_"+currentNPC.ID] = true
				g.AI.Mode = "explore"
				g.AI.Target = ""
				g.AI.TargetTicks = 0
				// Don't return - fall through to explore
			} else {
				// Keep current target, increment tick counter
				g.AI.TargetTicks++
				return
			}
		}
	}
	// Only look for NPCs if not close to exit (>15 tiles)
	if distToExit > 15 {
		nearbyNPC := g.findNearestNPC(8)
		if nearbyNPC != nil && !g.AI.GoalsComplete["talk_"+nearbyNPC.ID] && !g.AI.GoalsComplete["unreachable_npc_"+nearbyNPC.ID] {
			g.AI.Mode = "interact"
			g.AI.Target = nearbyNPC.ID
			g.AI.TargetTicks = 0 // Reset counter for new target
			return
		}
	}

	// Priority 5: Fight nearby visible enemies (within 5 tiles) - don't chase across map
	// If already in combat and oscillating or taking too long, mark enemy as unreachable
	if g.AI.Mode == "combat" && g.AI.Target != "" {
		if g.aiIsOscillating() || g.AI.TargetTicks > 30 {
			// Mark current target as temporarily unreachable using GoalsComplete map
			g.AI.GoalsComplete["stuck_enemy_"+g.AI.Target] = true
			g.AI.Mode = "explore"
			g.AI.Target = ""
			g.AI.TargetTicks = 0
			// Fall through to explore
		} else {
			// Keep fighting current target
			g.AI.TargetTicks++
			return
		}
	}
	visibleEnemy := g.findNearestEnemy(5)
	if visibleEnemy != nil && !isUnreachable(visibleEnemy) {
		g.AI.Mode = "combat"
		g.AI.Target = visibleEnemy.ID
		g.AI.TargetTicks = 0
		return
	}

	// Priority 6: Loot mode - if oscillating while looting, give up
	if g.AI.Mode == "loot" && g.aiIsOscillating() {
		g.AI.Mode = "explore"
		g.AI.Target = ""
	}

	// Default: Explore (find stairs, wander)
	// If oscillating in explore mode, clear target and let wander logic take over
	if g.AI.Mode == "explore" && g.aiIsOscillating() {
		g.AI.Target = ""
	}
	g.AI.Mode = "explore"
	g.AI.Target = "" // Clear target when entering explore mode
}

// aiHeal uses a healing item
func (g *Game) aiHeal() ActionType {
	for i, item := range g.Player.Inventory {
		if item.Type == ItemPotion && item.Effect > 0 {
			g.UseItem(i)
			g.AI.LastAction = "heal"
			return ActionUseItem
		}
	}
	// No healing items, switch to explore mode (will explore on next tick)
	g.AI.Mode = "explore"
	return ""
}

// aiEngageCombat initiates or continues combat
func (g *Game) aiEngageCombat() ActionType {
	enemy := g.getEnemyByID(g.AI.Target)
	if enemy == nil || enemy.State == StateDead {
		// Target gone, switch to explore (will explore on next tick)
		g.AI.Mode = "explore"
		g.AI.Target = ""
		return ""
	}

	// If adjacent, attack
	dx := abs(g.Player.X - enemy.X)
	dy := abs(g.Player.Y - enemy.Y)
	if dx <= 1 && dy <= 1 {
		// Start turn-based combat
		g.InitiateCombat()
		return ActionAttack
	}

	// If enemy is far (>10 tiles), give up and explore
	dist := dx + dy
	if dist > 10 {
		g.AI.Mode = "explore"
		g.AI.Target = ""
		return ""
	}

	// Pre-combat threat assessment using ODE evaluator
	if g.AI.CombatEvaluator != nil {
		// Gather all nearby threats
		threats := g.gatherNearbyThreats(8) // Within 8 tiles

		if len(threats) > 0 {
			assessment := g.AI.CombatEvaluator.PreCombatAssessment(
				g.Player.Health, g.Player.MaxHealth,
				g.Player.Attack+g.Player.Strength, g.Player.Defense,
				g.hasHealingPotion(), 30, threats,
			)

			switch assessment {
			case "heal_first":
				// Heal before engaging
				for i, item := range g.Player.Inventory {
					if item.Type == ItemPotion && item.Effect > 0 {
						g.UseItem(i)
						g.AI.LastAction = "pre_combat_heal"
						return ActionUseItem
					}
				}
			case "flee_zone":
				// Too dangerous, retreat from this area
				g.AI.Mode = "explore"
				g.AI.Target = ""
				g.AI.LastAction = "flee_zone"
				return g.aiMoveAwayFrom(enemy.X, enemy.Y)
			case "wait":
				// Wait for better positioning - let enemy come to us
				g.AI.LastAction = "wait_positioning"
				return ActionWait
			// "engage" falls through to normal approach logic
			}
		}
	}

	// Try BFS to find path to enemy
	if action := g.aiFindPathBFS(enemy.X, enemy.Y); action != "" {
		beforeX, beforeY := g.Player.X, g.Player.Y
		g.ProcessAction(action)
		g.AI.LastAction = string(action) + "_ENGAGE"
		if beforeX == g.Player.X && beforeY == g.Player.Y {
			g.AI.LastAction = string(action) + "_ENGAGE_BLOCKED"
		}
		return action
	}

	// No path found - check if we've been stuck trying to reach this enemy
	stuckKey := "stuck_enemy_" + enemy.ID
	if g.AI.GoalsComplete[stuckKey] {
		// Already failed once to reach this enemy, give up
		g.AI.Mode = "explore"
		g.AI.Target = ""
		delete(g.AI.GoalsComplete, stuckKey)
		return ""
	}

	// Mark that we failed to find a path, try once more with greedy/random
	g.AI.GoalsComplete[stuckKey] = true
	return g.aiMoveTowardSmart(enemy.X, enemy.Y)
}

// aiHandleCombat handles turn-based combat decisions
func (g *Game) aiHandleCombat() ActionType {
	if !g.Combat.PlayerTurn {
		return "" // Wait for enemy turn
	}

	// Check if all combatants are dead - if so, end combat
	allDead := true
	var firstLivingEnemy string
	for _, enemyID := range g.Combat.Combatants {
		enemy := g.getEnemyByID(enemyID)
		if enemy != nil && enemy.State != StateDead {
			allDead = false
			if firstLivingEnemy == "" {
				firstLivingEnemy = enemyID
			}
		}
	}
	if allDead || len(g.Combat.Combatants) == 0 {
		g.EndCombat()
		return ""
	}

	// Make sure we're targeting a living enemy in the combat
	targetEnemy := g.getEnemyByID(g.AI.Target)
	if targetEnemy == nil || targetEnemy.State == StateDead {
		// Switch to a living combatant
		g.AI.Target = firstLivingEnemy
		g.SetTargetEnemy(firstLivingEnemy)
		targetEnemy = g.getEnemyByID(firstLivingEnemy)
	}

	// Turn to face the enemy during combat
	if targetEnemy != nil {
		g.turnToward(targetEnemy.X, targetEnemy.Y)
	}

	// Use ODE-based combat evaluator for turn decisions
	if targetEnemy != nil && g.AI.CombatEvaluator != nil {
		sit := CombatSituation{
			PlayerHP:      g.Player.Health,
			PlayerMaxHP:   g.Player.MaxHealth,
			PlayerAttack:  g.Player.Attack + g.Player.Strength,
			EnemyHP:       targetEnemy.Health,
			EnemyMaxHP:    targetEnemy.MaxHealth,
			EnemyAttack:   targetEnemy.Damage,
			CanFlee:       true,
			HasHealPotion: g.hasHealingPotion(),
			PotionHealAmt: 30,
			EnemyCount:    len(g.Combat.Combatants),
			PlayerArmor:   g.Player.Defense,
			PlayerWeapon:  g.Player.Attack,
		}

		dx := abs(g.Player.X - targetEnemy.X)
		dy := abs(g.Player.Y - targetEnemy.Y)
		dist := dx + dy

		// Don't allow flee if we've committed to fighting after failed attempts
		canFlee := g.Combat.CurrentAP >= APCostMove*2 && !g.AI.CommitToFight

		opts := CombatTurnOptions{
			CanAttack:       g.Combat.CurrentAP >= APCostAttack && dist <= 1,
			CanAimedShot:    g.Combat.CurrentAP >= APCostAimedShot && dist <= 1,
			CanHeal:         g.Combat.CurrentAP >= APCostUseItem && g.hasHealingPotion(),
			CanMove:         g.Combat.CurrentAP >= APCostMove,
			CanFlee:         canFlee,
			CurrentAP:       g.Combat.CurrentAP,
			MaxAP:           g.Combat.MaxAP,
			DistanceToEnemy: dist,
		}

		// Check for mid-combat retreat (only if we haven't committed to fighting)
		if canFlee && g.AI.CombatEvaluator.ShouldRetreatMidCombat(sit, g.Combat.RoundNumber) {
			combatWasActive := g.Combat.Active
			g.ProcessCombatAction(ActionFlee, nil)
			if !g.Combat.Active && combatWasActive {
				g.AI.LastAction = "retreat_mid_combat"
				return ActionFlee
			}
			// Flee failed, fall through to normal combat logic
		}

		action, _ := g.AI.CombatEvaluator.EvaluateCombatTurn(sit, opts)

		switch action {
		case TurnActionHeal:
			for i, item := range g.Player.Inventory {
				if item.Type == ItemPotion && item.Effect > 0 {
					g.ProcessCombatAction(ActionUseItem, map[string]interface{}{"index": float64(i)})
					g.AI.LastAction = "ode_combat_heal"
					return ActionUseItem
				}
			}
		case TurnActionFlee:
			combatWasActive := g.Combat.Active
			g.ProcessCombatAction(ActionFlee, nil)
			if !g.Combat.Active && combatWasActive {
				g.AI.LastAction = "ode_flee"
				return ActionFlee
			}
			// Flee failed - attack instead of ending turn (more productive)
			if g.Combat.CurrentAP >= APCostAttack {
				if g.AI.Target != "" {
					g.SetTargetEnemy(g.AI.Target)
				}
				g.SetTargetPart(BodyTorso)
				g.ProcessCombatAction(ActionAttack, nil)
				g.AI.LastAction = "ode_flee_failed_attack"
				return ActionAttack
			}
			// No AP left, end turn
			g.ProcessCombatAction(ActionEndTurn, nil)
			g.AI.LastAction = "ode_flee_failed_end_turn"
			return ActionEndTurn
		case TurnActionAimedShot:
			// Sync target before attacking
			if g.AI.Target != "" {
				g.SetTargetEnemy(g.AI.Target)
			}
			g.SetTargetPart(BodyHead)
			g.ProcessCombatAction(ActionAimedShot, nil)
			g.AI.LastAction = "ode_aimed_shot"
			return ActionAimedShot
		case TurnActionAttack:
			// Sync target before attacking
			if g.AI.Target != "" {
				g.SetTargetEnemy(g.AI.Target)
			}
			g.SetTargetPart(BodyTorso)
			g.ProcessCombatAction(ActionAttack, nil)
			g.AI.LastAction = "ode_attack"
			return ActionAttack
		case TurnActionMove:
			// Fall through to movement logic below
		case TurnActionEndTurn:
			// CRITICAL: Never end turn at low HP if we have options
			healthPct := float64(g.Player.Health) / float64(g.Player.MaxHealth)
			if healthPct < 0.35 {
				// Try to heal first if we have a potion
				if g.hasHealingPotion() && g.Combat.CurrentAP >= APCostUseItem {
					for i, item := range g.Player.Inventory {
						if item.Type == ItemPotion && item.Effect > 0 {
							g.ProcessCombatAction(ActionUseItem, map[string]interface{}{"index": float64(i)})
							g.AI.LastAction = "critical_heal_override"
							return ActionUseItem
						}
					}
				}
				// Try to flee if we have AP and haven't committed to fight
				if g.Combat.CurrentAP >= APCostMove*2 && !g.AI.CommitToFight {
					combatWasActive := g.Combat.Active
					g.ProcessCombatAction(ActionFlee, nil)
					if !g.Combat.Active && combatWasActive {
						g.AI.LastAction = "critical_flee_override"
						return ActionFlee
					}
				}
			}
			g.ProcessCombatAction(ActionEndTurn, nil)
			g.AI.LastAction = "ode_end_turn"
			return ActionEndTurn
		}
	} else {
		// Fallback: simple heuristic if no evaluator
		// If low health and have healing, use it
		if g.Player.Health < g.Player.MaxHealth/4 && g.hasHealingItem() && g.Combat.CurrentAP >= APCostUseItem {
			for i, item := range g.Player.Inventory {
				if item.Type == ItemPotion && item.Effect > 0 {
					g.ProcessCombatAction(ActionUseItem, map[string]interface{}{"index": float64(i)})
					g.AI.LastAction = "combat_heal"
					return ActionUseItem
				}
			}
		}

		// Simple flee/fight decision - but don't try to flee if we've already committed to fighting
		if !g.AI.CommitToFight && g.Player.Health < g.Player.MaxHealth/3 && targetEnemy != nil {
			playerDamage := g.Player.Attack + g.Player.Strength
			playerHitsToKill := (targetEnemy.Health + playerDamage - 1) / max(playerDamage, 1)
			enemyHitsToKill := (g.Player.Health + targetEnemy.Damage - 1) / max(targetEnemy.Damage, 1)

			if playerHitsToKill > enemyHitsToKill && targetEnemy.Health > playerDamage*2 {
				// Try to flee - but check if it actually succeeded
				combatWasActive := g.Combat.Active
				g.ProcessCombatAction(ActionFlee, nil)
				if !g.Combat.Active && combatWasActive {
					// Flee succeeded, combat ended
					g.AI.LastAction = "flee"
					return ActionFlee
				}
				// Flee failed - don't return, fall through to attack instead
				g.AI.LastAction = "flee_failed"
			}
		}
	}

	// Check if we're adjacent to target enemy - if not, move closer
	targetEnemy = g.getEnemyByID(g.AI.Target)
	if targetEnemy != nil {
		dx := abs(g.Player.X - targetEnemy.X)
		dy := abs(g.Player.Y - targetEnemy.Y)
		dist := dx + dy
		if dist > 1 && g.Combat.CurrentAP >= APCostMove {
			// Check if we're oscillating (not making progress toward enemy)
			// If already marked as stuck, force end combat immediately
			stuckKey := "stuck_enemy_" + g.AI.Target
			if g.AI.GoalsComplete[stuckKey] {
				// Force end combat - enemy is unreachable, don't rely on flee roll
				g.EndCombat()
				g.AI.Mode = "explore"
				g.AI.Target = ""
				g.AI.LastAction = "flee_stuck"
				return ""
			}

			// Track if we're making progress - only try the directions that will reduce distance
			progressDirections := [][2]int{}
			// Only add directions that will decrease distance
			if targetEnemy.X < g.Player.X {
				progressDirections = append(progressDirections, [2]int{-1, 0})
			} else if targetEnemy.X > g.Player.X {
				progressDirections = append(progressDirections, [2]int{1, 0})
			}
			if targetEnemy.Y < g.Player.Y {
				progressDirections = append(progressDirections, [2]int{0, -1})
			} else if targetEnemy.Y > g.Player.Y {
				progressDirections = append(progressDirections, [2]int{0, 1})
			}

			// Try each direction that would reduce distance
			madeProgress := false
			for _, dir := range progressDirections {
				err := g.ProcessCombatAction(ActionCombatMove, map[string]interface{}{
					"dx": float64(dir[0]),
					"dy": float64(dir[1]),
				})
				if err == nil {
					g.AI.LastAction = "combat_move"
					madeProgress = true
					return ActionCombatMove
				}
			}

			// If no progress-making moves succeeded, enemy is unreachable - mark and force end combat
			if !madeProgress {
				g.AI.GoalsComplete[stuckKey] = true
				g.EndCombat()
				g.AI.Mode = "explore"
				g.AI.Target = ""
				g.AI.LastAction = "flee_unreachable"
				return ""
			}
		}
	}

	// Attack if we have AP and target is adjacent
	if g.Combat.CurrentAP >= APCostAttack {
		// Sync Combat.SelectedEnemy with AI.Target
		if g.AI.Target != "" {
			g.SetTargetEnemy(g.AI.Target)
		} else if g.Combat.SelectedEnemy == "" && len(g.Combat.Combatants) > 0 {
			// Pick closest target
			g.SetTargetEnemy(g.Combat.Combatants[0])
		}

		// Verify target is adjacent before attacking
		targetEnemy = g.getEnemyByID(g.Combat.SelectedEnemy)
		if targetEnemy != nil {
			dx := abs(g.Player.X - targetEnemy.X)
			dy := abs(g.Player.Y - targetEnemy.Y)
			if dx+dy > 1 {
				// Not adjacent - try to find an adjacent enemy in combatants
				for _, combatantID := range g.Combat.Combatants {
					combatant := g.getEnemyByID(combatantID)
					if combatant != nil && combatant.State != StateDead {
						cdx := abs(g.Player.X - combatant.X)
						cdy := abs(g.Player.Y - combatant.Y)
						if cdx+cdy <= 1 {
							// Found an adjacent enemy, switch to it
							g.AI.Target = combatantID
							g.SetTargetEnemy(combatantID)
							targetEnemy = combatant
							break
						}
					}
				}
				// Re-check if now adjacent
				if targetEnemy != nil {
					dx = abs(g.Player.X - targetEnemy.X)
					dy = abs(g.Player.Y - targetEnemy.Y)
					if dx+dy > 1 {
						// Still not adjacent, end turn (can't attack)
						g.ProcessCombatAction(ActionEndTurn, nil)
						g.AI.LastAction = "end_turn_not_adjacent"
						return ActionEndTurn
					}
				}
			}
		}

		// Occasionally do aimed shots at head for crits
		if g.rng.Intn(4) == 0 && g.Combat.CurrentAP >= APCostAimedShot {
			g.SetTargetPart(BodyHead)
			g.ProcessCombatAction(ActionAimedShot, nil)
			g.AI.LastAction = "aimed_shot_head"
			return ActionAimedShot
		}

		// Regular attack
		g.SetTargetPart(BodyTorso)
		g.ProcessCombatAction(ActionAttack, nil)
		g.AI.LastAction = "attack"
		return ActionAttack
	}

	// End turn if out of AP
	g.ProcessCombatAction(ActionEndTurn, nil)
	g.AI.LastAction = "end_turn"
	return ActionEndTurn
}

// aiInteract moves toward and talks to an NPC
func (g *Game) aiInteract() ActionType {
	npc := g.getNPCByID(g.AI.Target)
	if npc == nil {
		// NPC gone, switch to explore (will explore on next tick)
		g.AI.Mode = "explore"
		g.AI.Target = ""
		return ""
	}

	// Always turn to face NPC when interacting
	g.turnToward(npc.X, npc.Y)

	// If adjacent, talk
	dx := abs(g.Player.X - npc.X)
	dy := abs(g.Player.Y - npc.Y)
	if dx <= 1 && dy <= 1 {
		// Mark as talked immediately to prevent retrying
		g.AI.GoalsComplete["talk_"+npc.ID] = true
		g.ProcessAction(ActionTalk)
		g.AI.LastAction = "talk"
		g.AI.Mode = "explore" // Done with this NPC
		g.AI.Target = ""      // Clear target to prevent re-triggering
		return ActionTalk
	}

	// Move toward NPC (non-recursive version)
	return g.aiMoveTowardSimple(npc.X, npc.Y)
}

// aiHandleDialogue makes dialogue choices
func (g *Game) aiHandleDialogue() ActionType {
	// Get current dialogue
	npc := g.getNPCByID(g.DialogueNPC)
	if npc == nil {
		g.InDialogue = false
		return ""
	}

	// Always face the NPC during dialogue
	g.turnToward(npc.X, npc.Y)

	dialogue := GetDialogue(npc)
	var currentNode *DialogueNode
	for i := range dialogue {
		if dialogue[i].ID == g.DialogueNode {
			currentNode = &dialogue[i]
			break
		}
	}

	if currentNode == nil {
		g.InDialogue = false
		return ""
	}

	// Mark that we talked to this NPC
	g.AI.GoalsComplete["talk_"+npc.ID] = true

	// If there are choices, pick intelligently
	if len(currentNode.Choices) > 0 {
		// Helper to check if we can afford a choice
		canAfford := func(choice DialogueChoice) bool {
			if choice.Condition != nil && choice.Condition.RequireGold > g.Player.Gold {
				return false
			}
			return true
		}

		// Track visited nodes to detect loops (stored as "visited_<npc>_<node>")
		visitKey := "visited_" + npc.ID + "_" + currentNode.ID
		if g.AI.GoalsComplete[visitKey] {
			// We've been here before - prefer to exit
			for i, choice := range currentNode.Choices {
				if choice.NextID == "" || choice.NextID == "end" {
					g.ProcessDialogueChoice(i)
					return ""
				}
			}
			// No exit option, force end
			g.InDialogue = false
			g.DialogueNPC = ""
			g.DialogueNode = ""
			return ""
		}
		g.AI.GoalsComplete[visitKey] = true

		// First priority: Accept keys when offered (check if we're in key dialogue)
		if currentNode.ID == "has_key" || currentNode.ID == "key_info" {
			// Look for the choice that gives us the key
			for i, choice := range currentNode.Choices {
				if choice.NextID == "give_key" {
					if err := g.ProcessDialogueChoice(i); err == nil {
						return ""
					}
				}
			}
		}

		// Prefer accepting quests, buying healing items (if we can afford)
		for i, choice := range currentNode.Choices {
			if choice.Effect != nil && (choice.Effect.StartQuest != "" || choice.Effect.AddItem != "" || choice.Effect.Heal > 0) {
				if canAfford(choice) {
					if err := g.ProcessDialogueChoice(i); err == nil {
						return ""
					}
				}
			}
		}

		// Prefer shop options for merchants/healers if we have gold to spend
		// Look for choices leading to "shop" node (triggers InShop)
		if (npc.Type == NPCMerchant || npc.Type == NPCHealer) && g.Player.Gold > 20 {
			for i, choice := range currentNode.Choices {
				if choice.NextID == "shop" && canAfford(choice) {
					if err := g.ProcessDialogueChoice(i); err == nil {
						return ""
					}
				}
			}
		}

		// Pick any exit option first (prefer leaving over looping)
		for i, choice := range currentNode.Choices {
			if choice.NextID == "" || choice.NextID == "end" {
				g.ProcessDialogueChoice(i)
				return ""
			}
		}
		// Try non-exit options we can afford (may lead to new nodes)
		for i, choice := range currentNode.Choices {
			if choice.NextID != "" && choice.NextID != "end" && canAfford(choice) {
				// Only try if we haven't visited the target
				targetKey := "visited_" + npc.ID + "_" + choice.NextID
				if !g.AI.GoalsComplete[targetKey] {
					if err := g.ProcessDialogueChoice(i); err == nil {
						return ""
					}
				}
			}
		}
		// Last resort: force end dialogue
		g.InDialogue = false
		g.DialogueNPC = ""
		g.DialogueNode = ""
	} else {
		// No choices - check for auto-continue or end dialogue
		if currentNode.NextID != "" && currentNode.NextID != "end" {
			g.DialogueNode = currentNode.NextID
		} else {
			// End dialogue
			g.InDialogue = false
			g.DialogueNPC = ""
			g.DialogueNode = ""
		}
	}

	return ""
}

// aiHandleShop handles AI shopping decisions
// Buys useful items while avoiding duplicates (unless potions or rich)
func (g *Game) aiHandleShop() ActionType {
	npc := g.getNPCByID(g.DialogueNPC)
	if npc == nil || len(npc.Inventory) == 0 {
		g.CloseShop()
		g.AI.LastAction = "close_shop_empty"
		return ""
	}

	// Helper to check if player already has item (by ID)
	hasItem := func(itemID string) bool {
		for _, inv := range g.Player.Inventory {
			if inv.ID == itemID {
				return true
			}
		}
		return false
	}

	// Helper to count how many of an item type we have
	countItemType := func(itemType ItemType) int {
		count := 0
		for _, inv := range g.Player.Inventory {
			if inv.Type == itemType {
				count++
			}
		}
		return count
	}

	// Helper to check if item is an upgrade (better effect for weapons/armor)
	isUpgrade := func(item Item) bool {
		if item.Type == ItemWeapon {
			// Check if we have a weapon with better or equal effect
			for _, inv := range g.Player.Inventory {
				if inv.Type == ItemWeapon && inv.Effect >= item.Effect {
					return false
				}
			}
			return true
		}
		if item.Type == ItemArmor {
			// Check if we have armor with better or equal effect
			for _, inv := range g.Player.Inventory {
				if inv.Type == ItemArmor && inv.Effect >= item.Effect {
					return false
				}
			}
			return true
		}
		return false
	}

	// Calculate how much gold we're willing to spend
	// Keep at least 20 gold for emergencies, unless we're rich
	reserveGold := 20
	if g.Player.Gold > 200 {
		reserveGold = 0 // Rich enough to spend freely
	}
	spendableGold := g.Player.Gold - reserveGold
	if spendableGold < 0 {
		spendableGold = 0
	}

	// Priority buying logic:
	// 1. Health potions if health < 50% or we have fewer than 2
	// 2. Weapon/armor upgrades
	// 3. Extra potions if we have lots of gold (>100)
	// 4. Don't buy duplicates of non-consumables

	var itemToBuy *Item

	// Priority 1: Health potions when needed
	healthPercent := float64(g.Player.Health) / float64(g.Player.MaxHealth)
	potionCount := countItemType(ItemPotion)

	for i := range npc.Inventory {
		item := &npc.Inventory[i]
		if item.Value > spendableGold {
			continue // Can't afford
		}

		// Always buy health potions if health is low or we have few
		if item.Type == ItemPotion && (healthPercent < 0.5 || potionCount < 2) {
			itemToBuy = item
			break
		}
	}

	// Priority 2: Weapon/armor upgrades
	if itemToBuy == nil {
		for i := range npc.Inventory {
			item := &npc.Inventory[i]
			if item.Value > spendableGold {
				continue
			}

			if (item.Type == ItemWeapon || item.Type == ItemArmor) && isUpgrade(*item) {
				itemToBuy = item
				break
			}
		}
	}

	// Priority 3: Extra potions if rich (and don't have too many)
	if itemToBuy == nil && g.Player.Gold > 100 && potionCount < 5 {
		for i := range npc.Inventory {
			item := &npc.Inventory[i]
			if item.Value > spendableGold {
				continue
			}

			if item.Type == ItemPotion {
				itemToBuy = item
				break
			}
		}
	}

	// Priority 4: Quest items we don't have (rope, torch, etc.)
	if itemToBuy == nil && g.Player.Gold > 50 {
		for i := range npc.Inventory {
			item := &npc.Inventory[i]
			if item.Value > spendableGold {
				continue
			}

			if item.Type == ItemQuest && !hasItem(item.ID) {
				itemToBuy = item
				break
			}
		}
	}

	// Buy the selected item or close shop
	if itemToBuy != nil {
		err := g.BuyItem(itemToBuy.ID)
		if err == nil {
			g.AI.LastAction = "bought_" + itemToBuy.ID
			// Stay in shop to potentially buy more
			return ""
		}
	}

	// Nothing more to buy - close shop
	g.CloseShop()
	g.AI.LastAction = "close_shop_done"
	return ""
}

// aiLoot moves toward and picks up items
func (g *Game) aiLoot() ActionType {
	item := g.findNearestItem(10)
	if item == nil {
		// No items, switch to explore - fire event to sync state machine
		if g.AI.StateMachine != nil {
			g.AI.StateMachine.SendEvent(EventLootCollected)
		}
		g.AI.Mode = "explore"
		return ""
	}

	// Check if we're stuck trying to reach this item
	if g.aiIsOscillating() {
		// Give up on looting, switch to explore - fire event to sync state machine
		if g.AI.StateMachine != nil {
			g.AI.StateMachine.SendEvent(EventLootCollected)
		}
		g.AI.Mode = "explore"
		g.AI.Target = ""
		return ""
	}

	// Always face the item we're going for
	g.turnToward(item.X, item.Y)

	// If on item, it's auto-picked up, switch to explore
	if g.Player.X == item.X && g.Player.Y == item.Y {
		// Fire event to sync state machine
		if g.AI.StateMachine != nil {
			g.AI.StateMachine.SendEvent(EventLootCollected)
		}
		g.AI.Mode = "explore"
		return ""
	}

	// Move toward item (non-recursive version)
	return g.aiMoveTowardSimple(item.X, item.Y)
}

// aiFindKeyAction moves toward a known key on the ground or an NPC holding a key
func (g *Game) aiFindKeyAction() ActionType {
	// First, check if any NPC has a key - this is preferred since talking to them gives the key
	var keyHolderNPC *NPC
	keyHolderDist := 999
	for _, npc := range g.NPCs {
		if npc.HasKey && !npc.Hostile {
			dist := abs(g.Player.X-npc.X) + abs(g.Player.Y-npc.Y)
			if dist < keyHolderDist {
				keyHolderNPC = npc
				keyHolderDist = dist
			}
		}
	}

	if keyHolderNPC != nil {
		// Move toward NPC with key
		g.AI.LastAction = fmt.Sprintf("seeking key holder %s at %d,%d", keyHolderNPC.Name, keyHolderNPC.X, keyHolderNPC.Y)

		// If adjacent, talk to NPC (which transfers the key)
		if keyHolderDist <= 1 {
			g.talk()
			g.AI.Mode = "explore"
			g.AI.AvoidDoors = nil // Clear avoidance now that we have a key
			return ""
		}

		// Move toward NPC
		return g.aiMoveTowardPetri(keyHolderNPC.X, keyHolderNPC.Y)
	}

	// No NPC with key, check for keys on the ground
	keyItem := g.aiFindKey()
	if keyItem == nil {
		// No key found, explore to find one using Petri net pathfinding
		// First check if we can find a path to any key using reachability analysis
		keyLocations := g.getKeyLocations()
		if len(keyLocations) > 0 {
			pf := NewAIPathfinder(g.Dungeon, keyLocations, false)
			keyPos, keyPath := pf.FindPathToKey(g.Player.X, g.Player.Y)
			if keyPath != nil && len(keyPath) >= 2 {
				g.AI.LastAction = fmt.Sprintf("petri_path_to_key_%d_%d", keyPos[0], keyPos[1])
				nextX, nextY := keyPath[1][0], keyPath[1][1]
				dx := nextX - g.Player.X
				dy := nextY - g.Player.Y
				var action ActionType
				switch {
				case dx == 0 && dy == -1:
					action = ActionMoveUp
				case dx == 0 && dy == 1:
					action = ActionMoveDown
				case dx == -1 && dy == 0:
					action = ActionMoveLeft
				case dx == 1 && dy == 0:
					action = ActionMoveRight
				}
				if action != "" {
					g.ProcessAction(action)
					return action
				}
			}
		}
		// No keys on ground or unreachable, explore to find one
		g.AI.Mode = "explore"
		g.AI.LastAction = "searching for key"
		return g.aiRandomWalk()
	}

	// Always face the key we're going for
	g.turnToward(keyItem.X, keyItem.Y)

	// If on key, it's auto-picked up, clear locked doors (we can open them now)
	if g.Player.X == keyItem.X && g.Player.Y == keyItem.Y {
		g.AI.Mode = "explore"
		g.AI.AvoidDoors = nil // Clear avoidance now that we have a key
		g.AI.LastAction = "picked up key"
		return ""
	}

	// Move toward key using Petri net pathing (handles locked doors)
	g.AI.LastAction = fmt.Sprintf("moving to key at %d,%d", keyItem.X, keyItem.Y)
	return g.aiMoveTowardPetri(keyItem.X, keyItem.Y)
}

// aiExplore moves toward unexplored areas or stairs
func (g *Game) aiExplore() ActionType {
	// DEBUG trace
	debugExplore := false // Set to true to enable verbose logging
	if debugExplore {
		fmt.Printf("[aiExplore] Start: Player at (%d,%d) Mode=%s\n", g.Player.X, g.Player.Y, g.AI.Mode)
	}

	// Look for stairs down
	var stairsX, stairsY int
	foundStairs := false
	for y := 0; y < g.Dungeon.Height && !foundStairs; y++ {
		for x := 0; x < g.Dungeon.Width && !foundStairs; x++ {
			if g.Dungeon.Tiles[y][x] == TileStairsDown {
				stairsX, stairsY = x, y
				foundStairs = true
			}
		}
	}

	if foundStairs {
		// Always face the stairs when heading toward them
		g.turnToward(stairsX, stairsY)

		// If we're on the stairs, descend
		if g.Player.X == stairsX && g.Player.Y == stairsY {
			g.ProcessAction(ActionDescend)
			g.AI.LastAction = "descend"
			return ActionDescend
		}

		// Check if there's an enemy blocking the stairs - if so, navigate to fight it
		for _, enemy := range g.Enemies {
			if enemy.X == stairsX && enemy.Y == stairsY && enemy.State != StateDead {
				// Enemy is on the stairs!
				// Check if already adjacent - if so, attack
				dx := abs(g.Player.X - enemy.X)
				dy := abs(g.Player.Y - enemy.Y)
				if dx+dy == 1 {
					// Adjacent to enemy on stairs - initiate combat
					g.InitiateCombat()
					g.AI.Mode = "combat"
					g.AI.Target = enemy.ID
					g.AI.LastAction = "attack_stairs_enemy"
					return ""
				}
				// Not adjacent - find path to get adjacent using BFS that ignores enemies
				action := g.aiFindPathToAdjacentBFS(enemy.X, enemy.Y)
				if action != "" {
					g.ProcessAction(action)
					g.AI.LastAction = "approach_stairs_enemy"
					return action
				}
			}
		}

		// If stuck for too long, try random walk to break out
		if g.AI.StuckCounter > 5 {
			if debugExplore {
				fmt.Printf("[aiExplore] StuckCounter > 5, calling aiRandomWalk\n")
			}
			return g.aiRandomWalk()
		}

		// Try Petri net pathfinding first - it handles locked doors properly
		hasKey := g.Player.Keys["rusty_key"]
		keyLocations := g.getKeyLocations()
		pf := NewAIPathfinder(g.Dungeon, keyLocations, hasKey)
		petriAction := pf.GetNextMove(g.Player.X, g.Player.Y, stairsX, stairsY)

		if petriAction != "" {
			beforeX, beforeY := g.Player.X, g.Player.Y
			g.ProcessAction(petriAction)
			afterX, afterY := g.Player.X, g.Player.Y
			g.AI.LastAction = string(petriAction) + "_PETRI_EXPLORE"
			if beforeX == afterX && beforeY == afterY {
				g.AI.LastAction = string(petriAction) + "_PETRI_BLOCKED"
			} else {
				return petriAction
			}
		}

		// Petri net pathfinding didn't find direct path - may need key
		// Check if we need to get a key first
		if !hasKey && len(keyLocations) > 0 {
			keyPos, keyPath := pf.FindPathToKey(g.Player.X, g.Player.Y)
			if keyPath != nil && len(keyPath) >= 2 {
				nextX, nextY := keyPath[1][0], keyPath[1][1]
				dx := nextX - g.Player.X
				dy := nextY - g.Player.Y
				var keyAction ActionType
				switch {
				case dx == 0 && dy == -1:
					keyAction = ActionMoveUp
				case dx == 0 && dy == 1:
					keyAction = ActionMoveDown
				case dx == -1 && dy == 0:
					keyAction = ActionMoveLeft
				case dx == 1 && dy == 0:
					keyAction = ActionMoveRight
				}
				if keyAction != "" {
					g.ProcessAction(keyAction)
					g.AI.Mode = "find_key"
					g.AI.LastAction = fmt.Sprintf("petri_to_key_%d_%d", keyPos[0], keyPos[1])
					return keyAction
				}
			}
		}

		// Fall back to regular BFS
		action := g.aiFindPathBFS(stairsX, stairsY)
		if debugExplore {
			fmt.Printf("[aiExplore] BFS returned action='%s' for stairs at (%d,%d)\n", action, stairsX, stairsY)
		}
		// DEBUG: Log BFS result
		if action != "" {
			// Path found, take that step
			beforeX, beforeY := g.Player.X, g.Player.Y
			g.ProcessAction(action)
			afterX, afterY := g.Player.X, g.Player.Y
			g.AI.LastAction = string(action)
			// DEBUG: Check if move actually happened
			if beforeX == afterX && beforeY == afterY {
				g.AI.LastAction = string(action) + "_BLOCKED"
			}
			return action
		}
		// DEBUG: BFS returned empty - log why
		g.AI.LastAction = "bfs_failed"

		// BFS failed to find path - check if there are locked doors we need a key for
		if !g.aiHasKey() && g.aiHasLockedDoors() {
			// There are locked doors and no path to stairs - we need a key!
			keyItem := g.aiFindKey()
			if keyItem != nil {
				keyTarget := fmt.Sprintf("%d,%d", keyItem.X, keyItem.Y)
				if !g.AI.GoalsComplete["unreachable_key_"+keyTarget] {
					g.AI.Mode = "find_key"
					g.AI.Target = keyTarget
					g.AI.TargetTicks = 0
					g.AI.LastAction = "need key for locked doors"
					return g.aiMoveTowardPetri(keyItem.X, keyItem.Y)
				}
			}
		}

		// BFS failed and we have key (or no locked doors) - enemies might be blocking
		// Find the nearest live enemy that could be blocking path and go fight it
		var nearestEnemy *Enemy
		nearestDist := 9999
		for _, enemy := range g.Enemies {
			if enemy.State == StateDead {
				continue
			}
			dist := abs(g.Player.X-enemy.X) + abs(g.Player.Y-enemy.Y)
			if dist < nearestDist {
				nearestDist = dist
				nearestEnemy = enemy
			}
		}
		if nearestEnemy != nil {
			// Check if adjacent to enemy - if so, fight
			dx := abs(g.Player.X - nearestEnemy.X)
			dy := abs(g.Player.Y - nearestEnemy.Y)
			if dx+dy == 1 {
				g.InitiateCombat()
				g.AI.Mode = "combat"
				g.AI.Target = nearestEnemy.ID
				g.AI.LastAction = "attack_blocking_enemy"
				return ""
			}
			// Try to find path to get adjacent to nearest enemy
			pathAction := g.aiFindPathToAdjacentBFS(nearestEnemy.X, nearestEnemy.Y)
			if pathAction != "" {
				g.ProcessAction(pathAction)
				g.AI.LastAction = "approach_blocking_enemy"
				return pathAction
			}
		}

		// Fall back to greedy movement toward stairs
		return g.aiMoveTowardSmart(stairsX, stairsY)
	}

	// No stairs found, random walk
	return g.aiRandomWalk()
}

// aiHasLockedDoors checks if there are any locked doors in the dungeon
func (g *Game) aiHasLockedDoors() bool {
	for y := 0; y < g.Dungeon.Height; y++ {
		for x := 0; x < g.Dungeon.Width; x++ {
			if g.Dungeon.Tiles[y][x] == TileLockedDoor {
				return true
			}
		}
	}
	return false
}

// aiRandomWalk tries random directions, but biased toward the exit
func (g *Game) aiRandomWalk() ActionType {
	directions := []ActionType{ActionMoveUp, ActionMoveDown, ActionMoveLeft, ActionMoveRight}

	// 50% of the time, try to move toward the exit first
	if g.aiRng.Float64() < 0.5 {
		exitX, exitY := g.Dungeon.ExitX, g.Dungeon.ExitY
		dx := exitX - g.Player.X
		dy := exitY - g.Player.Y

		// Prioritize the direction that gets us closer to exit
		if abs(dx) > abs(dy) {
			if dx > 0 {
				directions = []ActionType{ActionMoveRight, ActionMoveUp, ActionMoveDown, ActionMoveLeft}
			} else {
				directions = []ActionType{ActionMoveLeft, ActionMoveUp, ActionMoveDown, ActionMoveRight}
			}
		} else {
			if dy > 0 {
				directions = []ActionType{ActionMoveDown, ActionMoveLeft, ActionMoveRight, ActionMoveUp}
			} else {
				directions = []ActionType{ActionMoveUp, ActionMoveLeft, ActionMoveRight, ActionMoveDown}
			}
		}
	} else {
		// Shuffle directions for randomness (using AI-specific RNG)
		for i := len(directions) - 1; i > 0; i-- {
			j := g.aiRng.Intn(i + 1)
			directions[i], directions[j] = directions[j], directions[i]
		}
	}

	// First pass: try with smart pathing (avoid locked doors)
	for _, dir := range directions {
		dx, dy := 0, 0
		switch dir {
		case ActionMoveUp:
			dy = -1
		case ActionMoveDown:
			dy = 1
		case ActionMoveLeft:
			dx = -1
		case ActionMoveRight:
			dx = 1
		}

		newX, newY := g.Player.X+dx, g.Player.Y+dy
		if g.aiCanMoveTo(newX, newY) {
			g.ProcessAction(dir)
			g.AI.LastAction = string(dir)
			return dir
		}
	}

	// Second pass: desperate mode - try basic movement (even into locked doors)
	for _, dir := range directions {
		dx, dy := 0, 0
		switch dir {
		case ActionMoveUp:
			dy = -1
		case ActionMoveDown:
			dy = 1
		case ActionMoveLeft:
			dx = -1
		case ActionMoveRight:
			dx = 1
		}

		newX, newY := g.Player.X+dx, g.Player.Y+dy
		if g.canMoveTo(newX, newY) {
			g.ProcessAction(dir)
			g.AI.LastAction = string(dir) + "_desperate"
			return dir
		}
	}

	// Wait if truly stuck
	g.ProcessAction(ActionWait)
	g.AI.LastAction = "wait"
	return ActionWait
}

// aiBreakOscillation tries to escape from oscillation by moving to a less-visited tile
func (g *Game) aiBreakOscillation(brain *AIBrain) ActionType {
	// Use fast BFS pathfinding to exit with key check
	hasKey := g.Player.Keys["rusty_key"]
	keyLocations := g.getKeyLocations()
	pf := NewAIPathfinder(g.Dungeon, keyLocations, hasKey)

	// Use the fast BFS method (avoids expensive Petri net construction)
	path := pf.findPathBFS(g.Player.X, g.Player.Y, g.Dungeon.ExitX, g.Dungeon.ExitY)
	if path != nil && len(path) >= 2 {
		nextX, nextY := path[1][0], path[1][1]
		dx := nextX - g.Player.X
		dy := nextY - g.Player.Y
		var action ActionType
		switch {
		case dx == 0 && dy == -1:
			action = ActionMoveUp
		case dx == 0 && dy == 1:
			action = ActionMoveDown
		case dx == -1 && dy == 0:
			action = ActionMoveLeft
		case dx == 1 && dy == 0:
			action = ActionMoveRight
		}
		if action != "" {
			g.ProcessAction(action)
			return action
		}
	}

	// Path to exit blocked - try to find a key
	if !hasKey && len(keyLocations) > 0 {
		for _, keyLoc := range keyLocations {
			keyPath := pf.findPathBFS(g.Player.X, g.Player.Y, keyLoc[0], keyLoc[1])
			if keyPath != nil && len(keyPath) >= 2 {
				nextX, nextY := keyPath[1][0], keyPath[1][1]
				dx := nextX - g.Player.X
				dy := nextY - g.Player.Y
				var keyAction ActionType
				switch {
				case dx == 0 && dy == -1:
					keyAction = ActionMoveUp
				case dx == 0 && dy == 1:
					keyAction = ActionMoveDown
				case dx == -1 && dy == 0:
					keyAction = ActionMoveLeft
				case dx == 1 && dy == 0:
					keyAction = ActionMoveRight
				}
				if keyAction != "" {
					g.ProcessAction(keyAction)
					g.AI.LastAction = fmt.Sprintf("unstuck_to_key_%d_%d", keyLoc[0], keyLoc[1])
					return keyAction
				}
			}
		}
	}

	// Fall back to random walk if BFS pathfinding fails
	// Get recent positions to avoid
	brain.Memory.mu.RLock()
	recentPositions := make(map[[2]int]bool)
	if len(brain.Memory.RecentPath) > 0 {
		start := len(brain.Memory.RecentPath) - 10
		if start < 0 {
			start = 0
		}
		for _, pos := range brain.Memory.RecentPath[start:] {
			recentPositions[pos] = true
		}
	}
	brain.Memory.mu.RUnlock()

	// Try each direction, preferring ones that lead to less-visited tiles
	directions := []struct {
		action ActionType
		dx, dy int
	}{
		{ActionMoveUp, 0, -1},
		{ActionMoveDown, 0, 1},
		{ActionMoveLeft, -1, 0},
		{ActionMoveRight, 1, 0},
	}

	// Shuffle for randomness
	for i := len(directions) - 1; i > 0; i-- {
		j := g.aiRng.Intn(i + 1)
		directions[i], directions[j] = directions[j], directions[i]
	}

	// First pass: try to find a direction leading to an unvisited tile
	for _, d := range directions {
		newX, newY := g.Player.X+d.dx, g.Player.Y+d.dy
		if !g.aiCanMoveTo(newX, newY) {
			continue
		}
		pos := [2]int{newX, newY}
		if !recentPositions[pos] {
			g.ProcessAction(d.action)
			return d.action
		}
	}

	// Second pass: any valid direction (shuffled, so random)
	for _, d := range directions {
		newX, newY := g.Player.X+d.dx, g.Player.Y+d.dy
		if g.aiCanMoveTo(newX, newY) {
			g.ProcessAction(d.action)
			return d.action
		}
	}

	// Fall back to wait
	return ActionWait
}

// aiMoveTowardSimple moves one step toward target using smart pathing
func (g *Game) aiMoveTowardSimple(targetX, targetY int) ActionType {
	// Delegate to aiMoveTowardSmart for better pathfinding
	return g.aiMoveTowardSmart(targetX, targetY)
}

// aiMoveTowardSmart moves one step toward target, avoiding locked doors (non-recursive)
func (g *Game) aiMoveTowardSmart(targetX, targetY int) ActionType {
	// Record current position for oscillation detection
	g.aiRecordPosition()

	// Already at target
	if g.Player.X == targetX && g.Player.Y == targetY {
		return ""
	}

	// If oscillating, use random walk to escape instead of greedy movement
	if g.aiIsOscillating() {
		return g.aiRandomWalk()
	}

	// Try BFS pathfinding first
	if action := g.aiFindPathBFS(targetX, targetY); action != "" {
		beforeX, beforeY := g.Player.X, g.Player.Y
		g.ProcessAction(action)
		g.AI.LastAction = string(action) + "_SMART"
		if beforeX == g.Player.X && beforeY == g.Player.Y {
			g.AI.LastAction = string(action) + "_SMART_BLOCKED"
		}
		return action
	}

	// BFS failed, try greedy movement
	dx := targetX - g.Player.X
	dy := targetY - g.Player.Y

	// Try all 4 directions in order of preference
	type dirOption struct {
		action ActionType
		dx, dy int
		score  int // Lower is better
	}

	options := []dirOption{
		{ActionMoveRight, 1, 0, 0},
		{ActionMoveLeft, -1, 0, 0},
		{ActionMoveDown, 0, 1, 0},
		{ActionMoveUp, 0, -1, 0},
	}

	// Score each option based on how much closer it gets us to target
	for i := range options {
		newX := g.Player.X + options[i].dx
		newY := g.Player.Y + options[i].dy
		newDx := abs(targetX - newX)
		newDy := abs(targetY - newY)
		options[i].score = newDx + newDy

		// Penalize heavily if can't move there
		if !g.aiCanMoveTo(newX, newY) {
			options[i].score += 1000
		}
	}

	// Sort by score (simple bubble sort for 4 elements)
	for i := 0; i < len(options)-1; i++ {
		for j := i + 1; j < len(options); j++ {
			if options[j].score < options[i].score {
				options[i], options[j] = options[j], options[i]
			}
		}
	}

	// Try each option in order
	for _, opt := range options {
		newX := g.Player.X + opt.dx
		newY := g.Player.Y + opt.dy
		if g.aiCanMoveTo(newX, newY) {
			g.ProcessAction(opt.action)
			g.AI.LastAction = string(opt.action)
			return opt.action
		}
	}

	// All directions blocked
	_ = dx + dy // silence unused variable warning
	return g.aiRandomWalk()
}

// gatherNearbyThreats collects information about enemies within range for ODE evaluation.
func (g *Game) gatherNearbyThreats(maxDist int) []EnemyThreat {
	var threats []EnemyThreat
	for _, e := range g.Enemies {
		if e.State == StateDead {
			continue
		}
		dx := abs(g.Player.X - e.X)
		dy := abs(g.Player.Y - e.Y)
		dist := dx + dy
		if dist <= maxDist {
			threats = append(threats, EnemyThreat{
				ID:        e.ID,
				HP:        e.Health,
				MaxHP:     e.MaxHealth,
				Attack:    e.Damage,
				Distance:  dist,
				IsAlerted: e.State == StateChasing || e.State == StateAttacking,
			})
		}
	}
	return threats
}

// aiMoveAwayFrom moves the player away from a dangerous position.
func (g *Game) aiMoveAwayFrom(dangerX, dangerY int) ActionType {
	// Try to move in the opposite direction from danger
	type dirOption struct {
		action ActionType
		dx, dy int
		score  int
	}

	options := []dirOption{
		{ActionMoveRight, 1, 0, 0},
		{ActionMoveLeft, -1, 0, 0},
		{ActionMoveDown, 0, 1, 0},
		{ActionMoveUp, 0, -1, 0},
	}

	// Score each option based on how much it increases distance from danger
	for i := range options {
		newX := g.Player.X + options[i].dx
		newY := g.Player.Y + options[i].dy
		newDistX := abs(newX - dangerX)
		newDistY := abs(newY - dangerY)
		oldDistX := abs(g.Player.X - dangerX)
		oldDistY := abs(g.Player.Y - dangerY)

		// Higher score = better (more distance from danger)
		options[i].score = (newDistX + newDistY) - (oldDistX + oldDistY)

		// Penalize if can't move there
		if !g.aiCanMoveTo(newX, newY) {
			options[i].score = -1000
		}
	}

	// Sort by score descending (highest first)
	for i := 0; i < len(options)-1; i++ {
		for j := i + 1; j < len(options); j++ {
			if options[j].score > options[i].score {
				options[i], options[j] = options[j], options[i]
			}
		}
	}

	// Try best option
	for _, opt := range options {
		newX := g.Player.X + opt.dx
		newY := g.Player.Y + opt.dy
		if g.aiCanMoveTo(newX, newY) {
			g.ProcessAction(opt.action)
			g.AI.LastAction = string(opt.action) + "_FLEE"
			return opt.action
		}
	}

	return ""
}

// aiMoveTowardPetri uses Petri net reachability analysis for pathfinding.
// This properly handles locked doors by finding paths that go through keys first.
// It also uses item-aware pathfinding to reward collecting items along the way.
func (g *Game) aiMoveTowardPetri(targetX, targetY int) ActionType {
	// Record current position for oscillation detection
	g.aiRecordPosition()

	// Already at target
	if g.Player.X == targetX && g.Player.Y == targetY {
		return ""
	}

	// If oscillating, use random walk to escape
	if g.aiIsOscillating() {
		return g.aiRandomWalk()
	}

	// Check if player has key
	hasKey := g.Player.Keys["rusty_key"]

	// Get key locations from ground items
	keyLocations := g.getKeyLocations()

	// Get item locations with values for path scoring
	itemLocations := g.getItemLocationsWithValues()

	// Get chest locations
	chestLocations := g.getChestLocations()

	// Create pathfinder with current state and item awareness
	pf := NewAIPathfinder(g.Dungeon, keyLocations, hasKey).
		WithItems(itemLocations).
		WithChests(chestLocations)

	// Get next move from Petri net pathfinder
	action := pf.GetNextMove(g.Player.X, g.Player.Y, targetX, targetY)

	if action != "" {
		beforeX, beforeY := g.Player.X, g.Player.Y
		g.ProcessAction(action)
		g.AI.LastAction = string(action) + "_PETRI"
		if beforeX == g.Player.X && beforeY == g.Player.Y {
			g.AI.LastAction = string(action) + "_PETRI_BLOCKED"
			// Path was blocked, fall back to BFS
			return g.aiMoveTowardSmart(targetX, targetY)
		}
		return action
	}

	// Petri net pathfinding failed - maybe we need to find a key first
	if !hasKey && len(keyLocations) > 0 {
		keyPos, keyPath := pf.FindPathToKey(g.Player.X, g.Player.Y)
		if keyPath != nil && len(keyPath) >= 2 {
			// Path to key exists - move toward it
			nextX, nextY := keyPath[1][0], keyPath[1][1]
			dx := nextX - g.Player.X
			dy := nextY - g.Player.Y

			var keyAction ActionType
			switch {
			case dx == 0 && dy == -1:
				keyAction = ActionMoveUp
			case dx == 0 && dy == 1:
				keyAction = ActionMoveDown
			case dx == -1 && dy == 0:
				keyAction = ActionMoveLeft
			case dx == 1 && dy == 0:
				keyAction = ActionMoveRight
			}

			if keyAction != "" {
				g.ProcessAction(keyAction)
				g.AI.LastAction = fmt.Sprintf("to_key_%d_%d", keyPos[0], keyPos[1])
				return keyAction
			}
		}
	}

	// Fall back to regular BFS/smart movement
	return g.aiMoveTowardSmart(targetX, targetY)
}

// AIFindPathPetri uses Petri net reachability to find path (exposed for testing)
func (g *Game) AIFindPathPetri(targetX, targetY int) ActionType {
	return g.aiMoveTowardPetri(targetX, targetY)
}

// aiFindPathBFS uses A* to find a path to target, returns first step direction.
// Despite the name (kept for compatibility), this now uses A* for better performance.
func (g *Game) aiFindPathBFS(targetX, targetY int) ActionType {
	// Already at target
	if g.Player.X == targetX && g.Player.Y == targetY {
		return ""
	}

	// A* node
	type astarNode struct {
		x, y      int
		g         int // Cost from start
		f         int // g + heuristic
		firstMove ActionType
		parent    *astarNode
	}

	// Manhattan distance heuristic
	heuristic := func(x, y int) int {
		dx := x - targetX
		if dx < 0 {
			dx = -dx
		}
		dy := y - targetY
		if dy < 0 {
			dy = -dy
		}
		return dx + dy
	}

	dirs := []struct {
		dx, dy int
		action ActionType
	}{
		{0, -1, ActionMoveUp},
		{0, 1, ActionMoveDown},
		{-1, 0, ActionMoveLeft},
		{1, 0, ActionMoveRight},
	}

	// Check immediate neighbors first (optimization for adjacent targets)
	for _, d := range dirs {
		nx, ny := g.Player.X+d.dx, g.Player.Y+d.dy
		if nx == targetX && ny == targetY && g.aiCanMoveTo(nx, ny) {
			return d.action
		}
	}

	// A* search
	startX, startY := g.Player.X, g.Player.Y
	open := []*astarNode{}
	closed := make(map[[2]int]bool)

	// Initialize with first moves
	for _, d := range dirs {
		nx, ny := startX+d.dx, startY+d.dy
		if g.aiCanMoveTo(nx, ny) {
			newG := 1
			newF := newG + heuristic(nx, ny)
			open = append(open, &astarNode{
				x: nx, y: ny, g: newG, f: newF,
				firstMove: d.action, parent: nil,
			})
		}
	}
	closed[[2]int{startX, startY}] = true

	for len(open) > 0 {
		// Find node with lowest f score
		minIdx := 0
		for i := 1; i < len(open); i++ {
			if open[i].f < open[minIdx].f {
				minIdx = i
			}
		}
		curr := open[minIdx]
		open = append(open[:minIdx], open[minIdx+1:]...)

		if curr.x == targetX && curr.y == targetY {
			return curr.firstMove
		}

		closed[[2]int{curr.x, curr.y}] = true

		for _, d := range dirs {
			nx, ny := curr.x+d.dx, curr.y+d.dy
			key := [2]int{nx, ny}

			if closed[key] || !g.aiCanMoveTo(nx, ny) {
				continue
			}

			newG := curr.g + 1
			newF := newG + heuristic(nx, ny)

			// Check if already in open set with better score
			inOpen := false
			for _, n := range open {
				if n.x == nx && n.y == ny {
					inOpen = true
					if newG < n.g {
						n.g = newG
						n.f = newF
					}
					break
				}
			}

			if !inOpen {
				open = append(open, &astarNode{
					x: nx, y: ny, g: newG, f: newF,
					firstMove: curr.firstMove, parent: curr,
				})
			}
		}
	}

	// No path found
	return ""
}

// aiFindPathToAdjacentBFS uses A* to find a path to get adjacent to target (for attacking enemies).
// Despite the name (kept for compatibility), this now uses A* for better performance.
// Unlike aiFindPathBFS, this ignores enemy positions during traversal and stops when adjacent to target.
func (g *Game) aiFindPathToAdjacentBFS(targetX, targetY int) ActionType {
	// Already adjacent to target
	dx := abs(g.Player.X - targetX)
	dy := abs(g.Player.Y - targetY)
	if dx+dy == 1 {
		return ""
	}

	// A* node
	type astarNode struct {
		x, y      int
		g         int // Cost from start
		f         int // g + heuristic
		firstMove ActionType
	}

	// Heuristic: distance to any adjacent cell of target
	heuristic := func(x, y int) int {
		// Minimum distance to any of the 4 adjacent cells
		minDist := 9999
		for _, adj := range [][2]int{{0, -1}, {0, 1}, {-1, 0}, {1, 0}} {
			ax, ay := targetX+adj[0], targetY+adj[1]
			dist := abs(x-ax) + abs(y-ay)
			if dist < minDist {
				minDist = dist
			}
		}
		return minDist
	}

	dirs := []struct {
		dx, dy int
		action ActionType
	}{
		{0, -1, ActionMoveUp},
		{0, 1, ActionMoveDown},
		{-1, 0, ActionMoveLeft},
		{1, 0, ActionMoveRight},
	}

	// Check immediate neighbors first
	for _, d := range dirs {
		nx, ny := g.Player.X+d.dx, g.Player.Y+d.dy
		if g.aiCanMoveToIgnoringEnemies(nx, ny) {
			tdx := abs(nx - targetX)
			tdy := abs(ny - targetY)
			if tdx+tdy == 1 {
				return d.action
			}
		}
	}

	// A* search
	startX, startY := g.Player.X, g.Player.Y
	open := []*astarNode{}
	closed := make(map[[2]int]bool)

	// Initialize with first moves
	for _, d := range dirs {
		nx, ny := startX+d.dx, startY+d.dy
		if g.aiCanMoveToIgnoringEnemies(nx, ny) {
			newG := 1
			newF := newG + heuristic(nx, ny)
			open = append(open, &astarNode{
				x: nx, y: ny, g: newG, f: newF,
				firstMove: d.action,
			})
		}
	}
	closed[[2]int{startX, startY}] = true

	for len(open) > 0 {
		// Find node with lowest f score
		minIdx := 0
		for i := 1; i < len(open); i++ {
			if open[i].f < open[minIdx].f {
				minIdx = i
			}
		}
		curr := open[minIdx]
		open = append(open[:minIdx], open[minIdx+1:]...)

		// Check if adjacent to target
		tdx := abs(curr.x - targetX)
		tdy := abs(curr.y - targetY)
		if tdx+tdy == 1 {
			return curr.firstMove
		}

		closed[[2]int{curr.x, curr.y}] = true

		for _, d := range dirs {
			nx, ny := curr.x+d.dx, curr.y+d.dy
			key := [2]int{nx, ny}

			if closed[key] || !g.aiCanMoveToIgnoringEnemies(nx, ny) {
				continue
			}

			newG := curr.g + 1
			newF := newG + heuristic(nx, ny)

			// Check if already in open set with better score
			inOpen := false
			for _, n := range open {
				if n.x == nx && n.y == ny {
					inOpen = true
					if newG < n.g {
						n.g = newG
						n.f = newF
					}
					break
				}
			}

			if !inOpen {
				open = append(open, &astarNode{
					x: nx, y: ny, g: newG, f: newF,
					firstMove: curr.firstMove,
				})
			}
		}
	}

	// No path found
	return ""
}

// aiCanMoveToIgnoringEnemies checks if a tile is walkable, ignoring enemy positions
// Used for pathfinding to get adjacent to enemies for combat
func (g *Game) aiCanMoveToIgnoringEnemies(x, y int) bool {
	if x < 0 || x >= g.Dungeon.Width || y < 0 || y >= g.Dungeon.Height {
		return false
	}
	tile := g.Dungeon.Tiles[y][x]

	// Handle locked doors - only passable if we have the key
	if tile == TileLockedDoor {
		if g.Player.Keys["rusty_key"] {
			return true // We can open it
		}
		return false
	}

	// Check walkable tiles (including water/lava which hurt but are passable)
	return tile == TileFloor || tile == TileDoor || tile == TileStairsUp || tile == TileStairsDown || tile == TileWater || tile == TileLava
}

// Helper functions for AI

// aiRecordPosition adds current position to recent history for oscillation detection
func (g *Game) aiRecordPosition() {
	pos := [2]int{g.Player.X, g.Player.Y}
	g.AI.RecentPos = append(g.AI.RecentPos, pos)
	// Keep only last 10 positions
	if len(g.AI.RecentPos) > 10 {
		g.AI.RecentPos = g.AI.RecentPos[1:]
	}
}

// aiIsOscillating checks if we're oscillating between 2-3 positions
func (g *Game) aiIsOscillating() bool {
	if len(g.AI.RecentPos) < 6 {
		return false
	}
	// Check last 6 positions - if we only visit 2-3 unique positions, we're oscillating
	uniquePos := make(map[[2]int]int)
	for _, pos := range g.AI.RecentPos[len(g.AI.RecentPos)-6:] {
		uniquePos[pos]++
	}
	return len(uniquePos) <= 3
}

// aiAvoidPosition checks if we should avoid going to a position to break oscillation
func (g *Game) aiAvoidPosition(x, y int) bool {
	if !g.aiIsOscillating() {
		return false
	}
	// If oscillating, avoid positions we've visited recently
	pos := [2]int{x, y}
	for _, recent := range g.AI.RecentPos {
		if recent == pos {
			return true
		}
	}
	return false
}

// turnToward makes the player face toward a target position
func (g *Game) turnToward(targetX, targetY int) {
	dx := targetX - g.Player.X
	dy := targetY - g.Player.Y

	// Normalize to -1, 0, or 1
	if dx > 0 {
		g.Player.FacingX = 1
	} else if dx < 0 {
		g.Player.FacingX = -1
	} else {
		g.Player.FacingX = 0
	}

	if dy > 0 {
		g.Player.FacingY = 1
	} else if dy < 0 {
		g.Player.FacingY = -1
	} else {
		g.Player.FacingY = 0
	}

	// If diagonal, prefer the axis with greater distance
	if abs(dx) > abs(dy) {
		g.Player.FacingY = 0
	} else if abs(dy) > abs(dx) {
		g.Player.FacingX = 0
	}
}

func (g *Game) hasHealingItem() bool {
	for _, item := range g.Player.Inventory {
		if item.Type == ItemPotion && item.Effect > 0 {
			return true
		}
	}
	return false
}

func (g *Game) findNearestEnemy(maxDist int) *Enemy {
	var nearest *Enemy
	nearestDist := maxDist + 1

	for _, e := range g.Enemies {
		if e.State == StateDead {
			continue
		}
		dx := abs(g.Player.X - e.X)
		dy := abs(g.Player.Y - e.Y)
		dist := dx + dy
		if dist < nearestDist {
			nearest = e
			nearestDist = dist
		}
	}
	return nearest
}

// findAdjacentEnemy finds an enemy that is truly adjacent and attackable.
// Excludes enemies stuck in walls.
func (g *Game) findAdjacentEnemy() *Enemy {
	dirs := [][2]int{{0, -1}, {0, 1}, {-1, 0}, {1, 0}}
	for _, e := range g.Enemies {
		if e.State == StateDead {
			continue
		}
		// Skip enemies stuck in walls
		if g.Dungeon.Tiles[e.Y][e.X] == TileWall {
			continue
		}
		for _, d := range dirs {
			if g.Player.X+d[0] == e.X && g.Player.Y+d[1] == e.Y {
				return e
			}
		}
	}
	return nil
}

// countAdjacentEnemies counts living enemies in adjacent tiles
func (g *Game) countAdjacentEnemies() int {
	dirs := [][2]int{{0, -1}, {0, 1}, {-1, 0}, {1, 0}}
	count := 0
	for _, e := range g.Enemies {
		if e.State == StateDead {
			continue
		}
		if g.Dungeon.Tiles[e.Y][e.X] == TileWall {
			continue
		}
		for _, d := range dirs {
			if g.Player.X+d[0] == e.X && g.Player.Y+d[1] == e.Y {
				count++
				break
			}
		}
	}
	return count
}

// countEnemiesInRange counts living enemies within Manhattan distance
func (g *Game) countEnemiesInRange(maxDist int) int {
	count := 0
	for _, e := range g.Enemies {
		if e.State == StateDead {
			continue
		}
		if g.Dungeon.Tiles[e.Y][e.X] == TileWall {
			continue
		}
		dist := abs(g.Player.X-e.X) + abs(g.Player.Y-e.Y)
		if dist <= maxDist {
			count++
		}
	}
	return count
}

// aiFindSaferPath finds a move direction that leads away from threat concentrations
// Returns empty string if no safer path exists
func (g *Game) aiFindSaferPath(threatX, threatY int) ActionType {
	dirs := []struct {
		dx, dy int
		action ActionType
	}{
		{0, -1, ActionMoveUp},
		{0, 1, ActionMoveDown},
		{-1, 0, ActionMoveLeft},
		{1, 0, ActionMoveRight},
	}

	type pathOption struct {
		action       ActionType
		threatScore  int // Lower is better
		towardExit   bool
	}

	var options []pathOption

	for _, d := range dirs {
		nx, ny := g.Player.X+d.dx, g.Player.Y+d.dy

		// Check if we can move there
		if !g.canMoveTo(nx, ny) {
			continue
		}

		// Calculate threat score at new position
		threatScore := 0
		for _, e := range g.Enemies {
			if e.State == StateDead {
				continue
			}
			if g.Dungeon.Tiles[e.Y][e.X] == TileWall {
				continue
			}
			dist := abs(nx-e.X) + abs(ny-e.Y)
			if dist == 0 {
				threatScore += 100 // On enemy
			} else if dist == 1 {
				threatScore += 50 // Adjacent
			} else if dist <= 3 {
				threatScore += 20 // Very close
			} else if dist <= 5 {
				threatScore += 5 // Close
			}
		}

		// Check if moving toward exit
		currentExitDist := abs(g.Player.X-g.Dungeon.ExitX) + abs(g.Player.Y-g.Dungeon.ExitY)
		newExitDist := abs(nx-g.Dungeon.ExitX) + abs(ny-g.Dungeon.ExitY)
		towardExit := newExitDist < currentExitDist

		options = append(options, pathOption{
			action:      d.action,
			threatScore: threatScore,
			towardExit:  towardExit,
		})
	}

	if len(options) == 0 {
		return ""
	}

	// Find the option with lowest threat score
	// Prefer paths toward exit as tiebreaker
	best := options[0]
	for _, opt := range options[1:] {
		if opt.threatScore < best.threatScore ||
			(opt.threatScore == best.threatScore && opt.towardExit && !best.towardExit) {
			best = opt
		}
	}

	// Current threat score
	currentThreat := 0
	for _, e := range g.Enemies {
		if e.State == StateDead {
			continue
		}
		dist := abs(g.Player.X-e.X) + abs(g.Player.Y-e.Y)
		if dist == 1 {
			currentThreat += 50
		} else if dist <= 3 {
			currentThreat += 20
		} else if dist <= 5 {
			currentThreat += 5
		}
	}

	// Only use if it significantly improves situation
	if best.threatScore < currentThreat-10 {
		return best.action
	}

	return ""
}

// aiFindEscapeRoute finds a direction to move that has no adjacent enemies
// Returns empty string if no escape route exists
func (g *Game) aiFindEscapeRoute() ActionType {
	dirs := []struct {
		dx, dy int
		action ActionType
	}{
		{0, -1, ActionMoveUp},
		{0, 1, ActionMoveDown},
		{-1, 0, ActionMoveLeft},
		{1, 0, ActionMoveRight},
	}

	type escape struct {
		action      ActionType
		enemyCount  int
		distToEnemy int
	}

	var escapes []escape

	for _, d := range dirs {
		nx, ny := g.Player.X+d.dx, g.Player.Y+d.dy

		// Check if we can move there
		if !g.canMoveTo(nx, ny) {
			continue
		}

		// Count how many enemies would be adjacent after this move
		adjCount := 0
		minDist := 1000
		for _, e := range g.Enemies {
			if e.State == StateDead {
				continue
			}
			if g.Dungeon.Tiles[e.Y][e.X] == TileWall {
				continue
			}
			dx := abs(nx - e.X)
			dy := abs(ny - e.Y)
			dist := dx + dy
			if dist == 1 {
				adjCount++
			}
			if dist < minDist {
				minDist = dist
			}
		}

		escapes = append(escapes, escape{
			action:      d.action,
			enemyCount:  adjCount,
			distToEnemy: minDist,
		})
	}

	if len(escapes) == 0 {
		return ""
	}

	// Find the escape with fewest adjacent enemies, breaking ties by distance
	best := escapes[0]
	for _, e := range escapes[1:] {
		if e.enemyCount < best.enemyCount ||
			(e.enemyCount == best.enemyCount && e.distToEnemy > best.distToEnemy) {
			best = e
		}
	}

	// Only use if it's actually an improvement (fewer adjacent enemies)
	currentAdj := g.countAdjacentEnemies()
	if best.enemyCount < currentAdj {
		return best.action
	}

	return ""
}

// findNearestReachableEnemy finds the nearest enemy that can be reached via pathfinding.
// Excludes enemies stuck in walls.
func (g *Game) findNearestReachableEnemy(maxDist int) *Enemy {
	var nearest *Enemy
	nearestDist := maxDist + 1

	for _, e := range g.Enemies {
		if e.State == StateDead {
			continue
		}
		// Skip enemies stuck in walls
		if g.Dungeon.Tiles[e.Y][e.X] == TileWall {
			continue
		}
		dx := abs(g.Player.X - e.X)
		dy := abs(g.Player.Y - e.Y)
		dist := dx + dy
		if dist >= nearestDist {
			continue
		}
		// Check if we can actually path to this enemy
		if g.canPathTo(e.X, e.Y) {
			nearest = e
			nearestDist = dist
		}
	}
	return nearest
}

// canPathTo checks if a path exists to the target position.
func (g *Game) canPathTo(targetX, targetY int) bool {
	// Quick BFS to check reachability
	visited := make(map[[2]int]bool)
	queue := [][2]int{{g.Player.X, g.Player.Y}}
	visited[[2]int{g.Player.X, g.Player.Y}] = true

	dirs := [][2]int{{0, -1}, {0, 1}, {-1, 0}, {1, 0}}

	for len(queue) > 0 && len(visited) < 200 {
		pos := queue[0]
		queue = queue[1:]

		// Check if we're adjacent to target (can attack from here)
		for _, d := range dirs {
			if pos[0]+d[0] == targetX && pos[1]+d[1] == targetY {
				return true
			}
		}

		for _, d := range dirs {
			nx, ny := pos[0]+d[0], pos[1]+d[1]
			key := [2]int{nx, ny}
			if visited[key] {
				continue
			}
			if nx < 0 || nx >= g.Dungeon.Width || ny < 0 || ny >= g.Dungeon.Height {
				continue
			}

			tile := g.Dungeon.Tiles[ny][nx]
			walkable := tile == TileFloor || tile == TileDoor ||
				tile == TileStairsUp || tile == TileStairsDown ||
				tile == TileWater || tile == TileLava

			if tile == TileLockedDoor && g.Player.Keys["rusty_key"] {
				walkable = true
			}

			if !walkable {
				continue
			}

			visited[key] = true
			queue = append(queue, key)
		}
	}
	return false
}

func (g *Game) findNearestItem(maxDist int) *GroundItem {
	var nearest *GroundItem
	nearestDist := maxDist + 1

	for _, item := range g.Items {
		dx := abs(g.Player.X - item.X)
		dy := abs(g.Player.Y - item.Y)
		dist := dx + dy
		if dist < nearestDist {
			nearest = item
			nearestDist = dist
		}
	}
	return nearest
}

// findNearestChest finds the nearest chest tile and returns its coordinates.
// Returns (-1, -1) if no chest found.
func (g *Game) findNearestChest() (int, int) {
	nearestX, nearestY := -1, -1
	nearestDist := 1000

	for y := 0; y < g.Dungeon.Height; y++ {
		for x := 0; x < g.Dungeon.Width; x++ {
			if g.Dungeon.Tiles[y][x] == TileChest {
				dist := abs(g.Player.X-x) + abs(g.Player.Y-y)
				if dist < nearestDist {
					nearestDist = dist
					nearestX, nearestY = x, y
				}
			}
		}
	}
	return nearestX, nearestY
}

// findNearestReachableChest finds the nearest chest that is actually reachable via BFS.
// Returns (-1, -1) if no reachable chest found.
func (g *Game) findNearestReachableChest() (int, int) {
	// Collect all chests
	var chests [][2]int
	for y := 0; y < g.Dungeon.Height; y++ {
		for x := 0; x < g.Dungeon.Width; x++ {
			if g.Dungeon.Tiles[y][x] == TileChest {
				chests = append(chests, [2]int{x, y})
			}
		}
	}

	if len(chests) == 0 {
		return -1, -1
	}

	// Sort by Manhattan distance
	for i := 0; i < len(chests)-1; i++ {
		for j := i + 1; j < len(chests); j++ {
			distI := abs(g.Player.X-chests[i][0]) + abs(g.Player.Y-chests[i][1])
			distJ := abs(g.Player.X-chests[j][0]) + abs(g.Player.Y-chests[j][1])
			if distJ < distI {
				chests[i], chests[j] = chests[j], chests[i]
			}
		}
	}

	// Check reachability in order of distance
	for _, chest := range chests {
		// Try to find path to adjacent tile (can't walk ON chest, need to be adjacent)
		for _, d := range [][2]int{{0, -1}, {0, 1}, {-1, 0}, {1, 0}} {
			adjX, adjY := chest[0]+d[0], chest[1]+d[1]
			if adjX >= 0 && adjX < g.Dungeon.Width && adjY >= 0 && adjY < g.Dungeon.Height {
				tile := g.Dungeon.Tiles[adjY][adjX]
				if tile == TileFloor || tile == TileDoor {
					// Check if we can path to this adjacent tile
					if action := g.aiFindPathBFS(adjX, adjY); action != "" {
						return chest[0], chest[1]
					}
				}
			}
		}
	}

	return -1, -1
}

// hasHealingPotion checks if the player has a healing potion in their inventory
func (g *Game) hasHealingPotion() bool {
	for _, item := range g.Player.Inventory {
		if item.Type == ItemPotion && item.Effect > 0 {
			return true
		}
	}
	return false
}

// countPotions returns the number of healing potions in inventory
func (g *Game) countPotions() int {
	count := 0
	for _, item := range g.Player.Inventory {
		if item.Type == ItemPotion && item.Effect > 0 {
			count++
		}
	}
	return count
}

func (g *Game) findNearestPotion(maxDist int) *GroundItem {
	var nearest *GroundItem
	nearestDist := maxDist + 1

	for _, item := range g.Items {
		if item.Item.Type != ItemPotion {
			continue
		}
		dx := abs(g.Player.X - item.X)
		dy := abs(g.Player.Y - item.Y)
		dist := dx + dy
		if dist < nearestDist {
			nearest = item
			nearestDist = dist
		}
	}
	return nearest
}

func (g *Game) findNearestNPC(maxDist int) *NPC {
	var nearest *NPC
	nearestDist := maxDist + 1

	for _, npc := range g.NPCs {
		dx := abs(g.Player.X - npc.X)
		dy := abs(g.Player.Y - npc.Y)
		dist := dx + dy
		if dist < nearestDist {
			nearest = npc
			nearestDist = dist
		}
	}
	return nearest
}

// findNearestPotionSeller finds the nearest merchant or healer that sells potions
// and the player can afford. Returns nil if none found within maxDist.
func (g *Game) findNearestPotionSeller(maxDist int) *NPC {
	var nearest *NPC
	nearestDist := maxDist + 1

	// Minimum gold needed for cheapest potion (healers sell for 20)
	minPotionCost := 20

	for _, npc := range g.NPCs {
		// Only merchants and healers sell potions
		if npc.Type != NPCMerchant && npc.Type != NPCHealer {
			continue
		}

		// Skip if already talked to and marked unreachable
		if g.AI.GoalsComplete["unreachable_npc_"+npc.ID] {
			continue
		}

		// Check if NPC has potions in inventory
		hasPotion := false
		for _, item := range npc.Inventory {
			if item.Type == ItemPotion && item.Effect > 0 && g.Player.Gold >= item.Value {
				hasPotion = true
				break
			}
		}
		if !hasPotion {
			continue
		}

		// Player needs enough gold
		if g.Player.Gold < minPotionCost {
			continue
		}

		dx := abs(g.Player.X - npc.X)
		dy := abs(g.Player.Y - npc.Y)
		dist := dx + dy
		if dist < nearestDist {
			nearest = npc
			nearestDist = dist
		}
	}
	return nearest
}

func (g *Game) isOnStairsDown() bool {
	return g.Dungeon.Tiles[g.Player.Y][g.Player.X] == TileStairsDown
}

// aiCanMoveTo checks if AI can move to a tile, considering locked doors
func (g *Game) aiCanMoveTo(x, y int) bool {
	if x < 0 || x >= g.Dungeon.Width || y < 0 || y >= g.Dungeon.Height {
		return false
	}
	tile := g.Dungeon.Tiles[y][x]

	// Handle locked doors - only passable if we have the key
	if tile == TileLockedDoor {
		if g.Player.Keys["rusty_key"] {
			return true // We can open it
		}
		// Remember this locked door
		g.aiRememberLockedDoor(x, y)
		return false
	}

	// Check walkable tiles (including water/lava which hurt but are passable)
	if !(tile == TileFloor || tile == TileDoor || tile == TileStairsUp || tile == TileStairsDown || tile == TileWater || tile == TileLava) {
		return false
	}

	// Check for enemy blocking the way
	if enemy := g.getEnemyAt(x, y); enemy != nil && enemy.State != StateDead {
		return false
	}

	return true
}

// aiRememberLockedDoor adds a locked door position to memory
func (g *Game) aiRememberLockedDoor(x, y int) {
	if g.AI.AvoidDoors == nil {
		g.AI.AvoidDoors = make(map[[2]int]bool)
	}
	pos := [2]int{x, y}
	if !g.AI.AvoidDoors[pos] {
		g.AI.AvoidDoors[pos] = true
		g.AI.LockedDoors = append(g.AI.LockedDoors, pos)
	}
}

// aiHasKey checks if AI has a key that could open locked doors
func (g *Game) aiHasKey() bool {
	return g.Player.Keys["rusty_key"]
}

// aiNeedsKey checks if there's a locked door blocking progress and no key
func (g *Game) aiNeedsKey() bool {
	return len(g.AI.LockedDoors) > 0 && !g.aiHasKey()
}

// aiFindKey looks for a key item on the ground
func (g *Game) aiFindKey() *GroundItem {
	for _, item := range g.Items {
		if item.Item.Type == ItemKey {
			return item
		}
	}
	return nil
}

// CacheStatsReport contains aggregated cache statistics for instrumentation.
type CacheStatsReport struct {
	Tick          int     `json:"tick"`
	Level         int     `json:"level"`
	BrainHits     int64   `json:"brain_hits"`
	BrainMisses   int64   `json:"brain_misses"`
	BrainHitRate  float64 `json:"brain_hit_rate"`
	BrainSize     int     `json:"brain_size"`
	CombatHits    int64   `json:"combat_hits"`
	CombatMisses  int64   `json:"combat_misses"`
	CombatHitRate float64 `json:"combat_hit_rate"`
	CombatSize    int     `json:"combat_size"`
}

// GetCacheStats returns current cache statistics for both brain and combat evaluators.
func (g *Game) GetCacheStats() CacheStatsReport {
	report := CacheStatsReport{
		Tick:  g.AI.ActionCount,
		Level: g.Level,
	}

	if g.AI.Brain != nil {
		if stats := g.AI.Brain.CacheStats(); stats != nil {
			report.BrainHits = stats.Hits
			report.BrainMisses = stats.Misses
			report.BrainHitRate = stats.HitRate
			report.BrainSize = stats.Size
		}
	}

	if g.AI.CombatEvaluator != nil {
		if stats := g.AI.CombatEvaluator.CacheStats(); stats != nil {
			report.CombatHits = stats.Hits
			report.CombatMisses = stats.Misses
			report.CombatHitRate = stats.HitRate
			report.CombatSize = stats.Size
		}
	}

	return report
}

// LogCacheStats prints cache statistics to stdout for instrumentation.
func (g *Game) LogCacheStats() {
	stats := g.GetCacheStats()
	fmt.Printf("[CACHE] tick=%d level=%d | brain: hits=%d misses=%d rate=%.1f%% size=%d | combat: hits=%d misses=%d rate=%.1f%% size=%d\n",
		stats.Tick, stats.Level,
		stats.BrainHits, stats.BrainMisses, stats.BrainHitRate*100, stats.BrainSize,
		stats.CombatHits, stats.CombatMisses, stats.CombatHitRate*100, stats.CombatSize)
}

// EnableCacheInstrumentation enables periodic cache stats logging.
// interval specifies how often (in ticks) to log stats. Use 0 to disable.
func (g *Game) EnableCacheInstrumentation(interval int) {
	g.AI.CacheStatsInterval = interval
	g.AI.LastCacheLog = 0
}

// ClearAllCaches clears both brain and combat evaluator caches.
// Call this on level transitions to prevent stale cache entries.
func (g *Game) ClearAllCaches() {
	if g.AI.Brain != nil {
		g.AI.Brain.ClearCache()
	}
	if g.AI.CombatEvaluator != nil {
		g.AI.CombatEvaluator.ClearCache()
	}
}
