// Package catacombs implements a roguelike dungeon crawler using Petri nets.
//
// "Catacombs of Pflow" - A procedurally generated dungeon with NPCs,
// dialogue, quests, and both top-down and 3D raycasting views.
package catacombs

import (
	"fmt"
	"math/rand"
)

// DungeonParams controls procedural generation
type DungeonParams struct {
	Width        int     `json:"width"`
	Height       int     `json:"height"`
	RoomCount    int     `json:"room_count"`
	MinRoomSize  int     `json:"min_room_size"`
	MaxRoomSize  int     `json:"max_room_size"`
	EnemyDensity float64 `json:"enemy_density"` // 0.0-1.0
	LootDensity  float64 `json:"loot_density"`  // 0.0-1.0
	NPCCount     int     `json:"npc_count"`
	Seed         int64   `json:"seed"`    // Map/game seed (0 = random)
	AISeed       int64   `json:"ai_seed"` // AI decision seed (0 = use Seed)
	Difficulty   int     `json:"difficulty"` // 1-10
}

// DefaultParams returns standard dungeon parameters
func DefaultParams() DungeonParams {
	return DungeonParams{
		Width:       40,
		Height:      30,
		RoomCount:   8,
		MinRoomSize: 4,
		MaxRoomSize: 8,
		EnemyDensity: 0.3,
		LootDensity:  0.2,
		NPCCount:    3,
		Seed:        0, // 0 = random
		Difficulty:  1,
	}
}

// TileType represents dungeon tiles
type TileType int

const (
	TileVoid TileType = iota
	TileFloor
	TileWall
	TileDoor
	TileLockedDoor
	TileStairsDown
	TileStairsUp
	TileWater
	TileLava
	TileChest
	TileAltar
)

// Room represents a dungeon room
type Room struct {
	X, Y          int
	Width, Height int
	Type          RoomType
	Connected     bool
}

// RoomType categorizes rooms
type RoomType int

const (
	RoomNormal RoomType = iota
	RoomTreasure
	RoomShrine
	RoomBoss
	RoomStart
	RoomExit
)

// Dungeon represents a generated level
type Dungeon struct {
	Width, Height int
	Tiles         [][]TileType
	Rooms         []*Room
	SpawnX, SpawnY int
	ExitX, ExitY   int
	Level         int
	Params        DungeonParams
}

// GenerateDungeon creates a new procedural dungeon
func GenerateDungeon(params DungeonParams, level int) *Dungeon {
	seed := params.Seed
	if seed == 0 {
		seed = rand.Int63()
	}
	rng := rand.New(rand.NewSource(seed))

	d := &Dungeon{
		Width:  params.Width,
		Height: params.Height,
		Tiles:  make([][]TileType, params.Height),
		Rooms:  make([]*Room, 0),
		Level:  level,
		Params: params,
	}

	// Initialize with void
	for y := 0; y < params.Height; y++ {
		d.Tiles[y] = make([]TileType, params.Width)
		for x := 0; x < params.Width; x++ {
			d.Tiles[y][x] = TileVoid
		}
	}

	// Generate rooms
	for i := 0; i < params.RoomCount*3 && len(d.Rooms) < params.RoomCount; i++ {
		room := d.tryPlaceRoom(rng, params)
		if room != nil {
			d.Rooms = append(d.Rooms, room)
		}
	}

	// Assign room types
	if len(d.Rooms) > 0 {
		d.Rooms[0].Type = RoomStart
		d.Rooms[len(d.Rooms)-1].Type = RoomExit

		// Add special rooms
		for i := 1; i < len(d.Rooms)-1; i++ {
			roll := rng.Float64()
			if roll < 0.15 {
				d.Rooms[i].Type = RoomTreasure
			} else if roll < 0.25 {
				d.Rooms[i].Type = RoomShrine
			} else if roll < 0.30 && level > 2 {
				d.Rooms[i].Type = RoomBoss
			}
		}
	}

	// Connect rooms with corridors
	d.connectRooms(rng)

	// Add doors at corridor-room junctions
	d.addDoors(rng)

	// Set spawn and exit
	if len(d.Rooms) > 0 {
		startRoom := d.Rooms[0]
		d.SpawnX = startRoom.X + startRoom.Width/2
		d.SpawnY = startRoom.Y + startRoom.Height/2
		d.Tiles[d.SpawnY][d.SpawnX] = TileStairsUp

		exitRoom := d.Rooms[len(d.Rooms)-1]
		d.ExitX = exitRoom.X + exitRoom.Width/2
		d.ExitY = exitRoom.Y + exitRoom.Height/2
		d.Tiles[d.ExitY][d.ExitX] = TileStairsDown
	}

	// Always place a locked door blocking the exit for testing
	// This makes keys more important and encourages NPC interaction
	d.PlaceLockedDoorBlockingExit(rng, 1.0)

	// Add room features
	d.addRoomFeatures(rng)

	// Add walls around floors
	d.addWalls()

	return d
}

func (d *Dungeon) tryPlaceRoom(rng *rand.Rand, params DungeonParams) *Room {
	w := params.MinRoomSize + rng.Intn(params.MaxRoomSize-params.MinRoomSize+1)
	h := params.MinRoomSize + rng.Intn(params.MaxRoomSize-params.MinRoomSize+1)
	x := 1 + rng.Intn(params.Width-w-2)
	y := 1 + rng.Intn(params.Height-h-2)

	// Check overlap with existing rooms (with padding)
	for _, room := range d.Rooms {
		if x < room.X+room.Width+2 && x+w+2 > room.X &&
			y < room.Y+room.Height+2 && y+h+2 > room.Y {
			return nil
		}
	}

	// Carve out the room
	for ry := y; ry < y+h; ry++ {
		for rx := x; rx < x+w; rx++ {
			d.Tiles[ry][rx] = TileFloor
		}
	}

	return &Room{X: x, Y: y, Width: w, Height: h, Type: RoomNormal}
}

func (d *Dungeon) connectRooms(rng *rand.Rand) {
	// Connect each room to the next using L-shaped corridors
	for i := 0; i < len(d.Rooms)-1; i++ {
		r1 := d.Rooms[i]
		r2 := d.Rooms[i+1]

		x1 := r1.X + r1.Width/2
		y1 := r1.Y + r1.Height/2
		x2 := r2.X + r2.Width/2
		y2 := r2.Y + r2.Height/2

		// Randomly choose horizontal-first or vertical-first
		if rng.Float64() < 0.5 {
			d.carveHorizontal(x1, x2, y1)
			d.carveVertical(y1, y2, x2)
		} else {
			d.carveVertical(y1, y2, x1)
			d.carveHorizontal(x1, x2, y2)
		}

		r1.Connected = true
		r2.Connected = true
	}
}

func (d *Dungeon) carveHorizontal(x1, x2, y int) {
	if x1 > x2 {
		x1, x2 = x2, x1
	}
	for x := x1; x <= x2; x++ {
		if y > 0 && y < d.Height-1 && x > 0 && x < d.Width-1 {
			if d.Tiles[y][x] == TileVoid {
				d.Tiles[y][x] = TileFloor
			}
		}
	}
}

func (d *Dungeon) carveVertical(y1, y2, x int) {
	if y1 > y2 {
		y1, y2 = y2, y1
	}
	for y := y1; y <= y2; y++ {
		if y > 0 && y < d.Height-1 && x > 0 && x < d.Width-1 {
			if d.Tiles[y][x] == TileVoid {
				d.Tiles[y][x] = TileFloor
			}
		}
	}
}

func (d *Dungeon) addDoors(rng *rand.Rand) {
	for y := 1; y < d.Height-1; y++ {
		for x := 1; x < d.Width-1; x++ {
			if d.Tiles[y][x] != TileFloor {
				continue
			}

			// Check for door-worthy positions (corridor meets room)
			horizWalls := d.Tiles[y-1][x] == TileVoid && d.Tiles[y+1][x] == TileVoid
			vertWalls := d.Tiles[y][x-1] == TileVoid && d.Tiles[y][x+1] == TileVoid

			if (horizWalls || vertWalls) && rng.Float64() < 0.3 {
				if rng.Float64() < 0.2 {
					d.Tiles[y][x] = TileLockedDoor
				} else {
					d.Tiles[y][x] = TileDoor
				}
			}
		}
	}
}

func (d *Dungeon) addRoomFeatures(rng *rand.Rand) {
	for _, room := range d.Rooms {
		switch room.Type {
		case RoomTreasure:
			// Add chests
			cx := room.X + room.Width/2
			cy := room.Y + room.Height/2
			d.Tiles[cy][cx] = TileChest

		case RoomShrine:
			// Add altar
			cx := room.X + room.Width/2
			cy := room.Y + room.Height/2
			d.Tiles[cy][cx] = TileAltar

		case RoomBoss:
			// Boss rooms are larger, add some lava
			for i := 0; i < 3; i++ {
				lx := room.X + 1 + rng.Intn(room.Width-2)
				ly := room.Y + 1 + rng.Intn(room.Height-2)
				if d.Tiles[ly][lx] == TileFloor {
					d.Tiles[ly][lx] = TileLava
				}
			}
		}
	}
}

func (d *Dungeon) addWalls() {
	// Add walls around all floor tiles
	for y := 0; y < d.Height; y++ {
		for x := 0; x < d.Width; x++ {
			if d.Tiles[y][x] != TileVoid {
				continue
			}

			// Check adjacent tiles
			hasFloorNeighbor := false
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					ny, nx := y+dy, x+dx
					if ny >= 0 && ny < d.Height && nx >= 0 && nx < d.Width {
						tile := d.Tiles[ny][nx]
						if tile == TileFloor || tile == TileDoor || tile == TileLockedDoor ||
							tile == TileStairsDown || tile == TileStairsUp ||
							tile == TileChest || tile == TileAltar {
							hasFloorNeighbor = true
							break
						}
					}
				}
				if hasFloorNeighbor {
					break
				}
			}

			if hasFloorNeighbor {
				d.Tiles[y][x] = TileWall
			}
		}
	}
}

// PlaceLockedDoorBlockingExit places a locked door on the path to the exit room
// with the given probability (0.0-1.0). This ensures the player needs a key to progress.
func (d *Dungeon) PlaceLockedDoorBlockingExit(rng *rand.Rand, probability float64) bool {
	if rng.Float64() >= probability {
		return false
	}

	// Find the path from spawn to exit using A*
	path := d.findPathAStar(d.SpawnX, d.SpawnY, d.ExitX, d.ExitY)
	if len(path) < 3 {
		return false // Path too short or doesn't exist
	}

	// Find a good position for a locked door - preferably in a corridor
	// (a tile with void/walls on opposite sides)
	var doorCandidates [][2]int
	for i := 1; i < len(path)-1; i++ { // Skip first and last (spawn and exit)
		x, y := path[i][0], path[i][1]
		tile := d.Tiles[y][x]

		// Only consider floor tiles
		if tile != TileFloor {
			continue
		}

		// Check if this is a corridor (walls/void on opposite sides)
		horizWalls := d.isBlockingTile(x, y-1) && d.isBlockingTile(x, y+1)
		vertWalls := d.isBlockingTile(x-1, y) && d.isBlockingTile(x+1, y)

		if horizWalls || vertWalls {
			doorCandidates = append(doorCandidates, path[i])
		}
	}

	// If no corridor positions found, use any position in the middle third of the path
	if len(doorCandidates) == 0 {
		startIdx := len(path) / 3
		endIdx := 2 * len(path) / 3
		if endIdx <= startIdx {
			endIdx = startIdx + 1
		}
		for i := startIdx; i < endIdx && i < len(path); i++ {
			if d.Tiles[path[i][1]][path[i][0]] == TileFloor {
				doorCandidates = append(doorCandidates, path[i])
			}
		}
	}

	if len(doorCandidates) == 0 {
		return false
	}

	// Pick a position (prefer one closer to the exit for more exploration before key)
	idx := len(doorCandidates) * 2 / 3 // Pick one in the later portion
	if idx >= len(doorCandidates) {
		idx = len(doorCandidates) - 1
	}
	pos := doorCandidates[idx]

	// Place the locked door
	d.Tiles[pos[1]][pos[0]] = TileLockedDoor
	return true
}

// findPathAStar finds a path from start to end using A* algorithm.
// A* is more efficient than BFS because it uses a heuristic to prioritize
// exploring nodes closer to the goal.
func (d *Dungeon) findPathAStar(sx, sy, ex, ey int) [][2]int {
	if sx == ex && sy == ey {
		return [][2]int{{sx, sy}}
	}

	// Priority queue node
	type astarNode struct {
		x, y   int
		g      int // Cost from start
		f      int // g + heuristic (estimated total cost)
		parent *astarNode
	}

	// Manhattan distance heuristic
	heuristic := func(x, y int) int {
		dx := x - ex
		if dx < 0 {
			dx = -dx
		}
		dy := y - ey
		if dy < 0 {
			dy = -dy
		}
		return dx + dy
	}

	// Open set as a simple slice (we'll find min manually for simplicity)
	// For larger maps, a proper heap would be more efficient
	open := []*astarNode{{x: sx, y: sy, g: 0, f: heuristic(sx, sy), parent: nil}}
	closed := make(map[[2]int]bool)

	dirs := [][2]int{{0, 1}, {0, -1}, {1, 0}, {-1, 0}}

	for len(open) > 0 {
		// Find node with lowest f score
		minIdx := 0
		for i := 1; i < len(open); i++ {
			if open[i].f < open[minIdx].f {
				minIdx = i
			}
		}
		curr := open[minIdx]
		// Remove from open set
		open = append(open[:minIdx], open[minIdx+1:]...)

		// Check if we reached the goal
		if curr.x == ex && curr.y == ey {
			// Reconstruct path
			var path [][2]int
			for n := curr; n != nil; n = n.parent {
				path = append([][2]int{{n.x, n.y}}, path...)
			}
			return path
		}

		closed[[2]int{curr.x, curr.y}] = true

		for _, dir := range dirs {
			nx, ny := curr.x+dir[0], curr.y+dir[1]
			key := [2]int{nx, ny}

			if closed[key] {
				continue
			}
			if nx < 0 || ny < 0 || nx >= d.Width || ny >= d.Height {
				continue
			}

			tile := d.Tiles[ny][nx]
			// Can traverse floor, door, stairs, chest, altar
			if tile != TileFloor && tile != TileDoor && tile != TileStairsDown &&
				tile != TileStairsUp && tile != TileChest && tile != TileAltar {
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

	return nil // No path found
}

// findPathBFS finds a path from start to end using BFS
// Deprecated: Use findPathAStar for better performance
func (d *Dungeon) findPathBFS(sx, sy, ex, ey int) [][2]int {
	if sx == ex && sy == ey {
		return [][2]int{{sx, sy}}
	}

	visited := make(map[[2]int]bool)
	parent := make(map[[2]int][2]int)
	queue := [][2]int{{sx, sy}}
	visited[[2]int{sx, sy}] = true

	dirs := [][2]int{{0, 1}, {0, -1}, {1, 0}, {-1, 0}}

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		if curr[0] == ex && curr[1] == ey {
			// Reconstruct path
			var path [][2]int
			pos := curr
			for pos[0] != sx || pos[1] != sy {
				path = append([][2]int{pos}, path...)
				pos = parent[pos]
			}
			path = append([][2]int{{sx, sy}}, path...)
			return path
		}

		for _, dir := range dirs {
			nx, ny := curr[0]+dir[0], curr[1]+dir[1]
			next := [2]int{nx, ny}

			if visited[next] {
				continue
			}
			if nx < 0 || ny < 0 || nx >= d.Width || ny >= d.Height {
				continue
			}

			tile := d.Tiles[ny][nx]
			// Can traverse floor, door, stairs, chest, altar
			if tile == TileFloor || tile == TileDoor || tile == TileStairsDown ||
				tile == TileStairsUp || tile == TileChest || tile == TileAltar {
				visited[next] = true
				parent[next] = curr
				queue = append(queue, next)
			}
		}
	}

	return nil // No path found
}

// isBlockingTile returns true if the tile blocks movement (wall or void)
func (d *Dungeon) isBlockingTile(x, y int) bool {
	if x < 0 || y < 0 || x >= d.Width || y >= d.Height {
		return true
	}
	tile := d.Tiles[y][x]
	return tile == TileVoid || tile == TileWall
}

// GetRoomAt returns the room containing a position, or nil
func (d *Dungeon) GetRoomAt(x, y int) *Room {
	for _, room := range d.Rooms {
		if x >= room.X && x < room.X+room.Width &&
			y >= room.Y && y < room.Y+room.Height {
			return room
		}
	}
	return nil
}

// ToASCII renders the dungeon as ASCII art
func (d *Dungeon) ToASCII() string {
	result := ""
	for y := 0; y < d.Height; y++ {
		for x := 0; x < d.Width; x++ {
			result += string(TileToRune(d.Tiles[y][x]))
		}
		result += "\n"
	}
	return result
}

// TileToRune converts a tile to its ASCII representation
func TileToRune(t TileType) rune {
	switch t {
	case TileVoid:
		return ' '
	case TileFloor:
		return '.'
	case TileWall:
		return '#'
	case TileDoor:
		return '+'
	case TileLockedDoor:
		return 'L'
	case TileStairsDown:
		return '>'
	case TileStairsUp:
		return '<'
	case TileWater:
		return '~'
	case TileLava:
		return '!'
	case TileChest:
		return '$'
	case TileAltar:
		return '_'
	default:
		return '?'
	}
}

// TileToName returns the tile name
func TileToName(t TileType) string {
	names := []string{
		"void", "floor", "wall", "door", "locked_door",
		"stairs_down", "stairs_up", "water", "lava", "chest", "altar",
	}
	if int(t) < len(names) {
		return names[t]
	}
	return fmt.Sprintf("unknown_%d", t)
}
