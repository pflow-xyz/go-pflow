// keystats - Check key/door usage stats
package main

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	db, _ := sql.Open("sqlite3", "catacombs.db")
	defer db.Close()

	fmt.Println("=== Key/Door Related Actions ===")
	rows, _ := db.Query(`
		SELECT action, COUNT(*)
		FROM actions
		WHERE action LIKE '%key%' OR action LIKE '%door%' OR action LIKE '%open%' OR action LIKE '%unlock%'
		GROUP BY action
	`)
	defer rows.Close()
	found := false
	for rows.Next() {
		var action string
		var count int
		rows.Scan(&action, &count)
		fmt.Printf("  %s: %d\n", action, count)
		found = true
	}
	if !found {
		fmt.Println("  No key/door actions found in logs")
	}

	fmt.Println("\n=== AI Modes with 'find_key' ===")
	rows2, _ := db.Query(`SELECT ai_mode, COUNT(*) FROM actions WHERE ai_mode LIKE '%key%' GROUP BY ai_mode`)
	defer rows2.Close()
	found2 := false
	for rows2.Next() {
		var mode string
		var count int
		rows2.Scan(&mode, &count)
		fmt.Printf("  %s: %d\n", mode, count)
		found2 = true
	}
	if !found2 {
		fmt.Println("  No find_key mode actions in logs")
	}

	fmt.Println("\n=== Extra Data with Keys ===")
	var keyData int
	db.QueryRow(`SELECT COUNT(*) FROM actions WHERE extra_data LIKE '%key%'`).Scan(&keyData)
	fmt.Printf("  Actions with key in extra_data: %d\n", keyData)

	fmt.Println("\n=== All Distinct AI Modes ===")
	rows3, _ := db.Query(`SELECT DISTINCT ai_mode FROM actions WHERE ai_mode IS NOT NULL AND ai_mode != ''`)
	defer rows3.Close()
	for rows3.Next() {
		var mode string
		rows3.Scan(&mode)
		fmt.Printf("  %s\n", mode)
	}

	fmt.Println("\n=== Sample Actions Around Level Transitions ===")
	rows4, _ := db.Query(`
		SELECT tick, level, action, ai_mode, player_hp
		FROM actions
		WHERE action = 'descend'
		LIMIT 10
	`)
	defer rows4.Close()
	for rows4.Next() {
		var tick, level, hp int
		var action, mode string
		rows4.Scan(&tick, &level, &action, &mode, &hp)
		fmt.Printf("  Tick %d: Level %d -> %s (mode: %s, HP: %d)\n", tick, level, action, mode, hp)
	}
}
