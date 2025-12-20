// Package doom implements a Doom-like first-person shooter using Petri nets.
//
// The game models a player navigating a 2D map, fighting enemies, collecting
// items, and finding the exit. The Petri net tracks:
// - Player health, ammo, armor, keys
// - Enemy health and state (idle, alert, attacking, dead)
// - Door states (locked, unlocked, open)
// - Item pickups
package doom

import (
	"fmt"
	"math"
	"math/rand"
	"sync"

	"github.com/pflow-xyz/go-pflow/petri"
)

// Game constants
const (
	MaxHealth = 100.0
	MaxArmor  = 100.0
	MaxAmmo   = 50.0

	// Damage values
	PistolDamage  = 15.0
	ShotgunDamage = 40.0
	EnemyDamage   = 10.0

	// Ammo costs
	PistolAmmo  = 1.0
	ShotgunAmmo = 2.0

	// Movement
	MoveSpeed = 0.1
	TurnSpeed = 0.1

	// Map dimensions
	MapWidth  = 16
	MapHeight = 16
)

// Direction constants
const (
	DirNorth = iota
	DirEast
	DirSouth
	DirWest
)

// TileType represents a map tile
type TileType int

const (
	TileFloor TileType = iota
	TileWall
	TileDoor
	TileLockedDoor
	TileExit
)

// EnemyType represents an enemy type
type EnemyType int

const (
	EnemyImp EnemyType = iota
	EnemyDemon
	EnemySoldier
)

// EnemyState represents enemy behavior state
type EnemyState int

const (
	EnemyIdle EnemyState = iota
	EnemyAlert
	EnemyAttacking
	EnemyDead
)

// ItemType represents a pickup item
type ItemType int

const (
	ItemHealth ItemType = iota
	ItemArmor
	ItemAmmo
	ItemShotgun
	ItemKeyRed
	ItemKeyBlue
)

// Enemy represents an enemy in the game
type Enemy struct {
	ID       int        `json:"ID"`
	Type     EnemyType  `json:"Type"`
	X        float64    `json:"X"`
	Y        float64    `json:"Y"`
	Health   float64    `json:"Health"`
	State    EnemyState `json:"State"`
	LastSeen float64    `json:"LastSeen"`
}

// Item represents a pickup item
type Item struct {
	ID     int      `json:"ID"`
	Type   ItemType `json:"Type"`
	X      float64  `json:"X"`
	Y      float64  `json:"Y"`
	Picked bool     `json:"Picked"`
}

// Player represents the player state
type Player struct {
	X          float64 `json:"X"`
	Y          float64 `json:"Y"`
	Angle      float64 `json:"Angle"`
	Health     float64 `json:"Health"`
	Armor      float64 `json:"Armor"`
	Ammo       float64 `json:"Ammo"`
	HasShotgun bool    `json:"HasShotgun"`
	HasKeyRed  bool    `json:"HasKeyRed"`
	HasKeyBlue bool    `json:"HasKeyBlue"`
}

// GameMap represents the level map
type GameMap struct {
	Width, Height int
	Tiles         [][]TileType
	Enemies       []*Enemy
	Items         []*Item
	SpawnX        float64
	SpawnY        float64
	ExitX         float64
	ExitY         float64
}

// GameState represents the observable game state
type GameState struct {
	Player      Player    `json:"player"`
	Enemies     []*Enemy  `json:"enemies"`
	Items       []*Item   `json:"items"`
	MapWidth    int       `json:"map_width"`
	MapHeight   int       `json:"map_height"`
	Tiles       [][]int   `json:"tiles"` // Simplified for JSON
	GameOver    bool      `json:"game_over"`
	Victory     bool      `json:"victory"`
	Message     string    `json:"message,omitempty"`
	KillCount   int       `json:"kill_count"`
	SecretCount int       `json:"secret_count"`
}

// ActionType represents player actions
type ActionType string

const (
	ActionMoveForward  ActionType = "move_forward"
	ActionMoveBackward ActionType = "move_backward"
	ActionStrafeLeft   ActionType = "strafe_left"
	ActionStrafeRight  ActionType = "strafe_right"
	ActionTurnLeft     ActionType = "turn_left"
	ActionTurnRight    ActionType = "turn_right"
	ActionShoot        ActionType = "shoot"
	ActionUse          ActionType = "use" // Open doors, activate switches
)

// Game represents a doom game instance
type Game struct {
	mu sync.RWMutex

	net    *petri.PetriNet
	rates  map[string]float64
	state  map[string]float64

	player    Player
	gameMap   *GameMap
	gameOver  bool
	victory   bool
	message   string
	killCount int
	tick      int
}

// NewGame creates a new doom game
func NewGame() *Game {
	gameMap := GenerateMap(1) // Level 1
	net := BuildDoomNet(gameMap)
	state := InitialState(net, gameMap)
	rates := DefaultRates(net)

	g := &Game{
		net:     net,
		rates:   rates,
		state:   state,
		gameMap: gameMap,
		player: Player{
			X:      gameMap.SpawnX,
			Y:      gameMap.SpawnY,
			Angle:  0,
			Health: MaxHealth,
			Armor:  0,
			Ammo:   20,
		},
	}

	return g
}

// GenerateMap creates a game level
func GenerateMap(level int) *GameMap {
	gm := &GameMap{
		Width:  MapWidth,
		Height: MapHeight,
		Tiles:  make([][]TileType, MapHeight),
	}

	// Initialize with walls
	for y := 0; y < MapHeight; y++ {
		gm.Tiles[y] = make([]TileType, MapWidth)
		for x := 0; x < MapWidth; x++ {
			if x == 0 || x == MapWidth-1 || y == 0 || y == MapHeight-1 {
				gm.Tiles[y][x] = TileWall
			} else {
				gm.Tiles[y][x] = TileFloor
			}
		}
	}

	// Add some internal walls to create rooms
	// Room 1 (spawn room)
	for x := 5; x <= 5; x++ {
		for y := 1; y < 6; y++ {
			gm.Tiles[y][x] = TileWall
		}
	}
	gm.Tiles[3][5] = TileDoor // Door to main hall

	// Room 2 (main hall)
	for x := 5; x <= 10; x++ {
		gm.Tiles[6][x] = TileWall
	}
	gm.Tiles[6][7] = TileDoor // Door to key room

	// Room 3 (key room)
	for y := 6; y < 10; y++ {
		gm.Tiles[y][10] = TileWall
	}
	gm.Tiles[8][10] = TileLockedDoor // Red key door

	// Exit room
	for x := 10; x < 14; x++ {
		gm.Tiles[10][x] = TileWall
	}
	gm.Tiles[10][12] = TileDoor

	// Exit tile
	gm.Tiles[12][12] = TileExit

	// Set spawn and exit
	gm.SpawnX = 2.5
	gm.SpawnY = 2.5
	gm.ExitX = 12.5
	gm.ExitY = 12.5

	// Add enemies
	gm.Enemies = []*Enemy{
		{ID: 0, Type: EnemyImp, X: 7.5, Y: 3.5, Health: 30, State: EnemyIdle},
		{ID: 1, Type: EnemySoldier, X: 8.5, Y: 8.5, Health: 40, State: EnemyIdle},
		{ID: 2, Type: EnemyDemon, X: 12.5, Y: 8.5, Health: 60, State: EnemyIdle},
	}

	// Add items
	gm.Items = []*Item{
		{ID: 0, Type: ItemHealth, X: 3.5, Y: 4.5},
		{ID: 1, Type: ItemAmmo, X: 6.5, Y: 2.5},
		{ID: 2, Type: ItemKeyRed, X: 8.5, Y: 7.5},
		{ID: 3, Type: ItemShotgun, X: 11.5, Y: 9.5},
		{ID: 4, Type: ItemArmor, X: 13.5, Y: 11.5},
	}

	return gm
}

// BuildDoomNet constructs the Petri net for the doom game
func BuildDoomNet(gm *GameMap) *petri.PetriNet {
	net := petri.NewPetriNet()

	// Player resource places
	net.AddPlace("player_health", MaxHealth, nil, 100, 50, nil)
	net.AddPlace("player_armor", 0, nil, 100, 100, nil)
	net.AddPlace("player_ammo", 20, nil, 100, 150, nil)
	net.AddPlace("player_has_shotgun", 0, nil, 100, 200, nil)
	net.AddPlace("player_has_key_red", 0, nil, 100, 250, nil)
	net.AddPlace("player_has_key_blue", 0, nil, 100, 300, nil)

	// Kill tracking
	net.AddPlace("kill_count", 0, nil, 100, 350, nil)

	// Enemy places (health for each enemy)
	for _, enemy := range gm.Enemies {
		placeName := fmt.Sprintf("enemy_%d_health", enemy.ID)
		net.AddPlace(placeName, enemy.Health, nil, 200+float64(enemy.ID)*50, 100, nil)

		// Enemy state places
		net.AddPlace(fmt.Sprintf("enemy_%d_alive", enemy.ID), 1, nil, 200+float64(enemy.ID)*50, 150, nil)
	}

	// Item places (1 = available, 0 = picked up)
	for _, item := range gm.Items {
		placeName := fmt.Sprintf("item_%d_available", item.ID)
		net.AddPlace(placeName, 1, nil, 300+float64(item.ID)*50, 200, nil)
	}

	// Door places
	net.AddPlace("door_main_open", 0, nil, 400, 100, nil)
	net.AddPlace("door_key_open", 0, nil, 400, 150, nil)
	net.AddPlace("door_exit_open", 0, nil, 400, 200, nil)

	// Player action transitions
	net.AddTransition("shoot_pistol", "default", 150, 100, nil)
	net.AddTransition("shoot_shotgun", "default", 150, 150, nil)
	net.AddTransition("take_damage", "default", 150, 200, nil)

	// Pickup transitions
	for _, item := range gm.Items {
		transName := fmt.Sprintf("pickup_item_%d", item.ID)
		net.AddTransition(transName, "default", 350+float64(item.ID)*50, 200, nil)
	}

	// Arcs for shooting
	net.AddArc("player_ammo", "shoot_pistol", PistolAmmo, false)
	net.AddArc("player_ammo", "shoot_shotgun", ShotgunAmmo, false)
	net.AddArc("player_has_shotgun", "shoot_shotgun", 1, false)
	net.AddArc("shoot_shotgun", "player_has_shotgun", 1, false) // Return shotgun

	// Damage arcs
	net.AddArc("player_health", "take_damage", EnemyDamage, false)

	return net
}

// InitialState returns the initial game state
func InitialState(net *petri.PetriNet, gm *GameMap) map[string]float64 {
	state := net.SetState(nil)
	return state
}

// DefaultRates returns default transition rates
func DefaultRates(net *petri.PetriNet) map[string]float64 {
	rates := make(map[string]float64)
	for trans := range net.Transitions {
		rates[trans] = 1.0
	}
	return rates
}

// GetState returns the current game state
func (g *Game) GetState() GameState {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Convert tiles to int array for JSON
	tiles := make([][]int, g.gameMap.Height)
	for y := 0; y < g.gameMap.Height; y++ {
		tiles[y] = make([]int, g.gameMap.Width)
		for x := 0; x < g.gameMap.Width; x++ {
			tiles[y][x] = int(g.gameMap.Tiles[y][x])
		}
	}

	return GameState{
		Player:    g.player,
		Enemies:   g.gameMap.Enemies,
		Items:     g.gameMap.Items,
		MapWidth:  g.gameMap.Width,
		MapHeight: g.gameMap.Height,
		Tiles:     tiles,
		GameOver:  g.gameOver,
		Victory:   g.victory,
		Message:   g.message,
		KillCount: g.killCount,
	}
}

// GetAvailableActions returns valid actions
func (g *Game) GetAvailableActions() []ActionType {
	g.mu.RLock()
	defer g.mu.RUnlock()

	actions := []ActionType{
		ActionMoveForward,
		ActionMoveBackward,
		ActionStrafeLeft,
		ActionStrafeRight,
		ActionTurnLeft,
		ActionTurnRight,
		ActionUse,
	}

	// Can shoot if has ammo
	if g.player.Ammo >= PistolAmmo {
		actions = append(actions, ActionShoot)
	}

	return actions
}

// ProcessAction handles a player action
func (g *Game) ProcessAction(action ActionType) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.gameOver {
		return fmt.Errorf("game is over")
	}

	g.message = ""

	switch action {
	case ActionMoveForward:
		g.move(1)
	case ActionMoveBackward:
		g.move(-1)
	case ActionStrafeLeft:
		g.strafe(-1)
	case ActionStrafeRight:
		g.strafe(1)
	case ActionTurnLeft:
		g.player.Angle -= TurnSpeed
	case ActionTurnRight:
		g.player.Angle += TurnSpeed
	case ActionShoot:
		g.shoot()
	case ActionUse:
		g.use()
	}

	// Normalize angle
	for g.player.Angle < 0 {
		g.player.Angle += 2 * math.Pi
	}
	for g.player.Angle >= 2*math.Pi {
		g.player.Angle -= 2 * math.Pi
	}

	// Check item pickups
	g.checkPickups()

	// Update enemies
	g.updateEnemies()

	// Check win/lose conditions
	g.checkGameEnd()

	g.tick++

	return nil
}

func (g *Game) move(direction float64) {
	newX := g.player.X + math.Cos(g.player.Angle)*MoveSpeed*direction
	newY := g.player.Y + math.Sin(g.player.Angle)*MoveSpeed*direction

	if g.canMoveTo(newX, newY) {
		g.player.X = newX
		g.player.Y = newY
	}
}

func (g *Game) strafe(direction float64) {
	strafeAngle := g.player.Angle + math.Pi/2
	newX := g.player.X + math.Cos(strafeAngle)*MoveSpeed*direction
	newY := g.player.Y + math.Sin(strafeAngle)*MoveSpeed*direction

	if g.canMoveTo(newX, newY) {
		g.player.X = newX
		g.player.Y = newY
	}
}

func (g *Game) canMoveTo(x, y float64) bool {
	// Check bounds
	if x < 0.5 || x >= float64(g.gameMap.Width)-0.5 ||
		y < 0.5 || y >= float64(g.gameMap.Height)-0.5 {
		return false
	}

	// Check tile
	tileX := int(x)
	tileY := int(y)

	tile := g.gameMap.Tiles[tileY][tileX]

	switch tile {
	case TileFloor, TileExit:
		return true
	case TileDoor:
		// Check if door is open
		return g.isDoorOpen(tileX, tileY)
	case TileLockedDoor:
		// Need key
		return g.isDoorOpen(tileX, tileY)
	default:
		return false
	}
}

func (g *Game) isDoorOpen(x, y int) bool {
	// Check specific doors by position
	if x == 5 && y == 3 {
		return g.state["door_main_open"] > 0.5
	}
	if x == 7 && y == 6 {
		return g.state["door_key_open"] > 0.5
	}
	if x == 12 && y == 10 {
		return g.state["door_exit_open"] > 0.5
	}
	return false
}

func (g *Game) shoot() {
	if g.player.Ammo < PistolAmmo {
		g.message = "No ammo!"
		return
	}

	damage := PistolDamage
	ammoCost := PistolAmmo

	// Use shotgun if available (more damage, more ammo)
	if g.player.HasShotgun && g.player.Ammo >= ShotgunAmmo {
		damage = ShotgunDamage
		ammoCost = ShotgunAmmo
	}

	g.player.Ammo -= ammoCost
	g.state["player_ammo"] = g.player.Ammo

	// Raycast to find target
	target := g.findTarget()
	if target != nil {
		target.Health -= damage
		g.state[fmt.Sprintf("enemy_%d_health", target.ID)] = target.Health

		if target.Health <= 0 {
			target.State = EnemyDead
			g.state[fmt.Sprintf("enemy_%d_alive", target.ID)] = 0
			g.killCount++
			g.state["kill_count"] = float64(g.killCount)
			g.message = "Enemy killed!"
		} else {
			target.State = EnemyAlert
			g.message = "Hit!"
		}
	} else {
		g.message = "Miss!"
	}
}

func (g *Game) findTarget() *Enemy {
	// Simple raycast - find closest enemy in facing direction
	for _, enemy := range g.gameMap.Enemies {
		if enemy.State == EnemyDead {
			continue
		}

		// Calculate angle to enemy
		dx := enemy.X - g.player.X
		dy := enemy.Y - g.player.Y
		dist := math.Sqrt(dx*dx + dy*dy)

		if dist > 10 { // Max range
			continue
		}

		angleToEnemy := math.Atan2(dy, dx)

		// Normalize angle difference
		angleDiff := angleToEnemy - g.player.Angle
		for angleDiff > math.Pi {
			angleDiff -= 2 * math.Pi
		}
		for angleDiff < -math.Pi {
			angleDiff += 2 * math.Pi
		}

		// Check if enemy is in front (within ~30 degree cone)
		if math.Abs(angleDiff) < 0.5 {
			// Check line of sight
			if g.hasLineOfSight(g.player.X, g.player.Y, enemy.X, enemy.Y) {
				return enemy
			}
		}
	}
	return nil
}

func (g *Game) hasLineOfSight(x1, y1, x2, y2 float64) bool {
	// Simple line of sight check
	dx := x2 - x1
	dy := y2 - y1
	dist := math.Sqrt(dx*dx + dy*dy)
	steps := int(dist * 10)

	for i := 0; i < steps; i++ {
		t := float64(i) / float64(steps)
		x := x1 + dx*t
		y := y1 + dy*t

		tileX := int(x)
		tileY := int(y)

		if tileX >= 0 && tileX < g.gameMap.Width &&
			tileY >= 0 && tileY < g.gameMap.Height {
			tile := g.gameMap.Tiles[tileY][tileX]
			if tile == TileWall {
				return false
			}
		}
	}
	return true
}

func (g *Game) use() {
	// Check for nearby doors/switches
	checkX := g.player.X + math.Cos(g.player.Angle)*0.8
	checkY := g.player.Y + math.Sin(g.player.Angle)*0.8

	tileX := int(checkX)
	tileY := int(checkY)

	if tileX < 0 || tileX >= g.gameMap.Width || tileY < 0 || tileY >= g.gameMap.Height {
		return
	}

	tile := g.gameMap.Tiles[tileY][tileX]

	switch tile {
	case TileDoor:
		g.openDoor(tileX, tileY)
	case TileLockedDoor:
		if g.player.HasKeyRed {
			g.openDoor(tileX, tileY)
			g.gameMap.Tiles[tileY][tileX] = TileDoor // Unlock it
			g.message = "Door unlocked!"
		} else {
			g.message = "Need red key!"
		}
	case TileExit:
		g.victory = true
		g.gameOver = true
		g.message = "Level Complete!"
	}
}

func (g *Game) openDoor(x, y int) {
	// Mark door as open in state
	if x == 5 && y == 3 {
		g.state["door_main_open"] = 1
		g.message = "Door opened!"
	} else if x == 7 && y == 6 {
		g.state["door_key_open"] = 1
		g.message = "Door opened!"
	} else if x == 12 && y == 10 {
		g.state["door_exit_open"] = 1
		g.message = "Door opened!"
	}
}

func (g *Game) checkPickups() {
	for _, item := range g.gameMap.Items {
		if item.Picked {
			continue
		}

		dx := item.X - g.player.X
		dy := item.Y - g.player.Y
		dist := math.Sqrt(dx*dx + dy*dy)

		if dist < 0.5 {
			g.pickupItem(item)
		}
	}
}

func (g *Game) pickupItem(item *Item) {
	item.Picked = true
	g.state[fmt.Sprintf("item_%d_available", item.ID)] = 0

	switch item.Type {
	case ItemHealth:
		g.player.Health = math.Min(MaxHealth, g.player.Health+25)
		g.state["player_health"] = g.player.Health
		g.message = "+25 Health"
	case ItemArmor:
		g.player.Armor = math.Min(MaxArmor, g.player.Armor+50)
		g.state["player_armor"] = g.player.Armor
		g.message = "+50 Armor"
	case ItemAmmo:
		g.player.Ammo = math.Min(MaxAmmo, g.player.Ammo+10)
		g.state["player_ammo"] = g.player.Ammo
		g.message = "+10 Ammo"
	case ItemShotgun:
		g.player.HasShotgun = true
		g.player.Ammo = math.Min(MaxAmmo, g.player.Ammo+8)
		g.state["player_has_shotgun"] = 1
		g.state["player_ammo"] = g.player.Ammo
		g.message = "Got Shotgun!"
	case ItemKeyRed:
		g.player.HasKeyRed = true
		g.state["player_has_key_red"] = 1
		g.message = "Got Red Key!"
	case ItemKeyBlue:
		g.player.HasKeyBlue = true
		g.state["player_has_key_blue"] = 1
		g.message = "Got Blue Key!"
	}
}

func (g *Game) updateEnemies() {
	for _, enemy := range g.gameMap.Enemies {
		if enemy.State == EnemyDead {
			continue
		}

		// Calculate distance to player
		dx := g.player.X - enemy.X
		dy := g.player.Y - enemy.Y
		dist := math.Sqrt(dx*dx + dy*dy)

		// Check if enemy can see player
		canSee := dist < 8 && g.hasLineOfSight(enemy.X, enemy.Y, g.player.X, g.player.Y)

		if canSee {
			enemy.State = EnemyAlert

			// Attack if close enough
			if dist < 2 {
				enemy.State = EnemyAttacking
				// Every few ticks, deal damage
				if g.tick%30 == 0 {
					damage := EnemyDamage
					if g.player.Armor > 0 {
						armorAbsorb := math.Min(g.player.Armor, damage*0.5)
						g.player.Armor -= armorAbsorb
						damage -= armorAbsorb
					}
					g.player.Health -= damage
					g.state["player_health"] = g.player.Health
					g.state["player_armor"] = g.player.Armor
					g.message = "Ouch!"
				}
			} else {
				// Move toward player
				moveX := dx / dist * 0.03
				moveY := dy / dist * 0.03

				newX := enemy.X + moveX
				newY := enemy.Y + moveY

				if g.canEnemyMoveTo(newX, newY) {
					enemy.X = newX
					enemy.Y = newY
				}
			}
		} else if enemy.State == EnemyAlert {
			// Wander randomly when lost sight
			if rand.Float64() < 0.02 {
				enemy.State = EnemyIdle
			}
		}
	}
}

func (g *Game) canEnemyMoveTo(x, y float64) bool {
	if x < 0.5 || x >= float64(g.gameMap.Width)-0.5 ||
		y < 0.5 || y >= float64(g.gameMap.Height)-0.5 {
		return false
	}

	tileX := int(x)
	tileY := int(y)

	tile := g.gameMap.Tiles[tileY][tileX]
	return tile == TileFloor
}

func (g *Game) checkGameEnd() {
	if g.player.Health <= 0 {
		g.gameOver = true
		g.victory = false
		g.message = "You died!"
	}

	// Check if at exit
	exitDX := g.player.X - g.gameMap.ExitX
	exitDY := g.player.Y - g.gameMap.ExitY
	if math.Sqrt(exitDX*exitDX+exitDY*exitDY) < 0.5 {
		g.gameOver = true
		g.victory = true
		g.message = "Level Complete!"
	}
}

// Reset resets the game
func (g *Game) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.gameMap = GenerateMap(1)
	g.state = InitialState(g.net, g.gameMap)

	g.player = Player{
		X:      g.gameMap.SpawnX,
		Y:      g.gameMap.SpawnY,
		Angle:  0,
		Health: MaxHealth,
		Armor:  0,
		Ammo:   20,
	}

	g.gameOver = false
	g.victory = false
	g.message = ""
	g.killCount = 0
	g.tick = 0
}

// GetNet returns the underlying Petri net
func (g *Game) GetNet() *petri.PetriNet {
	return g.net
}
