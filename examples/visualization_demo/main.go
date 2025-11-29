// Visualization demo - generates example SVG files for workflows and state machines
package main

import (
	"fmt"
	"time"

	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/statemachine"
	"github.com/pflow-xyz/go-pflow/visualization"
	"github.com/pflow-xyz/go-pflow/workflow"
)

func main() {
	fmt.Println("Generating visualization examples...")
	fmt.Println()

	// Generate Petri net examples
	generatePetriNetExamples()

	// Generate workflow examples
	generateWorkflowExamples()

	// Generate state machine examples
	generateStateMachineExamples()

	fmt.Println()
	fmt.Println("✓ All visualization examples generated!")
}

func generatePetriNetExamples() {
	fmt.Println("=== Petri Net Examples ===")

	// Simple SIR model
	sir := petri.NewPetriNet()
	sir.AddPlace("S", 999, nil, 100, 100, nil)
	sir.AddPlace("I", 1, nil, 200, 100, nil)
	sir.AddPlace("R", 0, nil, 300, 100, nil)
	sir.AddTransition("infect", "default", 150, 150, nil)
	sir.AddTransition("recover", "default", 250, 150, nil)
	sir.AddArc("S", "infect", 1, false)
	sir.AddArc("I", "infect", 1, false)
	sir.AddArc("infect", "I", 2, false)
	sir.AddArc("I", "recover", 1, false)
	sir.AddArc("recover", "R", 1, false)

	if err := visualization.SaveSVG(sir, "petri_sir.svg"); err != nil {
		fmt.Printf("  Error saving SIR model: %v\n", err)
	} else {
		fmt.Println("  ✓ petri_sir.svg")
	}

	// Producer-Consumer model
	pc := petri.NewPetriNet()
	pc.AddPlace("buffer", 0, nil, 200, 100, nil)
	pc.AddPlace("producer_ready", 1, nil, 100, 50, nil)
	pc.AddPlace("consumer_ready", 1, nil, 300, 50, nil)
	pc.AddTransition("produce", "default", 150, 100, nil)
	pc.AddTransition("consume", "default", 250, 100, nil)
	pc.AddArc("producer_ready", "produce", 1, false)
	pc.AddArc("produce", "buffer", 1, false)
	pc.AddArc("produce", "producer_ready", 1, false)
	pc.AddArc("buffer", "consume", 1, false)
	pc.AddArc("consumer_ready", "consume", 1, false)
	pc.AddArc("consume", "consumer_ready", 1, false)

	if err := visualization.SaveSVG(pc, "petri_producer_consumer.svg"); err != nil {
		fmt.Printf("  Error saving producer-consumer model: %v\n", err)
	} else {
		fmt.Println("  ✓ petri_producer_consumer.svg")
	}

	fmt.Println()
}

func generateWorkflowExamples() {
	fmt.Println("=== Workflow Examples ===")

	// Simple approval workflow
	approvalWF := workflow.New("approval").
		Name("Document Approval Workflow").
		Task("submit").
			Name("Submit Document").
			Manual().
			Duration(5 * time.Minute).
			Done().
		Task("review").
			Name("Review Document").
			Manual().
			Duration(30 * time.Minute).
			Done().
		Task("decide").
			Name("Approve?").
			Decision().
			Done().
		Task("approve").
			Name("Mark Approved").
			Automatic().
			Duration(1 * time.Minute).
			Done().
		Task("reject").
			Name("Mark Rejected").
			Automatic().
			Duration(1 * time.Minute).
			Done().
		Task("notify").
			Name("Send Notification").
			Automatic().
			Duration(1 * time.Minute).
			Done().
		Connect("submit", "review").
		Connect("review", "decide").
		Connect("decide", "approve").
		Connect("decide", "reject").
		Connect("approve", "notify").
		Connect("reject", "notify").
		Start("submit").
		End("notify").
		Build()

	if err := visualization.SaveWorkflowSVG(approvalWF, "workflow_approval.svg", nil); err != nil {
		fmt.Printf("  Error saving approval workflow: %v\n", err)
	} else {
		fmt.Println("  ✓ workflow_approval.svg")
	}

	// Parallel processing workflow
	parallelWF := workflow.New("parallel").
		Name("Parallel Processing Workflow").
		Task("start").
			Name("Start").
			Automatic().
			SplitAll().
			Done().
		Task("taskA").
			Name("Process A").
			Automatic().
			Duration(10 * time.Minute).
			Done().
		Task("taskB").
			Name("Process B").
			Automatic().
			Duration(15 * time.Minute).
			Done().
		Task("taskC").
			Name("Process C").
			Automatic().
			Duration(8 * time.Minute).
			Done().
		Task("sync").
			Name("Synchronize").
			Automatic().
			JoinAll().
			Done().
		Task("finish").
			Name("Finish").
			Automatic().
			Done().
		Connect("start", "taskA").
		Connect("start", "taskB").
		Connect("start", "taskC").
		Connect("taskA", "sync").
		Connect("taskB", "sync").
		Connect("taskC", "sync").
		Connect("sync", "finish").
		Start("start").
		End("finish").
		Build()

	if err := visualization.SaveWorkflowSVG(parallelWF, "workflow_parallel.svg", nil); err != nil {
		fmt.Printf("  Error saving parallel workflow: %v\n", err)
	} else {
		fmt.Println("  ✓ workflow_parallel.svg")
	}

	// Incident management workflow
	incidentWF := workflow.New("incident").
		Name("Incident Management").
		Task("create").
			Name("Create Ticket").
			Automatic().
			Done().
		Task("triage").
			Name("Triage").
			Manual().
			Duration(15 * time.Minute).
			Done().
		Task("assign").
			Name("Assign Engineer").
			Manual().
			Duration(5 * time.Minute).
			Done().
		Task("investigate").
			Name("Investigate").
			Manual().
			Duration(2 * time.Hour).
			Done().
		Task("resolve").
			Name("Implement Fix").
			Manual().
			Duration(4 * time.Hour).
			Done().
		Task("verify").
			Name("Verify Fix").
			Manual().
			Duration(30 * time.Minute).
			Done().
		Task("close").
			Name("Close Ticket").
			Automatic().
			Done().
		Connect("create", "triage").
		Connect("triage", "assign").
		Connect("assign", "investigate").
		Connect("investigate", "resolve").
		Connect("resolve", "verify").
		Connect("verify", "close").
		Start("create").
		End("close").
		Build()

	if err := visualization.SaveWorkflowSVG(incidentWF, "workflow_incident.svg", nil); err != nil {
		fmt.Printf("  Error saving incident workflow: %v\n", err)
	} else {
		fmt.Println("  ✓ workflow_incident.svg")
	}

	// Order fulfillment with subflow
	orderWF := workflow.New("order").
		Name("Order Fulfillment").
		Task("receive").
			Name("Receive Order").
			Automatic().
			Done().
		Task("validate").
			Name("Validate Order").
			Automatic().
			Done().
		Task("payment").
			Name("Process Payment").
			Type(workflow.TaskTypeSubflow).
			Done().
		Task("inventory").
			Name("Check Inventory").
			Automatic().
			Done().
		Task("ship").
			Name("Ship Order").
			Manual().
			Done().
		Task("complete").
			Name("Order Complete").
			Automatic().
			Done().
		Connect("receive", "validate").
		Connect("validate", "payment").
		Connect("payment", "inventory").
		Connect("inventory", "ship").
		Connect("ship", "complete").
		Start("receive").
		End("complete").
		Build()

	if err := visualization.SaveWorkflowSVG(orderWF, "workflow_order.svg", nil); err != nil {
		fmt.Printf("  Error saving order workflow: %v\n", err)
	} else {
		fmt.Println("  ✓ workflow_order.svg")
	}

	fmt.Println()
}

func generateStateMachineExamples() {
	fmt.Println("=== State Machine Examples ===")

	// Traffic light state machine
	trafficLight := &statemachine.Chart{
		Name: "Traffic Light",
		Regions: map[string]*statemachine.Region{
			"light": {
				Name:    "light",
				Initial: "red",
				States: map[string]*statemachine.State{
					"red":    {Name: "red", IsLeaf: true},
					"yellow": {Name: "yellow", IsLeaf: true},
					"green":  {Name: "green", IsLeaf: true},
				},
			},
		},
		Transitions: []*statemachine.Transition{
			{Event: "timer", Source: "red", Target: "green"},
			{Event: "timer", Source: "green", Target: "yellow"},
			{Event: "timer", Source: "yellow", Target: "red"},
		},
	}

	if err := visualization.SaveStateMachineSVG(trafficLight, "statemachine_traffic_light.svg", nil); err != nil {
		fmt.Printf("  Error saving traffic light: %v\n", err)
	} else {
		fmt.Println("  ✓ statemachine_traffic_light.svg")
	}

	// Order state machine
	orderSM := &statemachine.Chart{
		Name: "Order Status",
		Regions: map[string]*statemachine.Region{
			"order": {
				Name:    "order",
				Initial: "pending",
				States: map[string]*statemachine.State{
					"pending":    {Name: "pending", IsLeaf: true},
					"confirmed":  {Name: "confirmed", IsLeaf: true},
					"processing": {Name: "processing", IsLeaf: true},
					"shipped":    {Name: "shipped", IsLeaf: true},
					"delivered":  {Name: "delivered", IsLeaf: true},
					"cancelled":  {Name: "cancelled", IsLeaf: true},
				},
			},
		},
		Transitions: []*statemachine.Transition{
			{Event: "confirm", Source: "pending", Target: "confirmed"},
			{Event: "process", Source: "confirmed", Target: "processing"},
			{Event: "ship", Source: "processing", Target: "shipped"},
			{Event: "deliver", Source: "shipped", Target: "delivered"},
			{Event: "cancel", Source: "pending", Target: "cancelled"},
			{Event: "cancel", Source: "confirmed", Target: "cancelled"},
		},
	}

	if err := visualization.SaveStateMachineSVG(orderSM, "statemachine_order.svg", nil); err != nil {
		fmt.Printf("  Error saving order state machine: %v\n", err)
	} else {
		fmt.Println("  ✓ statemachine_order.svg")
	}

	// Media player with parallel regions
	mediaPlayer := &statemachine.Chart{
		Name: "Media Player",
		Regions: map[string]*statemachine.Region{
			"playback": {
				Name:    "playback",
				Initial: "stopped",
				States: map[string]*statemachine.State{
					"stopped": {Name: "stopped", IsLeaf: true},
					"playing": {Name: "playing", IsLeaf: true},
					"paused":  {Name: "paused", IsLeaf: true},
				},
			},
			"volume": {
				Name:    "volume",
				Initial: "normal",
				States: map[string]*statemachine.State{
					"muted":  {Name: "muted", IsLeaf: true},
					"normal": {Name: "normal", IsLeaf: true},
				},
			},
		},
		Transitions: []*statemachine.Transition{
			// Playback transitions
			{Event: "play", Source: "stopped", Target: "playing"},
			{Event: "pause", Source: "playing", Target: "paused"},
			{Event: "resume", Source: "paused", Target: "playing"},
			{Event: "stop", Source: "playing", Target: "stopped"},
			{Event: "stop", Source: "paused", Target: "stopped"},
			// Volume transitions
			{Event: "mute", Source: "normal", Target: "muted"},
			{Event: "unmute", Source: "muted", Target: "normal"},
		},
	}

	if err := visualization.SaveStateMachineSVG(mediaPlayer, "statemachine_media_player.svg", nil); err != nil {
		fmt.Printf("  Error saving media player: %v\n", err)
	} else {
		fmt.Println("  ✓ statemachine_media_player.svg")
	}

	// Connection state machine with self-loops
	connectionSM := &statemachine.Chart{
		Name: "Network Connection",
		Regions: map[string]*statemachine.Region{
			"connection": {
				Name:    "connection",
				Initial: "disconnected",
				States: map[string]*statemachine.State{
					"disconnected": {Name: "disconnected", IsLeaf: true},
					"connecting":   {Name: "connecting", IsLeaf: true},
					"connected":    {Name: "connected", IsLeaf: true},
					"reconnecting": {Name: "reconnecting", IsLeaf: true},
				},
			},
		},
		Transitions: []*statemachine.Transition{
			{Event: "connect", Source: "disconnected", Target: "connecting"},
			{Event: "success", Source: "connecting", Target: "connected"},
			{Event: "failure", Source: "connecting", Target: "disconnected"},
			{Event: "disconnect", Source: "connected", Target: "disconnected"},
			{Event: "error", Source: "connected", Target: "reconnecting"},
			{Event: "retry", Source: "reconnecting", Target: "connecting"},
			{Event: "ping", Source: "connected", Target: "connected"}, // self-loop
		},
	}

	if err := visualization.SaveStateMachineSVG(connectionSM, "statemachine_connection.svg", nil); err != nil {
		fmt.Printf("  Error saving connection state machine: %v\n", err)
	} else {
		fmt.Println("  ✓ statemachine_connection.svg")
	}

	// Document lifecycle with hierarchical states
	docLifecycle := &statemachine.Chart{
		Name: "Document Lifecycle",
		Regions: map[string]*statemachine.Region{
			"document": {
				Name:    "document",
				Initial: "draft",
				States: map[string]*statemachine.State{
					"draft":     {Name: "draft", IsLeaf: true},
					"review":    {Name: "review", IsLeaf: true},
					"approved":  {Name: "approved", IsLeaf: true},
					"published": {Name: "published", IsLeaf: true},
					"archived":  {Name: "archived", IsLeaf: true},
				},
			},
		},
		Transitions: []*statemachine.Transition{
			{Event: "submit", Source: "draft", Target: "review"},
			{Event: "reject", Source: "review", Target: "draft"},
			{Event: "approve", Source: "review", Target: "approved"},
			{Event: "publish", Source: "approved", Target: "published"},
			{Event: "archive", Source: "published", Target: "archived"},
			{Event: "edit", Source: "draft", Target: "draft"}, // self-loop for editing
		},
	}

	if err := visualization.SaveStateMachineSVG(docLifecycle, "statemachine_document.svg", nil); err != nil {
		fmt.Printf("  Error saving document lifecycle: %v\n", err)
	} else {
		fmt.Println("  ✓ statemachine_document.svg")
	}

	fmt.Println()
}
