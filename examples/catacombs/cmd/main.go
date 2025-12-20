// Catacombs - A roguelike dungeon crawler
package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"

	"github.com/pflow-xyz/go-pflow/examples/catacombs/server"
	"github.com/pflow-xyz/go-pflow/examples/catacombs/storage"
)

//go:embed client/*
var clientFiles embed.FS

func main() {
	port := flag.Int("port", 8082, "Server port")
	debug := flag.Bool("debug", false, "Enable AI debug logging")
	dbPath := flag.String("db", "", "SQLite database path for session logging (default: catacombs.db in current dir)")
	noDb := flag.Bool("no-db", false, "Disable SQLite session logging")
	flag.Parse()

	// Create game server
	srv := server.NewServer()
	srv.SetDebug(*debug)

	// Initialize SQLite storage unless disabled
	if !*noDb {
		dbFile := *dbPath
		if dbFile == "" {
			dbFile = "catacombs.db"
		}
		// Make path absolute if not already
		if !filepath.IsAbs(dbFile) {
			absPath, err := filepath.Abs(dbFile)
			if err == nil {
				dbFile = absPath
			}
		}

		store, err := storage.New(dbFile)
		if err != nil {
			log.Printf("Warning: Failed to initialize SQLite storage: %v", err)
			log.Printf("Session logging disabled. Use -no-db to suppress this warning.")
		} else {
			srv.SetStore(store)
			log.Printf("Session logging to: %s", dbFile)
			defer store.Close()
		}
	}

	// Serve static files
	clientFS, err := fs.Sub(clientFiles, "client")
	if err != nil {
		log.Fatal(err)
	}
	http.Handle("/", http.FileServer(http.FS(clientFS)))

	// WebSocket and health endpoints
	http.Handle("/ws", srv)
	http.Handle("/health", srv)

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Catacombs server starting on http://localhost%s", addr)
	log.Printf("Open http://localhost%s in your browser to play", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
