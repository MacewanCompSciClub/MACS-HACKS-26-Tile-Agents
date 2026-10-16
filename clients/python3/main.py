"""
MaCS tournament client.

Install: pip install websockets
Run:     python client.py
"""

import asyncio
import json
import random
import zlib

import websockets

from world import Vec2, World
from bot import choose_move


HOST = "hack.macewancsclub.ca"
PORT = 8080
TOKEN = "9f3a7c1e"

URL = f"ws://{HOST}:{PORT}/ws?token={TOKEN}"
LOCALURL = f"ws://localhost:{PORT}/ws?token={TOKEN}"

MSG_ROUND_START = 1
MSG_WORLD_STATE = 2
MSG_DELTA_STATE = 3
MSG_ROUND_END = 4
MSG_SPECTATOR_LOG = 10

NAME_TO_ID = {
    "round_start": MSG_ROUND_START,
    "world_state": MSG_WORLD_STATE,
    "delta_state": MSG_DELTA_STATE,
    "round_over": MSG_ROUND_END,
    "round_end": MSG_ROUND_END,
}



def make_move_message(tick: int, target: Vec2) -> str:
    """Serialize a move into JSON."""
    return json.dumps({
        "tick": tick,
        "move_x": target.x,
        "move_y": target.y,
    })


def handle_world_snapshot(message: bytes, width: int, height: int) -> World:
    """Decode a binary world snapshot."""
    world = World.from_snapshot(message, width, height)
    print(world)
    return world


def handle_round_start(data: dict) -> tuple[World, int, int]:
    """Initialize state at the beginning of a round."""
    width = data["world_size_x"]
    height = data["world_size_y"]
    world = World.empty(width, height)

    print(
        f"Round start: {width}x{height}, "
        f"{data['num_players']} players, "
        f"{data['response_time_ms']}ms per move"
    )

    return world, width, height


async def handle_delta_state(ws, world: World, data: dict) -> bool:
    """Apply a world update and send the next move.

    Returns False if the world hash does not match.
    """
    world.tick = data["current_tick"]
    world.local_player = Vec2(
        data["local_player_x"],
        data["local_player_y"],
    )

    for delta in data.get("delta_updates") or []:
        pos = delta["pos"]
        world.apply_delta(pos[0], pos[1], delta["tile_value"])

    actual_hash = zlib.crc32(world.data)
    if actual_hash != data["world_hash"]:
        print("World hash mismatch; closing connection.")
        await ws.close(code=1002, reason="world hash mismatch")
        return False

    target = choose_move(world)
    await ws.send(make_move_message(world.tick, target))
    return True


def handle_round_end(data: dict) -> None:
    """Print the final round results."""
    print(
        f"Round over: score={data.get('score')} "
        f"placement={data.get('placement')}"
    )


def decode_message(message: str) -> dict:
    """Parse JSON and normalize string message types."""
    data = json.loads(message)
    mtype = data.get("message_type")

    if isinstance(mtype, str):
        data["message_type"] = NAME_TO_ID.get(mtype)

    return data


async def main():
    world = None
    width = height = 0

    async with websockets.connect(URL) as ws:
        print("Connected")

        async for message in ws:
            # Binary messages: full world snapshots.
            if isinstance(message, bytes):
                if message and message[0] == MSG_WORLD_STATE:
                    world = handle_world_snapshot(message, width, height)
                else:
                    print("Unknown binary message:", message[:1])
                continue

            # Text messages: JSON protocol.
            data = decode_message(message)
            mtype = data.get("message_type")

            if mtype == MSG_ROUND_START:
                world, width, height = handle_round_start(data)

            elif mtype == MSG_DELTA_STATE:
                if world is None:
                    continue

                should_continue = await handle_delta_state(ws, world, data)
                if not should_continue:
                    break

            elif mtype == MSG_ROUND_END:
                handle_round_end(data)

            else:
                print("Unhandled message:", data)
                await ws.close(code=1002, reason="unknown message type")
                break


if __name__ == "__main__":
    asyncio.run(main())