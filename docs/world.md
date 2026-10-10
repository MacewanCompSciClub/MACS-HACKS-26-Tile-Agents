# World

The game world is represented by a standard 2D grid, oriented with (0,0) in the bottom left. The Y-Axis is positive in the up direction, and X-Axis is positive to the right. Coordinates begin at zero, such that a world that is 32x32 will have coordinates ranging from \[0, 31].

In memory the world is represented as an array of unsigned 8 bit integers.

GO:
```go
type WorldState struct {
	width, height uint32 
	playerX, playerY uint32
	currentTick uint32
	tiles []uint8
}
```
Python:
```python
class WorldState:
    def __init__(self, width: int, height:int,
                 player_x:int , player_y:int,
                 current_tick:int , tiles: bytearray):
        self.width = width
        self.height = height
        ...
        self.tiles = tiles 
```

| **Property** | **Type** | **Description** |
| --------------- | ------- | ------------------------------------- |
| **height**      | integer | The number of rows in the y-axis of the world.
| **width**       | integer | The number of columns in the x-axis of the world.
| **currentTick** | integer | Represents the units of time that have passed. Your score increases and touched tiles decay each tick.
| **tiles**       | array   | Array of integers each representing one tile in the world.|

In the client templates for each langauge we provide functions to:
- Check the health of a tile at a given position.
- Check if there is a player on a tile at a given position.
