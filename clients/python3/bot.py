import random

from world import Vec2, World


def choose_move(world: World) -> Vec2:
    """Choose the next tile to move to."""
    me = world.get_my_position()
    neighbours = world.get_neighbors(me.x, me.y)
    return random.choice(neighbours)