package main

import (
	"fmt"
<<<<<<< HEAD
	"hash/crc32"
=======
>>>>>>> isaac
)

type WorldState struct {
	w, h int32 
<<<<<<< HEAD
	tiles []uint8
	snapshotBufs[2][]uint8
	front int; // buffer to write to for current tick
=======
	cells []uint8
	updated map[Vec2]struct{}
}

func initWorld(size Vec2, tileHealth uint8, players []PlayerInstance) * WorldState {

	if tileHealth > 7 { panic(fmt.Sprintf("tileHealth must be less than 7! Got %d", tileHealth)) }
	if size.X < 8 || size.Y < 8 { panic(fmt.Sprintf("World size must be atleast 8x8! Got %+v", size))}

	w, h := int32(size.X), int32(size.Y)
	g := &WorldState{w: w, h: h, cells: make([]uint8, w*h), updated: make(map[Vec2]struct{}, 256)}
	for i := range g.cells {
		g.cells[i] = tileHealth;
	}

	for i := range players {
		p := &players[i]
		// Mark this tile as having player on it.
		g.cells[g.tileIndex(p.pos)] = tileHealth - 1
		g.cells[g.tileIndex(p.pos)] |= 0b1000
	}
	return g
}

func (world * WorldState) positionOutOfBounds(position Vec2) bool {
	return position.X < 0 || position.X >= world.w || position.Y < 0 || position.Y >= world.h
>>>>>>> isaac
}

func (world * WorldState) tileIndex(position Vec2) int32 {
	if position.X < 0 || position.X >= world.w || position.Y < 0 || position.Y >= world.h {
		panic(fmt.Sprintf("position out of bounds: (%d, %d) in %dx%d grid",
			position.X, position.Y, world.w, world.h))
	}
	return position.Y * world.w + position.X
}

func (world * WorldState) tilePosition(index int32) Vec2 {
<<<<<<< HEAD
	if index < 0 || index >= int32(len(world.tiles)) {
		panic(fmt.Sprintf("cell index out of bounds: %d in grid of %d cells", index, len(world.tiles)))
=======
	if index < 0 || index >= int32(len(world.cells)) {
		panic(fmt.Sprintf("cell index out of bounds: %d in grid of %d cells", index, len(world.cells)))
>>>>>>> isaac
	}

	return Vec2{X: index % world.w, Y: index / world.w}
}

<<<<<<< HEAD
func (world * WorldState) buildSnapshot(players []PlayerInstance) {
	buf := world.snapshotBufs[world.front]
	if len(buf) != len(world.tiles) {
		panic(fmt.Sprintf("snapshot buffer size mismatch: buf=%d tiles=%d", len(buf), len(world.tiles)))
	}
	copy(buf, world.tiles)

	for i := range players {
		p := &players[i]
		if !p.dead { buf[world.tileIndex(p.pos)] |= 0b1000 }
	}

	world.front = (world.front + 1) % 2
}


func (world *WorldState) latestSnapshot() []uint8 {
	return world.snapshotBufs[world.front^1]
}

func (world * WorldState) getSnapshotHash() uint32 {
	return crc32.ChecksumIEEE(world.latestSnapshot())
}

func initWorld(size Vec2, tileHealth uint8, players []PlayerInstance) * WorldState {
	if tileHealth > 7 { panic(fmt.Sprintf("tileHealth must be less than 7! Got %d", tileHealth)) }
	if size.X < 8 || size.Y < 8 { panic(fmt.Sprintf("World size must be atleast 8x8! Got %+v", size))}

	w, h := int32(size.X), int32(size.Y)
	n := w * h
	world := &WorldState{w: w,
						 h: h,
						 tiles: make([]uint8, n),
						 front: 0,
	 }

	world.snapshotBufs[0] = make([]uint8, n)
	world.snapshotBufs[1] = make([]uint8, n)

	for i := range world.tiles {
		world.tiles[i] = tileHealth;
	}

	for i := range players {
		p := &players[i]
		// Mark this tile as having player on it.
		world.tiles[world.tileIndex(p.pos)] = tileHealth - 1
	}
	world.buildSnapshot(players)

	return world
}

func (world * WorldState) positionOutOfBounds(position Vec2) bool {
	return position.X < 0 || position.X >= world.w || position.Y < 0 || position.Y >= world.h
}

func abs(n int32) int32 {
	if n < 0 {
		return -n
	}
	return n
}

func sign(n int32) int32 {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
=======
func (world * WorldState) updateTile(pos Vec2, newValue uint8) {
	world.updated[Vec2{X: pos.X, Y: pos.Y}] = struct{}{}
	world.cells[world.tileIndex(pos)] = newValue;
}

func (world * WorldState) isValidMovePos(pos Vec2) bool {
	if (world.positionOutOfBounds(pos)) { return false }
	if (getTileHealth(world.cells[world.tileIndex(pos)]) == 0) { return false; }
	if (isPlayerOnTile(world.cells[world.tileIndex(pos)]) == true) { return false; }
	return true;
>>>>>>> isaac
}

func makeTile(health uint8, playerOnTile bool) uint8 {
	tile := health & 0b0111;
	if (playerOnTile) { tile |= 0b1000; }
	return tile;
}

func isPlayerOnTile(tile uint8) bool {
	return (tile & 0b1000 != 0)
}

func getTileHealth(tile uint8) uint8 {
	return (tile & 0b00000111)
}
<<<<<<< HEAD

func (world * WorldState) isValidMovePos(from Vec2, target Vec2) bool {
	if (world.positionOutOfBounds(target)) { return false }

	// Staying in place is fine
	if from == target { return true }

	dx := abs(from.X - target.X)
	dy := abs(from.Y - target.Y)

	// Diagonal moves are not allowed
	if (dx == 0) == (dy == 0) { return false }

	switch dx + dy {
		case 1:
			return true
		case 2:
			mid := Vec2{
				X: from.X + sign(target.X-from.X),
				Y: from.Y + sign(target.Y-from.Y),
			}
			return getTileHealth(world.tiles[world.tileIndex(mid)]) == 0
		default:
			return false
	}
}
=======
>>>>>>> isaac
