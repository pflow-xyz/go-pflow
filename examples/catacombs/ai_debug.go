package catacombs

import (
	"fmt"
	"strings"
)

// AIDebugSnapshot captures AI state at a point in time for debugging
type AIDebugSnapshot struct {
	Tick         int    // Which tick this snapshot was taken
	Phase        string // "before_decide", "after_decide", "before_execute", "after_execute"
	PlayerX      int
	PlayerY      int
	Mode         string
	Target       string
	StuckCounter int
	LastAction   string
	ActionResult string // What action was returned
	Notes        string // Additional context
}

// AIDebugger provides structured debugging for AI behavior
type AIDebugger struct {
	Enabled   bool
	Snapshots []AIDebugSnapshot
	MaxSnaps  int // Circular buffer size
	Verbose   bool
}

// NewAIDebugger creates a new AI debugger
func NewAIDebugger() *AIDebugger {
	return &AIDebugger{
		Enabled:  false,
		MaxSnaps: 100,
		Verbose:  false,
	}
}

// Enable turns on debugging
func (d *AIDebugger) Enable() {
	d.Enabled = true
	d.Snapshots = make([]AIDebugSnapshot, 0, d.MaxSnaps)
}

// EnableVerbose turns on verbose console output
func (d *AIDebugger) EnableVerbose() {
	d.Enable()
	d.Verbose = true
}

// Snapshot records current AI state
func (d *AIDebugger) Snapshot(g *Game, tick int, phase, actionResult, notes string) {
	if !d.Enabled {
		return
	}

	snap := AIDebugSnapshot{
		Tick:         tick,
		Phase:        phase,
		PlayerX:      g.Player.X,
		PlayerY:      g.Player.Y,
		Mode:         g.AI.Mode,
		Target:       g.AI.Target,
		StuckCounter: g.AI.StuckCounter,
		LastAction:   g.AI.LastAction,
		ActionResult: actionResult,
		Notes:        notes,
	}

	// Circular buffer
	if len(d.Snapshots) >= d.MaxSnaps {
		d.Snapshots = d.Snapshots[1:]
	}
	d.Snapshots = append(d.Snapshots, snap)

	if d.Verbose {
		fmt.Printf("[AI T%d %s] pos=(%d,%d) mode=%s target=%s stuck=%d action=%s result=%s",
			tick, phase, snap.PlayerX, snap.PlayerY, snap.Mode, snap.Target,
			snap.StuckCounter, snap.LastAction, actionResult)
		if notes != "" {
			fmt.Printf(" notes=%s", notes)
		}
		fmt.Println()
	}
}

// GetRecentSnapshots returns the last N snapshots
func (d *AIDebugger) GetRecentSnapshots(n int) []AIDebugSnapshot {
	if n > len(d.Snapshots) {
		n = len(d.Snapshots)
	}
	return d.Snapshots[len(d.Snapshots)-n:]
}

// FindStuckPattern looks for patterns where AI is stuck
func (d *AIDebugger) FindStuckPattern() (bool, string) {
	if len(d.Snapshots) < 5 {
		return false, ""
	}

	recent := d.GetRecentSnapshots(10)

	// Check for position oscillation
	positions := make(map[[2]int]int)
	for _, s := range recent {
		pos := [2]int{s.PlayerX, s.PlayerY}
		positions[pos]++
	}

	// If same position appears many times
	for pos, count := range positions {
		if count >= 5 {
			return true, fmt.Sprintf("stuck at (%d,%d) for %d ticks", pos[0], pos[1], count)
		}
	}

	// Check for mode oscillation
	modes := make(map[string]int)
	for _, s := range recent {
		modes[s.Mode]++
	}

	// Check for empty action results
	emptyCount := 0
	for _, s := range recent {
		if s.ActionResult == "" {
			emptyCount++
		}
	}
	if emptyCount >= 5 {
		return true, fmt.Sprintf("AI returning empty action %d times in last %d ticks", emptyCount, len(recent))
	}

	return false, ""
}

// PrintDiagnosis outputs a diagnosis of the current AI state
func (d *AIDebugger) PrintDiagnosis(g *Game) {
	fmt.Println("\n=== AI DIAGNOSIS ===")
	fmt.Printf("Position: (%d, %d)\n", g.Player.X, g.Player.Y)
	fmt.Printf("Mode: %s\n", g.AI.Mode)
	fmt.Printf("Target: %s\n", g.AI.Target)
	fmt.Printf("StuckCounter: %d\n", g.AI.StuckCounter)
	fmt.Printf("LastAction: %s\n", g.AI.LastAction)
	fmt.Printf("ActionCount: %d\n", g.AI.ActionCount)

	fmt.Printf("\nExit: (%d, %d)\n", g.Dungeon.ExitX, g.Dungeon.ExitY)
	distToExit := abs(g.Player.X-g.Dungeon.ExitX) + abs(g.Player.Y-g.Dungeon.ExitY)
	fmt.Printf("Distance to exit: %d\n", distToExit)

	// Check what's on exit
	for _, npc := range g.NPCs {
		if npc.X == g.Dungeon.ExitX && npc.Y == g.Dungeon.ExitY {
			fmt.Printf("WARNING: NPC %s is on exit!\n", npc.ID)
		}
	}
	for _, e := range g.Enemies {
		if e.X == g.Dungeon.ExitX && e.Y == g.Dungeon.ExitY {
			fmt.Printf("WARNING: Enemy %s (state=%d) is on exit!\n", e.ID, e.State)
		}
	}

	// Check player position
	for _, e := range g.Enemies {
		if e.X == g.Player.X && e.Y == g.Player.Y {
			fmt.Printf("WARNING: Dead enemy %s at player position!\n", e.ID)
		}
	}

	// Recent snapshots
	if len(d.Snapshots) > 0 {
		fmt.Println("\n=== Recent Snapshots ===")
		for _, s := range d.GetRecentSnapshots(10) {
			fmt.Printf("T%d [%s]: (%d,%d) mode=%s result=%q notes=%s\n",
				s.Tick, s.Phase, s.PlayerX, s.PlayerY, s.Mode, s.ActionResult, s.Notes)
		}
	}

	// Stuck pattern detection
	if stuck, msg := d.FindStuckPattern(); stuck {
		fmt.Printf("\n!!! STUCK DETECTED: %s\n", msg)
	}

	fmt.Println("===================")
}

// DebugLocalMap returns a string representation of the local map
func DebugLocalMap(g *Game, radius int) string {
	var sb strings.Builder
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			x, y := g.Player.X+dx, g.Player.Y+dy
			if y < 0 || y >= g.Dungeon.Height || x < 0 || x >= g.Dungeon.Width {
				sb.WriteString("?")
				continue
			}
			if dx == 0 && dy == 0 {
				sb.WriteString("@")
				continue
			}

			// Check for entities
			hasEnemy := false
			hasDead := false
			hasNPC := false
			for _, e := range g.Enemies {
				if e.X == x && e.Y == y {
					if e.State == StateDead {
						hasDead = true
					} else {
						hasEnemy = true
					}
				}
			}
			for _, n := range g.NPCs {
				if n.X == x && n.Y == y {
					hasNPC = true
				}
			}

			if hasEnemy {
				sb.WriteString("E")
			} else if hasDead {
				sb.WriteString("x")
			} else if hasNPC {
				sb.WriteString("N")
			} else {
				tile := g.Dungeon.Tiles[y][x]
				switch tile {
				case TileWall:
					sb.WriteString("#")
				case TileFloor:
					sb.WriteString(".")
				case TileDoor:
					sb.WriteString("+")
				case TileLockedDoor:
					sb.WriteString("L")
				case TileStairsDown:
					sb.WriteString(">")
				case TileStairsUp:
					sb.WriteString("<")
				case TileWater:
					sb.WriteString("~")
				case TileLava:
					sb.WriteString("!")
				default:
					sb.WriteString(" ")
				}
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// DebugBFSPath runs BFS and returns debug info about why pathfinding failed or succeeded
func DebugBFSPath(g *Game, targetX, targetY int) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("BFS Debug: (%d,%d) -> (%d,%d)\n", g.Player.X, g.Player.Y, targetX, targetY))

	if g.Player.X == targetX && g.Player.Y == targetY {
		sb.WriteString("Already at target\n")
		return sb.String()
	}

	type node struct {
		x, y      int
		firstMove ActionType
	}

	visited := make(map[[2]int]bool)
	queue := []node{}
	dirs := []struct {
		dx, dy int
		action ActionType
		name   string
	}{
		{0, -1, ActionMoveUp, "up"},
		{0, 1, ActionMoveDown, "down"},
		{-1, 0, ActionMoveLeft, "left"},
		{1, 0, ActionMoveRight, "right"},
	}

	visited[[2]int{g.Player.X, g.Player.Y}] = true

	sb.WriteString("Initial directions:\n")
	for _, d := range dirs {
		nx, ny := g.Player.X+d.dx, g.Player.Y+d.dy
		sb.WriteString(fmt.Sprintf("  %s -> (%d,%d): ", d.name, nx, ny))

		if nx < 0 || nx >= g.Dungeon.Width || ny < 0 || ny >= g.Dungeon.Height {
			sb.WriteString("OUT OF BOUNDS\n")
			continue
		}

		tile := g.Dungeon.Tiles[ny][nx]
		walkable := tile == TileFloor || tile == TileDoor ||
			tile == TileStairsUp || tile == TileStairsDown ||
			tile == TileWater || tile == TileLava

		if tile == TileLockedDoor {
			if g.Player.Keys["rusty_key"] {
				walkable = true
				sb.WriteString("LOCKED (have key) ")
			} else {
				sb.WriteString("LOCKED (no key)\n")
				continue
			}
		}

		if !walkable {
			sb.WriteString(fmt.Sprintf("BLOCKED (tile=%d)\n", tile))
			continue
		}

		// Check for enemies
		for _, e := range g.Enemies {
			if e.X == nx && e.Y == ny && e.State != StateDead {
				sb.WriteString(fmt.Sprintf("ENEMY %s blocks\n", e.ID))
				walkable = false
				break
			}
		}
		if !walkable {
			continue
		}

		if nx == targetX && ny == targetY {
			sb.WriteString(fmt.Sprintf("FOUND (action=%s)\n", d.action))
			return sb.String()
		}

		sb.WriteString("OK\n")
		visited[[2]int{nx, ny}] = true
		queue = append(queue, node{nx, ny, d.action})
	}

	// BFS explore
	explored := 0
	for len(queue) > 0 && len(visited) < 500 {
		current := queue[0]
		queue = queue[1:]
		explored++

		for _, d := range dirs {
			nx, ny := current.x+d.dx, current.y+d.dy
			key := [2]int{nx, ny}
			if visited[key] {
				continue
			}

			if nx < 0 || nx >= g.Dungeon.Width || ny < 0 || ny >= g.Dungeon.Height {
				continue
			}

			tile := g.Dungeon.Tiles[ny][nx]
			walkable := tile == TileFloor || tile == TileDoor ||
				tile == TileStairsUp || tile == TileStairsDown ||
				tile == TileWater || tile == TileLava

			if tile == TileLockedDoor && g.Player.Keys["rusty_key"] {
				walkable = true
			}

			if !walkable {
				continue
			}

			// Check enemies
			blocked := false
			for _, e := range g.Enemies {
				if e.X == nx && e.Y == ny && e.State != StateDead {
					blocked = true
					break
				}
			}
			if blocked {
				continue
			}

			if nx == targetX && ny == targetY {
				sb.WriteString(fmt.Sprintf("Found path after exploring %d nodes, first move: %s\n",
					explored, current.firstMove))
				return sb.String()
			}

			visited[key] = true
			queue = append(queue, node{nx, ny, current.firstMove})
		}
	}

	sb.WriteString(fmt.Sprintf("No path found after exploring %d nodes (visited=%d)\n",
		explored, len(visited)))
	return sb.String()
}
