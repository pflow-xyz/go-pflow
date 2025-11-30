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
)

// Enemy represents a hostile creature
type Enemy struct {
	ID        string     `json:"id"`
	Type      EnemyType  `json:"type"`
	Name      string     `json:"name"`
	X         int        `json:"x"`
	Y         int        `json:"y"`
	Health    int        `json:"health"`
	MaxHealth int        `json:"max_health"`
	Damage    int        `json:"damage"`
	XP        int        `json:"xp"`
	State     EnemyState `json:"state"`
	AlertDist int        `json:"alert_dist"`
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
}

// EnemyView is the client-visible enemy data
type EnemyView struct {
	ID     string `json:"id"`
	Type   int    `json:"type"`
	Name   string `json:"name"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Health int    `json:"health"`
	State  int    `json:"state"`
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
}{
	EnemySkeleton: {"Skeleton", 20, 5, 10, 5},
	EnemyZombie:   {"Zombie", 30, 8, 15, 4},
	EnemyGhost:    {"Ghost", 15, 10, 20, 7},
	EnemySpider:   {"Giant Spider", 12, 6, 8, 6},
	EnemyBat:      {"Bat", 8, 3, 5, 8},
	EnemyRat:      {"Giant Rat", 10, 4, 5, 5},
	EnemyOrc:      {"Orc", 40, 12, 25, 6},
	EnemyTroll:    {"Troll", 60, 15, 40, 5},
	EnemyLich:     {"Lich King", 200, 30, 500, 10},
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
			ID:        fmt.Sprintf("enemy_%d", i),
			Type:      enemyType,
			Name:      template.Name,
			X:         x,
			Y:         y,
			Health:    int(float64(template.Health) * healthMod),
			MaxHealth: int(float64(template.Health) * healthMod),
			Damage:    int(float64(template.Damage) * damageMod),
			XP:        template.XP,
			State:     StateIdle,
			AlertDist: template.AlertDist,
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

		// Offset to avoid overlap
		for g.getEnemyAt(x, y) != nil || g.getNPCAt(x, y) != nil {
			x = room.X + 1 + g.rng.Intn(room.Width-2)
			y = room.Y + 1 + g.rng.Intn(room.Height-2)
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
	return tile == TileFloor || tile == TileDoor || tile == TileStairsUp || tile == TileStairsDown
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
			ID:     e.ID,
			Type:   int(e.Type),
			Name:   e.Name,
			X:      e.X,
			Y:      e.Y,
			Health: e.Health,
			State:  int(e.State),
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

	return state
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
