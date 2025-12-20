// Package catacombs provides a roguelike game with AI using pflow's modeling capabilities.
package catacombs

import (
	"github.com/pflow-xyz/go-pflow/statemachine"
)

// AIPhase represents high-level AI operational modes
type AIPhase string

const (
	PhaseExploration AIPhase = "exploration"
	PhaseCombat      AIPhase = "combat"
	PhaseInteraction AIPhase = "interaction"
	PhaseRecovery    AIPhase = "recovery"
)

// AISubphase represents sub-modes within exploration
type AISubphase string

const (
	SubphaseExplore AISubphase = "explore"
	SubphaseLoot    AISubphase = "loot"
	SubphaseFindKey AISubphase = "find_key"
	SubphaseWander  AISubphase = "wander"
)

// AIEvent represents events that trigger state machine transitions
type AIEvent string

const (
	// Combat events
	EventEnemyVisible    AIEvent = "enemy_visible"
	EventEnemyDead       AIEvent = "enemy_dead"
	EventNoEnemies       AIEvent = "no_enemies"
	EventHealthLow       AIEvent = "health_low"
	EventHealthOK        AIEvent = "health_ok"
	EventCannotFlee      AIEvent = "cannot_flee"
	EventShouldFlee      AIEvent = "should_flee"
	EventStandAndFight   AIEvent = "stand_and_fight"

	// Exploration events
	EventLootNearby      AIEvent = "loot_nearby"
	EventLootCollected   AIEvent = "loot_collected"
	EventKeyNeeded       AIEvent = "key_needed"
	EventKeyFound        AIEvent = "key_found"
	EventExitReached     AIEvent = "exit_reached"
	EventDeadEnd         AIEvent = "dead_end"

	// Interaction events
	EventNPCAdjacent     AIEvent = "npc_adjacent"
	EventDialogueStarted AIEvent = "dialogue_started"
	EventDialogueEnded   AIEvent = "dialogue_ended"
	EventShopEntered     AIEvent = "shop_entered"
	EventShopExited      AIEvent = "shop_exited"

	// Recovery events
	EventHealed          AIEvent = "healed"
	EventPotionUsed      AIEvent = "potion_used"
	EventRecoveryDone    AIEvent = "recovery_done"
)

// BuildAIStateMachine creates the state machine chart for AI decision making.
// This formalizes the mode transitions that were previously handled by aiDecideMode().
func BuildAIStateMachine() *statemachine.Chart {
	return statemachine.NewChart("catacombs_ai").
		// Main phase region - high level behavioral mode
		Region("phase").
			State("exploration").Initial().
			State("combat").
			State("interaction").
			State("recovery").
		EndRegion().

		// Exploration sub-mode region - what we're doing while exploring
		Region("explore_mode").
			State("explore").Initial().
			State("loot").
			State("find_key").
			State("wander").
		EndRegion().

		// Combat sub-mode region - how we're fighting
		Region("combat_mode").
			State("attack").Initial().
			State("flee").
			State("kite").
		EndRegion().

		// --- Phase Transitions ---

		// Enter combat when enemy is visible - combat has highest priority
		// From exploration
		When(string(EventEnemyVisible)).
			In("phase:exploration").
			GoTo("phase:combat").

		// From interaction (enemy surprised us while talking)
		When(string(EventEnemyVisible)).
			In("phase:interaction").
			GoTo("phase:combat").

		// From recovery (enemy caught up while healing)
		When(string(EventEnemyVisible)).
			In("phase:recovery").
			GoTo("phase:combat").

		// Return to exploration when all enemies are dead
		When(string(EventEnemyDead)).
			In("phase:combat").
			GoTo("phase:exploration").

		// Enter recovery mode when health is low and we can flee
		When(string(EventHealthLow)).
			In("phase:combat").
			GoTo("phase:recovery").
			If(func(s map[string]float64) bool {
				return s["can_flee"] > 0
			}).

		// Return to combat if we can't flee
		When(string(EventCannotFlee)).
			In("phase:recovery").
			GoTo("phase:combat").

		// Return to exploration once recovered
		When(string(EventRecoveryDone)).
			In("phase:recovery").
			GoTo("phase:exploration").

		// Enter interaction mode when NPC is adjacent
		When(string(EventNPCAdjacent)).
			In("phase:exploration").
			GoTo("phase:interaction").

		// Return to exploration when dialogue/shop ends
		When(string(EventDialogueEnded)).
			In("phase:interaction").
			GoTo("phase:exploration").

		// --- Exploration Sub-mode Transitions ---

		// Switch to loot mode when items nearby
		When(string(EventLootNearby)).
			In("explore_mode:explore").
			GoTo("explore_mode:loot").

		// Return to explore when done looting
		When(string(EventLootCollected)).
			In("explore_mode:loot").
			GoTo("explore_mode:explore").

		// Switch to find_key when needed for locked door (from explore or wander)
		When(string(EventKeyNeeded)).
			In("explore_mode:explore").
			GoTo("explore_mode:find_key").

		When(string(EventKeyNeeded)).
			In("explore_mode:wander").
			GoTo("explore_mode:find_key").

		When(string(EventKeyNeeded)).
			In("explore_mode:loot").
			GoTo("explore_mode:find_key").

		// Return to explore when key found
		When(string(EventKeyFound)).
			In("explore_mode:find_key").
			GoTo("explore_mode:explore").

		// Switch to wander when at dead end
		When(string(EventDeadEnd)).
			In("explore_mode:explore").
			GoTo("explore_mode:wander").

		// Return to explore when path found
		When(string(EventExitReached)).
			In("explore_mode:wander").
			GoTo("explore_mode:explore").

		// --- Combat Sub-mode Transitions ---

		// Switch to flee mode
		When(string(EventShouldFlee)).
			In("combat_mode:attack").
			GoTo("combat_mode:flee").

		// Switch back to attack (cornered)
		When(string(EventStandAndFight)).
			In("combat_mode:flee").
			GoTo("combat_mode:attack").

		Build()
}

// AIStateMachine wraps the state machine for runtime use by the AI.
type AIStateMachine struct {
	machine *statemachine.Machine
	chart   *statemachine.Chart
}

// NewAIStateMachine creates a new AI state machine instance.
func NewAIStateMachine() *AIStateMachine {
	chart := BuildAIStateMachine()
	return &AIStateMachine{
		machine: statemachine.NewMachine(chart),
		chart:   chart,
	}
}

// SendEvent dispatches an event to the state machine.
// Returns true if a transition occurred.
func (sm *AIStateMachine) SendEvent(event AIEvent) bool {
	return sm.machine.SendEvent(string(event))
}

// Phase returns the current high-level phase.
func (sm *AIStateMachine) Phase() AIPhase {
	return AIPhase(sm.machine.State("phase"))
}

// ExploreMode returns the current exploration sub-mode.
func (sm *AIStateMachine) ExploreMode() AISubphase {
	return AISubphase(sm.machine.State("explore_mode"))
}

// CombatMode returns the current combat sub-mode.
func (sm *AIStateMachine) CombatMode() string {
	return sm.machine.State("combat_mode")
}

// IsInPhase checks if we're in a specific phase.
func (sm *AIStateMachine) IsInPhase(phase AIPhase) bool {
	return sm.machine.IsIn("phase:" + string(phase))
}

// Mode returns the current mode as a string compatible with the old AI.Mode field.
// This provides backward compatibility during the transition.
func (sm *AIStateMachine) Mode() string {
	phase := sm.Phase()
	switch phase {
	case PhaseCombat:
		return "combat"
	case PhaseInteraction:
		return "interact"
	case PhaseRecovery:
		return "heal"
	case PhaseExploration:
		submode := sm.ExploreMode()
		switch submode {
		case SubphaseLoot:
			return "loot"
		case SubphaseFindKey:
			return "find_key"
		case SubphaseWander:
			return "wander"
		default:
			return "explore"
		}
	default:
		return "explore"
	}
}

// SetCanFlee updates the can_flee guard variable.
func (sm *AIStateMachine) SetCanFlee(canFlee bool) {
	state := sm.machine.GetState()
	if canFlee {
		state["can_flee"] = 1
	} else {
		state["can_flee"] = 0
	}
}

// Reset resets the state machine to initial state.
func (sm *AIStateMachine) Reset() {
	// Create a fresh machine
	sm.machine = statemachine.NewMachine(sm.chart)
}

// String returns a human-readable representation of the current state.
func (sm *AIStateMachine) String() string {
	return sm.machine.String()
}
