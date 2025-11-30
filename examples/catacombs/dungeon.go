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
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	RoomCount   int     `json:"room_count"`
	MinRoomSize int     `json:"min_room_size"`
	MaxRoomSize int     `json:"max_room_size"`
	EnemyDensity float64 `json:"enemy_density"` // 0.0-1.0
	LootDensity  float64 `json:"loot_density"`  // 0.0-1.0
	NPCCount    int     `json:"npc_count"`
	Seed        int64   `json:"seed"`
	Difficulty  int     `json:"difficulty"` // 1-10
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
