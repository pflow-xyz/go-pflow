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

// AIPathfinder uses Petri net reachability for AI pathfinding.
// This handles locked doors and keys properly through state space analysis.
// It also supports item-aware pathfinding that rewards collecting items along the way.
type AIPathfinder struct {
	dungeon      *Dungeon
	keyLocations [][2]int
	hasKey       bool
	// Item locations with their values for path scoring
	itemLocations map[[2]int]float64
	// Chest locations (chests are tiles, not items)
	chestLocations map[[2]int]bool
}

// NewAIPathfinder creates a pathfinder for the AI
func NewAIPathfinder(d *Dungeon, keyLocations [][2]int, hasKey bool) *AIPathfinder {
	return &AIPathfinder{
		dungeon:        d,
		keyLocations:   keyLocations,
		hasKey:         hasKey,
		itemLocations:  make(map[[2]int]float64),
		chestLocations: make(map[[2]int]bool),
	}
}

// WithItems adds item locations with their values to the pathfinder.
// This enables item-aware pathfinding that rewards paths through valuable items.
func (pf *AIPathfinder) WithItems(items map[[2]int]float64) *AIPathfinder {
	pf.itemLocations = items
	return pf
}

// WithChests adds chest locations to the pathfinder.
func (pf *AIPathfinder) WithChests(chests [][2]int) *AIPathfinder {
	pf.chestLocations = make(map[[2]int]bool)
	for _, c := range chests {
		pf.chestLocations[c] = true
	}
	return pf
}

// FindPath returns the path from start to target using Petri net reachability.
// Returns a slice of positions representing the path, or nil if no path exists.
// The path includes the starting position.
// Note: For performance, use FindPathFast for simple pathfinding without locked doors.
func (pf *AIPathfinder) FindPath(startX, startY, targetX, targetY int) [][2]int {
	// For very short distances, use simple BFS instead of Petri net
	manhattanDist := abs(targetX-startX) + abs(targetY-startY)
	if manhattanDist <= 3 {
		return pf.findPathBFS(startX, startY, targetX, targetY)
	}

	// Check if we need Petri net (locked doors present and no key)
	hasLockedDoors := false
	for y := 0; y < pf.dungeon.Height && !hasLockedDoors; y++ {
		for x := 0; x < pf.dungeon.Width && !hasLockedDoors; x++ {
			if pf.dungeon.Tiles[y][x] == TileLockedDoor {
				hasLockedDoors = true
			}
		}
	}

	// If no locked doors or we have key, use simple BFS
	if !hasLockedDoors || pf.hasKey {
		return pf.findPathBFS(startX, startY, targetX, targetY)
	}

	// Need Petri net analysis for locked doors
	// Build Petri net for the dungeon with current position as start
	net := pf.buildPathfindingNet(startX, startY, targetX, targetY)

	// Set initial marking: token at current position
	initial := reachability.Marking{
		fmt.Sprintf("pos_%d_%d", startX, startY): 1,
	}
	if pf.hasKey {
		initial["has_key"] = 1
	}

	// Set target marking: token at goal position
	target := reachability.Marking{
		fmt.Sprintf("pos_%d_%d", targetX, targetY): 1,
	}
	if pf.hasKey {
		target["has_key"] = 1
	}

	// Create analyzer and find path with limited state exploration
	analyzer := reachability.NewAnalyzer(net).
		WithInitialMarking(initial).
		WithMaxStates(1000). // Reduced limit for performance
		WithMaxTokens(10)

	transitions := analyzer.PathTo(target)
	if transitions == nil {
		// Petri net couldn't find path, fall back to BFS
		return pf.findPathBFS(startX, startY, targetX, targetY)
	}

	// Convert transition sequence to position sequence
	path := [][2]int{{startX, startY}}
	for _, trans := range transitions {
		// Parse transition name to extract destination coordinates
		// Format: "move_X_Y_to_X2_Y2" or "pickup_key_X_Y"
		var x1, y1, x2, y2 int
		if n, _ := fmt.Sscanf(trans, "move_%d_%d_to_%d_%d", &x1, &y1, &x2, &y2); n == 4 {
			path = append(path, [2]int{x2, y2})
		}
		// pickup_key transitions don't change position
	}

	return path
}

// findPathBFS finds a path using BFS with item-aware scoring.
// When items are configured, it explores multiple paths and selects the one
// with the best score (shortest path that collects valuable items along the way).
func (pf *AIPathfinder) findPathBFS(startX, startY, targetX, targetY int) [][2]int {
	if startX == targetX && startY == targetY {
		return [][2]int{{startX, startY}}
	}

	// If no items/chests configured, use simple BFS
	if len(pf.itemLocations) == 0 && len(pf.chestLocations) == 0 {
		return pf.findPathBFSSimple(startX, startY, targetX, targetY)
	}

	// Use scored BFS: find path that maximizes value collected while minimizing distance
	return pf.findPathBFSScored(startX, startY, targetX, targetY)
}

// findPathAStar finds a path using A* algorithm with Manhattan distance heuristic.
// A* is more efficient than BFS because it prioritizes exploring nodes closer to the goal.
func (pf *AIPathfinder) findPathAStar(startX, startY, targetX, targetY int) [][2]int {
	if startX == targetX && startY == targetY {
		return [][2]int{{startX, startY}}
	}

	// Priority queue node
	type astarNode struct {
		x, y   int
		g      int // Cost from start
		f      int // g + heuristic
		parent *astarNode
	}

	// Manhattan distance heuristic
	heuristic := func(x, y int) int {
		dx := x - targetX
		if dx < 0 {
			dx = -dx
		}
		dy := y - targetY
		if dy < 0 {
			dy = -dy
		}
		return dx + dy
	}

	isWalkable := func(x, y int) bool {
		if x < 0 || x >= pf.dungeon.Width || y < 0 || y >= pf.dungeon.Height {
			return false
		}
		tile := pf.dungeon.Tiles[y][x]
		switch tile {
		case TileFloor, TileDoor, TileStairsDown, TileStairsUp, TileChest, TileAltar:
			return true
		case TileLockedDoor:
			return pf.hasKey // Only walkable if we have key
		default:
			return false
		}
	}

	open := []*astarNode{{x: startX, y: startY, g: 0, f: heuristic(startX, startY), parent: nil}}
	closed := make(map[[2]int]bool)

	dirs := [][2]int{{0, -1}, {0, 1}, {-1, 0}, {1, 0}}

	for len(open) > 0 {
		// Find node with lowest f score
		minIdx := 0
		for i := 1; i < len(open); i++ {
			if open[i].f < open[minIdx].f {
				minIdx = i
			}
		}
		curr := open[minIdx]
		open = append(open[:minIdx], open[minIdx+1:]...)

		if curr.x == targetX && curr.y == targetY {
			// Reconstruct path
			var path [][2]int
			for n := curr; n != nil; n = n.parent {
				path = append([][2]int{{n.x, n.y}}, path...)
			}
			return path
		}

		closed[[2]int{curr.x, curr.y}] = true

		for _, d := range dirs {
			nx, ny := curr.x+d[0], curr.y+d[1]
			key := [2]int{nx, ny}

			if closed[key] || !isWalkable(nx, ny) {
				continue
			}

			newG := curr.g + 1
			newF := newG + heuristic(nx, ny)

			// Check if already in open set with better score
			inOpen := false
			for _, n := range open {
				if n.x == nx && n.y == ny {
					inOpen = true
					if newG < n.g {
						n.g = newG
						n.f = newF
						n.parent = curr
					}
					break
				}
			}

			if !inOpen {
				open = append(open, &astarNode{x: nx, y: ny, g: newG, f: newF, parent: curr})
			}
		}
	}

	return nil
}

// findPathBFSSimple is the original simple BFS without item scoring
// Deprecated: Use findPathAStar for better performance
func (pf *AIPathfinder) findPathBFSSimple(startX, startY, targetX, targetY int) [][2]int {
	// Now just calls A* for better performance
	return pf.findPathAStar(startX, startY, targetX, targetY)
}

// findPathBFSScored finds a path using scored BFS that rewards item collection.
// It explores paths and scores them by: value_collected - (extra_steps * cost_per_step)
// This allows small detours to pick up valuable items along the way.
func (pf *AIPathfinder) findPathBFSScored(startX, startY, targetX, targetY int) [][2]int {
	const (
		stepCost       = 1.0  // Cost per extra step beyond shortest path
		chestValue     = 15.0 // Value of passing through a chest tile
		maxExtraSteps  = 5    // Maximum extra steps allowed for item collection
		maxSearchNodes = 2000 // Limit search space
	)

	type scoredNode struct {
		x, y         int
		path         [][2]int
		valueCollected float64
		itemsVisited map[[2]int]bool // Track which items we've counted
	}

	isWalkable := func(x, y int) bool {
		if x < 0 || x >= pf.dungeon.Width || y < 0 || y >= pf.dungeon.Height {
			return false
		}
		tile := pf.dungeon.Tiles[y][x]
		switch tile {
		case TileFloor, TileDoor, TileStairsDown, TileStairsUp, TileChest, TileAltar:
			return true
		case TileLockedDoor:
			return pf.hasKey
		default:
			return false
		}
	}

	// First, find the shortest path length using simple BFS
	shortestPath := pf.findPathBFSSimple(startX, startY, targetX, targetY)
	if shortestPath == nil {
		return nil
	}
	shortestLen := len(shortestPath)

	// Now search for paths up to maxExtraSteps longer that might collect items
	maxLen := shortestLen + maxExtraSteps

	// Track best path found
	var bestPath [][2]int
	bestScore := float64(-1000) // Start negative

	// BFS with scoring
	visited := make(map[[2]int]int) // Position -> shortest path length to reach it
	initialItems := make(map[[2]int]bool)
	queue := []scoredNode{{
		x: startX, y: startY,
		path:         [][2]int{{startX, startY}},
		valueCollected: 0,
		itemsVisited: initialItems,
	}}
	visited[[2]int{startX, startY}] = 1

	dirs := [][2]int{{0, -1}, {0, 1}, {-1, 0}, {1, 0}}

	nodesExplored := 0
	for len(queue) > 0 && nodesExplored < maxSearchNodes {
		nodesExplored++
		curr := queue[0]
		queue = queue[1:]

		// Don't explore paths longer than our limit
		if len(curr.path) > maxLen {
			continue
		}

		// Check if we've reached target
		if curr.x == targetX && curr.y == targetY {
			// Calculate score: value - (extra_steps * cost)
			extraSteps := len(curr.path) - shortestLen
			score := curr.valueCollected - float64(extraSteps)*stepCost

			if score > bestScore || (score == bestScore && len(curr.path) < len(bestPath)) {
				bestScore = score
				bestPath = curr.path
			}
			continue // Don't explore further from target
		}

		for _, d := range dirs {
			nx, ny := curr.x+d[0], curr.y+d[1]
			key := [2]int{nx, ny}

			if !isWalkable(nx, ny) {
				continue
			}

			newPathLen := len(curr.path) + 1

			// Allow revisiting if this path is not significantly longer
			if prevLen, seen := visited[key]; seen && newPathLen > prevLen+2 {
				continue // Don't revisit if much longer path
			}

			// Calculate value at this position
			newValue := curr.valueCollected
			newItems := make(map[[2]int]bool)
			for k, v := range curr.itemsVisited {
				newItems[k] = v
			}

			// Check for item at this position (only count once per path)
			if !newItems[key] {
				if itemVal, hasItem := pf.itemLocations[key]; hasItem {
					newValue += itemVal
					newItems[key] = true
				}
				if pf.chestLocations[key] {
					newValue += chestValue
					newItems[key] = true
				}
			}

			newPath := make([][2]int, newPathLen)
			copy(newPath, curr.path)
			newPath[len(curr.path)] = key

			// Update visited with shortest path length
			if prevLen, seen := visited[key]; !seen || newPathLen < prevLen {
				visited[key] = newPathLen
			}

			queue = append(queue, scoredNode{
				x: nx, y: ny,
				path:           newPath,
				valueCollected: newValue,
				itemsVisited:   newItems,
			})
		}
	}

	// Return best path found, or shortest if no better scored path
	if bestPath != nil {
		return bestPath
	}
	return shortestPath
}

// GetNextMove returns the next action to take toward the target.
// Returns empty string if already at target or no path exists.
func (pf *AIPathfinder) GetNextMove(startX, startY, targetX, targetY int) ActionType {
	if startX == targetX && startY == targetY {
		return "" // Already at target
	}

	path := pf.FindPath(startX, startY, targetX, targetY)
	if path == nil || len(path) < 2 {
		return "" // No path or already at target
	}

	// Get direction from first to second position in path
	nextX, nextY := path[1][0], path[1][1]
	dx := nextX - startX
	dy := nextY - startY

	switch {
	case dx == 0 && dy == -1:
		return ActionMoveUp
	case dx == 0 && dy == 1:
		return ActionMoveDown
	case dx == -1 && dy == 0:
		return ActionMoveLeft
	case dx == 1 && dy == 0:
		return ActionMoveRight
	default:
		return "" // Invalid move
	}
}

// buildPathfindingNet creates a Petri net for pathfinding from current position
func (pf *AIPathfinder) buildPathfindingNet(startX, startY, targetX, targetY int) *petri.PetriNet {
	net := petri.NewPetriNet()

	// Add "has_key" place
	keyTokens := 0.0
	if pf.hasKey {
		keyTokens = 1.0
	}
	net.AddPlace("has_key", keyTokens, nil, 0, 0, nil)

	// Track which positions we've added
	addedPlaces := make(map[string]bool)

	// Helper to check walkability
	isWalkable := func(x, y int) bool {
		if x < 0 || x >= pf.dungeon.Width || y < 0 || y >= pf.dungeon.Height {
			return false
		}
		tile := pf.dungeon.Tiles[y][x]
		switch tile {
		case TileFloor, TileDoor, TileStairsDown, TileStairsUp, TileChest, TileAltar:
			return true
		case TileLockedDoor:
			return true // Can walk through if we have key (handled by transition)
		default:
			return false
		}
	}

	isLockedDoor := func(x, y int) bool {
		if x < 0 || x >= pf.dungeon.Width || y < 0 || y >= pf.dungeon.Height {
			return false
		}
		return pf.dungeon.Tiles[y][x] == TileLockedDoor
	}

	// Add places for walkable tiles
	for y := 0; y < pf.dungeon.Height; y++ {
		for x := 0; x < pf.dungeon.Width; x++ {
			if isWalkable(x, y) {
				placeID := fmt.Sprintf("pos_%d_%d", x, y)
				tokens := 0.0
				if x == startX && y == startY {
					tokens = 1.0
				}
				net.AddPlace(placeID, tokens, nil, float64(x*10), float64(y*10), nil)
				addedPlaces[placeID] = true
			}
		}
	}

	// Add transitions for movements
	directions := [][2]int{{0, -1}, {0, 1}, {-1, 0}, {1, 0}}

	for y := 0; y < pf.dungeon.Height; y++ {
		for x := 0; x < pf.dungeon.Width; x++ {
			if !isWalkable(x, y) {
				continue
			}

			fromPlace := fmt.Sprintf("pos_%d_%d", x, y)

			for _, dir := range directions {
				nx, ny := x+dir[0], y+dir[1]
				if !isWalkable(nx, ny) {
					continue
				}

				toPlace := fmt.Sprintf("pos_%d_%d", nx, ny)
				transID := fmt.Sprintf("move_%d_%d_to_%d_%d", x, y, nx, ny)

				// Check if destination is a locked door
				if isLockedDoor(nx, ny) {
					// Requires key to move through locked door
					net.AddTransition(transID, "locked", 0, 0, nil)
					net.AddArc(fromPlace, transID, 1.0, false)  // consume position
					net.AddArc("has_key", transID, 1.0, false)  // require key
					net.AddArc(transID, toPlace, 1.0, false)    // produce new position
					net.AddArc(transID, "has_key", 1.0, false)  // keep the key
				} else {
					// Normal movement
					net.AddTransition(transID, "move", 0, 0, nil)
					net.AddArc(fromPlace, transID, 1.0, false)
					net.AddArc(transID, toPlace, 1.0, false)
				}
			}
		}
	}

	// Add key pickup transitions
	for i, keyLoc := range pf.keyLocations {
		kx, ky := keyLoc[0], keyLoc[1]
		posPlace := fmt.Sprintf("pos_%d_%d", kx, ky)
		keyPlace := fmt.Sprintf("key_%d", i)

		// Add key place (starts with 1 token = key exists)
		net.AddPlace(keyPlace, 1.0, nil, 0, 0, nil)

		// Pickup transition
		transID := fmt.Sprintf("pickup_key_%d_%d", kx, ky)
		net.AddTransition(transID, "pickup", 0, 0, nil)
		net.AddArc(posPlace, transID, 1.0, false)    // at key location
		net.AddArc(keyPlace, transID, 1.0, false)    // key exists
		net.AddArc(transID, posPlace, 1.0, false)    // stay at position
		net.AddArc(transID, "has_key", 1.0, false)   // gain key
	}

	return net
}

// FindPathToKey finds the nearest reachable key.
// Returns the key position and the path to it, or nil if no key is reachable.
func (pf *AIPathfinder) FindPathToKey(startX, startY int) ([2]int, [][2]int) {
	var bestPath [][2]int
	var bestKey [2]int
	bestLen := -1

	for _, keyLoc := range pf.keyLocations {
		path := pf.FindPath(startX, startY, keyLoc[0], keyLoc[1])
		if path != nil && (bestLen < 0 || len(path) < bestLen) {
			bestPath = path
			bestKey = keyLoc
			bestLen = len(path)
		}
	}

	return bestKey, bestPath
}
