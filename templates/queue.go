package templates

import (
	"fmt"

	"github.com/pflow-xyz/go-pflow/petri"
)

// QueueTemplate implements a queueing system
type QueueTemplate struct{}

func (t *QueueTemplate) Name() string {
	return "queue"
}

func (t *QueueTemplate) Description() string {
	return "Queueing system with arrivals, waiting queue, and service"
}

func (t *QueueTemplate) Parameters() []Parameter {
	return []Parameter{
		{
			Name:        "servers",
			Description: "Number of servers",
			Type:        "int",
			Default:     1,
			Required:    false,
		},
		{
			Name:        "initial_queue",
			Description: "Initial queue length",
			Type:        "int",
			Default:     0,
			Required:    false,
		},
		{
			Name:        "queue_capacity",
			Description: "Maximum queue size (0 = unlimited)",
			Type:        "int",
			Default:     0,
			Required:    false,
		},
	}
}

func (t *QueueTemplate) Generate(params map[string]interface{}) (*petri.PetriNet, error) {
	servers := getIntParam(params, "servers", 1)
	initialQueue := getIntParam(params, "initial_queue", 0)
	queueCapacity := getIntParam(params, "queue_capacity", 0)

	net := petri.NewPetriNet()

	// Add places
	var queueCap *float64
	if queueCapacity > 0 {
		cap := float64(queueCapacity)
		queueCap = &cap
	}

	net.AddPlace("Queue", float64(initialQueue), queueCap, 100, 100, strPtr("Waiting Queue"))
	net.AddPlace("Processing", 0.0, nil, 200, 100, strPtr("Being Processed"))
	net.AddPlace("Completed", 0.0, nil, 300, 100, strPtr("Completed"))
	net.AddPlace("Servers", float64(servers), nil, 200, 50, strPtr("Available Servers"))

	// Add transitions
	net.AddTransition("arrive", "default", 50, 100, strPtr("Arrival"))
	net.AddTransition("start_service", "default", 150, 100, strPtr("Start Service"))
	net.AddTransition("complete", "default", 250, 100, strPtr("Complete"))

	// Arrivals: → Queue
	net.AddArc("arrive", "Queue", 1.0, false)

	// Start service: Queue + Server → Processing
	net.AddArc("Queue", "start_service", 1.0, false)
	net.AddArc("Servers", "start_service", 1.0, false)
	net.AddArc("start_service", "Processing", 1.0, false)

	// Complete: Processing → Completed + Server
	net.AddArc("Processing", "complete", 1.0, false)
	net.AddArc("complete", "Completed", 1.0, false)
	net.AddArc("complete", "Servers", 1.0, false)

	return net, nil
}

// ProducerConsumerTemplate implements producer-consumer pattern
type ProducerConsumerTemplate struct{}

func (t *ProducerConsumerTemplate) Name() string {
	return "producer-consumer"
}

func (t *ProducerConsumerTemplate) Description() string {
	return "Producer-consumer pattern with buffer"
}

func (t *ProducerConsumerTemplate) Parameters() []Parameter {
	return []Parameter{
		{
			Name:        "buffer_size",
			Description: "Buffer capacity",
			Type:        "int",
			Default:     10,
			Required:    false,
		},
		{
			Name:        "initial_buffer",
			Description: "Initial items in buffer",
			Type:        "int",
			Default:     0,
			Required:    false,
		},
		{
			Name:        "producers",
			Description: "Number of producers",
			Type:        "int",
			Default:     1,
			Required:    false,
		},
		{
			Name:        "consumers",
			Description: "Number of consumers",
			Type:        "int",
			Default:     1,
			Required:    false,
		},
	}
}

func (t *ProducerConsumerTemplate) Generate(params map[string]interface{}) (*petri.PetriNet, error) {
	bufferSize := getIntParam(params, "buffer_size", 10)
	initialBuffer := getIntParam(params, "initial_buffer", 0)
	producers := getIntParam(params, "producers", 1)
	consumers := getIntParam(params, "consumers", 1)

	if initialBuffer > bufferSize {
		return nil, fmt.Errorf("initial_buffer (%d) cannot exceed buffer_size (%d)", initialBuffer, bufferSize)
	}

	net := petri.NewPetriNet()

	bufCap := float64(bufferSize)

	// Add places
	net.AddPlace("Buffer", float64(initialBuffer), &bufCap, 200, 100, strPtr("Buffer"))
	net.AddPlace("ProducerReady", float64(producers), nil, 100, 50, strPtr("Producers Ready"))
	net.AddPlace("ConsumerReady", float64(consumers), nil, 300, 50, strPtr("Consumers Ready"))
	net.AddPlace("Consumed", 0.0, nil, 400, 100, strPtr("Consumed Items"))

	// Add transitions
	net.AddTransition("produce", "default", 150, 100, strPtr("Produce"))
	net.AddTransition("consume", "default", 250, 100, strPtr("Consume"))

	// Produce: ProducerReady → Buffer + ProducerReady
	net.AddArc("ProducerReady", "produce", 1.0, false)
	net.AddArc("produce", "Buffer", 1.0, false)
	net.AddArc("produce", "ProducerReady", 1.0, false)

	// Consume: Buffer + ConsumerReady → Consumed + ConsumerReady
	net.AddArc("Buffer", "consume", 1.0, false)
	net.AddArc("ConsumerReady", "consume", 1.0, false)
	net.AddArc("consume", "Consumed", 1.0, false)
	net.AddArc("consume", "ConsumerReady", 1.0, false)

	return net, nil
}

// WorkflowTemplate implements a simple sequential workflow
type WorkflowTemplate struct{}

func (t *WorkflowTemplate) Name() string {
	return "workflow"
}

func (t *WorkflowTemplate) Description() string {
	return "Sequential workflow with multiple stages"
}

func (t *WorkflowTemplate) Parameters() []Parameter {
	return []Parameter{
		{
			Name:        "stages",
			Description: "Number of workflow stages",
			Type:        "int",
			Default:     3,
			Required:    false,
		},
		{
			Name:        "initial_items",
			Description: "Initial items at start",
			Type:        "int",
			Default:     10,
			Required:    false,
		},
	}
}

func (t *WorkflowTemplate) Generate(params map[string]interface{}) (*petri.PetriNet, error) {
	stages := getIntParam(params, "stages", 3)
	initialItems := getIntParam(params, "initial_items", 10)

	if stages < 2 {
		return nil, fmt.Errorf("stages must be >= 2")
	}

	net := petri.NewPetriNet()

	// Create places for each stage
	for i := 0; i <= stages; i++ {
		var initial float64
		var label *string
		if i == 0 {
			initial = float64(initialItems)
			lbl := "Start"
			label = &lbl
		} else if i == stages {
			initial = 0.0
			lbl := "Complete"
			label = &lbl
		} else {
			initial = 0.0
			lbl := fmt.Sprintf("Stage %d", i)
			label = &lbl
		}

		placeName := fmt.Sprintf("Stage%d", i)
		x := float64(100 + i*100)
		net.AddPlace(placeName, initial, nil, x, 100, label)
	}

	// Create transitions between stages
	for i := 0; i < stages; i++ {
		transName := fmt.Sprintf("process%d", i+1)
		label := fmt.Sprintf("Process Step %d", i+1)
		x := float64(150 + i*100)

		net.AddTransition(transName, "default", x, 100, &label)

		// Connect stages
		srcPlace := fmt.Sprintf("Stage%d", i)
		dstPlace := fmt.Sprintf("Stage%d", i+1)

		net.AddArc(srcPlace, transName, 1.0, false)
		net.AddArc(transName, dstPlace, 1.0, false)
	}

	return net, nil
}
