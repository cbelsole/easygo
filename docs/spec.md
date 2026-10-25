# Go on Easy Mode — Spec

Source of truth for scope. Resolved questions and overrides are in `decisions.md`.
The Joseki Dojo design doc supplies the Pattern/MoveNode model and the curriculum
framing; where it conflicts with this spec, this spec wins.

## 1. Functional requirements

### Core engine
- **FR-1** Render a Go board
- **FR-2** Stone placement, black first, alternating
- **FR-3** Mode switch: Live Game / Teaching Game / Joseki Example Set
- **FR-4** Board size configurable (decision: 9, 13, 19)

### Toggleable features
- **FR-5** Move tracker with branching paths
- **FR-6** Shadow/ghost moves: left-click places, right-click removes the last shadow move, never touches real game state, drawn at reduced opacity
- **FR-7** Edge coordinates
- **FR-8** Liberty-count overlay
- **FR-9** Territory overlay (resolved territory)
- **FR-10** Annotation tool: numbers, letters, or symbol marks on stones

### Client
- **FR-11** OGS-style panels: player name, rank, location, captures, per-player clock
- **FR-12** Main time + byo-yomi, each independently disableable
- **FR-13** KataGo next-move suggestion
- **FR-14** Live joseki suggester: recognizes corner patterns mid-game
- **FR-15** Shape sensor: ladder, net, snapback, knight's move, large knight's move (extensible registry)
- **FR-16** Life/death judgment per group
- **FR-17** Strong/weak group indicator

## 2. Feasibility tiers

| Tier | Covers | Method |
|---|---|---|
| A | FR-1–10, liberties, ladders | Deterministic engine. A ladder is a forward simulation, not a search. |
| B | Net/geta, snapback | Bounded local capture-race lookahead. Exact only for the local shape; ko and seki can change the answer, so results carry a confidence flag. |
| C | Knight's / large knight's move | Coordinate geometry plus a small template library |
| D | Next move, life/death, contested territory, strength | KataGo |
| E | Joseki suggester | Database lookup; pattern match over the 8 symmetries |

## 3. Architecture

```
/internal
  /board     rules engine: groups, liberties, ko, scoring
  /tree      branching move tree
  /sgf       SGF import/export (hand-rolled)
  /shapes    ladder / net / snapback / knight templates
  /katago    subprocess lifecycle + analysis-engine JSON client
  /joseki    pattern DB + live corner matcher
  /session   mode state + feature toggles
  /clock     main time + byo-yomi state machine
  /server    HTTP + SSE
  /auth      Google sign-in (+ dev stub)
  /store     SQLite
/web         browser UI, embedded into the binary
```

## 4. KataGo

One integration point serves FR-13, FR-16, contested FR-9, and FR-17: winrate,
score lead, per-point ownership and policy from a single analysis query.
Use the JSON analysis engine (`katago analysis`), one long-lived process.
Binary, network and config paths are user-configurable.

## 5. Joseki suggester

Per move: take the corner region, normalize across the 8 symmetries, match
against the pattern DB by position hash (so transpositions match), surface the
match once enough moves agree. No KataGo involved.

## 6. Shape sensor

Registry of independent detectors; adding one touches no trainer or session code.
Ladder: play out the forced sequence on the real board. Net and snapback:
bounded local search. Knight/large knight: geometry plus templates.

## 7. Layout

```
┌───────────────────────────────────────────────────────────────┐
│ ○ White — name rank location        ⏱ clock   ●●● periods     │
├──────────────────────────────────────────┬────────────────────┤
│                                          │ Move N             │
│                 goban                    │ move list+branches │
│                                          │ Libs: B n W n      │
│                                          │ Suggests: ...      │
├──────────────────────────────────────────┴────────────────────┤
│ ● Black — name rank location        ⏱ clock   ●●●●● periods   │
├───────────────────────────────────────────────────────────────┤
│ Pass · Resign · Score │ shadow · libs · territory · joseki · shapes │
└───────────────────────────────────────────────────────────────┘
```

## 8. Toggle matrix

| Toggle | Modes | Powered by |
|---|---|---|
| Move tracker + branches | All | engine |
| Shadow moves | All | engine |
| Edge coordinates | All | engine |
| Liberty count | All | engine |
| Territory (settled) | All | engine |
| Territory (contested) | Live, Teaching | KataGo |
| Annotations | Teaching, Joseki Example | engine |
| KataGo next move | Live, Teaching | KataGo |
| Joseki suggester | Live, Teaching | joseki DB |
| Shape sensor | All | engine + templates |
| Life/death judge | Live, Teaching | KataGo |
| Strong/weak groups | Live, Teaching | KataGo |
| Clock | Live, Teaching (off by default for Joseki Example) | clock |

## 9. Clock

Main time in minutes per side (0 = none). Byo-yomi as periods × seconds
(0 periods = none), Japanese style: a period is consumed only if it expires
before the move is played. Whole clock is toggleable off per game.

## 10. Phases

1. Engine + FR-1–10
2. Shape sensor, joseki suggester, OGS-style panel + clock
3. KataGo integration: FR-13, FR-16, FR-17, contested territory
