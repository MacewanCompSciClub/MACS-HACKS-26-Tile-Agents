package main

import (
	"fmt"
)

type WorldState struct {
	w, h int32 
	cells []uint8
	updated map[Vec2]struct{}
}

func initWorld(size Vec2, tileHealth uint8, players []PlayerInstance) * WorldState {

	if tileHealth > 7 { panic(fmt.Sprintf("tileHealth must be less than 7! Got %d", tileHealth)) }
	if size.x < 8 || size.y < 8 { panic(fmt.Sprintf("World size must be atleast 8x8! Got %+v", size))}

	w, h := int32(size.x), int32(size.y)
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

func (world * WorldState) tileIndex(position Vec2) int32 {
	if position.x < 0 || position.x >= world.w || position.y < 0 || position.y >= world.h {
		panic(fmt.Sprintf("position out of bounds: (%d, %d) in %dx%d grid",
			position.x, position.y, world.w, world.h))
	}
	return position.y * world.w + position.x
}

func (world * WorldState) tilePosition(index int32) Vec2 {
	if index < 0 || index >= int32(len(world.cells)) {
		panic(fmt.Sprintf("cell index out of bounds: %d in grid of %d cells", index, len(world.cells)))
	}

	return Vec2{x: index % world.w, y: index / world.w}
}

func (world * WorldState) updateTile(pos Vec2, newValue uint8) {
	world.updated[Vec2{x: pos.x, y: pos.y}] = struct{}{}
	world.cells[world.tileIndex(pos)] = newValue;
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
