# Catacombs Game - Development Guide

## Architecture Overview

Catacombs is a roguelike dungeon crawler with:
- **Procedural dungeon generation** using BSP (Binary Space Partitioning)
- **Turn-based gameplay** with real-time web interface
- **AI system** for automated play and testing
- **WebSocket server** for browser-based play

## Key Files

| File | Purpose |
|------|---------|
| `game.go` | Core game logic, AI, dungeon generation |
| `ai_petri.go` | Petri net-based AI brain with ODE evaluation and memory |
| `ai_statemachine.go` | AI state machine using pflow statechart (phases, events) |
| `ai_evaluator.go` | Combat-specific ODE evaluator (separate from brain) |
| `ai_debug.go` | AI debugging tools (snapshots, local map, BFS debug) |
| `dungeon.go` | Dungeon generation (BSP), tiles, rooms |
| `npc.go` | NPC types, dialogue, merchants |
| `reachability.go` | Path analysis, reachability checks |
| `storage/storage.go` | SQLite session/action logging |
| `server/server.go` | WebSocket server, HTTP handlers |
| `cmd/main.go` | CLI entry point |
| `cmd/dbquery/main.go` | CLI tool to query session database |
| `ai_test.go` | AI behavior tests |

## Game State Flow

```
NewGame() → generateDungeon() → populateEnemies/NPCs/Items
    ↓
Player actions (move/attack/use) → state updates → check win/lose
    ↓
tryDescend() → new level → regenerate dungeon
```

## AI System

The AI uses a mode-based state machine:

```
┌─────────┐    ┌─────────┐    ┌─────────┐
│ explore │───▶│  loot   │───▶│ combat  │
└─────────┘    └─────────┘    └─────────┘
     │              │              │
     ▼              ▼              ▼
┌─────────┐    ┌─────────┐    ┌─────────┐
│ wander  │    │interact │    │  heal   │
└─────────┘    └─────────┘    └─────────┘
```

**Mode priorities** (checked in order):
1. `combat` - Enemy adjacent
2. `heal` - Low health + have potion
3. `loot` - Item visible and reachable
4. `interact` - NPC nearby
5. `find_key` - Locked door blocks exit, need key
6. `explore` - Path to exit exists
7. `wander` - Fallback when stuck

## AI State Machine Events (ai_statemachine.go)

The state machine uses typed events for transitions between phases:

### Phase Events (High-Level Mode Transitions)

| Event | Triggers | From → To |
|-------|----------|-----------|
| `EventEnemyVisible` | Enemy in range | exploration/interaction/recovery → combat |
| `EventEnemyDead` | All enemies killed | combat → exploration |
| `EventHealthLow` | HP below threshold | combat → recovery (if can flee) |
| `EventCannotFlee` | Cornered | recovery → combat |
| `EventRecoveryDone` | Healed enough | recovery → exploration |
| `EventNPCAdjacent` | NPC nearby | exploration → interaction |
| `EventDialogueEnded` | Conversation done | interaction → exploration |

### Exploration Sub-Events

| Event | Triggers | From → To |
|-------|----------|-----------|
| `EventLootNearby` | Item detected | explore → loot |
| `EventLootCollected` | Item picked up | loot → explore |
| `EventKeyNeeded` | Locked door blocks path | explore/wander/loot → find_key |
| `EventKeyFound` | Key acquired | find_key → explore |
| `EventDeadEnd` | No path forward | explore → wander |
| `EventExitReached` | At stairs | wander → explore |

### Combat Sub-Events

| Event | Triggers | From → To |
|-------|----------|-----------|
| `EventShouldFlee` | Danger too high | attack → flee |
| `EventStandAndFight` | Cannot escape | flee → attack |

## AI Decision Systems Overview

The AI has **two separate ODE-based decision systems** that serve different purposes:

### 1. AIBrain (ai_petri.go) - Strategic Decisions
- **Purpose**: High-level action selection (move to exit, attack, heal, explore)
- **Scope**: Entire game state (position, threats, items, progress)
- **Cache**: Disabled (state changes every tick → 0% hit rate)
- **Used for**: `brain.EvaluateActions()` to choose what to do next

### 2. CombatEvaluator (ai_evaluator.go) - Tactical Combat
- **Purpose**: Combat-specific decisions (attack, flee, heal, target selection)
- **Scope**: Combat situation only (HP, damage, enemy stats)
- **Cache**: Enabled with 5000 entries (99%+ hit rate - combat states repeat)
- **Used for**: `evaluator.EvaluateCombat()`, `ShouldFlee()`, `SelectBestTarget()`

### Why Two Systems?

| Aspect | AIBrain | CombatEvaluator |
|--------|---------|-----------------|
| State size | ~30 places | ~12 places |
| State changes | Every tick | Only in combat |
| Cache effectiveness | 0% | 99%+ |
| Decision complexity | Multiple goals | Single goal (survive + win) |

The separation allows combat decisions to benefit from caching while keeping strategic decisions fresh.

### CombatEvaluator API (ai_evaluator.go)

```go
eval := NewCombatEvaluator()

// Build a combat situation
sit := CombatSituation{
    PlayerHP: 80, PlayerMaxHP: 100, PlayerAttack: 15,
    EnemyHP: 50, EnemyMaxHP: 50, EnemyAttack: 10,
    CanFlee: true, HasHealPotion: true, PotionHealAmt: 30,
    EnemyCount: 1, PlayerArmor: 5,
}

// Get best action
action, score := eval.EvaluateCombat(sit)  // EvalActionAttack, EvalActionFlee, EvalActionHeal, EvalActionWait

// Convenience methods
shouldFlee := eval.ShouldFlee(sit)
shouldHeal := eval.ShouldHeal(sit)

// Pre-combat assessment (before engaging)
advice := eval.PreCombatAssessment(...)  // "heal_first", "engage", "wait", "flee_zone"

// Target selection (multiple enemies)
targetID, score := eval.SelectBestTarget(...)

// Mid-combat turn decisions (with AP costs)
turnAction, score := eval.EvaluateCombatTurn(sit, turnOpts)
```

## Petri Net AI Brain (ai_petri.go)

The AI can use a Petri net-based "brain" for ODE-driven decision making via the `hypothesis` package.

### Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                          AIBrain                                 │
├─────────────────────────────────────────────────────────────────┤
│ Petri Net Model                                                  │
│   Places: health, potions, dist_to_exit, threat_level, etc.     │
│   Transitions: move_to_exit, attack, heal, flee, wander, etc.   │
├─────────────────────────────────────────────────────────────────┤
│ Hypothesis Evaluator                                             │
│   - Converts game state → token counts                          │
│   - Generates action candidates                                  │
│   - Runs ODE simulation for each candidate                      │
│   - Scores outcomes (survival, progress, health)                │
│   - Selects best action                                         │
├─────────────────────────────────────────────────────────────────┤
│ Memory System                                                    │
│   - VisitedTiles: map[[2]int]int                                │
│   - KnownEnemies: last seen position, HP, aggression            │
│   - KnownItems: position, type, estimated value                 │
│   - DangerZones: positions where damage was taken               │
│   - RecentPath: oscillation detection                           │
├─────────────────────────────────────────────────────────────────┤
│ Goals                                                            │
│   - ExitReached, EnemiesKilled, ItemsCollected                  │
│   - LevelsComplete, TilesExplored                               │
└─────────────────────────────────────────────────────────────────┘
```

### Using the Brain

```go
// Enable AI (automatically creates brain)
g.EnableAI()

// Access the brain
brain := g.AI.Brain

// Update memory each tick
brain.UpdateMemory(g, tickNumber)

// Evaluate actions using ODE simulation
action, reason := brain.EvaluateActions(g)

// Or use the integrated method
action := g.AITickWithBrain()
```

### State to Tokens Conversion

The brain converts game state to Petri net tokens:

| Place | Description |
|-------|-------------|
| `health` | Current player HP |
| `potions` | Number of healing potions |
| `dist_to_exit` | Manhattan distance to stairs |
| `dist_to_enemy` | Distance to nearest enemy |
| `threat_level` | Combined danger score |
| `enemies_nearby` | Count of enemies within 5 tiles |
| `can_attack` | 1.0 if enemy adjacent |
| `can_heal` | 1.0 if have potion and hurt |
| `can_descend` | 1.0 if on exit stairs |

### Scoring Function

The evaluator scores outcomes:
```go
score := 0.0
if dead { return -10000 }
score += progress * 100
score += level_complete * 1000
score += health_pct * 50
score -= dist_to_exit * 0.5
score -= threat_level * 10
```

### Memory Features

```go
// Check if oscillating (stuck in loop)
if brain.IsOscillating() {
    // Force random walk
}

// Record damage for danger zone
brain.RecordDamage(x, y, damage)

// Check danger at position
danger := brain.GetDangerAt(x, y)

// Get current state
state := brain.GetState()
```

### Target Locking (Anti-Oscillation)

The brain uses **target locking** to prevent oscillation between goals:

```go
// Lock in on a target
brain.SetTarget("exit", exitX, exitY, tick, currentDist)

// Check if should switch targets
if brain.ShouldSwitchTarget(currentDist, tick, pathExists) {
    brain.ClearTarget()
    // Pick new target
}

// Get current target
targetType, x, y := brain.GetTarget()
```

**Targets are abandoned when:**
- Target reached (distance < 1)
- No path exists to target
- Stale: 50+ ticks without progress

### Memory Persistence

| Memory Type | Decay | Reset |
|-------------|-------|-------|
| VisitedTiles | None (accumulates) | On new game |
| KnownEnemies | None (updates position) | On new game |
| DangerZones | None (accumulates damage) | On new game |
| RecentPath | Rolling window (50 entries) | On new game |
| Target | Abandoned when stale/reached | On level change or manual clear |

**Note**: Memory does NOT decay over time. Danger zones accumulate indefinitely, which can cause the AI to avoid areas forever. Consider clearing danger zones on level change if this becomes problematic.

## Lessons Learned

### 1. Spawn Position Edge Cases

**Problem**: Items spawning at player spawn position caused AI to get stuck.

**Root cause**: `pickupItems()` only triggers on player movement. If an item spawns directly under the player, it's never picked up, but AI's loot logic sees it and returns empty action.

**Fix**: Prevent items from spawning at spawn position in `populateItems()`:
```go
if x == g.Dungeon.SpawnX && y == g.Dungeon.SpawnY {
    continue
}
```

**Lesson**: Always consider the initial state, not just state transitions. Test what happens when entities overlap at spawn time.

### 2. Debugging AI Stuck States

When the AI gets stuck:

1. **Check `LastAction`** - But beware stale values from previous levels
2. **Check mode transitions** - Is `aiDecideMode()` returning expected mode?
3. **Check BFS pathfinding** - Can the AI actually reach its target?
4. **Check for blocking entities** - Items, NPCs, enemies at key positions

**Debug pattern**:
```go
fmt.Printf("[AI] Mode=%s Pos=(%d,%d) Target=(%d,%d) Action=%s\n",
    g.AI.Mode, g.Player.X, g.Player.Y, targetX, targetY, action)
```

### 3. Level Transition Bugs

Level transitions (`tryDescend()`) are high-risk for bugs:
- State from previous level can leak (stale `LastAction`, etc.)
- New dungeon generation can create invalid configurations
- Player position relative to new entities matters

**Testing approach**: Run AI through multiple level transitions with fixed seeds to catch edge cases.

### 4. Pathfinding Considerations

BFS pathfinding (`aiFindPathBFS`) can fail when:
- Locked doors block all paths (need `find_key` mode)
- Enemies block narrow corridors
- Map has disconnected regions (generation bug)

When BFS fails, AI falls back to `wander` mode with random movement. This can take many ticks to resolve.

### 5. Mode Handler Empty Returns

**Problem**: AI wastes ticks when mode handlers return empty without taking action.

**Example**: `aiDecideMode()` sets `mode=combat`, but `aiEngageCombat()` finds target is dead/invalid, sets `mode=explore`, returns "". The tick is wasted - no action taken.

**Fix**: When mode handlers return empty (indicating invalid state), fall through to a sensible default:
```go
case "combat":
    if action := g.aiEngageCombat(); action != "" {
        return action
    }
    // Combat target invalid, fall through to explore
    return g.aiExplore()
```

**Lesson**: Mode handlers that can fail should either take action or allow fallthrough on the same tick.

### 6. State Machine Synchronization

**Problem**: AI gets stuck when mode handlers set `g.AI.Mode` directly but don't notify the state machine.

**Root cause**: The AI has both `g.AI.Mode` (string) and `g.AI.StateMachine` (pflow state machine). When `aiDecideModeStateMachine()` runs, it sets `g.AI.Mode = sm.Mode()`. If a mode handler like `aiLoot()` directly sets `g.AI.Mode = "explore"` without firing the corresponding event, the state machine stays in the old state. Next tick, `aiDecideModeStateMachine()` overwrites the mode back.

**Example**: Item detected → `EventLootNearby` fired → state machine enters "loot" → item picked up → `aiLoot()` finds no items, sets `g.AI.Mode = "explore"` → next tick: `g.AI.Mode = sm.Mode()` sets it back to "loot" → infinite loop.

**Fix**: Mode handlers MUST fire the corresponding event to sync the state machine:
```go
// In aiLoot() when no item found:
if item == nil {
    if g.AI.StateMachine != nil {
        g.AI.StateMachine.SendEvent(EventLootCollected)  // Sync state machine
    }
    g.AI.Mode = "explore"
    return ""
}
```

**Lesson**: When using dual state tracking (mode string + state machine), always keep them in sync by firing events, not just setting the mode string.

### 7. Testing with Seeds

Use fixed seeds for reproducible bugs:
```go
params := catacombs.DefaultParams()
params.Seed = 442  // Specific seed that triggers bug
g := catacombs.NewGameWithParams(params)
```

**Useful test seeds**:
- `442` - Previously triggered level 10 spawn item bug
- `123` - Previously triggered state machine sync bug (loot mode stuck)
- `456` - Quick completion seed for fast tests

## Running Tests

```bash
# All AI tests
go test ./examples/catacombs/... -v -run TestAI

# Specific test
go test ./examples/catacombs/... -v -run TestAICompletesLevel

# With timeout for stuck detection
go test ./examples/catacombs/... -v -run TestAI -timeout 60s
```

## Running the Server

```bash
# Default port 8080
go run ./examples/catacombs/cmd

# Custom port
go run ./examples/catacombs/cmd -port 8082

# Test specific seed in browser
# http://localhost:8080/?seed=442&infinite=1
```

## URL Parameters

| Parameter | Description |
|-----------|-------------|
| `seed=N` | Fixed RNG seed for reproducible dungeons |
| `infinite=1` | Infinite mode (no level 10 cap) |
| `ai=1` | Start with AI enabled |

## AI Debug Infrastructure

The `ai_debug.go` file provides debug tools for AI issues:

### AIDebugger - Snapshot Buffer

```go
// Create debugger with 100 snapshot buffer
debugger := catacombs.NewAIDebugger(100)
debugger.Enabled = true

// Add snapshots during AITick
debugger.AddSnapshot(catacombs.AIDebugSnapshot{
    Tick:         tick,
    Phase:        "before_decide",
    PlayerX:      g.Player.X,
    PlayerY:      g.Player.Y,
    Mode:         g.AI.Mode,
    StuckCounter: g.AI.StuckCounter,
})

// Find stuck patterns
patterns := debugger.FindStuckPattern()
```

### DebugLocalMap - Visual Grid

```go
// Get 5-tile radius around player
mapStr := catacombs.DebugLocalMap(g, 5)
fmt.Println(mapStr)
// Output:
// . . . # .
// . @ . # .  // @ = player
// . . E # .  // E = enemy
// . I . # .  // I = item
```

### DebugBFSPath - Pathfinding Debug

```go
// Debug why BFS fails to reach target
pathInfo := catacombs.DebugBFSPath(g, targetX, targetY)
fmt.Println(pathInfo)
// Shows visited tiles, blockers, and path
```

### PrintDiagnosis - AI State Dump

```go
// Full diagnostic output
diagnosis := catacombs.PrintDiagnosis(g)
fmt.Println(diagnosis)
// Shows mode, position, nearby entities, path status
```

## Common Debug Patterns

### Trace AI decisions for N ticks
```go
for i := 0; i < 100; i++ {
    before := g.Player
    action := g.AITick()
    fmt.Printf("T%d: (%d,%d) mode=%s action=%s\n",
        i, before.X, before.Y, g.AI.Mode, action)
}
```

### Check items at player position
```go
for _, item := range g.Items {
    if item.X == g.Player.X && item.Y == g.Player.Y {
        fmt.Printf("Item under player: %s\n", item.Item.Name)
    }
}
```

### Visualize local map
```go
for y := g.Player.Y - 3; y <= g.Player.Y + 3; y++ {
    for x := g.Player.X - 5; x <= g.Player.X + 5; x++ {
        tile := g.Dungeon.Tiles[y][x]
        // Print tile character
    }
}
```

## Headless Browser Debugging

The `debugging/` folder contains Puppeteer-based scripts for headless browser testing. These are useful for:
- Automated UI testing without manual interaction
- Capturing screenshots of specific game states
- Testing combat system and AI behavior programmatically

### Setup

```bash
# From project root
npm install puppeteer
```

### Available Scripts

| Script | Purpose |
|--------|---------|
| `debugging/test_combat_ui.js` | Test combat UI by moving player toward enemies |
| `debugging/test_ai_combat.js` | Watch AI play and detect combat encounters |

### Running Headless Tests

```bash
# Start the game server first
go run ./examples/catacombs/cmd -port 8082

# In another terminal, run headless tests
node debugging/test_combat_ui.js
node debugging/test_ai_combat.js
```

### test_combat_ui.js

Tests manual combat initiation:
1. Loads game with specific seed
2. Finds nearest enemy
3. Moves player toward enemy
4. Presses Tab to initiate combat
5. Captures screenshot to `/tmp/combat_test.png`

Useful for testing:
- Combat panel visibility
- WebSocket game state updates
- Player movement toward targets

### test_ai_combat.js

Tests AI-driven combat detection:
1. Loads game with seed
2. Enables AI mode via UI toggle
3. Watches for combat state for up to 100 ticks
4. Logs AI mode, position, health at intervals
5. Captures screenshot when combat detected

Useful for testing:
- AI combat engagement
- Combat state propagation to UI
- Long-running AI behavior

### Accessing Game State in Browser

The game exposes `window.getGameState()` for testing:

```javascript
// In Puppeteer
const state = await page.evaluate(() => {
    const gs = window.getGameState();
    return {
        playerPos: { x: gs.player.x, y: gs.player.y },
        combat: gs.combat,
        enemies: gs.enemies.map(e => ({ name: e.name, x: e.x, y: e.y }))
    };
});
```

### Debugging Tips

**Capture specific states**:
```javascript
// Take screenshot when condition is met
if (state.combat && state.combat.active) {
    await page.screenshot({ path: '/tmp/combat_state.png', fullPage: true });
}
```

**Watch console logs**:
```javascript
page.on('console', msg => console.log('BROWSER:', msg.text()));
page.on('pageerror', err => console.log('PAGE ERROR:', err.message));
```

**Test with different seeds**:
```javascript
await page.goto('http://localhost:8082/?seed=42&ai=1');
```

## SQLite Session Logging

Sessions and actions are logged to SQLite for replay analysis and debugging.

### Server Flags

```bash
# Default: logs to catacombs.db in current directory
go run ./examples/catacombs/cmd

# Custom database path
go run ./examples/catacombs/cmd -db /path/to/sessions.db

# Disable SQLite logging
go run ./examples/catacombs/cmd -no-db
```

### Database Schema

**sessions** table:
| Column | Description |
|--------|-------------|
| `id` | Session ID |
| `seed` | RNG seed |
| `mode` | "normal", "demo", "infinite" |
| `started_at` | Session start time |
| `ended_at` | Session end time |
| `final_level` | Last level reached |
| `final_hp` | HP when session ended |
| `total_ticks` | Total actions taken |
| `ai_enabled` | Whether AI was used |
| `game_over` | Player died |
| `victory` | Player won |

**actions** table:
| Column | Description |
|--------|-------------|
| `session_id` | Foreign key to session |
| `tick` | Action number |
| `level` | Dungeon level |
| `action` | Action name (e.g., "ai_move_up") |
| `player_x`, `player_y` | Position |
| `player_hp`, `player_max_hp` | Health |
| `ai_mode`, `ai_target` | AI state |
| `enemies_alive` | Enemy count |
| `in_combat` | Combat active |

### Query Tool (dbquery)

```bash
# List recent sessions
go run ./examples/catacombs/cmd/dbquery -cmd recent

# Find sessions by seed
go run ./examples/catacombs/cmd/dbquery -cmd seed -seed 1764721355276511500

# Show session details and level summaries
go run ./examples/catacombs/cmd/dbquery -cmd session -session <id>

# Show action log for a session
go run ./examples/catacombs/cmd/dbquery -cmd level -session <id>

# Show actions for specific level only
go run ./examples/catacombs/cmd/dbquery -cmd level -session <id> -level 9

# Export session as JSON
go run ./examples/catacombs/cmd/dbquery -cmd export -session <id> > session.json
```

### Programmatic Access

```go
import "github.com/pflow-xyz/go-pflow/examples/catacombs/storage"

store, _ := storage.New("catacombs.db")
defer store.Close()

// Query sessions by seed
sessions, _ := store.GetSessionsBySeed(1764721355276511500)

// Get all actions for a session
actions, _ := store.GetActions(sessionID)

// Get actions for specific level
actions, _ := store.GetActionsForLevel(sessionID, 9)

// Get level summaries (ticks, HP changes, combat count per level)
summaries, _ := store.GetLevelSummaries(sessionID)

// Export full session as JSON
jsonData, _ := store.ExportSessionJSON(sessionID)
```

## Performance Notes

- Level 10 with seed 442 takes ~6000 ticks (vs ~100-300 for other levels)
- This is due to pathfinding difficulties on that particular layout
- Not a bug, but indicates room for pathfinding optimization
- Consider A* or better heuristics if performance becomes critical
