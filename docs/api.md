# HTTP API

All bodies are JSON. Colors are numbers everywhere: `0` empty, `1` black, `2` white.
Points are `{x, y}` with the origin at the **top-left** (`x` = column, `y` = row).
Errors are `{"error": "message"}` with a meaningful status:

| Status | Meaning |
|---|---|
| 400 | Bad request (also: unreadable SGF, bad rules) |
| 401 | Not signed in |
| 403 | Signed in but not allowed (not your game, AI help is off) |
| 404 | No such game / node |
| 409 | Not allowed right now (not your turn, game not in progress, mode forbids it, KataGo down) |
| 422 | Illegal move: occupied, suicide, ko, off the board |

Sign-in is a cookie (`eg_session`, HttpOnly). Everything that creates or changes a game needs it.
Reading a game by id and watching its event stream do not.

## Session

| Call | Notes |
|---|---|
| `GET /api/config` | `{presets:[{name,rules}], sizes:[9,13,19], katago:KataGoStatus, joseki:JosekiInfo, googleAuth:bool, devLogin:bool}` |
| `GET /api/me` | `{user: User \| null}`; `User = {id,name,email?,picture?,rank,location,provider}` |
| `PUT /api/me` | `{name, rank, location}` -> `{user}` |
| `POST /api/auth/dev-login` | `{name}` -> `User`. Only when `devLogin` is true |
| `GET /auth/google/login` | Browser navigation (redirects to Google). Only when `googleAuth` is true |
| `POST /api/auth/logout` | 204 |

## Games

### `POST /api/games` -> 201 `GameView`

```json
{
  "name": "Friday game",
  "mode": "live | teaching | joseki",
  "size": 19,
  "rules": "japanese",
  "komi": 6.5,
  "handicap": 0,
  "opponent": "local | ai | open",
  "color": "black | white | random",
  "aiVisits": 200,
  "clock": {"enabled": true, "mainSeconds": 600, "periods": 5, "periodSeconds": 30}
}
```

* `rules` is a preset name from `/api/config` or a full rules object (below). `komi` overrides it.
* `opponent` only applies to `live`. `local` = one person plays both sides (hot-seat),
  `ai` = KataGo (409 if it is not running), `open` = an empty seat someone else joins.
* `teaching` and `joseki` games are driven by their owner: free navigation, branching, annotations.
* `clock.enabled=false` (or both times zero) means an untimed game.

Rules object (mirrors KataGo's): `{ko: SIMPLE|POSITIONAL|SITUATIONAL, scoring: AREA|TERRITORY,
tax: NONE|SEKI|ALL, suicide: bool, hasButton: bool, whiteHandicapBonus: "0"|"N-1"|"N", komi: number}`.

### Other calls

| Call | Body | Notes |
|---|---|---|
| `GET /api/games` | | `{mine:[GameSummary], open:[GameSummary]}`; `open` are games waiting for a second player |
| `GET /api/games/{id}` | | `GameView` for the caller |
| `GET /api/games/{id}/events` | | **SSE.** `event: state`, `data: GameView` (per viewer). Sent immediately, on every change, and every 10 s |
| `POST /api/games/import` | `{sgf, mode?, name?}` | Opens an SGF as a `teaching` (default) or `joseki` game. 9/13/19 only |
| `POST /api/joseki/example` | | Opens the downloaded dictionary as a `joseki` game (409 if not downloaded) |
| `POST /api/games/{id}/join` | `{color?: "black"\|"white"}` | Take an empty seat |
| `POST /api/games/{id}/move` | `{x,y}` or `{pass:true}`; `color?` in teaching/joseki | |
| `POST /api/games/{id}/resign` | | live only |
| `POST /api/games/{id}/undo` | | teaching/joseki; live only against the computer or in a local game. Against the computer it also takes back the reply |
| `POST /api/games/{id}/goto` | `{node}` | teaching/joseki only (live games are never navigated server-side; browse client-side) |
| `POST /api/games/{id}/nodes/{node}/annotate` | `{comment?, name?, judgment?, marks?}` | `judgment`: `""`,`correct`,`playable`,`mistake`. `marks`: `[{x,y,kind,text?}]`, `kind`: `label`,`triangle`,`square`,`circle`,`cross`. Replaces the node's marks |
| `DELETE /api/games/{id}/nodes/{node}` | | delete a branch |
| `POST /api/games/{id}/nodes/{node}/promote` | | move a branch toward the main line |
| `POST /api/games/{id}/dead` | `{x,y,dead}` | Scoring phase: mark the group at (x,y) dead or alive. Cancels earlier acceptance |
| `POST /api/games/{id}/accept` | | Scoring phase: accept the score. Both players must; local games and the computer agree automatically |
| `POST /api/games/{id}/assist` | `{on}` | Vote for AI help. In human-vs-human games it needs both players |
| `GET /api/games/{id}/sgf` | | download |
| `DELETE /api/games/{id}` | | owner only; 204 |

Every mutating call returns the caller's `GameView`.

## GameView

```
id, name, mode, status, result, size, rules, handicap, version
status:   waiting (empty seat) | playing | scoring (two passes) | finished
toPlay:   1 | 2          moveNumber, cursor (node id)
board:    int[size*size]  row-major from the top; 0/1/2
liberties:int[size*size]  liberty count of the group on each stone, 0 on empty points
captures: [black's captures, white's captures]
lastMove: {x,y} | absent
nodes:    [{id, parent, color, pass?, x, y, comment?, name?, judgment?, marks?:[{x,y,kind,text?}], children:[id]}]
mainLine: [id]            first-child chain from the root
partial:  true when nodes is only a window of a huge tree (the joseki dictionary): follow
          `children` ids that are missing from nodes by calling /goto to them
players:  [black, white]  each {name, rank, location, kind: human|ai|open, you}
clock:    [black, white]  each {enabled, mainMs, periodMs, periods, inByo, flagged, running, byoConfigured, config}
you:      {seat: 0 watching | 1 | 2 | 3 both (local), canMove, owner}
territory:int[size*size]  settled-position territory: 0 neutral, 1 black, 2 white (dead stones removed)
dead:     [{x,y}]         stones currently judged/marked dead
score:    {black, white, margin, blackTerritory, whiteTerritory}  margin > 0 = Black ahead
accepted: [black, white]  scoring acceptance
aiAssist: bool
```

The clock values are a snapshot. Count down locally for the `running` side between events.
`inByo` sides count `periodMs` down; otherwise `mainMs`. Flagging is decided by the server only.

In a **live** game the server cursor is always the latest move; browse history client-side
from `nodes`/`mainLine` (every position can be reconstructed by replaying moves, or just show
the move list and jump the *view* without calling `/goto`).

## Analysis (KataGo and other helpers)

`GET /api/games/{id}/analysis?visits=200&node=ID` -> 

```
{available, toPlay, winrate (Black's), scoreLead (Black's), visits,
 suggestions:[{x,y,pass?,winrate,scoreLead,visits,prior,pv:[{x,y}]}],
 ownership:[float; +1 black, -1 white], territory:[0|1|2]  (contested/live territory),
 groups:[{stones:[{x,y}], color, status: alive|dead|seki|unsettled, strength: strong|moderate|weak, own}]}
```
`available:false` when KataGo is not running. 403 when AI help is off for this game or the
caller is a spectator of a game in progress.

`GET /api/games/{id}/shapes` -> `{findings:[{kind, result?, color, move?, points, breaker?, confidence, text}]}`
`kind`: `ladder`, `net`, `snapback`, `knight`, `large-knight`. `result`: `works` | `broken`.
`confidence`: `exact` or `bounded` (net/snapback are depth-limited; ko and seki can change them).

`GET /api/games/{id}/joseki` -> `{loaded, corners:[{corner: tl|tr|bl|br, status: joseki|deviated,
stones, comment?, next:[{color,x,y,comment?}], deviatedAt?, swapped?}]}`.
Only reported on 13x13 and 19x19, and once a corner has 3 or more stones.

## Settings

| Call | Notes |
|---|---|
| `GET /api/katago` | `{available, starting, version?, error?, config:{binary,model,config}}` |
| `PUT /api/katago` | `{binary, model, config}`: saves and restarts KataGo; 202 |
| `GET /api/joseki` | `{loaded, positions, source, error?}` |
| `POST /api/joseki/fetch` | Downloads Kogo's Joseki Dictionary and rebuilds the index |

## Fair play

Computer help is cheating between two people. In a live game between two networked humans,
`/analysis`, `/shapes` and `/joseki` return 403 until **both** players have voted for AI help
(`POST /assist`), and always for spectators while the game is in progress. Teaching, joseki,
hot-seat, versus-KataGo and finished games are not restricted.
