"""
world.py - a friendly wrapper around the world's tile bytes.

!!! IMPORTANT !!
"""
from typing import List, NamedTuple, Optional

# NOTE: the lowest 3 bits of a tile byte are its health (0-7)
HEALTH_MASK = 0b00000111
# NOTE: the 4th bit of a tile byte is set when a player is standing there
PLAYER_FLAG = 0b00001000


class Vec2(NamedTuple):
    """A simple (x, y) position. Use pos.x / pos.y, or unpack: x, y = pos"""
    x: int
    y: int

    def __add__(self, other: "Vec2") -> "Vec2":
        return Vec2(self.x + other.x, self.y + other.y)

    def __sub__(self, other: "Vec2") -> "Vec2":
        return Vec2(self.x - other.x, self.y - other.y)

    def __mul__(self, scalar: int) -> "Vec2":
        return Vec2(self.x * scalar, self.y * scalar)

    __rmul__ = __mul__  # allows 3 * pos as well as pos * 3

    def __floordiv__(self, scalar: int) -> "Vec2":
        return Vec2(self.x // scalar, self.y // scalar)

    def __neg__(self) -> "Vec2":
        return Vec2(-self.x, -self.y)

    def __str__(self):
        return f"({self.x}, {self.y})"


class World:
    def __init__(self, data: bytes, width: int, height: int,
                 tick: int = 0, local_player: Optional[Vec2] = None):
        if len(data) != width * height:
            raise ValueError(
                f"Expected {width * height} bytes for a {width}x{height} world, got {len(data)}"
            )
        self.data = bytearray(data)   # tile bytes, row by row. Mutable so deltas can update it.
        self.width = width
        self.height = height
        self.tick = tick
        self.local_player = local_player  # YOUR position (from the delta messages)

    # ---------- building / updating a World ----------

    @classmethod
    def empty(cls, width: int, height: int) -> "World":
        """A world where every tile byte is 0. Used until the first snapshot arrives."""
        return cls(bytes(width * height), width, height)

    @classmethod
    def from_snapshot(cls, snapshot: bytes, width: int, height: int,
                      byte_order: str = "little") -> "World":
        """Build from the binary message: 1 byte type + 4 byte tick + width*height tile bytes.
        If the tick looks wrong, try byte_order="big"."""
        tick = int.from_bytes(snapshot[1:5], byte_order)
        return cls(snapshot[5:5 + width * height], width, height, tick)

    def apply_delta(self, x: int, y: int, value: int) -> None:
        """Set one tile's raw byte. Called for every entry in a delta message."""
        self.data[self._index(x, y)] = value

    # ---------- basic helpers ----------

    def in_bounds(self, x: int, y: int) -> bool:
        return 0 <= x < self.width and 0 <= y < self.height

    def _index(self, x: int, y: int) -> int:
        if not self.in_bounds(x, y):
            raise IndexError(f"({x}, {y}) is outside the {self.width}x{self.height} world")
        return y * self.width + x

    def get_raw_tile(self, x: int, y: int) -> int:
        """The raw byte for a tile (0-255). Handy for figuring out the encoding."""
        return self.data[self._index(x, y)]

    # ---------- tile questions ----------

    def get_tile_health(self, x: int, y: int) -> int:
        return self.get_raw_tile(x, y) & HEALTH_MASK

    def is_tile_alive(self, x: int, y: int) -> bool:
        """True if the tile still has health left (health > 0)."""
        return self.get_tile_health(x, y) > 0

    def is_player_on_tile(self, x: int, y: int) -> bool:
        """True if any player (including you) is standing on (x, y)."""
        return bool(self.get_raw_tile(x, y) & PLAYER_FLAG)

    # ---------- players ----------

    def get_player_positions(self) -> List[Vec2]:
        """Every player in the world, including you."""
        return [
            Vec2(x, y)
            for y in range(self.height)
            for x in range(self.width)
            if self.is_player_on_tile(x, y)
        ]

    def get_my_position(self) -> Optional[Vec2]:
        """Where YOU are, as reported by the server."""
        return self.local_player


    # ---------- navigation ----------

    def get_neighbors(self, x: int, y: int) -> List[Vec2]:
        """In-bounds positions next to (x, y): up, down, left, right (plus diagonals if asked)."""
        offsets = [(0, -1), (0, 1), (-1, 0), (1, 0)]
        return [Vec2(x + dx, y + dy) for dx, dy in offsets if self.in_bounds(x + dx, y + dy)]

    @staticmethod
    def distance(a: Vec2, b: Vec2) -> int:
        """Grid distance (steps up/down/left/right) between two positions."""
        return abs(a.x - b.x) + abs(a.y - b.y)

    # ---------- debugging ----------

    def __str__(self) -> str:
        """@ = you, P = other player, digits = tile health (capped at 9), . = no health."""
        lines = []
        for y in range(self.height):
            row = ""
            for x in range(self.width):
                if self.local_player == Vec2(x, y):
                    row += "@"
                elif self.is_player_on_tile(x, y):
                    row += "P"
                else:
                    h = self.get_tile_health(x, y)
                    row += "." if h == 0 else str(min(h, 9))
            lines.append(row)
        return f"World {self.width}x{self.height} at tick {self.tick}\n" + "\n".join(lines)


if __name__ == "__main__":
    demo = bytes([
        5, 5, 5, 0,
        5, 5 | PLAYER_FLAG, 3, 0,
        5, 5, 5, 5 | PLAYER_FLAG,
    ])
    w = World(demo, 4, 3, tick=1, local_player=Vec2(1, 1))
    w.apply_delta(2, 1, 9)
    print(w)
    print("Enemies:", [str(p) for p in w.get_enemy_positions()])
    print("My safe moves:", [str(p) for p in w.get_my_safe_moves()])