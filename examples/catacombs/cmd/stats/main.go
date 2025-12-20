// stats - Analyze database stats
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"sort"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	dbPath := flag.String("db", "catacombs.db", "SQLite database path")
	flag.Parse()

	db, err := sql.Open("sqlite3", *dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	fmt.Println("==================================================")
	fmt.Println("CATACOMBS GAME STATISTICS")
	fmt.Println("==================================================")

	// Total sessions
	var totalSessions int
	db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&totalSessions)
	fmt.Printf("\nTotal Games: %d\n", totalSessions)

	// Victories and deaths
	var victories, deaths int
	db.QueryRow("SELECT COUNT(*) FROM sessions WHERE victory = 1").Scan(&victories)
	db.QueryRow("SELECT COUNT(*) FROM sessions WHERE game_over = 1").Scan(&deaths)
	fmt.Printf("Victories:   %d (%.1f%%)\n", victories, float64(victories)/float64(totalSessions)*100)
	fmt.Printf("Deaths:      %d (%.1f%%)\n", deaths, float64(deaths)/float64(totalSessions)*100)

	// Average stats
	var avgLevel, avgHP, avgTicks float64
	db.QueryRow("SELECT AVG(final_level), AVG(final_hp), AVG(total_ticks) FROM sessions").Scan(&avgLevel, &avgHP, &avgTicks)
	fmt.Printf("\nAverage Level:  %.2f\n", avgLevel)
	fmt.Printf("Average HP:     %.1f\n", avgHP)
	fmt.Printf("Average Ticks:  %.0f\n", avgTicks)

	// Level distribution
	fmt.Println("\n--- Level Distribution ---")
	rows, _ := db.Query(`
		SELECT final_level, COUNT(*) as count,
		       SUM(CASE WHEN victory=1 THEN 1 ELSE 0 END) as wins,
		       SUM(CASE WHEN game_over=1 THEN 1 ELSE 0 END) as deaths
		FROM sessions
		GROUP BY final_level
		ORDER BY final_level
	`)
	defer rows.Close()

	for rows.Next() {
		var level, count, wins, deaths int
		rows.Scan(&level, &count, &wins, &deaths)
		pct := float64(count) / float64(totalSessions) * 100
		bar := ""
		for i := 0; i < int(pct/2); i++ {
			bar += "#"
		}
		status := ""
		if wins > 0 {
			status = fmt.Sprintf(" (W:%d)", wins)
		}
		if deaths > 0 {
			status += fmt.Sprintf(" (D:%d)", deaths)
		}
		fmt.Printf("Level %2d: %4d (%5.1f%%) %s%s\n", level, count, pct, bar, status)
	}

	// Total actions
	var totalActions int
	db.QueryRow("SELECT COUNT(*) FROM actions").Scan(&totalActions)
	fmt.Printf("\nTotal Actions Logged: %d\n", totalActions)

	// Action type distribution
	fmt.Println("\n--- Action Types ---")
	actionRows, _ := db.Query(`
		SELECT action, COUNT(*) as count
		FROM actions
		GROUP BY action
		ORDER BY count DESC
		LIMIT 30
	`)
	defer actionRows.Close()

	type ActionCount struct {
		Action string
		Count  int
	}
	var actions []ActionCount
	for actionRows.Next() {
		var ac ActionCount
		actionRows.Scan(&ac.Action, &ac.Count)
		actions = append(actions, ac)
	}

	for _, ac := range actions {
		pct := float64(ac.Count) / float64(totalActions) * 100
		fmt.Printf("  %-25s %8d (%5.2f%%)\n", ac.Action, ac.Count, pct)
	}

	// Movement stats
	fmt.Println("\n--- Movement Analysis ---")
	var moveUp, moveDown, moveLeft, moveRight int
	db.QueryRow("SELECT COUNT(*) FROM actions WHERE action LIKE '%move_up%'").Scan(&moveUp)
	db.QueryRow("SELECT COUNT(*) FROM actions WHERE action LIKE '%move_down%'").Scan(&moveDown)
	db.QueryRow("SELECT COUNT(*) FROM actions WHERE action LIKE '%move_left%'").Scan(&moveLeft)
	db.QueryRow("SELECT COUNT(*) FROM actions WHERE action LIKE '%move_right%'").Scan(&moveRight)
	totalMoves := moveUp + moveDown + moveLeft + moveRight
	fmt.Printf("Total Moves: %d\n", totalMoves)
	if totalMoves > 0 {
		fmt.Printf("  Up:    %d (%.1f%%)\n", moveUp, float64(moveUp)/float64(totalMoves)*100)
		fmt.Printf("  Down:  %d (%.1f%%)\n", moveDown, float64(moveDown)/float64(totalMoves)*100)
		fmt.Printf("  Left:  %d (%.1f%%)\n", moveLeft, float64(moveLeft)/float64(totalMoves)*100)
		fmt.Printf("  Right: %d (%.1f%%)\n", moveRight, float64(moveRight)/float64(totalMoves)*100)
	}

	// Combat stats
	fmt.Println("\n--- Combat Stats ---")
	var combatTicks int
	db.QueryRow("SELECT COUNT(*) FROM actions WHERE in_combat = 1").Scan(&combatTicks)
	fmt.Printf("Combat Ticks: %d (%.1f%% of all actions)\n", combatTicks, float64(combatTicks)/float64(totalActions)*100)

	var attacks int
	db.QueryRow("SELECT COUNT(*) FROM actions WHERE action LIKE '%attack%'").Scan(&attacks)
	fmt.Printf("Attack Actions: %d\n", attacks)

	// NPC interactions
	fmt.Println("\n--- NPC Interactions ---")
	var talks, interacts int
	db.QueryRow("SELECT COUNT(*) FROM actions WHERE action LIKE '%talk%'").Scan(&talks)
	db.QueryRow("SELECT COUNT(*) FROM actions WHERE action LIKE '%interact%'").Scan(&interacts)
	fmt.Printf("Talk Actions:     %d\n", talks)
	fmt.Printf("Interact Actions: %d\n", interacts)

	// Door stats
	fmt.Println("\n--- Doors & Keys ---")
	var doorActions int
	db.QueryRow("SELECT COUNT(*) FROM actions WHERE action LIKE '%door%'").Scan(&doorActions)
	fmt.Printf("Door Actions: %d\n", doorActions)

	// AI mode distribution
	fmt.Println("\n--- AI Modes ---")
	modeRows, _ := db.Query(`
		SELECT ai_mode, COUNT(*) as count
		FROM actions
		WHERE ai_mode IS NOT NULL AND ai_mode != ''
		GROUP BY ai_mode
		ORDER BY count DESC
	`)
	defer modeRows.Close()

	for modeRows.Next() {
		var mode string
		var count int
		modeRows.Scan(&mode, &count)
		pct := float64(count) / float64(totalActions) * 100
		fmt.Printf("  %-15s %8d (%5.2f%%)\n", mode, count, pct)
	}

	// Level summaries
	fmt.Println("\n--- Per-Level Performance ---")
	levelRows, _ := db.Query(`
		SELECT level,
		       COUNT(DISTINCT session_id) as sessions,
		       AVG(player_hp) as avg_hp,
		       MIN(player_hp) as min_hp,
		       SUM(CASE WHEN in_combat=1 THEN 1 ELSE 0 END) as combat_ticks
		FROM actions
		GROUP BY level
		ORDER BY level
	`)
	defer levelRows.Close()

	fmt.Printf("%-7s %8s %10s %10s %12s\n", "Level", "Sessions", "Avg HP", "Min HP", "Combat Ticks")
	for levelRows.Next() {
		var level, sessions, combatTicks int
		var avgHP float64
		var minHP int
		levelRows.Scan(&level, &sessions, &avgHP, &minHP, &combatTicks)
		fmt.Printf("%-7d %8d %10.1f %10d %12d\n", level, sessions, avgHP, minHP, combatTicks)
	}

	// Item usage
	fmt.Println("\n--- Item Usage ---")
	var itemUse int
	db.QueryRow("SELECT COUNT(*) FROM actions WHERE action LIKE '%use_item%'").Scan(&itemUse)
	fmt.Printf("Use Item Actions: %d\n", itemUse)

	// Descend stats
	fmt.Println("\n--- Level Transitions ---")
	var descends int
	db.QueryRow("SELECT COUNT(*) FROM actions WHERE action LIKE '%descend%'").Scan(&descends)
	fmt.Printf("Descend Actions: %d\n", descends)

	// Wait stats
	var waits int
	db.QueryRow("SELECT COUNT(*) FROM actions WHERE action LIKE '%wait%'").Scan(&waits)
	fmt.Printf("Wait Actions:    %d\n", waits)

	// Interesting patterns - high tick games
	fmt.Println("\n--- Longest Games (by ticks) ---")
	longRows, _ := db.Query(`
		SELECT id, seed, final_level, final_hp, total_ticks,
		       CASE WHEN victory=1 THEN 'WIN' ELSE CASE WHEN game_over=1 THEN 'DEAD' ELSE 'IN PROGRESS' END END as outcome
		FROM sessions
		ORDER BY total_ticks DESC
		LIMIT 10
	`)
	defer longRows.Close()

	fmt.Printf("%-18s %-12s %6s %6s %8s %s\n", "Session", "Seed", "Level", "HP", "Ticks", "Outcome")
	for longRows.Next() {
		var id string
		var seed int64
		var level, hp, ticks int
		var outcome string
		longRows.Scan(&id, &seed, &level, &hp, &ticks, &outcome)
		fmt.Printf("%-18s %-12d %6d %6d %8d %s\n", id[:16]+"...", seed, level, hp, ticks, outcome)
	}

	// Quickest victories
	fmt.Println("\n--- Quickest Victories ---")
	quickRows, _ := db.Query(`
		SELECT id, seed, final_level, final_hp, total_ticks
		FROM sessions
		WHERE victory = 1
		ORDER BY total_ticks ASC
		LIMIT 10
	`)
	defer quickRows.Close()

	fmt.Printf("%-18s %-12s %6s %6s %8s\n", "Session", "Seed", "Level", "HP", "Ticks")
	for quickRows.Next() {
		var id string
		var seed int64
		var level, hp, ticks int
		quickRows.Scan(&id, &seed, &level, &hp, &ticks)
		fmt.Printf("%-18s %-12d %6d %6d %8d\n", id[:16]+"...", seed, level, hp, ticks)
	}

	// Calculate key stats
	type KeyStat struct {
		action string
		count  int
	}
	keyStats := []KeyStat{}
	keyRows, _ := db.Query(`
		SELECT action, COUNT(*) as c FROM actions
		WHERE action LIKE '%key%' OR extra_data LIKE '%key%'
		GROUP BY action
	`)
	defer keyRows.Close()
	for keyRows.Next() {
		var ks KeyStat
		keyRows.Scan(&ks.action, &ks.count)
		keyStats = append(keyStats, ks)
	}
	if len(keyStats) > 0 {
		fmt.Println("\n--- Key-Related Actions ---")
		sort.Slice(keyStats, func(i, j int) bool { return keyStats[i].count > keyStats[j].count })
		for _, ks := range keyStats {
			fmt.Printf("  %-30s %d\n", ks.action, ks.count)
		}
	}

	fmt.Println("\n==================================================")
	fmt.Println("END OF STATISTICS")
	fmt.Println("==================================================")
}
