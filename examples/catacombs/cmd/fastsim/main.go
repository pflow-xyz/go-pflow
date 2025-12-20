// fastsim - Fast batch simulation (no per-action logging)
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pflow-xyz/go-pflow/examples/catacombs"
)

type GameResult struct {
	Seed        int64
	FinalLevel  int
	FinalHP     int
	TotalTicks  int
	Victory     bool
	GameOver    bool
	ActionCount int

	// Action counts
	MoveUp, MoveDown, MoveLeft, MoveRight int
	Attack, AimedShot, CombatMove          int
	Flee, EndTurn, UseItem                 int
	Talk, Descend                          int
	Other                                  int

	// Combat stats
	CombatTicks int

	// NPC interactions
	NPCTalks int
}

type Stats struct {
	TotalGames   int
	Victories    int
	Deaths       int
	TotalTicks   int64
	LevelDist    map[int]int
	Results      []GameResult

	// Aggregated action counts
	TotalMoveUp, TotalMoveDown, TotalMoveLeft, TotalMoveRight int64
	TotalAttack, TotalAimedShot, TotalCombatMove              int64
	TotalFlee, TotalEndTurn, TotalUseItem                     int64
	TotalTalk, TotalDescend, TotalOther                       int64
	TotalCombatTicks                                          int64
}

func main() {
	numGames := flag.Int("games", 1000, "Number of games to simulate")
	maxTicks := flag.Int("max-ticks", 50000, "Max ticks per game")
	workers := flag.Int("workers", 8, "Number of parallel workers")
	seed := flag.Int64("seed", 0, "Base seed (0 = random)")
	flag.Parse()

	fmt.Printf("Running %d games with %d workers...\n", *numGames, *workers)
	startTime := time.Now()

	baseSeed := *seed
	if baseSeed == 0 {
		baseSeed = time.Now().UnixNano()
	}

	// Use worker pool
	jobs := make(chan int64, *numGames)
	results := make(chan GameResult, *numGames)

	var completed int64
	var wg sync.WaitGroup

	// Start workers
	for w := 0; w < *workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for gameSeed := range jobs {
				result := runGame(gameSeed, *maxTicks)
				results <- result
				c := atomic.AddInt64(&completed, 1)
				if c%100 == 0 {
					fmt.Printf("Progress: %d/%d games (%.1f%%)\n", c, *numGames, float64(c)/float64(*numGames)*100)
				}
			}
		}()
	}

	// Send jobs
	go func() {
		for i := 0; i < *numGames; i++ {
			jobs <- baseSeed + int64(i)
		}
		close(jobs)
	}()

	// Collect results
	go func() {
		wg.Wait()
		close(results)
	}()

	stats := Stats{
		LevelDist: make(map[int]int),
		Results:   make([]GameResult, 0, *numGames),
	}

	for result := range results {
		stats.TotalGames++
		stats.Results = append(stats.Results, result)
		stats.TotalTicks += int64(result.TotalTicks)
		stats.LevelDist[result.FinalLevel]++

		if result.Victory {
			stats.Victories++
		}
		if result.GameOver {
			stats.Deaths++
		}

		// Aggregate action counts
		stats.TotalMoveUp += int64(result.MoveUp)
		stats.TotalMoveDown += int64(result.MoveDown)
		stats.TotalMoveLeft += int64(result.MoveLeft)
		stats.TotalMoveRight += int64(result.MoveRight)
		stats.TotalAttack += int64(result.Attack)
		stats.TotalAimedShot += int64(result.AimedShot)
		stats.TotalCombatMove += int64(result.CombatMove)
		stats.TotalFlee += int64(result.Flee)
		stats.TotalEndTurn += int64(result.EndTurn)
		stats.TotalUseItem += int64(result.UseItem)
		stats.TotalTalk += int64(result.Talk)
		stats.TotalDescend += int64(result.Descend)
		stats.TotalOther += int64(result.Other)
		stats.TotalCombatTicks += int64(result.CombatTicks)
	}

	duration := time.Since(startTime)
	printStats(stats, duration)
}

func runGame(seed int64, maxTicks int) GameResult {
	params := catacombs.DefaultParams()
	params.Seed = seed

	g := catacombs.NewGameWithParams(params)
	g.EnableAI()

	result := GameResult{
		Seed: seed,
	}

	for tick := 0; tick < maxTicks && !g.GameOver && !g.Victory; tick++ {
		action := g.AITick()
		result.TotalTicks++

		// Track combat
		if g.Combat.Active {
			result.CombatTicks++
		}

		// Track action types
		switch action {
		case catacombs.ActionMoveUp:
			result.MoveUp++
		case catacombs.ActionMoveDown:
			result.MoveDown++
		case catacombs.ActionMoveLeft:
			result.MoveLeft++
		case catacombs.ActionMoveRight:
			result.MoveRight++
		case catacombs.ActionAttack:
			result.Attack++
		case catacombs.ActionAimedShot:
			result.AimedShot++
		case catacombs.ActionCombatMove:
			result.CombatMove++
		case catacombs.ActionFlee:
			result.Flee++
		case catacombs.ActionEndTurn:
			result.EndTurn++
		case catacombs.ActionUseItem:
			result.UseItem++
		case catacombs.ActionTalk:
			result.Talk++
			result.NPCTalks++
		case catacombs.ActionDescend:
			result.Descend++
		default:
			result.Other++
		}
	}

	result.FinalLevel = g.Level
	result.FinalHP = g.Player.Health
	result.Victory = g.Victory
	result.GameOver = g.GameOver
	result.ActionCount = g.AI.ActionCount

	return result
}

func printStats(s Stats, duration time.Duration) {
	fmt.Println("\n==================================================")
	fmt.Println("BATCH SIMULATION RESULTS (1000 GAMES)")
	fmt.Println("==================================================")

	fmt.Printf("\nGames Played:    %d\n", s.TotalGames)
	fmt.Printf("Duration:        %v\n", duration)
	fmt.Printf("Games/Second:    %.1f\n", float64(s.TotalGames)/duration.Seconds())

	fmt.Println("\n--- Outcomes ---")
	fmt.Printf("Victories:       %d (%.1f%%)\n", s.Victories, float64(s.Victories)/float64(s.TotalGames)*100)
	fmt.Printf("Deaths:          %d (%.1f%%)\n", s.Deaths, float64(s.Deaths)/float64(s.TotalGames)*100)

	// Calculate averages
	avgTicks := float64(s.TotalTicks) / float64(s.TotalGames)
	avgLevel := 0.0
	avgHP := 0.0
	for _, r := range s.Results {
		avgLevel += float64(r.FinalLevel)
		avgHP += float64(r.FinalHP)
	}
	avgLevel /= float64(s.TotalGames)
	avgHP /= float64(s.TotalGames)

	fmt.Println("\n--- Level Stats ---")
	fmt.Printf("Average Level:   %.2f\n", avgLevel)
	fmt.Printf("Average HP:      %.1f\n", avgHP)

	fmt.Println("\n--- Performance ---")
	fmt.Printf("Total Ticks:     %d\n", s.TotalTicks)
	fmt.Printf("Avg Ticks/Game:  %.1f\n", avgTicks)

	// Level distribution
	fmt.Println("\n--- Level Distribution ---")
	levels := make([]int, 0, len(s.LevelDist))
	for l := range s.LevelDist {
		levels = append(levels, l)
	}
	sort.Ints(levels)

	maxLevel := 0
	for _, r := range s.Results {
		if r.FinalLevel > maxLevel {
			maxLevel = r.FinalLevel
		}
	}

	for _, l := range levels {
		count := s.LevelDist[l]
		pct := float64(count) / float64(s.TotalGames) * 100
		bar := ""
		for i := 0; i < int(pct/2); i++ {
			bar += "#"
		}
		outcome := ""
		wins := 0
		deaths := 0
		for _, r := range s.Results {
			if r.FinalLevel == l {
				if r.Victory {
					wins++
				}
				if r.GameOver {
					deaths++
				}
			}
		}
		if wins > 0 {
			outcome += fmt.Sprintf(" (W:%d)", wins)
		}
		if deaths > 0 {
			outcome += fmt.Sprintf(" (D:%d)", deaths)
		}
		fmt.Printf("  Level %2d: %4d (%5.1f%%) %s%s\n", l, count, pct, bar, outcome)
	}

	// Action stats
	totalMoves := s.TotalMoveUp + s.TotalMoveDown + s.TotalMoveLeft + s.TotalMoveRight
	totalActions := totalMoves + s.TotalAttack + s.TotalAimedShot + s.TotalCombatMove +
		s.TotalFlee + s.TotalEndTurn + s.TotalUseItem + s.TotalTalk + s.TotalDescend + s.TotalOther

	fmt.Println("\n--- Movement Stats ---")
	fmt.Printf("Total Moves:     %d\n", totalMoves)
	if totalMoves > 0 {
		fmt.Printf("  Up:            %d (%.1f%%)\n", s.TotalMoveUp, float64(s.TotalMoveUp)/float64(totalMoves)*100)
		fmt.Printf("  Down:          %d (%.1f%%)\n", s.TotalMoveDown, float64(s.TotalMoveDown)/float64(totalMoves)*100)
		fmt.Printf("  Left:          %d (%.1f%%)\n", s.TotalMoveLeft, float64(s.TotalMoveLeft)/float64(totalMoves)*100)
		fmt.Printf("  Right:         %d (%.1f%%)\n", s.TotalMoveRight, float64(s.TotalMoveRight)/float64(totalMoves)*100)
	}

	fmt.Println("\n--- Combat Stats ---")
	fmt.Printf("Combat Ticks:    %d (%.1f%% of actions)\n", s.TotalCombatTicks, float64(s.TotalCombatTicks)/float64(totalActions)*100)
	fmt.Printf("Attack:          %d\n", s.TotalAttack)
	fmt.Printf("Aimed Shot:      %d\n", s.TotalAimedShot)
	fmt.Printf("Combat Move:     %d\n", s.TotalCombatMove)
	fmt.Printf("Flee:            %d\n", s.TotalFlee)
	fmt.Printf("End Turn:        %d\n", s.TotalEndTurn)

	fmt.Println("\n--- NPC Interactions ---")
	fmt.Printf("Talk Actions:    %d\n", s.TotalTalk)
	npcTalks := 0
	for _, r := range s.Results {
		npcTalks += r.NPCTalks
	}
	gamesWithNPC := 0
	for _, r := range s.Results {
		if r.NPCTalks > 0 {
			gamesWithNPC++
		}
	}
	fmt.Printf("Games w/ NPC:    %d (%.1f%%)\n", gamesWithNPC, float64(gamesWithNPC)/float64(s.TotalGames)*100)

	fmt.Println("\n--- Other Actions ---")
	fmt.Printf("Use Item:        %d\n", s.TotalUseItem)
	fmt.Printf("Descend:         %d\n", s.TotalDescend)
	fmt.Printf("Other/Empty:     %d\n", s.TotalOther)

	// Key stats (we can't track this well without extra game state)
	keysCollected := 0
	for _, r := range s.Results {
		// This is a rough estimate - we'd need to track keys in game state
		// For now just show descend actions as proxy for level progress
		keysCollected += r.Descend
	}
	fmt.Println("\n--- Keys & Doors ---")
	fmt.Printf("Level Descends:  %d\n", s.TotalDescend)
	fmt.Printf("Avg Descends:    %.1f per game\n", float64(s.TotalDescend)/float64(s.TotalGames))

	// Longest/shortest games
	sort.Slice(s.Results, func(i, j int) bool {
		return s.Results[i].TotalTicks > s.Results[j].TotalTicks
	})

	fmt.Println("\n--- Longest Games ---")
	for i := 0; i < 5 && i < len(s.Results); i++ {
		r := s.Results[i]
		outcome := "IN PROGRESS"
		if r.Victory {
			outcome = "WIN"
		} else if r.GameOver {
			outcome = "DEAD"
		}
		fmt.Printf("  Seed %d: Level %d, %d ticks, HP %d (%s)\n",
			r.Seed, r.FinalLevel, r.TotalTicks, r.FinalHP, outcome)
	}

	// Shortest victories
	var victories []GameResult
	for _, r := range s.Results {
		if r.Victory {
			victories = append(victories, r)
		}
	}
	sort.Slice(victories, func(i, j int) bool {
		return victories[i].TotalTicks < victories[j].TotalTicks
	})

	if len(victories) > 0 {
		fmt.Println("\n--- Quickest Victories ---")
		for i := 0; i < 5 && i < len(victories); i++ {
			r := victories[i]
			fmt.Printf("  Seed %d: %d ticks, HP %d\n",
				r.Seed, r.TotalTicks, r.FinalHP)
		}
	}

	fmt.Println("\n==================================================")
	fmt.Println("END OF STATISTICS")
	fmt.Println("==================================================")
}

func init() {
	rand.Seed(time.Now().UnixNano())
}
