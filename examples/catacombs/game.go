// Package catacombs implements a roguelike dungeon crawler using Petri nets.
package catacombs

import (
	"fmt"
	"math"
	"math/rand"
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
	rng           *rand.Rand
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
		rng:        rng,
	}

	// Generate first dungeon level
	g.Dungeon = GenerateDungeon(params, 1)
	g.Player.X = g.Dungeon.SpawnX
	g.Player.Y = g.Dungeon.SpawnY

	// Populate with enemies
	g.populateEnemies(params)

	// Add NPCs
	g.populateNPCs(params)

	// Add ground items
	g.populateItems(params)

	// Build Petri net for resource tracking
	g.buildPetriNet()

	g.addMessage("You descend into the Catacombs of Pflow...")

	return g
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

			g.Items = append(g.Items, &GroundItem{
				Item: item,
				X:    x,
				Y:    y,
			})
		}
	}
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

// attemptFlee tries to escape combat
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
		g.EndCombat()
		return nil
	}

	g.Combat.CurrentAP -= APCostMove * 2
	g.addCombatLog("Failed to flee! You stumble and lose AP.")
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
	g.DialogueNode = "start"
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
			}
			break
		}
	}

	return nil
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

	// Victory check
	if g.Level >= 10 {
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
		Enabled:       true,
		Mode:          "explore",
		ThinkDelay:    0,
		GoalsComplete: make(map[string]bool),
	}
	g.addMessage("AI Mode enabled - watching the adventure unfold...")
}

// DisableAI turns off AI player mode
func (g *Game) DisableAI() {
	g.AI.Enabled = false
	g.addMessage("AI Mode disabled - you're in control!")
}

// AITick performs one AI decision/action cycle
// Returns the action taken (or empty string if waiting)
func (g *Game) AITick() ActionType {
	if !g.AI.Enabled || g.GameOver {
		return ""
	}

	g.AI.ActionCount++

	// Track if we're stuck in the same position
	if g.Player.X == g.AI.LastX && g.Player.Y == g.AI.LastY {
		g.AI.StuckCounter++
	} else {
		g.AI.StuckCounter = 0
	}
	g.AI.LastX = g.Player.X
	g.AI.LastY = g.Player.Y

	// Handle dialogue with AI
	if g.InDialogue {
		return g.aiHandleDialogue()
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
		return g.aiEngageCombat()
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

// aiDecideMode chooses what the AI should focus on
func (g *Game) aiDecideMode() {
	// Priority 0: Fight adjacent enemies (in combat or attacking us)
	adjacentEnemy := g.findNearestEnemy(1)
	if adjacentEnemy != nil {
		g.AI.Mode = "combat"
		g.AI.Target = adjacentEnemy.ID
		return
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
	if nearbyEnemy != nil && nearbyEnemy.State == StateChasing {
		g.AI.Mode = "combat"
		g.AI.Target = nearbyEnemy.ID
		return
	}

	// Priority 3: Pick up nearby items (prioritize keys if we need one)
	nearbyItem := g.findNearestItem(3)
	if nearbyItem != nil {
		g.AI.Mode = "loot"
		return
	}

	// Priority 3.5: If we found locked doors and don't have a key, look for keys
	if g.aiNeedsKey() {
		keyItem := g.aiFindKey()
		if keyItem != nil {
			g.AI.Mode = "find_key"
			g.AI.Target = fmt.Sprintf("%d,%d", keyItem.X, keyItem.Y)
			return
		}
	}

	// Priority 4: Talk to nearby NPCs we haven't talked to (within 3 tiles only)
	nearbyNPC := g.findNearestNPC(3)
	if nearbyNPC != nil && !g.AI.GoalsComplete["talk_"+nearbyNPC.ID] {
		g.AI.Mode = "interact"
		g.AI.Target = nearbyNPC.ID
		return
	}

	// Priority 5: Fight nearby visible enemies (within 8 tiles) - don't chase across map
	visibleEnemy := g.findNearestEnemy(8)
	if visibleEnemy != nil {
		g.AI.Mode = "combat"
		g.AI.Target = visibleEnemy.ID
		return
	}

	// Default: Explore (find stairs, wander)
	g.AI.Mode = "explore"
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

	// Move toward enemy using smart pathing
	return g.aiMoveTowardSmart(enemy.X, enemy.Y)
}

// aiHandleCombat handles turn-based combat decisions
func (g *Game) aiHandleCombat() ActionType {
	if !g.Combat.PlayerTurn {
		return "" // Wait for enemy turn
	}

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

	// Try to flee if critically low on health
	if g.Player.Health < g.Player.MaxHealth/5 {
		g.ProcessCombatAction(ActionFlee, nil)
		g.AI.LastAction = "flee"
		return ActionFlee
	}

	// Attack if we have AP
	if g.Combat.CurrentAP >= APCostAttack {
		// Pick a target if none selected
		if g.Combat.SelectedEnemy == "" && len(g.Combat.Combatants) > 0 {
			g.SetTargetEnemy(g.Combat.Combatants[0])
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

	// If adjacent, talk
	dx := abs(g.Player.X - npc.X)
	dy := abs(g.Player.Y - npc.Y)
	if dx <= 1 && dy <= 1 {
		// Mark as talked immediately to prevent retrying
		g.AI.GoalsComplete["talk_"+npc.ID] = true
		g.ProcessAction(ActionTalk)
		g.AI.LastAction = "talk"
		g.AI.Mode = "explore" // Done with this NPC
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

// aiLoot moves toward and picks up items
func (g *Game) aiLoot() ActionType {
	item := g.findNearestItem(10)
	if item == nil {
		// No items, switch to explore (will explore on next tick)
		g.AI.Mode = "explore"
		return ""
	}

	// If on item, it's auto-picked up, switch to explore
	if g.Player.X == item.X && g.Player.Y == item.Y {
		g.AI.Mode = "explore"
		return ""
	}

	// Move toward item (non-recursive version)
	return g.aiMoveTowardSimple(item.X, item.Y)
}

// aiFindKeyAction moves toward a known key on the ground
func (g *Game) aiFindKeyAction() ActionType {
	keyItem := g.aiFindKey()
	if keyItem == nil {
		// No key found, explore to find one
		g.AI.Mode = "explore"
		g.AI.LastAction = "searching for key"
		return g.aiRandomWalk()
	}

	// If on key, it's auto-picked up, clear locked doors (we can open them now)
	if g.Player.X == keyItem.X && g.Player.Y == keyItem.Y {
		g.AI.Mode = "explore"
		g.AI.AvoidDoors = nil // Clear avoidance now that we have a key
		g.AI.LastAction = "picked up key"
		return ""
	}

	// Move toward key using smart pathing
	g.AI.LastAction = fmt.Sprintf("moving to key at %d,%d", keyItem.X, keyItem.Y)
	return g.aiMoveTowardSmart(keyItem.X, keyItem.Y)
}

// aiExplore moves toward unexplored areas or stairs
func (g *Game) aiExplore() ActionType {
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
		// If we're on the stairs, descend
		if g.Player.X == stairsX && g.Player.Y == stairsY {
			g.ProcessAction(ActionDescend)
			g.AI.LastAction = "descend"
			return ActionDescend
		}

		// If stuck for too long, try random walk to break out
		if g.AI.StuckCounter > 5 {
			return g.aiRandomWalk()
		}

		// Move toward stairs
		return g.aiMoveTowardSmart(stairsX, stairsY)
	}

	// No stairs found, random walk
	return g.aiRandomWalk()
}

// aiRandomWalk tries random directions
func (g *Game) aiRandomWalk() ActionType {
	directions := []ActionType{ActionMoveUp, ActionMoveDown, ActionMoveLeft, ActionMoveRight}

	// Shuffle directions for randomness
	for i := len(directions) - 1; i > 0; i-- {
		j := g.rng.Intn(i + 1)
		directions[i], directions[j] = directions[j], directions[i]
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

// aiMoveTowardSimple moves one step toward target using smart pathing
func (g *Game) aiMoveTowardSimple(targetX, targetY int) ActionType {
	// Delegate to aiMoveTowardSmart for better pathfinding
	return g.aiMoveTowardSmart(targetX, targetY)
}

// aiMoveTowardSmart moves one step toward target, avoiding locked doors (non-recursive)
func (g *Game) aiMoveTowardSmart(targetX, targetY int) ActionType {
	// Already at target
	if g.Player.X == targetX && g.Player.Y == targetY {
		return ""
	}

	// Try BFS pathfinding first
	if action := g.aiFindPathBFS(targetX, targetY); action != "" {
		g.ProcessAction(action)
		g.AI.LastAction = string(action)
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

// aiFindPathBFS uses BFS to find a path to target, returns first step direction
func (g *Game) aiFindPathBFS(targetX, targetY int) ActionType {
	type node struct {
		x, y      int
		firstMove ActionType
	}

	// Already at target
	if g.Player.X == targetX && g.Player.Y == targetY {
		return ""
	}

	// BFS
	visited := make(map[[2]int]bool)
	queue := []node{}
	dirs := []struct {
		dx, dy int
		action ActionType
	}{
		{0, -1, ActionMoveUp},
		{0, 1, ActionMoveDown},
		{-1, 0, ActionMoveLeft},
		{1, 0, ActionMoveRight},
	}

	// Start from player position
	visited[[2]int{g.Player.X, g.Player.Y}] = true
	for _, d := range dirs {
		nx, ny := g.Player.X+d.dx, g.Player.Y+d.dy
		if g.aiCanMoveTo(nx, ny) {
			if nx == targetX && ny == targetY {
				return d.action
			}
			visited[[2]int{nx, ny}] = true
			queue = append(queue, node{nx, ny, d.action})
		}
	}

	// BFS explore (limit to 500 nodes to avoid performance issues)
	for len(queue) > 0 && len(visited) < 500 {
		current := queue[0]
		queue = queue[1:]

		for _, d := range dirs {
			nx, ny := current.x+d.dx, current.y+d.dy
			key := [2]int{nx, ny}
			if visited[key] {
				continue
			}
			if !g.aiCanMoveTo(nx, ny) {
				continue
			}
			if nx == targetX && ny == targetY {
				return current.firstMove
			}
			visited[key] = true
			queue = append(queue, node{nx, ny, current.firstMove})
		}
	}

	// No path found
	return ""
}

// Helper functions for AI

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
