// Package catacombs provides a roguelike game with AI using pflow's modeling capabilities.
package catacombs

import (
	"math"

	"github.com/pflow-xyz/go-pflow/hypothesis"
	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

// CombatEvaluator uses ODE simulation to evaluate combat decisions.
// It models the combat dynamics as a Petri net and uses the hypothesis
// package to find optimal actions.
type CombatEvaluator struct {
	net       *petri.PetriNet
	rates     map[string]float64
	evaluator *hypothesis.Evaluator
}

// NewCombatEvaluator creates a new combat evaluator with a Petri net model
// of combat dynamics.
func NewCombatEvaluator() *CombatEvaluator {
	net, rates := buildCombatNet()

	// Create evaluator with a scoring function that prefers:
	// - Player surviving (highest priority)
	// - Enemy taking damage
	// - Player taking less damage
	// - Faster resolution
	scorer := func(final map[string]float64) float64 {
		playerHP := final["player_hp"]
		enemyHP := final["enemy_hp"]
		playerAlive := final["player_alive"]

		// If player dies, extremely negative score
		if playerAlive < 0.5 || playerHP <= 0 {
			return -1000
		}

		// Score based on:
		// - Enemy HP reduced (good)
		// - Player HP remaining (good)
		// - Victory achieved (best)
		score := 0.0

		// Big bonus for winning
		if enemyHP <= 0 {
			score += 500
		}

		// Bonus for damage dealt
		enemyDamageDealt := final["enemy_max_hp"] - enemyHP
		score += enemyDamageDealt * 10

		// Bonus for HP remaining (normalized)
		score += playerHP * 5

		return score
	}

	eval := hypothesis.NewEvaluator(net, rates, scorer).
		WithTimeSpan(0, 10.0). // Simulate 10 time units
		WithOptions(solver.FastOptions()).
		WithCache(5000) // Cache up to 5000 combat evaluations

	return &CombatEvaluator{
		net:       net,
		rates:     rates,
		evaluator: eval,
	}
}

// buildCombatNet creates a Petri net model of combat dynamics.
// Places represent health and damage potential.
// Transitions represent attack/defense actions.
func buildCombatNet() (*petri.PetriNet, map[string]float64) {
	net := petri.NewPetriNet()

	// Player state places
	net.AddPlace("player_hp", 100, nil, 100, 100, nil)
	net.AddPlace("player_max_hp", 100, nil, 100, 50, nil)
	net.AddPlace("player_attack", 10, nil, 100, 150, nil) // Base attack power
	net.AddPlace("player_alive", 1, nil, 50, 100, nil)

	// Enemy state places
	net.AddPlace("enemy_hp", 50, nil, 300, 100, nil)
	net.AddPlace("enemy_max_hp", 50, nil, 300, 50, nil)
	net.AddPlace("enemy_attack", 8, nil, 300, 150, nil)
	net.AddPlace("enemy_alive", 1, nil, 350, 100, nil)

	// Combat action places (enable/disable)
	net.AddPlace("can_attack", 0, nil, 200, 50, nil)
	net.AddPlace("can_flee", 0, nil, 200, 100, nil)
	net.AddPlace("fleeing", 0, nil, 200, 150, nil)
	net.AddPlace("fled", 0, nil, 200, 200, nil)

	// Player attacks enemy
	net.AddTransition("player_attacks", "default", 200, 100, nil)
	net.AddArc("can_attack", "player_attacks", 0.1, false)  // Enabled by action choice
	net.AddArc("player_attacks", "can_attack", 0.1, false)  // Restore enabler (continuous)
	net.AddArc("player_attack", "player_attacks", 1, false) // Consume attack power
	net.AddArc("player_attacks", "player_attack", 1, false) // Restore attack power
	net.AddArc("enemy_hp", "player_attacks", 1, false)      // Consume enemy HP (damage)
	net.AddArc("player_alive", "player_attacks", 0.01, false)
	net.AddArc("player_attacks", "player_alive", 0.01, false)
	net.AddArc("enemy_alive", "player_attacks", 0.01, false)
	net.AddArc("player_attacks", "enemy_alive", 0.01, false)

	// Enemy attacks player (continuous background damage)
	net.AddTransition("enemy_attacks", "default", 200, 150, nil)
	net.AddArc("enemy_attack", "enemy_attacks", 1, false)
	net.AddArc("enemy_attacks", "enemy_attack", 1, false)
	net.AddArc("player_hp", "enemy_attacks", 1, false)
	net.AddArc("enemy_alive", "enemy_attacks", 0.01, false)
	net.AddArc("enemy_attacks", "enemy_alive", 0.01, false)
	net.AddArc("player_alive", "enemy_attacks", 0.01, false)
	net.AddArc("enemy_attacks", "player_alive", 0.01, false)
	net.AddArc("fleeing", "enemy_attacks", 0.01, true) // Inhibitor: can't attack if we fled

	// Flee action - reduces incoming damage
	net.AddTransition("flee_action", "default", 200, 200, nil)
	net.AddArc("can_flee", "flee_action", 0.1, false)
	net.AddArc("flee_action", "fled", 1, false)
	net.AddArc("fled", "fleeing", 1, false) // Fled leads to fleeing

	// Death transitions (remove alive token when HP depleted)
	net.AddTransition("player_dies", "default", 50, 150, nil)
	net.AddArc("player_alive", "player_dies", 1, false)
	// Player dies when HP <= 0 (modeled as low HP threshold)

	net.AddTransition("enemy_dies", "default", 350, 150, nil)
	net.AddArc("enemy_alive", "enemy_dies", 1, false)

	// Rates control how fast actions happen
	rates := map[string]float64{
		"player_attacks": 1.0,  // Player attack rate
		"enemy_attacks":  0.8,  // Enemy slightly slower
		"flee_action":    2.0,  // Fleeing is fast when chosen
		"player_dies":    0.0,  // Only happens when HP is gone
		"enemy_dies":     0.0,  // Only happens when HP is gone
	}

	return net, rates
}

// CombatSituation describes the current combat state for evaluation.
type CombatSituation struct {
	PlayerHP       int
	PlayerMaxHP    int
	PlayerAttack   int
	EnemyHP        int
	EnemyMaxHP     int
	EnemyAttack    int
	CanFlee        bool
	HasHealPotion  bool
	PotionHealAmt  int
	EnemyCount     int // Multiple enemies increase danger
	PlayerArmor    int
	PlayerWeapon   int // Weapon bonus
}

// EvalAction represents possible actions evaluated by ODE simulation
type EvalAction int

const (
	EvalActionAttack EvalAction = iota
	EvalActionFlee
	EvalActionHeal
	EvalActionWait
)

// EvaluateCombat evaluates the best action in a combat situation.
// Returns the recommended action and a confidence score.
func (ce *CombatEvaluator) EvaluateCombat(sit CombatSituation) (EvalAction, float64) {
	// Convert situation to initial state
	baseState := ce.situationToState(sit)

	// Generate possible actions as state updates
	var actions []map[string]float64
	var actionTypes []EvalAction

	// Attack option (always available)
	attackState := map[string]float64{
		"can_attack": 1,
		"can_flee":   0,
	}
	actions = append(actions, attackState)
	actionTypes = append(actionTypes, EvalActionAttack)

	// Flee option (if path available)
	if sit.CanFlee {
		fleeState := map[string]float64{
			"can_attack": 0,
			"can_flee":   1,
		}
		actions = append(actions, fleeState)
		actionTypes = append(actionTypes, EvalActionFlee)
	}

	// Heal option (if potion available and injured)
	if sit.HasHealPotion && sit.PlayerHP < sit.PlayerMaxHP {
		healState := map[string]float64{
			"can_attack": 0,
			"can_flee":   0,
			"player_hp":  float64(sit.PlayerHP + sit.PotionHealAmt),
		}
		actions = append(actions, healState)
		actionTypes = append(actionTypes, EvalActionHeal)
	}

	// Wait option (defensive)
	waitState := map[string]float64{
		"can_attack": 0,
		"can_flee":   0,
	}
	actions = append(actions, waitState)
	actionTypes = append(actionTypes, EvalActionWait)

	// Evaluate all actions in parallel
	bestIdx, bestScore := ce.evaluator.FindBestParallel(baseState, actions)

	if bestIdx < 0 {
		return EvalActionAttack, 0 // Default to attack
	}

	return actionTypes[bestIdx], bestScore
}

// situationToState converts a CombatSituation to Petri net state.
func (ce *CombatEvaluator) situationToState(sit CombatSituation) map[string]float64 {
	// Calculate effective attack with weapon bonus
	playerAttack := float64(sit.PlayerAttack + sit.PlayerWeapon)
	enemyAttack := float64(sit.EnemyAttack)

	// Armor reduces effective enemy damage
	if sit.PlayerArmor > 0 {
		enemyAttack = math.Max(1, enemyAttack-float64(sit.PlayerArmor)/2)
	}

	// Multiple enemies increase danger
	if sit.EnemyCount > 1 {
		enemyAttack *= float64(sit.EnemyCount)
	}

	return map[string]float64{
		"player_hp":       float64(sit.PlayerHP),
		"player_max_hp":   float64(sit.PlayerMaxHP),
		"player_attack":   playerAttack,
		"player_alive":    1,
		"enemy_hp":        float64(sit.EnemyHP),
		"enemy_max_hp":    float64(sit.EnemyMaxHP),
		"enemy_attack":    enemyAttack,
		"enemy_alive":     1,
		"can_attack":      0,
		"can_flee":        0,
		"fleeing":         0,
		"fled":            0,
	}
}

// ShouldFlee uses ODE evaluation to decide if fleeing is better than fighting.
// This replaces the heuristic shouldFlee() function.
func (ce *CombatEvaluator) ShouldFlee(sit CombatSituation) bool {
	if !sit.CanFlee {
		return false
	}

	action, _ := ce.EvaluateCombat(sit)
	return action == EvalActionFlee
}

// ShouldHeal uses ODE evaluation to decide if healing is optimal.
// This replaces the heuristic shouldHealInCombat() function.
func (ce *CombatEvaluator) ShouldHeal(sit CombatSituation) bool {
	if !sit.HasHealPotion || sit.PlayerHP >= sit.PlayerMaxHP {
		return false
	}

	action, _ := ce.EvaluateCombat(sit)
	return action == EvalActionHeal
}

// EnemyThreat represents a potential combat target for evaluation.
type EnemyThreat struct {
	ID        string
	HP        int
	MaxHP     int
	Attack    int
	Distance  int  // Manhattan distance from player
	IsAlerted bool // Is the enemy aware of player
}

// PreCombatAssessment evaluates what to do before engaging enemies.
// Returns recommended action: "heal_first", "engage", "wait", "flee_zone"
func (ce *CombatEvaluator) PreCombatAssessment(playerHP, playerMaxHP, playerAttack, playerArmor int,
	hasPotion bool, potionHeal int, threats []EnemyThreat) string {

	if len(threats) == 0 {
		return "engage" // No threats, proceed normally
	}

	// Calculate total threat level
	totalThreatHP := 0
	totalThreatDamage := 0
	nearestDist := 1000
	for _, t := range threats {
		totalThreatHP += t.HP
		totalThreatDamage += t.Attack
		if t.Distance < nearestDist {
			nearestDist = t.Distance
		}
	}

	// Build situation for nearest/strongest enemy
	strongestThreat := threats[0]
	for _, t := range threats {
		if t.Attack > strongestThreat.Attack {
			strongestThreat = t
		}
	}

	sit := CombatSituation{
		PlayerHP:      playerHP,
		PlayerMaxHP:   playerMaxHP,
		PlayerAttack:  playerAttack,
		EnemyHP:       strongestThreat.HP,
		EnemyMaxHP:    strongestThreat.MaxHP,
		EnemyAttack:   strongestThreat.Attack,
		CanFlee:       nearestDist > 1, // Can flee if not adjacent
		HasHealPotion: hasPotion,
		PotionHealAmt: potionHeal,
		EnemyCount:    len(threats),
		PlayerArmor:   playerArmor,
	}

	action, score := ce.EvaluateCombat(sit)

	// If heal is recommended and we have distance, heal first
	if action == EvalActionHeal && nearestDist > 2 {
		return "heal_first"
	}

	// If flee is recommended and multiple enemies, leave the area
	if action == EvalActionFlee && len(threats) > 1 {
		return "flee_zone"
	}

	// If flee but single enemy, just wait for better position
	if action == EvalActionFlee {
		return "wait"
	}

	// Low confidence in attack against multiple enemies
	if score < 100 && len(threats) > 1 {
		return "wait"
	}

	// CRITICAL: Don't engage without potions if HP is already low
	healthPct := float64(playerHP) / float64(playerMaxHP)
	if !hasPotion && healthPct < 0.5 && len(threats) > 0 {
		// Without healing, need to be very conservative
		if healthPct < 0.3 {
			return "flee_zone" // Critically low, get out
		}
		return "wait" // Low HP, wait for better opportunity or find potions
	}

	// Don't engage groups without potions
	if !hasPotion && len(threats) > 1 {
		return "flee_zone" // Multiple enemies without healing = death
	}

	return "engage"
}

// SelectBestTarget evaluates multiple enemies and returns the optimal target ID.
// Uses ODE simulation to determine which enemy to prioritize.
func (ce *CombatEvaluator) SelectBestTarget(playerHP, playerMaxHP, playerAttack, playerArmor int,
	hasPotion bool, potionHeal int, enemies []EnemyThreat) (string, float64) {

	if len(enemies) == 0 {
		return "", 0
	}
	if len(enemies) == 1 {
		return enemies[0].ID, 100
	}

	bestID := enemies[0].ID
	bestScore := math.Inf(-1)

	for _, enemy := range enemies {
		sit := CombatSituation{
			PlayerHP:      playerHP,
			PlayerMaxHP:   playerMaxHP,
			PlayerAttack:  playerAttack,
			EnemyHP:       enemy.HP,
			EnemyMaxHP:    enemy.MaxHP,
			EnemyAttack:   enemy.Attack,
			CanFlee:       true,
			HasHealPotion: hasPotion,
			PotionHealAmt: potionHeal,
			EnemyCount:    1, // Evaluate 1v1
			PlayerArmor:   playerArmor,
		}

		_, score := ce.EvaluateCombat(sit)

		// Adjust score based on distance (prefer closer enemies)
		distPenalty := float64(enemy.Distance) * 5
		adjustedScore := score - distPenalty

		// Bonus for low HP enemies (finish them off)
		if enemy.HP <= playerAttack {
			adjustedScore += 100 // Can one-shot
		}

		// Bonus for high-damage enemies (eliminate threats)
		adjustedScore += float64(enemy.Attack) * 2

		if adjustedScore > bestScore {
			bestScore = adjustedScore
			bestID = enemy.ID
		}
	}

	return bestID, bestScore
}

// MidCombatDecision evaluates what to do during an active combat turn.
// More nuanced than EvaluateCombat - considers AP costs, aimed shots, positioning.
type CombatTurnOptions struct {
	CanAttack      bool
	CanAimedShot   bool // Costs more AP but higher damage
	CanHeal        bool
	CanMove        bool
	CanFlee        bool
	CurrentAP      int
	MaxAP          int
	DistanceToEnemy int
}

// CombatTurnAction represents possible turn actions
type CombatTurnAction int

const (
	TurnActionAttack CombatTurnAction = iota
	TurnActionAimedShot
	TurnActionHeal
	TurnActionMove
	TurnActionFlee
	TurnActionEndTurn
)

// EvaluateCombatTurn decides the best action for the current combat turn.
func (ce *CombatEvaluator) EvaluateCombatTurn(sit CombatSituation, opts CombatTurnOptions) (CombatTurnAction, float64) {
	baseState := ce.situationToState(sit)

	var actions []map[string]float64
	var actionTypes []CombatTurnAction

	// Attack (if adjacent and have AP)
	if opts.CanAttack && opts.DistanceToEnemy <= 1 {
		attackState := map[string]float64{
			"can_attack":  1,
			"can_flee":    0,
			"enemy_hp":    math.Max(0, float64(sit.EnemyHP)-float64(sit.PlayerAttack)),
		}
		actions = append(actions, attackState)
		actionTypes = append(actionTypes, TurnActionAttack)
	}

	// Aimed shot (higher damage, costs more AP)
	if opts.CanAimedShot && opts.DistanceToEnemy <= 1 {
		aimedDamage := float64(sit.PlayerAttack) * 1.5 // Aimed shots do 50% more
		aimedState := map[string]float64{
			"can_attack":  1,
			"can_flee":    0,
			"enemy_hp":    math.Max(0, float64(sit.EnemyHP)-aimedDamage),
		}
		actions = append(actions, aimedState)
		actionTypes = append(actionTypes, TurnActionAimedShot)
	}

	// Heal (if have potion and injured)
	if opts.CanHeal && sit.HasHealPotion && sit.PlayerHP < sit.PlayerMaxHP {
		healState := map[string]float64{
			"can_attack": 0,
			"can_flee":   0,
			"player_hp":  math.Min(float64(sit.PlayerMaxHP), float64(sit.PlayerHP+sit.PotionHealAmt)),
		}
		actions = append(actions, healState)
		actionTypes = append(actionTypes, TurnActionHeal)
	}

	// Move closer (if not adjacent)
	if opts.CanMove && opts.DistanceToEnemy > 1 {
		moveState := map[string]float64{
			"can_attack": 0,
			"can_flee":   0,
			// Moving closer means we'll be able to attack next
		}
		actions = append(actions, moveState)
		actionTypes = append(actionTypes, TurnActionMove)
	}

	// Flee (if available)
	if opts.CanFlee {
		fleeState := map[string]float64{
			"can_attack": 0,
			"can_flee":   1,
		}
		actions = append(actions, fleeState)
		actionTypes = append(actionTypes, TurnActionFlee)
	}

	// End turn (always available as fallback)
	endState := map[string]float64{
		"can_attack": 0,
		"can_flee":   0,
	}
	actions = append(actions, endState)
	actionTypes = append(actionTypes, TurnActionEndTurn)

	if len(actions) == 0 {
		return TurnActionEndTurn, 0
	}

	bestIdx, bestScore := ce.evaluator.FindBestParallel(baseState, actions)
	if bestIdx < 0 {
		return TurnActionEndTurn, 0
	}

	// Special logic: prefer aimed shot if enemy is low HP and we can finish them
	if opts.CanAimedShot && opts.DistanceToEnemy <= 1 {
		aimedDamage := int(float64(sit.PlayerAttack) * 1.5)
		if sit.EnemyHP <= aimedDamage && sit.EnemyHP > sit.PlayerAttack {
			// Aimed shot can kill but regular attack can't
			return TurnActionAimedShot, bestScore + 50
		}
	}

	// Special logic: if very low HP and healing available, prioritize heal
	healthPct := float64(sit.PlayerHP) / float64(sit.PlayerMaxHP)
	if healthPct < 0.25 && opts.CanHeal && sit.HasHealPotion {
		// Check if we'd die before killing enemy
		turnsToKillEnemy := (sit.EnemyHP + sit.PlayerAttack - 1) / max(sit.PlayerAttack, 1)
		turnsToBeKilled := (sit.PlayerHP + sit.EnemyAttack - 1) / max(sit.EnemyAttack, 1)
		if turnsToBeKilled <= turnsToKillEnemy {
			return TurnActionHeal, bestScore + 100
		}
	}

	return actionTypes[bestIdx], bestScore
}

// ShouldRetreatMidCombat evaluates whether to flee during active combat.
// More aggressive retreat check than ShouldFlee - considers combat state.
func (ce *CombatEvaluator) ShouldRetreatMidCombat(sit CombatSituation, turnsElapsed int) bool {
	if !sit.CanFlee {
		return false
	}

	// Calculate expected outcome
	playerDamagePerTurn := sit.PlayerAttack
	enemyDamagePerTurn := sit.EnemyAttack

	// Account for armor
	if sit.PlayerArmor > 0 {
		enemyDamagePerTurn = max(1, enemyDamagePerTurn-sit.PlayerArmor/2)
	}

	turnsToKillEnemy := (sit.EnemyHP + playerDamagePerTurn - 1) / max(playerDamagePerTurn, 1)
	turnsToBeKilled := (sit.PlayerHP + enemyDamagePerTurn - 1) / max(enemyDamagePerTurn, 1)

	// Retreat if we'll die before killing enemy
	if turnsToBeKilled < turnsToKillEnemy {
		// But if we have healing, maybe we can turn it around
		if sit.HasHealPotion {
			healedHP := sit.PlayerHP + sit.PotionHealAmt
			turnsToBeKilledHealed := (healedHP + enemyDamagePerTurn - 1) / max(enemyDamagePerTurn, 1)
			if turnsToBeKilledHealed >= turnsToKillEnemy {
				return false // Healing would save us
			}
		}
		return true
	}

	// Retreat if combat is dragging on and we're losing HP
	if turnsElapsed > 5 && float64(sit.PlayerHP)/float64(sit.PlayerMaxHP) < 0.3 {
		return true
	}

	// CRITICAL: Retreat earlier against multiple enemies - they deal cumulative damage
	if sit.EnemyCount > 1 {
		effectiveDamage := enemyDamagePerTurn * sit.EnemyCount
		turnsToBeKilledMulti := (sit.PlayerHP + effectiveDamage - 1) / max(effectiveDamage, 1)
		if turnsToBeKilledMulti < turnsToKillEnemy {
			return true // Multi-enemy cumulative damage is too dangerous
		}
		// Retreat at higher HP threshold against groups
		if float64(sit.PlayerHP)/float64(sit.PlayerMaxHP) < 0.5 && !sit.HasHealPotion {
			return true // Low HP + no potion + multiple enemies = very dangerous
		}
	}

	// Be more conservative when we don't have healing
	if !sit.HasHealPotion && float64(sit.PlayerHP)/float64(sit.PlayerMaxHP) < 0.4 {
		return true // Without potions, 40% HP is already dangerous
	}

	// Use ODE simulation for edge cases
	action, _ := ce.EvaluateCombat(sit)
	return action == EvalActionFlee
}

// CacheStats returns cache statistics for the combat evaluator.
// Returns nil if caching is not enabled.
func (ce *CombatEvaluator) CacheStats() *hypothesis.CacheStats {
	return ce.evaluator.CacheStats()
}

// ClearCache clears the combat evaluator's cache.
func (ce *CombatEvaluator) ClearCache() {
	ce.evaluator.ClearCache()
}
