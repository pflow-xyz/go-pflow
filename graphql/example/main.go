// Example GraphQL server for Petri net models.
//
// Run with: go run ./graphql/example
// Then open http://localhost:8080/graphql/i in your browser.
package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/pflow-xyz/go-pflow/eventsource"
	"github.com/pflow-xyz/go-pflow/graphql"
	"github.com/pflow-xyz/go-pflow/petri"
)

func main() {
	// Create a simple approval workflow model
	model := petri.NewPetriNet()
	model.AddPlace("pending", 1, 0, 0, 0, nil)
	model.AddPlace("approved", 0, 0, 100, 0, nil)
	model.AddPlace("rejected", 0, 0, 100, 100, nil)
	model.AddTransition("approve", "", 50, 0, nil)
	model.AddTransition("reject", "", 50, 100, nil)
	model.AddArc("pending", "approve", 1, false)
	model.AddArc("approve", "approved", 1, false)
	model.AddArc("pending", "reject", 1, false)
	model.AddArc("reject", "rejected", 1, false)

	// Create an event-sourced store
	eventStore := eventsource.NewMemoryStore()
	defer eventStore.Close()
	store := graphql.NewEventSourceStore(eventStore, model, "approval")

	// Create the GraphQL server
	server := graphql.NewServer(
		graphql.WithModel("approval", model, store),
		graphql.WithPlayground("/graphql/i"),
	)

	// Print the generated schema
	fmt.Println("Generated GraphQL Schema:")
	fmt.Println("========================")
	fmt.Println(server.Schema())

	// Start the server
	addr := ":8080"
	log.Printf("GraphQL server running at http://localhost%s/graphql", addr)
	log.Printf("Playground available at http://localhost%s/graphql/i", addr)

	if err := http.ListenAndServe(addr, server.Mux()); err != nil {
		log.Fatal(err)
	}
}
