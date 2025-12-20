// dbquery - Query catacombs session database
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/pflow-xyz/go-pflow/examples/catacombs/storage"
)

func main() {
	dbPath := flag.String("db", "catacombs.db", "SQLite database path")
	cmd := flag.String("cmd", "recent", "Command: recent, session, seed, level, export")
	sessionID := flag.String("session", "", "Session ID for session/level/export commands")
	seed := flag.Int64("seed", 0, "Seed for seed command")
	level := flag.Int("level", 0, "Level for level command")
	limit := flag.Int("limit", 10, "Limit for recent command")
	flag.Parse()

	store, err := storage.New(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	switch *cmd {
	case "recent":
		cmdRecent(store, *limit)
	case "session":
		if *sessionID == "" {
			fmt.Fprintln(os.Stderr, "Session ID required: -session <id>")
			os.Exit(1)
		}
		cmdSession(store, *sessionID)
	case "seed":
		if *seed == 0 {
			fmt.Fprintln(os.Stderr, "Seed required: -seed <seed>")
			os.Exit(1)
		}
		cmdSeed(store, *seed)
	case "level":
		if *sessionID == "" {
			fmt.Fprintln(os.Stderr, "Session ID required: -session <id>")
			os.Exit(1)
		}
		cmdLevel(store, *sessionID, *level)
	case "export":
		if *sessionID == "" {
			fmt.Fprintln(os.Stderr, "Session ID required: -session <id>")
			os.Exit(1)
		}
		cmdExport(store, *sessionID)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", *cmd)
		os.Exit(1)
	}
}

func cmdRecent(store *storage.Store, limit int) {
	sessions, err := store.RecentSessions(limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(sessions) == 0 {
		fmt.Println("No sessions found")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSeed\tMode\tLevel\tHP\tTicks\tAI\tStarted")
	for _, s := range sessions {
		ai := "-"
		if s.AIEnabled {
			ai = "AI"
		}
		status := ""
		if s.GameOver {
			status = " (dead)"
		}
		if s.Victory {
			status = " (win)"
		}
		fmt.Fprintf(w, "%s\t%d\t%s\t%d%s\t%d\t%d\t%s\t%s\n",
			s.ID[:16], s.Seed, s.Mode, s.FinalLevel, status, s.FinalHP, s.TotalTicks, ai,
			s.StartedAt.Format("2006-01-02 15:04"))
	}
	w.Flush()
}

func cmdSession(store *storage.Store, sessionID string) {
	sess, err := store.GetSession(sessionID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Session: %s\n", sess.ID)
	fmt.Printf("Seed: %d\n", sess.Seed)
	fmt.Printf("Mode: %s\n", sess.Mode)
	fmt.Printf("Started: %s\n", sess.StartedAt.Format("2006-01-02 15:04:05"))
	if sess.EndedAt != nil {
		fmt.Printf("Ended: %s\n", sess.EndedAt.Format("2006-01-02 15:04:05"))
	}
	fmt.Printf("Final Level: %d\n", sess.FinalLevel)
	fmt.Printf("Final HP: %d\n", sess.FinalHP)
	fmt.Printf("Total Ticks: %d\n", sess.TotalTicks)
	fmt.Printf("AI Enabled: %v\n", sess.AIEnabled)
	fmt.Printf("Game Over: %v\n", sess.GameOver)
	fmt.Printf("Victory: %v\n", sess.Victory)

	// Get level summaries
	summaries, err := store.GetLevelSummaries(sessionID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting summaries: %v\n", err)
		return
	}

	if len(summaries) > 0 {
		fmt.Println("\nLevel Summaries:")
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "Level\tTicks\tStart HP\tEnd HP\tMin HP\tCombats")
		for _, ls := range summaries {
			fmt.Fprintf(w, "%d\t%d\t%d\t%d\t%d\t%d\n",
				ls.Level, ls.Ticks, ls.StartHP, ls.EndHP, ls.MinHP, ls.Combats)
		}
		w.Flush()
	}
}

func cmdSeed(store *storage.Store, seed int64) {
	sessions, err := store.GetSessionsBySeed(seed)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(sessions) == 0 {
		fmt.Printf("No sessions found for seed %d\n", seed)
		return
	}

	fmt.Printf("Sessions with seed %d:\n\n", seed)
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tMode\tLevel\tHP\tTicks\tAI\tStarted")
	for _, s := range sessions {
		ai := "-"
		if s.AIEnabled {
			ai = "AI"
		}
		status := ""
		if s.GameOver {
			status = " (dead)"
		}
		if s.Victory {
			status = " (win)"
		}
		fmt.Fprintf(w, "%s\t%s\t%d%s\t%d\t%d\t%s\t%s\n",
			s.ID[:16], s.Mode, s.FinalLevel, status, s.FinalHP, s.TotalTicks, ai,
			s.StartedAt.Format("2006-01-02 15:04"))
	}
	w.Flush()
}

func cmdLevel(store *storage.Store, sessionID string, level int) {
	var actions []*storage.Action
	var err error

	if level > 0 {
		actions, err = store.GetActionsForLevel(sessionID, level)
	} else {
		actions, err = store.GetActions(sessionID)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(actions) == 0 {
		fmt.Println("No actions found")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Tick\tLevel\tAction\tPos\tHP\tMode\tTarget\tEnemies\tCombat")
	for _, a := range actions {
		combat := "-"
		if a.InCombat {
			combat = "COMBAT"
		}
		fmt.Fprintf(w, "%d\t%d\t%s\t(%d,%d)\t%d/%d\t%s\t%s\t%d\t%s\n",
			a.Tick, a.Level, a.Action, a.PlayerX, a.PlayerY,
			a.PlayerHP, a.PlayerMaxHP, a.AIMode, a.AITarget,
			a.EnemiesAlive, combat)
	}
	w.Flush()

	fmt.Printf("\nTotal: %d actions\n", len(actions))
}

func cmdExport(store *storage.Store, sessionID string) {
	data, err := store.ExportSessionJSON(sessionID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Pretty print
	var v any
	json.Unmarshal(data, &v)
	output, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(output))
}
