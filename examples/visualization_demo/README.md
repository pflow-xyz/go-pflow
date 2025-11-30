# Visualization Demo

Generates example SVG visualizations for Petri nets, workflows, and state machines.

## What It Does

Creates SVG files demonstrating go-pflow's visualization capabilities:
- **Petri Nets** - SIR epidemic and producer-consumer models
- **Workflows** - Approval, parallel, incident, and order processing
- **State Machines** - Traffic light, order status, media player, network connection, document lifecycle

## Running

```bash
cd examples/visualization_demo
go run main.go
```

## Generated Visualizations

### Petri Net Examples

#### SIR Epidemic Model

![SIR Petri Net](petri_sir.svg)

#### Producer-Consumer

![Producer Consumer](petri_producer_consumer.svg)

### Workflow Examples

#### Document Approval

![Approval Workflow](workflow_approval.svg)

#### Parallel Processing

![Parallel Workflow](workflow_parallel.svg)

#### Incident Management

![Incident Workflow](workflow_incident.svg)

#### Order Fulfillment

![Order Workflow](workflow_order.svg)

### State Machine Examples

#### Traffic Light

![Traffic Light](statemachine_traffic_light.svg)

#### Order Status

![Order Status](statemachine_order.svg)

#### Media Player (Parallel Regions)

![Media Player](statemachine_media_player.svg)

#### Network Connection

![Connection](statemachine_connection.svg)

#### Document Lifecycle

![Document Lifecycle](statemachine_document.svg)

## Visualization Types

### Petri Nets
- **Places** (circles): Hold tokens
- **Transitions** (rectangles): Transform state
- **Arcs**: Directed edges with weights
- **Token display**: Shows initial marking

### Workflows
- **Task nodes**: Manual, automatic, decision, subflow types
- **Flow edges**: Show task dependencies
- **Start/End markers**: Entry and exit points
- **Split/Join patterns**: AND/XOR parallel execution

### State Machines
- **States**: Rounded rectangles
- **Transitions**: Labeled arrows with events
- **Regions**: Parallel orthogonal areas
- **Self-loops**: States with transitions back to themselves

## Code Examples

### Petri Net Visualization
```go
net := petri.NewPetriNet()
// ... build net ...
visualization.SaveSVG(net, "my_petri_net.svg")
```

### Workflow Visualization
```go
wf := workflow.New("my-flow").
    Task("start").Automatic().Done().
    Task("end").Automatic().Done().
    Connect("start", "end").
    Build()
visualization.SaveWorkflowSVG(wf, "my_workflow.svg", nil)
```

### State Machine Visualization
```go
chart := &statemachine.Chart{
    Name: "MyMachine",
    Regions: map[string]*statemachine.Region{...},
    Transitions: []*statemachine.Transition{...},
}
visualization.SaveStateMachineSVG(chart, "my_statemachine.svg", nil)
```

## Packages Used

- `petri` - Petri net construction
- `workflow` - Workflow definition
- `statemachine` - State machine charts
- `visualization` - SVG rendering for all model types
