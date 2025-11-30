package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/pflow-xyz/go-pflow/examples/doom/server"
	"github.com/pflow-xyz/go-pflow/examples/doom"
	"github.com/pflow-xyz/go-pflow/visualization"
)

//go:embed client/*
var clientFiles embed.FS

func main() {
	port := flag.Int("port", 8081, "Server port")
	saveSVG := flag.Bool("svg", false, "Save Petri net visualization")
	flag.Parse()

	if *saveSVG {
		saveModelVisualization()
		return
	}

	runServer(*port)
}

func runServer(port int) {
	srv := server.NewServer()

	// Serve embedded static files
	clientFS, err := fs.Sub(clientFiles, "client")
	if err != nil {
		log.Fatalf("Failed to get client files: %v", err)
	}
	http.Handle("/", http.FileServer(http.FS(clientFS)))
	http.Handle("/ws", srv)
	http.Handle("/health", srv)

	addr := fmt.Sprintf(":%d", port)
	log.Printf("DOOM Game Server starting on http://localhost%s", addr)
	log.Printf("WebSocket endpoint: ws://localhost%s/ws", addr)

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Println("Shutting down server...")
		os.Exit(0)
	}()

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func saveModelVisualization() {
	game := doom.NewGame()
	net := game.GetNet()

	if err := visualization.SaveSVG(net, "doom_model.svg"); err != nil {
		log.Fatalf("Error saving SVG: %v", err)
	}
	fmt.Println("Saved Petri net visualization to doom_model.svg")
}
