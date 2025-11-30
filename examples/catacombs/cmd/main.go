// Catacombs of Pflow - A roguelike dungeon crawler
package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"

	"github.com/pflow-xyz/go-pflow/examples/catacombs/server"
)

//go:embed client/*
var clientFiles embed.FS

func main() {
	port := flag.Int("port", 8082, "Server port")
	flag.Parse()

	// Create game server
	srv := server.NewServer()

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
	log.Printf("Catacombs of Pflow server starting on http://localhost%s", addr)
	log.Printf("Open http://localhost%s in your browser to play", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
