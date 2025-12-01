package catacombs

import (
	"fmt"

	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/reachability"
)

// DungeonReachability models dungeon connectivity using Petri nets
// to verify that stairs are reachable from spawn, considering locked doors and keys
type DungeonReachability struct {
	dungeon    *Dungeon
	keyLocations [][2]int
}

// NewDungeonReachability creates a reachability analyzer for the dungeon
func NewDungeonReachability(d *Dungeon, keyLocations [][2]int) *DungeonReachability {
	return &DungeonReachability{
		dungeon:      d,
		keyLocations: keyLocations,
	}
}

// isWalkable returns true if a tile can be walked on (ignoring locked doors)
func (dr *DungeonReachability) isWalkable(x, y int) bool {
	if x < 0 || x >= dr.dungeon.Width || y < 0 || y >= dr.dungeon.Height {
		return false
	}
	tile := dr.dungeon.Tiles[y][x]
	switch tile {
	case TileFloor, TileDoor, TileStairsDown, TileStairsUp, TileChest, TileAltar:
		return true
	default:
		return false
	}
}

// isWalkableWithKey returns true if a tile can be walked on when player has a key
func (dr *DungeonReachability) isWalkableWithKey(x, y int) bool {
	if x < 0 || x >= dr.dungeon.Width || y < 0 || y >= dr.dungeon.Height {
		return false
	}
	tile := dr.dungeon.Tiles[y][x]
	switch tile {
	case TileFloor, TileDoor, TileLockedDoor, TileStairsDown, TileStairsUp, TileChest, TileAltar:
		return true
	default:
		return false
	}
}

// isLockedDoor returns true if the tile is a locked door
func (dr *DungeonReachability) isLockedDoor(x, y int) bool {
	if x < 0 || x >= dr.dungeon.Width || y < 0 || y >= dr.dungeon.Height {
		return false
	}
	return dr.dungeon.Tiles[y][x] == TileLockedDoor
}

// BuildPetriNet creates a Petri net model of the dungeon
// Places:
//   - "pos_X_Y" for each walkable tile
//   - "has_key" for key possession state
//   - "at_stairs" goal state
// Transitions:
//   - "move_X_Y_to_X2_Y2" for each valid movement
//   - "pickup_key_X_Y" for key locations
//   - "unlock_X_Y" for locked doors (requires has_key)
func (dr *DungeonReachability) BuildPetriNet() *petri.PetriNet {
	net := petri.NewPetriNet()

	// Add "has_key" place (starts with 0 tokens)
	net.AddPlace("has_key", 0, nil, 0, 0, nil)

	// Add "at_stairs" goal place
	net.AddPlace("at_stairs", 0, nil, 0, 0, nil)

	// Track which positions we've added
	addedPlaces := make(map[string]bool)

	// First pass: add places for all walkable tiles
	for y := 0; y < dr.dungeon.Height; y++ {
		for x := 0; x < dr.dungeon.Width; x++ {
			if dr.isWalkableWithKey(x, y) {
				placeID := fmt.Sprintf("pos_%d_%d", x, y)
				// Start with 1 token at spawn position
				tokens := 0.0
				if x == dr.dungeon.SpawnX && y == dr.dungeon.SpawnY {
					tokens = 1.0
				}
				net.AddPlace(placeID, tokens, nil, float64(x*10), float64(y*10), nil)
				addedPlaces[placeID] = true
			}
		}
	}

	// Second pass: add transitions for movements
	directions := [][2]int{{0, -1}, {0, 1}, {-1, 0}, {1, 0}}

	for y := 0; y < dr.dungeon.Height; y++ {
		for x := 0; x < dr.dungeon.Width; x++ {
			if !dr.isWalkableWithKey(x, y) {
				continue
			}

			fromPlace := fmt.Sprintf("pos_%d_%d", x, y)

			for _, dir := range directions {
				nx, ny := x+dir[0], y+dir[1]
				if !dr.isWalkableWithKey(nx, ny) {
					continue
				}

				toPlace := fmt.Sprintf("pos_%d_%d", nx, ny)
				transID := fmt.Sprintf("move_%d_%d_to_%d_%d", x, y, nx, ny)

				// Check if destination is a locked door
				if dr.isLockedDoor(nx, ny) {
					// Requires key to move through locked door
					net.AddTransition(transID, "locked", 0, 0, nil)
					net.AddArc(fromPlace, transID, 1.0, false)  // consume position
					net.AddArc("has_key", transID, 1.0, false)  // require key (but don't consume)
					net.AddArc(transID, toPlace, 1.0, false)    // produce new position
					net.AddArc(transID, "has_key", 1.0, false)  // keep the key
				} else {
					// Normal movement
					net.AddTransition(transID, "move", 0, 0, nil)
					net.AddArc(fromPlace, transID, 1.0, false)
					net.AddArc(transID, toPlace, 1.0, false)
				}
			}

			// If this is the stairs, add transition to goal
			if x == dr.dungeon.ExitX && y == dr.dungeon.ExitY {
				transID := fmt.Sprintf("reach_stairs_%d_%d", x, y)
				net.AddTransition(transID, "goal", 0, 0, nil)
				net.AddArc(fromPlace, transID, 1.0, false)
				net.AddArc(transID, "at_stairs", 1.0, false)
				net.AddArc(transID, fromPlace, 1.0, false) // Stay at position too
			}
		}
	}

	// Add key pickup transitions
	for i, keyLoc := range dr.keyLocations {
		x, y := keyLoc[0], keyLoc[1]
		posPlace := fmt.Sprintf("pos_%d_%d", x, y)
		keyPlace := fmt.Sprintf("key_%d", i)

		// Add key place (starts with 1 token = key exists)
		net.AddPlace(keyPlace, 1.0, nil, 0, 0, nil)

		// Pickup transition
		transID := fmt.Sprintf("pickup_key_%d_%d", x, y)
		net.AddTransition(transID, "pickup", 0, 0, nil)
		net.AddArc(posPlace, transID, 1.0, false)    // at key location
		net.AddArc(keyPlace, transID, 1.0, false)    // key exists
		net.AddArc(transID, posPlace, 1.0, false)    // stay at position
		net.AddArc(transID, "has_key", 1.0, false)   // gain key
	}

	return net
}

// CheckStairsReachable uses discrete reachability analysis to verify
// that the stairs can be reached from spawn
func (dr *DungeonReachability) CheckStairsReachable() (bool, string) {
	net := dr.BuildPetriNet()

	// Create analyzer with reasonable limits
	analyzer := reachability.NewAnalyzer(net).
		WithMaxStates(10000).
		WithMaxTokens(100)

	// Check if "at_stairs" can have a token
	target := reachability.Marking{"at_stairs": 1}

	if analyzer.IsReachable(target) {
		return true, "Stairs reachable from spawn"
	}

	// Analyze why not reachable
	result := analyzer.Analyze()

	// Check if there are locked doors
	hasLockedDoors := false
	for y := 0; y < dr.dungeon.Height; y++ {
		for x := 0; x < dr.dungeon.Width; x++ {
			if dr.dungeon.Tiles[y][x] == TileLockedDoor {
				hasLockedDoors = true
				break
			}
		}
		if hasLockedDoors {
			break
		}
	}

	if hasLockedDoors && len(dr.keyLocations) == 0 {
		return false, "Locked doors exist but no keys available"
	}

	if hasLockedDoors && len(dr.keyLocations) > 0 {
		// Check if keys are reachable without going through locked doors
		for _, keyLoc := range dr.keyLocations {
			if dr.isKeyReachableWithoutLockedDoors(keyLoc[0], keyLoc[1]) {
				// Key is reachable, but stairs still aren't - might be disconnected
				return false, fmt.Sprintf("Key at (%d,%d) is reachable but stairs are not (map may be disconnected)", keyLoc[0], keyLoc[1])
			}
		}
		return false, "Keys exist but are behind locked doors"
	}

	return false, fmt.Sprintf("Stairs unreachable: explored %d states, %d deadlocks", result.StateCount, len(result.Deadlocks))
}

// isKeyReachableWithoutLockedDoors checks if a key can be reached without going through locked doors
func (dr *DungeonReachability) isKeyReachableWithoutLockedDoors(keyX, keyY int) bool {
	// Simple BFS from spawn to key, ignoring locked doors
	visited := make(map[[2]int]bool)
	queue := [][2]int{{dr.dungeon.SpawnX, dr.dungeon.SpawnY}}

	directions := [][2]int{{0, -1}, {0, 1}, {-1, 0}, {1, 0}}

	for len(queue) > 0 {
		pos := queue[0]
		queue = queue[1:]

		if pos[0] == keyX && pos[1] == keyY {
			return true
		}

		if visited[pos] {
			continue
		}
		visited[pos] = true

		for _, dir := range directions {
			nx, ny := pos[0]+dir[0], pos[1]+dir[1]
			if dr.isWalkable(nx, ny) && !dr.isLockedDoor(nx, ny) {
				queue = append(queue, [2]int{nx, ny})
			}
		}
	}

	return false
}

// SimpleReachabilityCheck performs a fast BFS-based reachability check
// Returns true if stairs are reachable (with or without key)
func (dr *DungeonReachability) SimpleReachabilityCheck() (bool, bool, string) {
	// First check: can we reach stairs without needing any keys?
	stairsReachableWithoutKey := dr.canReachWithBFS(
		dr.dungeon.SpawnX, dr.dungeon.SpawnY,
		dr.dungeon.ExitX, dr.dungeon.ExitY,
		false, // don't allow locked doors
	)

	if stairsReachableWithoutKey {
		return true, false, "Stairs directly reachable"
	}

	// Check if any keys are reachable
	keyReachable := false
	var reachableKeyLoc [2]int
	for _, keyLoc := range dr.keyLocations {
		if dr.canReachWithBFS(dr.dungeon.SpawnX, dr.dungeon.SpawnY, keyLoc[0], keyLoc[1], false) {
			keyReachable = true
			reachableKeyLoc = keyLoc
			break
		}
	}

	if !keyReachable && len(dr.keyLocations) > 0 {
		return false, true, "Keys exist but are not reachable"
	}

	// Check if stairs are reachable with key (allowing locked doors)
	stairsReachableWithKey := dr.canReachWithBFS(
		dr.dungeon.SpawnX, dr.dungeon.SpawnY,
		dr.dungeon.ExitX, dr.dungeon.ExitY,
		true, // allow locked doors
	)

	if stairsReachableWithKey && keyReachable {
		return true, true, fmt.Sprintf("Stairs reachable via key at (%d,%d)", reachableKeyLoc[0], reachableKeyLoc[1])
	}

	return false, false, "Stairs not reachable even with keys"
}

// canReachWithBFS performs BFS pathfinding
func (dr *DungeonReachability) canReachWithBFS(startX, startY, targetX, targetY int, allowLockedDoors bool) bool {
	visited := make(map[[2]int]bool)
	queue := [][2]int{{startX, startY}}

	directions := [][2]int{{0, -1}, {0, 1}, {-1, 0}, {1, 0}}

	for len(queue) > 0 {
		pos := queue[0]
		queue = queue[1:]

		if pos[0] == targetX && pos[1] == targetY {
			return true
		}

		if visited[pos] {
			continue
		}
		visited[pos] = true

		for _, dir := range directions {
			nx, ny := pos[0]+dir[0], pos[1]+dir[1]

			walkable := dr.isWalkable(nx, ny)
			if !walkable && allowLockedDoors && dr.isLockedDoor(nx, ny) {
				walkable = true
			}

			if walkable {
				queue = append(queue, [2]int{nx, ny})
			}
		}
	}

	return false
}

// ValidateDungeon checks dungeon connectivity and returns whether it's valid
func ValidateDungeon(d *Dungeon, keyLocations [][2]int) (bool, string) {
	dr := NewDungeonReachability(d, keyLocations)
	reachable, needsKey, reason := dr.SimpleReachabilityCheck()
	_ = needsKey // we just care if it's reachable
	return reachable, reason
}

// ValidateDungeonWithPetriNet uses full Petri net reachability analysis
func ValidateDungeonWithPetriNet(d *Dungeon, keyLocations [][2]int) (bool, string) {
	dr := NewDungeonReachability(d, keyLocations)
	return dr.CheckStairsReachable()
}
