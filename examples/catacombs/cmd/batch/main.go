// batch - Run batch simulations and collect stats
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"time"

	"github.com/pflow-xyz/go-pflow/examples/catacombs"
	"github.com/pflow-xyz/go-pflow/examples/catacombs/storage"
)

type GameStats struct {
	SessionID   string
	Seed        int64
	FinalLevel  int
	FinalHP     int
	TotalTicks  int
	Victory     bool
	GameOver    bool
	ActionCount int
}

type ActionStats struct {
	MoveUp      int
	MoveDown    int
	MoveLeft    int
	MoveRight   int
	Attack      int
	Wait        int
	UseItem     int
	Descend     int
	Interact    int
	Talk        int
	OpenDoor    int
	Other       int
}

type NPCStats struct {
	TotalInteractions int
	TalkEvents        int
	MerchantVisits    int
}

type KeyStats struct {
	KeysCollected   int
	DoorsUnlocked   int
	GamesWithKeys   int
}

type CombatStats struct {
	TotalCombats    int
	EnemiesKilled   int
	PlayerDeaths    int
	FleeAttempts    int
}

type BatchResults struct {
	GamesPlayed     int
	Victories       int
	Deaths          int
	AvgLevel        float64
	MaxLevel        int
	MinLevel        int
	AvgTicks        float64
	AvgHP           float64
	TotalTicks      int
	LevelDistrib    map[int]int
	ActionStats     ActionStats
	NPCStats        NPCStats
	KeyStats        KeyStats
	CombatStats     CombatStats
	GameResults     []GameStats
	Duration        time.Duration
}

func main() {
	numGames := flag.Int("games", 1000, "Number of games to simulate")
	dbPath := flag.String("db", "catacombs.db", "SQLite database path")
	maxTicks := flag.Int("max-ticks", 50000, "Max ticks per game")
	verbose := flag.Bool("v", false, "Verbose output")
	seed := flag.Int64("seed", 0, "Base seed (0 = random)")
	flag.Parse()

	fmt.Printf("Running %d games...\n", *numGames)
	startTime := time.Now()

	// Initialize storage
	store, err := storage.New(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	results := BatchResults{
		LevelDistrib: make(map[int]int),
		GameResults:  make([]GameStats, 0, *numGames),
	}

	baseSeed := *seed
	if baseSeed == 0 {
		baseSeed = time.Now().UnixNano()
	}

	for i := 0; i < *numGames; i++ {
		gameSeed := baseSeed + int64(i)
		stats := runGame(gameSeed, *maxTicks, store, *verbose)

		results.GamesPlayed++
		results.GameResults = append(results.GameResults, stats)

		if stats.Victory {
			results.Victories++
		}
		if stats.GameOver {
			results.Deaths++
		}

		results.TotalTicks += stats.TotalTicks
		results.LevelDistrib[stats.FinalLevel]++

		if stats.FinalLevel > results.MaxLevel {
			results.MaxLevel = stats.FinalLevel
		}
		if results.MinLevel == 0 || stats.FinalLevel < results.MinLevel {
			results.MinLevel = stats.FinalLevel
		}

		if *verbose || (i+1)%100 == 0 {
			fmt.Printf("Progress: %d/%d games (%.1f%%)\n", i+1, *numGames, float64(i+1)/float64(*numGames)*100)
		}
	}

	results.Duration = time.Since(startTime)
	results.AvgLevel = 0
	results.AvgHP = 0
	results.AvgTicks = float64(results.TotalTicks) / float64(results.GamesPlayed)

	for _, gs := range results.GameResults {
		results.AvgLevel += float64(gs.FinalLevel)
		results.AvgHP += float64(gs.FinalHP)
	}
	results.AvgLevel /= float64(results.GamesPlayed)
	results.AvgHP /= float64(results.GamesPlayed)

	// Query database for detailed action stats
	queryActionStats(store, &results)

	// Print results
	printResults(results)
}

func runGame(seed int64, maxTicks int, store *storage.Store, verbose bool) GameStats {
	params := catacombs.DefaultParams()
	params.Seed = seed

	g := catacombs.NewGameWithParams(params)
	g.EnableAI()

	sessionID := fmt.Sprintf("batch-%d-%d", seed, time.Now().UnixNano())
	store.CreateSession(sessionID, seed, "normal")

	tick := 0
	hasKey := false
	for tick < maxTicks && !g.GameOver && !g.Victory {
		hadKey := hasKey
		action := g.AITick()
		tick++
		hasKey = g.Player.Keys["rusty_key"]

		// Build extra data for key tracking
		extraData := ""
		if hasKey && !hadKey {
			extraData = "obtained_key"
		}

		// Log every action for complete stats
		store.LogAction(&storage.Action{
				SessionID:    sessionID,
				Tick:         tick,
				Level:        g.Level,
				Action:       string(action),
				PlayerX:      g.Player.X,
				PlayerY:      g.Player.Y,
				PlayerHP:     g.Player.Health,
				PlayerMaxHP:  g.Player.MaxHealth,
				AIMode:       g.AI.Mode,
				EnemiesAlive: countAliveEnemies(g),
				InCombat:     g.Combat.Active,
				ExtraData:    extraData,
		})

		if verbose && tick%1000 == 0 {
			fmt.Printf("  Seed %d: tick %d, level %d, HP %d/%d\n",
				seed, tick, g.Level, g.Player.Health, g.Player.MaxHealth)
		}
	}

	// Update session
	store.EndSession(sessionID, g.Level, g.Player.Health, tick, g.GameOver, g.Victory)

	return GameStats{
		SessionID:   sessionID,
		Seed:        seed,
		FinalLevel:  g.Level,
		FinalHP:     g.Player.Health,
		TotalTicks:  tick,
		Victory:     g.Victory,
		GameOver:    g.GameOver,
		ActionCount: g.AI.ActionCount,
	}
}

func countAliveEnemies(g *catacombs.Game) int {
	count := 0
	for _, e := range g.Enemies {
		if e.State != catacombs.StateDead {
			count++
		}
	}
	return count
}

func queryActionStats(store *storage.Store, results *BatchResults) {
	// Query action type distribution from database
	db := store.DB()

	rows, err := db.Query(`
		SELECT action, COUNT(*) as count
		FROM actions
		GROUP BY action
		ORDER BY count DESC
	`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error querying actions: %v\n", err)
		return
	}
	defer rows.Close()

	fmt.Println("\n=== Action Distribution ===")
	for rows.Next() {
		var action string
		var count int
		rows.Scan(&action, &count)
		fmt.Printf("  %-20s %d\n", action, count)

		// Aggregate into ActionStats
		switch action {
		case "move_up", "ai_move_up":
			results.ActionStats.MoveUp += count
		case "move_down", "ai_move_down":
			results.ActionStats.MoveDown += count
		case "move_left", "ai_move_left":
			results.ActionStats.MoveLeft += count
		case "move_right", "ai_move_right":
			results.ActionStats.MoveRight += count
		case "attack", "ai_attack":
			results.ActionStats.Attack += count
		case "wait", "ai_wait":
			results.ActionStats.Wait += count
		case "use_item", "ai_use_item":
			results.ActionStats.UseItem += count
		case "descend", "ai_descend":
			results.ActionStats.Descend += count
		case "interact", "ai_interact":
			results.ActionStats.Interact += count
		case "talk", "ai_talk":
			results.ActionStats.Talk += count
		case "open_door", "ai_open_door":
			results.ActionStats.OpenDoor += count
		default:
			results.ActionStats.Other += count
		}
	}

	// Query combat stats
	var combatTicks int
	db.QueryRow(`SELECT COUNT(*) FROM actions WHERE in_combat = 1`).Scan(&combatTicks)
	results.CombatStats.TotalCombats = combatTicks

	// Query NPC interactions
	var npcInteractions int
	db.QueryRow(`SELECT COUNT(*) FROM actions WHERE action LIKE '%interact%' OR action LIKE '%talk%'`).Scan(&npcInteractions)
	results.NPCStats.TotalInteractions = npcInteractions

	// Query level progression
	fmt.Println("\n=== Sessions by Final Level ===")
	rows2, _ := db.Query(`
		SELECT final_level, COUNT(*) as count,
		       AVG(total_ticks) as avg_ticks,
		       AVG(final_hp) as avg_hp
		FROM sessions
		GROUP BY final_level
		ORDER BY final_level
	`)
	defer rows2.Close()

	for rows2.Next() {
		var level, count int
		var avgTicks, avgHP float64
		rows2.Scan(&level, &count, &avgTicks, &avgHP)
		fmt.Printf("  Level %2d: %4d games (avg ticks: %.0f, avg HP: %.1f)\n",
			level, count, avgTicks, avgHP)
	}

	// Query AI mode distribution
	fmt.Println("\n=== AI Mode Distribution ===")
	rows3, _ := db.Query(`
		SELECT ai_mode, COUNT(*) as count
		FROM actions
		WHERE ai_mode IS NOT NULL AND ai_mode != ''
		GROUP BY ai_mode
		ORDER BY count DESC
	`)
	defer rows3.Close()

	for rows3.Next() {
		var mode string
		var count int
		rows3.Scan(&mode, &count)
		fmt.Printf("  %-15s %d\n", mode, count)
	}

	// Query key/door stats using extra_data
	fmt.Println("\n=== Key/Door Stats ===")
	var keyActions, doorActions int
	db.QueryRow(`SELECT COUNT(*) FROM actions WHERE action LIKE '%key%' OR extra_data LIKE '%key%'`).Scan(&keyActions)
	db.QueryRow(`SELECT COUNT(*) FROM actions WHERE action LIKE '%door%'`).Scan(&doorActions)
	fmt.Printf("  Key-related actions: %d\n", keyActions)
	fmt.Printf("  Door-related actions: %d\n", doorActions)
	results.KeyStats.KeysCollected = keyActions
	results.KeyStats.DoorsUnlocked = doorActions
}

func printResults(r BatchResults) {
	fmt.Println("\n" + "==================================================")
	fmt.Println("BATCH SIMULATION RESULTS")
	fmt.Println("==================================================")

	fmt.Printf("\nGames Played:    %d\n", r.GamesPlayed)
	fmt.Printf("Duration:        %v\n", r.Duration)
	fmt.Printf("Games/Second:    %.1f\n", float64(r.GamesPlayed)/r.Duration.Seconds())

	fmt.Println("\n--- Outcomes ---")
	fmt.Printf("Victories:       %d (%.1f%%)\n", r.Victories, float64(r.Victories)/float64(r.GamesPlayed)*100)
	fmt.Printf("Deaths:          %d (%.1f%%)\n", r.Deaths, float64(r.Deaths)/float64(r.GamesPlayed)*100)

	fmt.Println("\n--- Level Stats ---")
	fmt.Printf("Average Level:   %.2f\n", r.AvgLevel)
	fmt.Printf("Max Level:       %d\n", r.MaxLevel)
	fmt.Printf("Min Level:       %d\n", r.MinLevel)

	fmt.Println("\n--- Performance ---")
	fmt.Printf("Total Ticks:     %d\n", r.TotalTicks)
	fmt.Printf("Avg Ticks/Game:  %.1f\n", r.AvgTicks)
	fmt.Printf("Average HP:      %.1f\n", r.AvgHP)

	fmt.Println("\n--- Level Distribution ---")
	levels := make([]int, 0, len(r.LevelDistrib))
	for l := range r.LevelDistrib {
		levels = append(levels, l)
	}
	sort.Ints(levels)
	for _, l := range levels {
		count := r.LevelDistrib[l]
		pct := float64(count) / float64(r.GamesPlayed) * 100
		bar := ""
		for i := 0; i < int(pct/2); i++ {
			bar += "█"
		}
		fmt.Printf("  Level %2d: %4d (%5.1f%%) %s\n", l, count, pct, bar)
	}

	fmt.Println("\n--- Movement Stats ---")
	totalMoves := r.ActionStats.MoveUp + r.ActionStats.MoveDown + r.ActionStats.MoveLeft + r.ActionStats.MoveRight
	fmt.Printf("Total Moves:     %d\n", totalMoves)
	fmt.Printf("  Up:            %d (%.1f%%)\n", r.ActionStats.MoveUp, float64(r.ActionStats.MoveUp)/float64(totalMoves)*100)
	fmt.Printf("  Down:          %d (%.1f%%)\n", r.ActionStats.MoveDown, float64(r.ActionStats.MoveDown)/float64(totalMoves)*100)
	fmt.Printf("  Left:          %d (%.1f%%)\n", r.ActionStats.MoveLeft, float64(r.ActionStats.MoveLeft)/float64(totalMoves)*100)
	fmt.Printf("  Right:         %d (%.1f%%)\n", r.ActionStats.MoveRight, float64(r.ActionStats.MoveRight)/float64(totalMoves)*100)

	fmt.Println("\n--- Combat Stats ---")
	fmt.Printf("Combat Ticks:    %d\n", r.CombatStats.TotalCombats)
	fmt.Printf("Attacks:         %d\n", r.ActionStats.Attack)

	fmt.Println("\n--- NPC Interactions ---")
	fmt.Printf("Total:           %d\n", r.NPCStats.TotalInteractions)
	fmt.Printf("Talk Actions:    %d\n", r.ActionStats.Talk)
	fmt.Printf("Interact:        %d\n", r.ActionStats.Interact)

	fmt.Println("\n--- Keys & Doors ---")
	fmt.Printf("Keys Collected:  %d\n", r.KeyStats.KeysCollected)
	fmt.Printf("Doors Opened:    %d\n", r.ActionStats.OpenDoor)

	fmt.Println("\n--- Other Actions ---")
	fmt.Printf("Wait:            %d\n", r.ActionStats.Wait)
	fmt.Printf("Use Item:        %d\n", r.ActionStats.UseItem)
	fmt.Printf("Descend:         %d\n", r.ActionStats.Descend)
}

// Extend Store to expose DB
func init() {
	rand.Seed(time.Now().UnixNano())
}
