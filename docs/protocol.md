The game uses websockets to connect to the server. For your connection request to be accepted your client must connect using a valid token.

## Connection Tokens
Connections tokens are how the server knows who is connected and playing.

The server has a list of valid tokens.
Multiple clients can't connect at the same time with the same token.
To connect to the MaCS server for the tournament your team should have been given a token.

```json
// server/allowed_connections.json
{
  "allowed_connections": {
    "crimson_wolves": "9f3a7c1e",
    "shadow_vipers":  "b7e4c1a9"
  },
  "spectator_token": "FFFFFFFF",
  "master_spectator_token": "e3a1f9c7"
}
```
When running the server locally you can edit this file and add more tokens as you like so you can test against multiple clients!

The client sends the token when it makes a connection request to the server like this:
`ws://localhost:8080/ws?token=9f3a7c1e` 

### Round Start Message Format
```json
{
  "message_type": "round_start",
  "world_size_x": 64,
  "world_size_y": 64,
  "num_players": 4,
  "response_time_ms": 500
}
```
| **Property** | **Type** | **Description** |
| --------------- | ------- | ------------------------------------- |
| **message_type**  | integer | The type of message.
| **world_size_x**  | integer | The size of the world on the X axis.
| **world_size_y**  | array   | The size of the world on the Y axis.|
| **reponse_time_ms** | integer | The time (in milliseconds) that the client must respond within to send their move otherwise it will be late.

### World Snapshot Message
Sent to clients when the client dies or is the last one alive in the round.

This is a **binary** WebSocket message.

| **Offset** | **Size (bytes)** | **Type** | **Field** | **Description** |
| ---------- | ---------------- | -------- | --------- | --------------- |
| 0 | 1 | uint8 | `message_type` | The type of message. |
| 1 | 4 | uint32 | `tick` | The game tick the message represents. |
| 5 | h * w | bytes | `world_snapshot` | The serialized world snapshot. |

### Round End Message Format
This is sent to clients when the client dies or is the last one alive in the round.
```json
{
  "message_type": "round_over",
  "score": 850,
  "placement": 2
}
```
| **Property** | **Type** | **Description** |
| --------------- | ------- | ------------------------------------- |
| **message_type**  | integer | The type of message.
| **score** | integer | How long the player survived in the round, measured in game ticks.
| **placement** | integer | The player's final rank for the round, where 1 is first place (the last player surviving). Ranges from 1 to the number of players in the round. |