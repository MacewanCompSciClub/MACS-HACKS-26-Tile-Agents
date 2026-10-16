"""
MaCS tournament client.

Install:  pip install websockets
Run:      python client.py
"""
import asyncio
import json
import random # you can remove this later
import zlib
import websockets

from world import Vec2, World

HOST = "hack.macewancsclub.ca"
PORT = 8080
TOKEN = "9f3a7c1e"  # the token your team was given

URL = f"ws://{HOST}:{PORT}/ws?token={TOKEN}"

# Message types (from the server's MessageType constants)
MSG_ROUND_START = 1
MSG_WORLD_STATE = 2
MSG_DELTA_STATE = 3
MSG_ROUND_END = 4
MSG_SPECTATOR_LOG = 10

# The docs show string names in the JSON examples, so accept those too.
NAME_TO_ID = {
    "round_start": MSG_ROUND_START,
    "world_state": MSG_WORLD_STATE,
    "delta_state": MSG_DELTA_STATE,
    "round_over": MSG_ROUND_END,
    "round_end": MSG_ROUND_END,
}


def choose_move(world: World) -> Vec2:
    """
    YOUR BOT LOGIC GOES HERE. Return the tile you want to move to.

    This starter just steps onto the neighboring tile at random
    """
    me = world.get_my_position()
    neighbours = world.get_neighbors(me.x, me.y)
    return random.choice(neighbours)

def make_move_message(tick: int, target: Vec2) -> str:
    """
    just turns the sh*t into json (I hate that guy)
    """
    return json.dumps({"tick": tick, "move_x": target.x, "move_y": target.y})


async def main():
    world = None
    width = height = 0

    async with websockets.connect(URL) as ws:
        print("Connected")
        async for message in ws:

            # ---- binary: full world snapshot ----
            if isinstance(message, bytes):
                if message and message[0] == MSG_WORLD_STATE:
                    world = World.from_snapshot(message, width, height)
                    print(world)
                else:
                    print("Unknown binary message, first byte:", message[:1])
                continue

            # ---- text: JSON ----
            data = json.loads(message)
            mtype = data.get("message_type")
            if isinstance(mtype, str):
                mtype = NAME_TO_ID.get(mtype)

            if mtype == MSG_ROUND_START:
                width = data["world_size_x"]
                height = data["world_size_y"]
                # Start with a blank world; a snapshot or deltas will fill it in.
                world = World.empty(width, height)
                print(f"Round start: {width}x{height}, {data['num_players']} players, "
                      f"{data['response_time_ms']}ms per move")

            elif mtype == MSG_DELTA_STATE:
                if world is None:
                    continue
                world.tick = data["current_tick"]
                world.local_player = Vec2(data["local_player_x"], data["local_player_y"])

                # Go sends an empty slice as null, so fall back to []
                for delta in data.get("delta_updates") or []:
                    pos = delta["pos"]  # Go's Vec2 has no json tags, so keys are "X" and "Y"
                    world.apply_delta(pos[0], pos[1], delta["tile_value"])


                # crc32ieee
                checksum = zlib.crc32(world.data)
                if checksum != data["world_hash"]:
                    # Crash out
                    print("We are crashing the F out")
                    await ws.close(code=1002, reason="world hash mismatch")
                    break
                    

                target = choose_move(world)
                await ws.send(make_move_message(world.tick, target))

            elif mtype == MSG_ROUND_END:
                print(f"Round over: score={data.get('score')} placement={data.get('placement')}")
                await ws.close(code=1000)

            else:
                print("Unhandled message:", data)
                await ws.close(code=1002, reason="unknown message type")


if __name__ == "__main__":
    asyncio.run(main())