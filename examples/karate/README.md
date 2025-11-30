# Karate Fighting Game

A 2-player karate fighting game built with Petri nets and ODE-based AI.

## Features

- **Single Player (vs AI)**: Play against an AI opponent powered by ODE simulation
- **Multiplayer (PvP)**: Match with other players via WebSocket matchmaking
- **Petri Net Core**: Game logic modeled as a Petri net for formal verification
- **Real-time Web Client**: JavaScript client with visual game interface

## Game Mechanics

### Fighters
- **Health**: 100 HP - depleted health = defeat
- **Stamina**: 50 SP - required for all actions, regenerates over time
- **Position**: 5 positions (0-4), must be adjacent to attack

### Actions
| Action | Stamina Cost | Damage | Notes |
|--------|-------------|--------|-------|
| Punch | 5 | 10 | Quick attack |
| Kick | 8 | 15 | Medium attack |
| Special | 15 | 25 | Heavy attack |
| Block | 3 | - | Reduces incoming damage by 50% |
| Move Left/Right | 2 | - | Changes position |
| Recover | 0 | - | Restores 2 stamina |

### Combat
- Players must be adjacent (distance <= 1) to deal damage
- Both players submit actions simultaneously
- Actions resolve in the same turn
- Block only protects during the turn it's used

## Running the Game

### CLI Interactive Mode (Play vs AI)
```bash
make run-karate-cli
# or
go run ./examples/karate/cmd/cli
```

### CLI AI vs AI Mode
```bash
go run ./examples/karate/cmd/cli -ai
go run ./examples/karate/cmd/cli -ai -speed=200  # Faster playback
```

### CLI Benchmark Mode
```bash
# Test AI win rates
go run ./examples/karate/cmd/cli -bench=100 -p1=ode -p2=heuristic
```

Available AI types: `ode`, `heuristic`, `random`

### Demo Mode (AI vs AI - legacy)
```bash
make run-karate
# or
go run ./examples/karate/cmd -demo
```

### Server Mode
```bash
make run-karate-server
# or
go run ./examples/karate/cmd -port 8080
```

Then open `http://localhost:8080` in your browser.

### Generate Visualization
```bash
go run ./examples/karate/cmd -svg
```

## API Reference

### WebSocket Messages

Connect to `ws://localhost:8080/ws`

#### Client -> Server

**Join Game**
```json
{
  "type": "join",
  "payload": {
    "player_id": "player123",
    "mode": "ai"
  }
}
```
- `mode`: `"ai"` for single player, `"pvp"` for matchmaking

**Submit Action**
```json
{
  "type": "action",
  "payload": {
    "action": "punch"
  }
}
```
- Valid actions: `punch`, `kick`, `special`, `block`, `move_left`, `move_right`, `recover`

**Leave Game**
```json
{"type": "leave"}
```

#### Server -> Client

**Match Found**
```json
{
  "type": "match_found",
  "payload": {
    "session_id": "abc123",
    "player": 1,
    "opponent": "AI",
    "is_vs_ai": true
  }
}
```

**Game State**
```json
{
  "type": "game_state",
  "payload": {
    "state": {
      "p1_health": 100,
      "p1_stamina": 50,
      "p1_position": 1,
      "p1_blocking": false,
      "p2_health": 100,
      "p2_stamina": 50,
      "p2_position": 3,
      "p2_blocking": false,
      "game_over": false,
      "turn_num": 1
    },
    "available_actions": ["punch", "kick", "move_right", "recover"]
  }
}
```

**Game Over**
```json
{
  "type": "game_over",
  "payload": {
    "winner": 1,
    "state": {...}
  }
}
```

## JavaScript Client

```javascript
const client = new KarateClient('ws://localhost:8080/ws');

client.on('match_found', (data) => {
  console.log('Match started!', data);
});

client.on('game_state', ({state, availableActions}) => {
  console.log('State:', state);
  console.log('Can do:', availableActions);
});

client.on('game_over', ({winner}) => {
  console.log('Winner:', winner);
});

await client.connect();
client.joinGame('player1', 'ai');  // or 'pvp'

// Submit actions
client.submitAction('punch');
client.submitAction(KarateClient.Actions.KICK);
```

## Petri Net Model

The game uses a Petri net with:

**Places**:
- `P1_health`, `P2_health` - Fighter HP
- `P1_stamina`, `P2_stamina` - Fighter stamina
- `P1_pos0`-`P1_pos4`, `P2_pos0`-`P2_pos4` - Position tokens
- `P1_blocking`, `P2_blocking` - Block state
- `in_range` - Distance indicator
- `P1_wins`, `P2_wins` - Win conditions

**Transitions**:
- `P1_punch`, `P1_kick`, `P1_special`, `P1_block`
- `P1_move_left`, `P1_move_right`, `P1_recover`
- Same for P2

## AI Implementation

Three AI strategies are available:

### ODE AI (Default)
Uses ODE-based hypothesis evaluation:
1. For each available action, create a hypothetical state
2. Run ODE simulation forward
3. Score the final state: `P2_health - P1_health` (or vice versa for P1)
4. Choose the action with highest score

Uses `hypothesis.Evaluator` with `solver.FastOptions()` for quick evaluation.

### Heuristic AI
Simple rule-based strategy:
1. If low stamina, recover
2. If low health, block
3. If not in range, move toward opponent
4. If in range, attack (prefer kick > punch)

### Random AI
Picks a random available action each turn. Useful for baseline testing.

## AI Benchmark Results

Win rates from 100-game benchmarks:

| Matchup | P1 Wins | P2 Wins | Draws | Analysis |
|---------|---------|---------|-------|----------|
| ODE vs ODE | 0% | 0% | 100% | Symmetric stalemate |
| Heuristic vs Heuristic | 0% | 0% | 100% | Symmetric stalemate |
| ODE vs Random | 98% | 0% | 2% | ODE dominates |
| Heuristic vs Random | 47% | 13% | 40% | Heuristic beats random |
| **ODE vs Heuristic** | **100%** | **0%** | **0%** | **ODE much stronger** |

### Key Findings

1. **ODE AI significantly outperforms heuristic AI** - The ability to simulate future consequences gives a decisive advantage.

2. **Symmetric matchups result in draws** - When both players use identical deterministic strategies, they make the same decisions leading to 100-turn stalemates.

3. **No first-mover advantage** - The game is positionally balanced; P1 starts at position 1, P2 at position 3, with equal distances to center.

4. **Average game length**:
   - ODE vs Heuristic: ~11 turns (quick domination)
   - ODE vs Random: ~38 turns
   - Heuristic vs Random: ~58 turns

Run your own benchmarks:
```bash
go run ./examples/karate/cmd/cli -bench=100 -p1=ode -p2=heuristic
go run ./examples/karate/cmd/cli -bench=100 -p1=heuristic -p2=random
```

## Files

- `game.go` - Core game logic and Petri net model
- `game_test.go` - Unit tests
- `server/server.go` - WebSocket game server
- `cmd/main.go` - Server/demo CLI entry point
- `cmd/cli/main.go` - Interactive CLI client with AI benchmarking
- `cmd/client/` - Embedded web client files
- `client/karate-client.js` - JavaScript client module
- `client/index.html` - Web UI

## Architecture

```
┌─────────────────────────────────────────────────┐
│                  Web Browser                     │
│  ┌───────────────┐    ┌──────────────────────┐ │
│  │  index.html   │    │  karate-client.js    │ │
│  │  (UI/Canvas)  │◄──►│  (WebSocket Client)  │ │
│  └───────────────┘    └──────────────────────┘ │
└───────────────────────────┬─────────────────────┘
                            │ WebSocket
                            ▼
┌─────────────────────────────────────────────────┐
│                 Game Server                      │
│  ┌───────────────┐    ┌──────────────────────┐ │
│  │ server.go     │    │    game.go           │ │
│  │ - Sessions    │◄──►│    - Petri Net       │ │
│  │ - Matchmaking │    │    - ODE AI          │ │
│  │ - WebSocket   │    │    - State Machine   │ │
│  └───────────────┘    └──────────────────────┘ │
└─────────────────────────────────────────────────┘
```
