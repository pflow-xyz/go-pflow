// Package catacombs provides a roguelike game with AI using pflow's Petri net modeling.
package catacombs

import (
	"fmt"
	"math"
	"sync"

	"github.com/pflow-xyz/go-pflow/hypothesis"
	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
	"github.com/pflow-xyz/go-pflow/stateutil"
)

// AIBrain is the Petri net-based AI decision maker.
// It models the game state as token counts and uses ODE simulation
// to evaluate which actions lead to the best outcomes.
type AIBrain struct {
	// Core Petri net model
	net   *petri.PetriNet
	rates map[string]float64

	// Hypothesis evaluator for action selection
	evaluator *hypothesis.Evaluator

	// Memory systems
	Memory *AIMemory

	// Goal tracking
	Goals *AIGoals

	// Cached state for efficient updates
	lastState map[string]float64
}

// AIMemory tracks what the AI has learned about the dungeon.
type AIMemory struct {
	mu sync.RWMutex

	// Spatial memory
	VisitedTiles   map[[2]int]int    // Position -> visit count
	KnownEnemies   map[string]*EnemyMemory
	KnownItems     map[string]*ItemMemory
	KnownNPCs      map[string]*NPCMemory
	LockedDoors    map[[2]int]bool   // Doors we've encountered
	UnlockedDoors  map[[2]int]bool   // Doors we've opened

	// Danger zones - places where we took damage
	DangerZones    map[[2]int]float64 // Position -> danger score

	// Path memory - helps avoid oscillation
	RecentPath     [][2]int
	MaxPathMemory  int

	// Level tracking
	CurrentLevel   int
	LevelStartTick int

	// Target tracking - lock in on a specific target to avoid oscillation
	TargetType     string   // "exit", "chest", "enemy", "key", "item", ""
	TargetX        int      // Target X coordinate
	TargetY        int      // Target Y coordinate
	TargetSetTick  int      // Tick when target was set
	TargetProgress float64  // Progress toward target (distance when set - current distance)
}

// EnemyMemory stores what we know about an enemy.
type EnemyMemory struct {
	ID            string
	LastSeenX     int
	LastSeenY     int
	LastSeenTick  int
	LastSeenHP    int
	DamageDealt   int // Damage we've dealt to this enemy
	DamageTaken   int // Damage this enemy dealt to us
	IsAggressive  bool
	IsDead        bool
}

// ItemMemory stores what we know about an item.
type ItemMemory struct {
	X, Y         int
	Type         ItemType
	Value        float64 // Estimated value
	PickedUp     bool
	Tick         int
}

// NPCMemory stores what we know about an NPC.
type NPCMemory struct {
	ID           string
	X, Y         int
	TalkedTo     bool
	IsShopkeeper bool
	LastSeenTick int
}

// AIGoals represents the AI's objectives with Petri net tokens.
type AIGoals struct {
	// Primary goal: reach exit
	ExitReached    float64

	// Secondary goals
	EnemiesKilled  float64
	ItemsCollected float64
	NPCsTalkedTo   float64
	LevelsComplete float64

	// Health/resource goals
	HealthMaintained float64
	PotionsUsed      float64

	// Exploration goals
	TilesExplored    float64
	SecretsFound     float64
}

// NewAIBrain creates a new Petri net-based AI brain.
func NewAIBrain() *AIBrain {
	brain := &AIBrain{
		Memory: NewAIMemory(),
		Goals:  &AIGoals{},
	}

	brain.net, brain.rates = brain.buildAINet()
	brain.evaluator = brain.buildEvaluator()

	return brain
}

// NewAIMemory creates a new memory system.
func NewAIMemory() *AIMemory {
	return &AIMemory{
		VisitedTiles:  make(map[[2]int]int),
		KnownEnemies:  make(map[string]*EnemyMemory),
		KnownItems:    make(map[string]*ItemMemory),
		KnownNPCs:     make(map[string]*NPCMemory),
		LockedDoors:   make(map[[2]int]bool),
		UnlockedDoors: make(map[[2]int]bool),
		DangerZones:   make(map[[2]int]float64),
		RecentPath:    make([][2]int, 0, 50),
		MaxPathMemory: 50,
	}
}

// buildAINet creates the core Petri net model for AI decision making.
// Places represent state aspects, transitions represent actions/events.
func (b *AIBrain) buildAINet() (*petri.PetriNet, map[string]float64) {
	net := petri.NewPetriNet()

	// === PLAYER STATE PLACES ===
	net.AddPlace("health", 100, nil, 100, 50, nil)         // Current health
	net.AddPlace("max_health", 100, nil, 100, 100, nil)    // Max health
	net.AddPlace("potions", 0, nil, 100, 150, nil)         // Healing potions
	net.AddPlace("keys", 0, nil, 100, 200, nil)            // Keys held
	net.AddPlace("gold", 0, nil, 100, 250, nil)            // Gold collected
	net.AddPlace("alive", 1, nil, 50, 100, nil)            // Player alive

	// === POSITIONAL STATE ===
	net.AddPlace("dist_to_exit", 100, nil, 200, 50, nil)   // Distance to stairs
	net.AddPlace("dist_to_enemy", 100, nil, 200, 100, nil) // Distance to nearest enemy
	net.AddPlace("dist_to_item", 100, nil, 200, 150, nil)  // Distance to nearest item
	net.AddPlace("dist_to_npc", 100, nil, 200, 200, nil)   // Distance to nearest NPC

	// === THREAT STATE ===
	net.AddPlace("threat_level", 0, nil, 300, 50, nil)     // Overall danger
	net.AddPlace("enemies_nearby", 0, nil, 300, 100, nil)  // Count of nearby enemies
	net.AddPlace("enemy_hp_total", 0, nil, 300, 150, nil)  // Total HP of nearby enemies
	net.AddPlace("in_combat", 0, nil, 300, 200, nil)       // Currently fighting

	// === EXPLORATION STATE ===
	net.AddPlace("unexplored", 100, nil, 400, 50, nil)     // Unexplored tiles nearby
	net.AddPlace("known_tiles", 0, nil, 400, 100, nil)     // Tiles we've visited
	net.AddPlace("path_exists", 1, nil, 400, 150, nil)     // Can reach exit

	// === GOAL STATE ===
	net.AddPlace("progress", 0, nil, 500, 50, nil)         // Overall progress
	net.AddPlace("exit_reached", 0, nil, 500, 100, nil)    // At the exit
	net.AddPlace("level_complete", 0, nil, 500, 150, nil)  // Level finished

	// === ACTION TOKENS (enable/disable actions) ===
	net.AddPlace("can_move", 1, nil, 600, 50, nil)         // Can take move action
	net.AddPlace("can_attack", 0, nil, 600, 100, nil)      // Can attack
	net.AddPlace("can_heal", 0, nil, 600, 150, nil)        // Can use potion
	net.AddPlace("can_interact", 0, nil, 600, 200, nil)    // Can talk to NPC
	net.AddPlace("can_descend", 0, nil, 600, 250, nil)     // Can descend stairs

	// === ACTION TRANSITIONS ===

	// Movement toward exit (exploration)
	net.AddTransition("move_to_exit", "default", 150, 50, nil)
	net.AddArc("can_move", "move_to_exit", 0.1, false)
	net.AddArc("move_to_exit", "can_move", 0.1, false)
	net.AddArc("dist_to_exit", "move_to_exit", 1, false)   // Reduces distance
	net.AddArc("move_to_exit", "progress", 0.5, false)     // Increases progress
	net.AddArc("move_to_exit", "known_tiles", 0.1, false)
	net.AddArc("alive", "move_to_exit", 0.01, false)
	net.AddArc("move_to_exit", "alive", 0.01, false)

	// Movement toward item (looting)
	net.AddTransition("move_to_item", "default", 150, 100, nil)
	net.AddArc("can_move", "move_to_item", 0.1, false)
	net.AddArc("move_to_item", "can_move", 0.1, false)
	net.AddArc("dist_to_item", "move_to_item", 1, false)
	net.AddArc("move_to_item", "known_tiles", 0.1, false)
	net.AddArc("alive", "move_to_item", 0.01, false)
	net.AddArc("move_to_item", "alive", 0.01, false)

	// Combat approach
	net.AddTransition("approach_enemy", "default", 150, 150, nil)
	net.AddArc("can_move", "approach_enemy", 0.1, false)
	net.AddArc("approach_enemy", "can_move", 0.1, false)
	net.AddArc("dist_to_enemy", "approach_enemy", 1, false)
	net.AddArc("alive", "approach_enemy", 0.01, false)
	net.AddArc("approach_enemy", "alive", 0.01, false)

	// Attack action
	net.AddTransition("attack", "default", 250, 100, nil)
	net.AddArc("can_attack", "attack", 1, false)
	net.AddArc("attack", "can_attack", 0.5, false)  // Can attack again after cooldown
	net.AddArc("enemy_hp_total", "attack", 10, false) // Deal damage
	net.AddArc("alive", "attack", 0.01, false)
	net.AddArc("attack", "alive", 0.01, false)
	net.AddArc("attack", "progress", 0.2, false)

	// Enemy attacks back (background threat)
	net.AddTransition("enemy_attack", "default", 250, 150, nil)
	net.AddArc("in_combat", "enemy_attack", 0.1, false)
	net.AddArc("enemy_attack", "in_combat", 0.1, false)
	net.AddArc("health", "enemy_attack", 5, false)        // Take damage
	net.AddArc("alive", "enemy_attack", 0.01, false)
	net.AddArc("enemy_attack", "alive", 0.01, false)

	// Healing action
	net.AddTransition("heal", "default", 250, 200, nil)
	net.AddArc("can_heal", "heal", 1, false)
	net.AddArc("potions", "heal", 1, false)               // Consume potion
	net.AddArc("heal", "health", 30, false)               // Restore health
	net.AddArc("alive", "heal", 0.01, false)
	net.AddArc("heal", "alive", 0.01, false)

	// Collect item
	net.AddTransition("collect_item", "default", 350, 100, nil)
	net.AddArc("dist_to_item", "collect_item", 0.5, false) // Must be close
	net.AddArc("collect_item", "progress", 0.3, false)
	net.AddArc("alive", "collect_item", 0.01, false)
	net.AddArc("collect_item", "alive", 0.01, false)

	// Talk to NPC
	net.AddTransition("talk_npc", "default", 350, 150, nil)
	net.AddArc("can_interact", "talk_npc", 1, false)
	net.AddArc("talk_npc", "progress", 0.1, false)
	net.AddArc("alive", "talk_npc", 0.01, false)
	net.AddArc("talk_npc", "alive", 0.01, false)

	// Descend stairs (goal completion)
	net.AddTransition("descend", "default", 350, 200, nil)
	net.AddArc("can_descend", "descend", 1, false)
	net.AddArc("descend", "exit_reached", 1, false)
	net.AddArc("descend", "level_complete", 1, false)
	net.AddArc("descend", "progress", 10, false)
	net.AddArc("alive", "descend", 0.01, false)
	net.AddArc("descend", "alive", 0.01, false)

	// Flee action (move away from danger)
	net.AddTransition("flee", "default", 250, 250, nil)
	net.AddArc("can_move", "flee", 0.1, false)
	net.AddArc("flee", "can_move", 0.1, false)
	net.AddArc("flee", "dist_to_enemy", 2, false)         // Increase distance
	net.AddArc("threat_level", "flee", 0.5, false)        // Reduces threat
	net.AddArc("alive", "flee", 0.01, false)
	net.AddArc("flee", "alive", 0.01, false)

	// Wander (explore unknown areas)
	net.AddTransition("wander", "default", 350, 250, nil)
	net.AddArc("can_move", "wander", 0.1, false)
	net.AddArc("wander", "can_move", 0.1, false)
	net.AddArc("unexplored", "wander", 1, false)          // Explore
	net.AddArc("wander", "known_tiles", 1, false)
	net.AddArc("alive", "wander", 0.01, false)
	net.AddArc("wander", "alive", 0.01, false)

	// Death transition
	net.AddTransition("die", "default", 50, 150, nil)
	net.AddArc("alive", "die", 1, false)
	// Fires when health reaches 0 (modeled by low health enabling)

	// Rates for different actions
	rates := map[string]float64{
		"move_to_exit":   1.0,  // Base movement speed
		"move_to_item":   0.8,  // Slightly slower for caution
		"approach_enemy": 0.6,  // Careful approach
		"attack":         2.0,  // Fast attacks
		"enemy_attack":   1.5,  // Enemy attack rate
		"heal":           3.0,  // Quick healing
		"collect_item":   2.0,  // Fast pickup
		"talk_npc":       1.0,  // Normal speed
		"descend":        5.0,  // Very fast when enabled
		"flee":           1.5,  // Fast escape
		"wander":         0.5,  // Slow exploration
		"die":            0.0,  // Only fires when HP = 0
	}

	return net, rates
}

// buildEvaluator creates the hypothesis evaluator for action selection.
func (b *AIBrain) buildEvaluator() *hypothesis.Evaluator {
	scorer := func(final map[string]float64) float64 {
		score := 0.0

		// Primary: survival (highest weight)
		if final["alive"] < 0.5 || final["health"] <= 0 {
			return -10000 // Death is worst outcome
		}

		// Primary: progress toward exit
		score += final["progress"] * 100
		score += final["level_complete"] * 1000
		score += final["exit_reached"] * 500

		// Health management
		healthPct := final["health"] / math.Max(final["max_health"], 1)
		score += healthPct * 50

		// Distance penalties (lower is better)
		score -= final["dist_to_exit"] * 0.5

		// Threat reduction bonus
		score -= final["threat_level"] * 10
		score -= final["enemies_nearby"] * 5

		// Exploration bonus
		score += final["known_tiles"] * 0.2

		// Loot bonus
		score += final["gold"] * 0.1
		score += final["potions"] * 5 // Potions are valuable

		return score
	}

	return hypothesis.NewEvaluator(b.net, b.rates, scorer).
		WithTimeSpan(0, 10.0).
		WithOptions(solver.FastOptions()).
		WithEarlyTermination(func(state map[string]float64) bool {
			return state["alive"] < 0.5 || state["health"] <= 0
		}).
		WithInfeasibleScore(-10000)
	// Note: No cache for brain - state changes every tick (0% hit rate measured)
	// Combat cache is effective (99%+ hit rate) and kept in ai_evaluator.go
}

// GameStateToTokens converts the current game state to Petri net tokens.
func (b *AIBrain) GameStateToTokens(g *Game) map[string]float64 {
	state := make(map[string]float64)

	// Player state
	state["health"] = float64(g.Player.Health)
	state["max_health"] = float64(g.Player.MaxHealth)
	state["alive"] = 1.0
	state["gold"] = float64(g.Player.Gold)

	// Count potions
	potions := 0
	for _, item := range g.Player.Inventory {
		if item.Type == ItemPotion {
			potions++
		}
	}
	state["potions"] = float64(potions)

	// Count keys
	keys := 0
	for _, has := range g.Player.Keys {
		if has {
			keys++
		}
	}
	state["keys"] = float64(keys)

	// Distance calculations
	state["dist_to_exit"] = float64(abs(g.Player.X-g.Dungeon.ExitX) + abs(g.Player.Y-g.Dungeon.ExitY))

	// Find nearest enemy (excluding enemies stuck in walls)
	nearestEnemyDist := 1000.0
	enemiesNearby := 0
	totalEnemyHP := 0
	for _, e := range g.Enemies {
		if e.State == StateDead {
			continue
		}
		// Skip enemies stuck in walls
		if g.Dungeon.Tiles[e.Y][e.X] == TileWall {
			continue
		}
		dist := float64(abs(g.Player.X-e.X) + abs(g.Player.Y-e.Y))
		if dist < nearestEnemyDist {
			nearestEnemyDist = dist
		}
		if dist <= 5 {
			enemiesNearby++
			totalEnemyHP += e.Health
		}
	}
	state["dist_to_enemy"] = nearestEnemyDist
	state["enemies_nearby"] = float64(enemiesNearby)
	state["enemy_hp_total"] = float64(totalEnemyHP)

	// Find nearest item and specifically nearest potion
	nearestItemDist := 1000.0
	nearestPotionDist := 1000.0
	for _, item := range g.Items {
		dist := float64(abs(g.Player.X-item.X) + abs(g.Player.Y-item.Y))
		if dist < nearestItemDist {
			nearestItemDist = dist
		}
		// Track potions specifically - critical for survival
		if item.Item.Type == ItemPotion && item.Item.Effect > 0 {
			if dist < nearestPotionDist {
				nearestPotionDist = dist
			}
		}
	}
	state["dist_to_item"] = nearestItemDist
	state["dist_to_potion"] = nearestPotionDist

	// Find nearest chest (chests are tiles, not items)
	nearestChestDist := 1000.0
	chestCount := 0
	for y := 0; y < g.Dungeon.Height; y++ {
		for x := 0; x < g.Dungeon.Width; x++ {
			if g.Dungeon.Tiles[y][x] == TileChest {
				dist := float64(abs(g.Player.X-x) + abs(g.Player.Y-y))
				if dist < nearestChestDist {
					nearestChestDist = dist
				}
				chestCount++
			}
		}
	}
	state["dist_to_chest"] = nearestChestDist
	state["chests_remaining"] = float64(chestCount)

	// Find nearest NPC
	nearestNPCDist := 1000.0
	for _, npc := range g.NPCs {
		dist := float64(abs(g.Player.X-npc.X) + abs(g.Player.Y-npc.Y))
		if dist < nearestNPCDist {
			nearestNPCDist = dist
		}
	}
	state["dist_to_npc"] = nearestNPCDist

	// Threat level
	threatLevel := float64(enemiesNearby * 10)
	if g.Combat.Active {
		threatLevel += 50
		state["in_combat"] = 1.0
	} else {
		state["in_combat"] = 0.0
	}
	state["threat_level"] = threatLevel

	// Exploration state
	unexplored := b.countUnexploredNearby(g, 5)
	state["unexplored"] = float64(unexplored)
	state["known_tiles"] = float64(len(b.Memory.VisitedTiles))

	// Check if path to exit exists
	if b.canReachExit(g) {
		state["path_exists"] = 1.0
	} else {
		state["path_exists"] = 0.0
	}

	// Check for locked doors and keys
	hasKey := false
	for k := range g.Player.Keys {
		if k == "rusty_key" {
			hasKey = true
			break
		}
	}
	state["has_key"] = 0.0
	if hasKey {
		state["has_key"] = 1.0
	}

	// Check if locked door blocks path (needs_key = no path AND no key AND locked door exists)
	needsKey := false
	if state["path_exists"] == 0 && !hasKey {
		// Check if there are locked doors
		for y := 0; y < g.Dungeon.Height; y++ {
			for x := 0; x < g.Dungeon.Width; x++ {
				if g.Dungeon.Tiles[y][x] == TileLockedDoor {
					needsKey = true
					break
				}
			}
			if needsKey {
				break
			}
		}
	}
	state["needs_key"] = 0.0
	if needsKey {
		state["needs_key"] = 1.0
	}

	// Progress
	state["progress"] = b.Goals.TilesExplored + b.Goals.EnemiesKilled*5 + b.Goals.ItemsCollected*2

	// Time tracking - urgency increases over time on a level
	ticksOnLevel := float64(g.AI.ActionCount - b.Memory.LevelStartTick)
	state["ticks_on_level"] = ticksOnLevel
	// Urgency ramps up: 0 at start, 1.0 at 100 ticks, 2.0 at 200 ticks, etc.
	state["urgency"] = ticksOnLevel / 100.0

	// Action availability
	state["can_move"] = 1.0

	// Check for truly adjacent enemy (must be on walkable tile, not in wall)
	hasAdjacentEnemy := false
	dirs := [][2]int{{0, -1}, {0, 1}, {-1, 0}, {1, 0}}
	for _, e := range g.Enemies {
		if e.State == StateDead {
			continue
		}
		// Check if enemy is on a walkable tile (not stuck in wall)
		enemyTile := g.Dungeon.Tiles[e.Y][e.X]
		if enemyTile == TileWall {
			continue // Enemy is stuck in wall, can't attack
		}
		for _, d := range dirs {
			if g.Player.X+d[0] == e.X && g.Player.Y+d[1] == e.Y {
				hasAdjacentEnemy = true
				break
			}
		}
		if hasAdjacentEnemy {
			break
		}
	}
	if hasAdjacentEnemy {
		state["can_attack"] = 1.0
	} else {
		state["can_attack"] = 0.0
	}

	if potions > 0 && g.Player.Health < g.Player.MaxHealth {
		state["can_heal"] = 1.0
	} else {
		state["can_heal"] = 0.0
	}

	if nearestNPCDist <= 1.5 {
		state["can_interact"] = 1.0
	} else {
		state["can_interact"] = 0.0
	}

	if g.Player.X == g.Dungeon.ExitX && g.Player.Y == g.Dungeon.ExitY {
		state["can_descend"] = 1.0
		state["exit_reached"] = 1.0
	} else {
		state["can_descend"] = 0.0
		state["exit_reached"] = 0.0
	}

	// Target tracking state
	b.Memory.mu.RLock()
	if b.Memory.TargetType != "" {
		state["has_target"] = 1.0
		distToTarget := float64(abs(g.Player.X-b.Memory.TargetX) + abs(g.Player.Y-b.Memory.TargetY))
		state["dist_to_target"] = distToTarget
		// Track if we're making progress toward target
		if distToTarget < b.Memory.TargetProgress {
			state["target_progress"] = 1.0
		} else {
			state["target_progress"] = 0.0
		}
	} else {
		state["has_target"] = 0.0
		state["dist_to_target"] = 0.0
		state["target_progress"] = 0.0
	}
	b.Memory.mu.RUnlock()

	b.lastState = state
	return state
}

// countUnexploredNearby counts unexplored tiles within radius.
func (b *AIBrain) countUnexploredNearby(g *Game, radius int) int {
	b.Memory.mu.RLock()
	defer b.Memory.mu.RUnlock()

	count := 0
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			x, y := g.Player.X+dx, g.Player.Y+dy
			if x < 0 || x >= g.Dungeon.Width || y < 0 || y >= g.Dungeon.Height {
				continue
			}
			pos := [2]int{x, y}
			if _, visited := b.Memory.VisitedTiles[pos]; !visited {
				// Only count walkable tiles
				tile := g.Dungeon.Tiles[y][x]
				if tile == TileFloor || tile == TileDoor || tile == TileStairsDown {
					count++
				}
			}
		}
	}
	return count
}

// canReachExit checks if a path to exit exists (simplified).
func (b *AIBrain) canReachExit(g *Game) bool {
	// Use simple BFS to check reachability
	visited := make(map[[2]int]bool)
	queue := [][2]int{{g.Player.X, g.Player.Y}}
	visited[[2]int{g.Player.X, g.Player.Y}] = true

	dirs := [][2]int{{0, -1}, {0, 1}, {-1, 0}, {1, 0}}

	for len(queue) > 0 && len(visited) < 1000 {
		pos := queue[0]
		queue = queue[1:]

		if pos[0] == g.Dungeon.ExitX && pos[1] == g.Dungeon.ExitY {
			return true
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
				tile == TileWater || tile == TileLava ||
				tile == TileChest || tile == TileAltar

			// Handle locked doors
			if tile == TileLockedDoor {
				if g.Player.Keys["rusty_key"] {
					walkable = true
				}
			}

			if walkable {
				visited[key] = true
				queue = append(queue, key)
			}
		}
	}
	return false
}

// ActionCandidate represents a possible action with its state update.
type ActionCandidate struct {
	Action  ActionType
	Updates map[string]float64
	Desc    string
}

// EvaluateActions uses ODE simulation to find the best action.
func (b *AIBrain) EvaluateActions(g *Game) (ActionType, string) {
	baseState := b.GameStateToTokens(g)

	// Generate candidate actions based on current state
	candidates := b.generateCandidates(g, baseState)

	if len(candidates) == 0 {
		return ActionWait, "no_candidates"
	}

	// Extract just the updates for parallel evaluation
	updates := make([]map[string]float64, len(candidates))
	for i, c := range candidates {
		updates[i] = c.Updates
	}

	// Use hypothesis evaluator to find best action
	bestIdx, score := b.evaluator.FindBestParallel(baseState, updates)

	if bestIdx < 0 {
		return ActionWait, "no_best"
	}

	best := candidates[bestIdx]
	return best.Action, fmt.Sprintf("%s (score: %.1f)", best.Desc, score)
}

// generateCandidates creates action candidates based on game state.
func (b *AIBrain) generateCandidates(g *Game, state map[string]float64) []ActionCandidate {
	var candidates []ActionCandidate

	// If at exit, descend is always best
	if state["can_descend"] > 0 {
		candidates = append(candidates, ActionCandidate{
			Action:  ActionDescend,
			Updates: map[string]float64{"level_complete": 1, "progress": 100},
			Desc:    "descend_exit",
		})
		return candidates // Only option when at exit
	}

	// Get current target info for commitment bonus
	targetType, targetX, targetY := b.GetTarget()
	hasTarget := targetType != ""

	// Calculate target commitment bonus - rewards staying on current target
	targetBonus := func(candidateType string, candidateX, candidateY int) float64 {
		if !hasTarget {
			return 0
		}
		// Big bonus for continuing toward current target
		if candidateType == targetType {
			// Check if this candidate moves toward the target
			currentDist := float64(abs(g.Player.X-targetX) + abs(g.Player.Y-targetY))
			// Candidate generally moves toward target = bonus
			if currentDist > 1 {
				return 15 // Significant bonus for staying on target
			}
		}
		return 0
	}

	// Low health + have potions = consider healing
	healthPct := state["health"] / math.Max(state["max_health"], 1)
	if state["can_heal"] > 0 && healthPct < 0.4 {
		candidates = append(candidates, ActionCandidate{
			Action:  ActionUseItem,
			Updates: map[string]float64{"health": state["health"] + 30, "potions": state["potions"] - 1},
			Desc:    "heal_low_hp",
		})
	}

	// Can attack = in melee range - HIGHEST PRIORITY (must clear enemies to progress)
	// This is critical: if we can attack, we MUST attack or we'll get stuck oscillating
	if state["can_attack"] > 0 {
		// Attack is CRITICAL: removes threat and clears path
		// Give it extremely high progress to override ANY other action
		// This prevents the AI from trying to run past enemies and getting stuck
		attackBonus := targetBonus("enemy", 0, 0) // Enemy coords don't matter for attack
		candidates = append(candidates, ActionCandidate{
			Action:  ActionAttack,
			Updates: map[string]float64{
				"enemy_hp_total": math.Max(0, state["enemy_hp_total"]-10),
				"progress":       state["progress"] + 50 + attackBonus, // VERY HIGH priority
				"threat_level":   math.Max(0, state["threat_level"] - 10),
				"enemies_nearby": math.Max(0, state["enemies_nearby"] - 0.5),
			},
			Desc: "attack_enemy",
		})

		// Also consider fleeing if hurt
		if healthPct < 0.3 && state["in_combat"] > 0 {
			candidates = append(candidates, ActionCandidate{
				Action:  ActionMoveRight, // Direction determined later
				Updates: map[string]float64{"dist_to_enemy": state["dist_to_enemy"] + 2, "threat_level": state["threat_level"] - 10},
				Desc:    "flee_danger",
			})
		}
	}

	// Movement options
	if state["can_move"] > 0 {
		// If we need a key (path blocked by locked door), prioritize finding it
		if state["needs_key"] > 0 {
			keyBonus := targetBonus("key", 0, 0)
			candidates = append(candidates, ActionCandidate{
				Action:  ActionMoveDown, // Direction determined by exploration
				Updates: map[string]float64{"needs_key": 0.5, "progress": state["progress"] + 5 + keyBonus, "has_key": 0.5},
				Desc:    "find_key",
			})
		}

		// Move toward exit (primary goal) - only if path exists
		// Urgency bonus increases over time, making exit more attractive
		if state["dist_to_exit"] > 0 && state["path_exists"] > 0 {
			distAfter := math.Max(0, state["dist_to_exit"]-1)
			// Base progress + urgency bonus (urgency * 5 means at 100 ticks we get +5 progress per move)
			// Start with base 5 to compete with chest looting
			urgencyBonus := state["urgency"] * 5
			exitBonus := targetBonus("exit", g.Dungeon.ExitX, g.Dungeon.ExitY)
			candidates = append(candidates, ActionCandidate{
				Action:  ActionMoveDown, // Direction determined by pathfinding
				Updates: map[string]float64{"dist_to_exit": distAfter, "progress": state["progress"] + 5 + urgencyBonus + exitBonus},
				Desc:    "move_to_exit",
			})
		}

		// Move toward item
		if state["dist_to_item"] < 10 && state["dist_to_item"] > 0 {
			distAfter := math.Max(0, state["dist_to_item"]-1)
			itemBonus := targetBonus("item", 0, 0)
			candidates = append(candidates, ActionCandidate{
				Action:  ActionMoveDown, // Direction determined by pathfinding
				Updates: map[string]float64{"dist_to_item": distAfter, "progress": state["progress"] + 0.5 + itemBonus},
				Desc:    "move_to_item",
			})
		}

		// HIGH PRIORITY: Move toward potions when we need them
		// This is critical for survival - seed 87 died with 0 potions!
		potionCount := state["potions"]
		if state["dist_to_potion"] < 15 && state["dist_to_potion"] > 0 && potionCount < 2 {
			distAfter := math.Max(0, state["dist_to_potion"]-1)
			potionBonus := targetBonus("potion", 0, 0)
			// Very high priority when we have no potions
			priorityBonus := 8.0 // Base priority higher than exit
			if potionCount == 0 {
				priorityBonus = 15.0 // Desperate - potions are critical!
			}
			candidates = append(candidates, ActionCandidate{
				Action:  ActionMoveDown, // Direction determined by pathfinding
				Updates: map[string]float64{"dist_to_potion": distAfter, "progress": state["progress"] + priorityBonus + potionBonus, "potions": potionCount + 0.1},
				Desc:    "find_potion",
			})
		}

		// Move toward chest (moderate priority - chests contain loot but shouldn't block exit)
		// Only pursue chests if they're nearby and we're not too far from exit
		if state["dist_to_chest"] < 10 && state["dist_to_chest"] > 0 && state["dist_to_exit"] < 25 {
			distAfter := math.Max(0, state["dist_to_chest"]-1)
			chestBonus := targetBonus("chest", 0, 0)
			// Moderate progress bonus - less than exit priority
			candidates = append(candidates, ActionCandidate{
				Action:  ActionMoveDown, // Direction determined by pathfinding
				Updates: map[string]float64{"dist_to_chest": distAfter, "progress": state["progress"] + 3 + chestBonus, "chests_remaining": state["chests_remaining"] - 0.1},
				Desc:    "loot_chest",
			})
		}

		// Approach enemy (if not too dangerous)
		if state["dist_to_enemy"] > 1 && state["dist_to_enemy"] < 8 && healthPct > 0.3 {
			distAfter := math.Max(1, state["dist_to_enemy"]-1)
			enemyBonus := targetBonus("enemy", 0, 0)
			candidates = append(candidates, ActionCandidate{
				Action:  ActionMoveDown, // Direction determined by pathfinding
				Updates: map[string]float64{"dist_to_enemy": distAfter, "progress": state["progress"] + enemyBonus},
				Desc:    "approach_enemy",
			})
		}

		// Explore unknown areas
		if state["unexplored"] > 0 {
			candidates = append(candidates, ActionCandidate{
				Action:  ActionMoveDown,
				Updates: map[string]float64{"unexplored": state["unexplored"] - 1, "known_tiles": state["known_tiles"] + 1},
				Desc:    "explore",
			})
		}
	}

	// NPC interaction
	if state["can_interact"] > 0 {
		candidates = append(candidates, ActionCandidate{
			Action:  ActionTalk,
			Updates: map[string]float64{"progress": state["progress"] + 1},
			Desc:    "talk_npc",
		})
	}

	// Fallback: wait
	if len(candidates) == 0 {
		candidates = append(candidates, ActionCandidate{
			Action:  ActionWait,
			Updates: map[string]float64{},
			Desc:    "wait",
		})
	}

	return candidates
}

// UpdateMemory updates the AI's memory based on current game state.
func (b *AIBrain) UpdateMemory(g *Game, tick int) {
	b.Memory.mu.Lock()
	defer b.Memory.mu.Unlock()

	// Record current position
	pos := [2]int{g.Player.X, g.Player.Y}
	b.Memory.VisitedTiles[pos]++

	// Update recent path
	b.Memory.RecentPath = append(b.Memory.RecentPath, pos)
	if len(b.Memory.RecentPath) > b.Memory.MaxPathMemory {
		b.Memory.RecentPath = b.Memory.RecentPath[1:]
	}

	// Update enemy memory
	for _, e := range g.Enemies {
		mem, exists := b.Memory.KnownEnemies[e.ID]
		if !exists {
			mem = &EnemyMemory{ID: e.ID}
			b.Memory.KnownEnemies[e.ID] = mem
		}
		mem.LastSeenX = e.X
		mem.LastSeenY = e.Y
		mem.LastSeenTick = tick
		mem.LastSeenHP = e.Health
		mem.IsAggressive = e.State == StateChasing || e.State == StateAttacking
		mem.IsDead = e.State == StateDead
	}

	// Update item memory
	for _, item := range g.Items {
		key := fmt.Sprintf("%d_%d", item.X, item.Y)
		if _, exists := b.Memory.KnownItems[key]; !exists {
			b.Memory.KnownItems[key] = &ItemMemory{
				X:     item.X,
				Y:     item.Y,
				Type:  item.Item.Type,
				Value: b.estimateItemValue(&item.Item),
				Tick:  tick,
			}
		}
	}

	// Update NPC memory
	for _, npc := range g.NPCs {
		mem, exists := b.Memory.KnownNPCs[npc.ID]
		if !exists {
			mem = &NPCMemory{ID: npc.ID}
			b.Memory.KnownNPCs[npc.ID] = mem
		}
		mem.X = npc.X
		mem.Y = npc.Y
		mem.LastSeenTick = tick
		mem.IsShopkeeper = npc.Type == NPCMerchant
	}

	// Update level tracking
	if b.Memory.CurrentLevel != g.Dungeon.Level {
		b.Memory.CurrentLevel = g.Dungeon.Level
		b.Memory.LevelStartTick = tick
		b.Goals.LevelsComplete++
	}

	// Update goals
	b.Goals.TilesExplored = float64(len(b.Memory.VisitedTiles))
}

// estimateItemValue estimates the value of an item for prioritization.
func (b *AIBrain) estimateItemValue(item *Item) float64 {
	if item == nil {
		return 0
	}
	switch item.Type {
	case ItemKey:
		return 100 // Keys are very valuable
	case ItemPotion:
		return 50 // Potions are valuable
	case ItemScroll:
		return 20
	case ItemWeapon:
		return 30 + float64(item.Effect*5) // Effect is damage bonus
	case ItemArmor:
		return 30 + float64(item.Effect*5) // Effect is defense bonus
	case ItemGold:
		return float64(item.Value) * 0.5
	case ItemQuest:
		return 75 // Quest items are important
	default:
		return 10
	}
}

// IsOscillating detects if the AI is stuck in a loop.
// Uses multiple window sizes to detect different oscillation patterns.
func (b *AIBrain) IsOscillating() bool {
	b.Memory.mu.RLock()
	defer b.Memory.mu.RUnlock()

	if len(b.Memory.RecentPath) < 6 {
		return false
	}

	// Check multiple window sizes for different patterns
	windows := []int{6, 10, 20}
	for _, windowSize := range windows {
		if len(b.Memory.RecentPath) < windowSize {
			continue
		}
		recent := b.Memory.RecentPath[len(b.Memory.RecentPath)-windowSize:]

		// Count unique positions in this window
		uniquePos := make(map[[2]int]bool)
		for _, pos := range recent {
			uniquePos[pos] = true
		}

		// If using very few unique positions relative to window, we're stuck
		// Window 6: <= 2 unique positions means oscillating
		// Window 10: <= 3 unique positions means oscillating
		// Window 20: <= 5 unique positions means oscillating
		maxUnique := windowSize / 4
		if maxUnique < 2 {
			maxUnique = 2
		}
		if len(uniquePos) <= maxUnique {
			return true
		}
	}

	// Check for movement vector reversal pattern (A->B->A->B)
	if len(b.Memory.RecentPath) >= 8 {
		// Calculate movement vectors for last 8 moves
		recent := b.Memory.RecentPath[len(b.Memory.RecentPath)-8:]
		vectors := make([][2]int, 7)
		for i := 0; i < 7; i++ {
			vectors[i] = [2]int{
				recent[i+1][0] - recent[i][0],
				recent[i+1][1] - recent[i][1],
			}
		}

		// Count reversals (vector followed by its negative)
		reversals := 0
		for i := 0; i < 6; i++ {
			if vectors[i][0] == -vectors[i+1][0] && vectors[i][1] == -vectors[i+1][1] {
				reversals++
			}
		}
		// If more than half are reversals, we're oscillating
		if reversals >= 3 {
			return true
		}
	}

	return false
}

// GetDangerAt returns the danger score at a position.
func (b *AIBrain) GetDangerAt(x, y int) float64 {
	b.Memory.mu.RLock()
	defer b.Memory.mu.RUnlock()
	return b.Memory.DangerZones[[2]int{x, y}]
}

// RecordDamage records that damage was taken at a position.
func (b *AIBrain) RecordDamage(x, y int, amount int) {
	b.Memory.mu.Lock()
	defer b.Memory.mu.Unlock()
	b.Memory.DangerZones[[2]int{x, y}] += float64(amount)
}

// Reset clears memory for a new game.
func (b *AIBrain) Reset() {
	b.Memory = NewAIMemory()
	b.Goals = &AIGoals{}
	b.lastState = nil
}

// SetTarget locks in on a specific target to avoid oscillation.
// The target is persisted until reached or cleared.
func (b *AIBrain) SetTarget(targetType string, x, y int, tick int, currentDist float64) {
	b.Memory.mu.Lock()
	defer b.Memory.mu.Unlock()

	// Don't switch targets unless we've reached the current one or it's stale
	if b.Memory.TargetType != "" && b.Memory.TargetType == targetType {
		// Same type - update position in case target moved
		b.Memory.TargetX = x
		b.Memory.TargetY = y
		return
	}

	b.Memory.TargetType = targetType
	b.Memory.TargetX = x
	b.Memory.TargetY = y
	b.Memory.TargetSetTick = tick
	b.Memory.TargetProgress = currentDist
}

// ClearTarget removes the current target.
func (b *AIBrain) ClearTarget() {
	b.Memory.mu.Lock()
	defer b.Memory.mu.Unlock()
	b.Memory.TargetType = ""
	b.Memory.TargetX = 0
	b.Memory.TargetY = 0
	b.Memory.TargetSetTick = 0
	b.Memory.TargetProgress = 0
}

// GetTarget returns the current target info.
func (b *AIBrain) GetTarget() (string, int, int) {
	b.Memory.mu.RLock()
	defer b.Memory.mu.RUnlock()
	return b.Memory.TargetType, b.Memory.TargetX, b.Memory.TargetY
}

// HasTarget returns true if a target is set.
func (b *AIBrain) HasTarget() bool {
	b.Memory.mu.RLock()
	defer b.Memory.mu.RUnlock()
	return b.Memory.TargetType != ""
}

// ShouldSwitchTarget returns true if the current target should be abandoned.
// Targets are abandoned if:
// - Target reached (dist < 1)
// - Target stale (too many ticks without progress)
// - No path to target
func (b *AIBrain) ShouldSwitchTarget(currentDist float64, tick int, pathExists bool) bool {
	b.Memory.mu.RLock()
	defer b.Memory.mu.RUnlock()

	if b.Memory.TargetType == "" {
		return true // No target, need to set one
	}

	// Reached target
	if currentDist < 1 {
		return true
	}

	// No path to target
	if !pathExists {
		return true
	}

	// Stale: no progress for 50 ticks
	ticksOnTarget := tick - b.Memory.TargetSetTick
	if ticksOnTarget > 50 && currentDist >= b.Memory.TargetProgress {
		return true
	}

	return false
}

// GetState returns a copy of the last computed state.
func (b *AIBrain) GetState() map[string]float64 {
	return stateutil.Copy(b.lastState)
}

// String returns a debug string of the current state.
func (b *AIBrain) String() string {
	if b.lastState == nil {
		return "AIBrain: no state"
	}
	return fmt.Sprintf("AIBrain: health=%.0f dist_exit=%.0f enemies=%.0f threat=%.0f progress=%.1f",
		b.lastState["health"],
		b.lastState["dist_to_exit"],
		b.lastState["enemies_nearby"],
		b.lastState["threat_level"],
		b.lastState["progress"])
}

// CacheStats returns cache statistics for the AI brain's evaluator.
// Returns nil - brain caching is disabled (0% hit rate measured).
func (b *AIBrain) CacheStats() *hypothesis.CacheStats {
	// Brain cache disabled - state changes every tick, no cache hits
	return nil
}

// ClearCache is a no-op - brain caching is disabled.
func (b *AIBrain) ClearCache() {
	// Brain cache disabled
}
