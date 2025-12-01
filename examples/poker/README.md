# Texas Hold'em Poker - Petri Net Edition

A Texas Hold'em poker implementation demonstrating how Petri nets can model game state and ODE simulation can help with bet estimation.

## Overview

This example shows how Petri nets can be used to:
- Model the flow of a Texas Hold'em poker hand through phases
- Track player states (active, folded, chip counts)
- Represent betting actions as transitions
- Use ODE-based simulation for bet sizing decisions

## Game Rules

**Texas Hold'em** (No-Limit):
- Each player is dealt 2 hole cards
- 5 community cards are revealed across 3 stages:
  - **Flop**: 3 cards
  - **Turn**: 1 card
  - **River**: 1 card
- Players bet in rounds after each stage
- Best 5-card hand using any combination of hole and community cards wins
- Players can **Fold**, **Check**, **Call**, **Raise**, or go **All-in**

## Quick Start

```bash
# Build
go build -o poker ./cmd

# Play with ODE-based AI vs Random AI
./poker

# Play with verbose output (see bet evaluations)
./poker -v

# Analyze the Petri net model
./poker --analyze

# Benchmark different strategies
./poker --benchmark --games 100
```

## Command Line Options

| Flag | Default | Description |
|------|---------|-------------|
| `-p1` | ode | Player 1 strategy (human, random, ode) |
| `-p2` | random | Player 2 strategy (human, random, ode) |
| `-delay` | 1 | Delay between moves in seconds |
| `-v` | false | Verbose mode (show evaluation details) |
| `-benchmark` | false | Run benchmark mode |
| `-games` | 100 | Number of games for benchmark |
| `-analyze` | false | Analyze the Petri net model |
| `-chips` | 1000 | Initial chip count |
| `-sb` | 1 | Small blind |
| `-bb` | 2 | Big blind |

## Petri Net Model

The game is modeled as a Petri net with the following structure:

### Places

**Phase Places**:
- `phase_preflop`: Pre-flop betting
- `phase_flop`: Post-flop betting
- `phase_turn`: Post-turn betting
- `phase_river`: Post-river betting
- `phase_showdown`: Final comparison
- `phase_complete`: Hand finished

**Player State Places** (for each player):
- `p*_active`: Player still in hand
- `p*_folded`: Player has folded
- `p*_acted`: Player has acted this round
- `p*_turn`: It's this player's turn
- `p*_bet`: Amount bet this round
- `p*_chips`: Stack size
- `p*_hand_str`: Normalized hand strength (0-1) - computed via ODE
- `p*_wins`: Win accumulator

**Hand Strength ODE Places** (for each player):
- `p*_rank_input`: Normalized hand rank score (0-1) from card evaluation
- `p*_highcard_input`: Normalized high card value (0-1) from card evaluation
- `p*_str_delta`: Change in hand strength from ODE dynamics

**Betting Places**:
- `pot`: Current pot size
- `bet_to_call`: Amount needed to call
- `min_raise`: Minimum raise amount

### Transitions

**Phase Transitions**:
- `deal_hole`: Deal hole cards
- `deal_flop`: Deal flop
- `deal_turn`: Deal turn
- `deal_river`: Deal river
- `to_showdown`: Move to showdown
- `end_hand`: Complete hand

**Action Transitions** (per player):
- `p*_fold`: Fold hand
- `p*_check`: Check (no bet)
- `p*_call`: Call current bet
- `p*_raise`: Raise the bet
- `p*_all_in`: Go all-in

**Hand Strength ODE Transitions** (per player):
- `p*_compute_str`: Computes hand strength from rank and highcard inputs
- `p*_update_str`: Updates hand strength from delta

**Win Transitions**:
- `p1_wins_pot`: Player 1 wins pot
- `p2_wins_pot`: Player 2 wins pot

## ODE-Based Hand Strength Computation

A key feature is that hand strength is computed through the ODE dynamics rather than
being set directly. This models hand strength as a continuous flow through the Petri net:

1. **Input Places**: `p*_rank_input` and `p*_highcard_input` receive normalized values from card evaluation
2. **Computation Transitions**: `p*_compute_str` combines inputs using mass-action kinetics
3. **Update Transitions**: `p*_update_str` flows the computed strength to `p*_hand_str`

The ODE-based computation uses the formula:
```
strength = (rank_norm × 0.9) + (highcard_norm × 0.1) + delta_adjustment
```

Where:
- `rank_norm`: Normalized hand rank (0 = High Card, 1 = Royal Flush)
- `highcard_norm`: Normalized high card value (2-14 → 0.14-1.0)
- `delta_adjustment`: Small adjustment from ODE dynamics (allows smooth transitions)

## ODE-Based Bet Estimation

The key innovation is using ODE simulation for betting decisions:

### Hand Strength

Hand strength is normalized to 0-1 based on poker hand rankings:
- High Card: ~0.007
- One Pair: ~0.11-0.12
- Two Pair: ~0.22-0.24
- Three of Kind: ~0.33-0.35
- Straight: ~0.44-0.46
- Flush: ~0.55-0.57
- Full House: ~0.66-0.68
- Four of Kind: ~0.77-0.79
- Straight Flush: ~0.88-0.90
- Royal Flush: ~0.99

### Expected Value Calculation

For each action, we calculate expected value:

**Fold**:
```
EV = -pot/4  (losing your investment)
```

**Check**:
```
EV = strength × pot
```

**Call**:
```
EV = strength × (pot + toCall) - (1 - strength) × toCall
```

**Raise**:
```
foldEquity = 0.3 × (1 - strength)
EV = foldEquity × pot + (1 - foldEquity) × [strength × (pot + amount) - (1 - strength) × amount]
```

### Rate Adjustments

Transition rates are dynamically adjusted based on:

1. **Hand Strength**: Strong hands increase raise rates, decrease fold rates
2. **Position**: Button position gets aggressive bonus
3. **Pot Odds**: Good odds increase call rate

```go
// Strength-adjusted rates
rates["p1_fold"] = baseRate × (1.0 - handStrength)
rates["p1_raise"] = baseRate × handStrength × 2.0
rates["p1_call"] = baseRate × (0.5 + handStrength × 0.5)
```

## Adversarial Monitoring

The model includes card tracking and opponent range estimation:

### Card Tracking

The `CardTracker` monitors:
- **Known cards**: Community cards and our hole cards
- **Dead cards**: Cards that can't be in opponent's hand
- **Remaining deck**: All possible cards opponent might hold

### Opponent Hand Range Estimation

Based on visible cards, the AI estimates:
- **Hand type probabilities**: Likelihood of opponent having pair, two pair, flush, etc.
- **Estimated strength**: Weighted average of possible opponent hands
- **Danger cards**: Cards that would significantly improve opponent's hand

```go
// Estimate opponent's likely range
estimate := tracker.EstimateOpponentRange(aggressionFactor)
fmt.Printf("Opponent est. strength: %.1f%%\n", estimate.EstimatedStrength*100)
fmt.Printf("Pair probability: %.1f%%\n", estimate.OnePairProb*100)
```

### Aggression Tracking

Opponent betting patterns are tracked to refine range estimates:
- **Fold** = 0.0 (very weak)
- **Check** = 0.4 (slightly weak or trapping)  
- **Call** = 0.5 (neutral)
- **Raise** = 0.5-0.8 (based on bet size vs pot)
- **All-in** = 1.0 (maximum strength or bluff)

### Board Texture Analysis

The board is analyzed for:
- **Flush draws**: 3+ cards of same suit
- **Straight draws**: Connected cards
- **Paired boards**: Increases full house/quads risk
- **High cards**: Broadway card count (T, J, Q, K, A)

### Adversarial Analysis Output

When using verbose mode (`-v`), the AI shows:

```
Player 1 (ode) evaluating...
  Hand: One Pair (K high) (strength: 0.120)
  Pot: 100, To Call: 50, Pot Odds: 66.7%
  Opponent aggression: 70.0%, Est. opponent strength: 8.5%
  Equity advantage: +3.5%, Board danger: 25.0%
  Watch for: A♥ K♦ Q♣ J♠ 10♥
```

This helps the AI make informed decisions by:
1. Comparing our hand strength to estimated opponent strength
2. Adjusting fold equity based on opponent tendencies
3. Penalizing plays on dangerous boards
4. Identifying specific cards that could hurt us

## Example Games

### ODE vs Random

```bash
$ ./poker -p1 ode -p2 random -v

=== Pre-flop ===
Pot: 3 | Current Bet: 2
Community: 

Player 1: K♠ Q♥ | Chips: 999 | Bet: 1 | High Card (K high)
Player 2: 7♣ 4♦ | Chips: 998 | Bet: 2 | High Card (7 high)

Player 1 (ode) evaluating...
  Hand: High Card (K high) (strength: 0.016)
  Pot: 3, To Call: 1, Pot Odds: 75.0%
  Opponent aggression: 50.0%, Est. opponent strength: 1.2%
  Equity advantage: +0.4%, Board danger: 0.0%
    Fold: EV = -1 (adjusted for danger 0.0%)
    Call: EV = 1.8 (win prob 52.3% vs opp strength 0.012)
    Raise 3: EV = 2.5 (fold equity 60.0%, win prob 52.3%)
  Best action: Raise (3) with score 2.500
```

### Benchmark Results

```bash
$ ./poker --benchmark --games 100

=== Summary ===
Win rate matrix (P1 wins %):
          random     ode
  random    35.0%     5.0%
     ode    92.0%    48.0%
```

The ODE-based AI significantly outperforms random play by:
- Folding weak hands
- Value betting strong hands
- Using pot odds correctly
- Applying appropriate aggression
- **Tracking opponent tendencies** (aggression factor)
- **Estimating opponent hand ranges** based on visible cards
- **Analyzing board danger** to avoid traps

## Hand Evaluation

The hand evaluator correctly identifies all poker hands:

| Hand | Example | Detection |
|------|---------|-----------|
| Royal Flush | A♠ K♠ Q♠ J♠ 10♠ | Straight + Flush + Ace high |
| Straight Flush | 9♥ 8♥ 7♥ 6♥ 5♥ | Straight + Flush |
| Four of a Kind | A♠ A♥ A♦ A♣ K♠ | 4 same rank |
| Full House | K♠ K♥ K♦ Q♠ Q♥ | 3 + 2 same rank |
| Flush | A♥ J♥ 8♥ 5♥ 3♥ | 5 same suit |
| Straight | 9♠ 8♥ 7♦ 6♣ 5♠ | 5 consecutive ranks |
| Three of Kind | Q♠ Q♥ Q♦ 9♠ 7♥ | 3 same rank |
| Two Pair | J♠ J♥ 8♠ 8♦ A♣ | 2 pairs |
| One Pair | 10♠ 10♥ A♠ K♥ Q♦ | 2 same rank |
| High Card | A♠ K♥ Q♦ J♣ 9♠ | Nothing special |

Special cases handled:
- **Wheel Straight**: A-2-3-4-5 (Five high)
- **Best 5 of 7**: Evaluates all 21 combinations

## Architecture

```
poker/
├── cards.go        # Card, Deck, Hand evaluation
├── cards_test.go   # Card tests
├── tracker.go      # Card tracking and opponent range estimation
├── tracker_test.go # Tracker tests
├── model.go        # Petri net model creation
├── model_test.go   # Model tests  
├── game.go         # Game logic and AI
├── game_test.go    # Game tests
├── cmd/
│   └── main.go     # CLI application
└── README.md       # This file
```

## How Petri Nets Help

1. **State Representation**: The Petri net naturally models the game state through token distribution across places.

2. **Action Modeling**: Player actions are transitions that move tokens between states.

3. **Rate-Based Decisions**: Transition rates encode betting preferences based on hand strength and position.

4. **ODE Simulation**: The continuous interpretation allows simulating expected outcomes.

5. **Analysis**: Reachability analysis can identify possible game states and detect issues.

## Extensions

Possible extensions to this model:

1. **Multi-player**: Extend to more than 2 players
2. **Tournament**: Multiple hands with chip accumulation
3. **Monte Carlo**: Sample community cards for better equity estimates
4. **Opponent Modeling**: Track opponent patterns over time
5. **Bluffing**: Add random bluff factor to raise rates
6. **Hand Ranges**: Model opponent hand ranges for better decisions

## References

- [Texas Hold'em Rules](https://en.wikipedia.org/wiki/Texas_hold_%27em)
- [Poker Hand Rankings](https://en.wikipedia.org/wiki/List_of_poker_hands)
- [Pot Odds and Equity](https://www.pokerstrategy.com/strategy/bss/pot-odds-and-equity/)
- [Expected Value in Poker](https://www.pokernews.com/strategy/what-is-expected-value-ev-32427.htm)
