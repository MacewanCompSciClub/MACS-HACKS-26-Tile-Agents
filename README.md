# MACS HACKS GUIDED CHALLENGED

## Objective

Be the last player alive by moving strategically across the game board and avoiding tiles that collapse. Survive longer than your opponents to achieve a higher score and win the round.

## What you need

- Go (the programming language) version 1.27.1 https://go.dev/doc/install
- A programming language with a websocket library (We have sample clients written in python and go)
- We will each team a token to sign into the server with.

## How to Fork and Clone a GitHub Repository

### 1. Fork the repository

A fork creates a copy of someone else's repository under your own GitHub account.

1. Click Fork in the top-right corner.
2. Select your GitHub account as the owner.
3. Click Create fork.

You now have your own copy of the repository on GitHub.

### 2. Clone your fork

Cloning downloads the repository to your computer so you can edit and run the code locally.

1. Open your fork on GitHub.
2. Click the green Code button.
3. Copy the HTTPS URL.
4. Open a terminal and navigate to the directory where you want the project saved.
5. Run:

bash
```
git clone https://github.com/YOUR-USERNAME/REPOSITORY-NAME.git
```

Replace `YOUR-USERNAME` and `REPOSITORY-NAME` with the appropriate values.

### 3. Enter the project directory

bash
```
cd REPOSITORY-NAME
```

You can now edit the files on your computer.

### Authentication

Each team will be provided with a unique authentication token. To connect to the server, include your token as a query parameter in the WebSocket URL:

```
ws://SERVER_HOST:8080/ws?token=YOUR_TOKEN
```

Replace `YOUR_TOKEN` with the token assigned to your team. Connections with invalid tokens will be rejected.

## Game Rules


### Movement

- Players move around a grid-based game board.
- On each turn, players choose a target tile to move to.
- Moves must follow the movement rules enforced by the server.
- **Jumping over a tile is allowed if and only if that tile's health is 0.** Tiles with health greater than 0 cannot be jumped over.
- Plan your movements carefully to avoid getting trapped or landing on dangerous tiles.

### Tile Health

- Each tile has a health value that determines how long it remains safe.
- Tiles start to degrade as soon as a player comes in contact and will continue to degrade until there is no tile.
- A tile with health 0 is dead and cannot safely support a player.

### Collisions

- If two or more players occupy the same tile after moving, they are eliminated.
- Avoid moving onto a tile that another player is likely to occupy.

### Elimination

- A player is eliminated if they occupy a tile with health 0.
- Players who are eliminated cannot continue moving during that round.

### Scoring and Winning

- Players gain points for each tick they remain alive.
- The round ends when all players have been eliminated.
- Survive as long as possible to maximize your score.



