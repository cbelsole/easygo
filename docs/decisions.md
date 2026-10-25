# Decisions

Answers to the planning questions, plus where the two source documents
(`spec.md` = "Go on Easy Mode", and the earlier Joseki Dojo design doc) conflict.
Easy Mode spec and these answers win over Joseki Dojo wherever they disagree.

## Answers

| # | Topic | Decision |
|---|---|---|
| 1 | Stack | Go backend + browser frontend served locally. |
| 3 | Joseki data | Source it from free collections via an import script; license checked before anything is committed. |
| 4 | Networking | Networked play is in scope. Google sign-in for players. |
| 5 | KataGo | GPU machine (RTX 5080). KataGo set up outside the repo, OpenCL build. |
| 6 | Scope | Build every phase. |
| 7 | Board sizes | 9x9, 13x13, 19x19. |
| 8 | Rules | All rule options (ko, scoring, tax, suicide, komi, handicap) with named presets. |
| 9 | Life/death | KataGo identifies it automatically; user can override by marking groups. |
| 10 | Strength/territory without KataGo | Greyed out. |
| 11 | Shape sensor | Ladder, net, snapback, knight's move, large knight's move; registry is extensible. |
| 12 | Ladders | Breakers handled by reading the ladder out on the real board. |
| 13 | Clock | Byo-yomi, timeout loses; clock can be disabled per game. |
| 14 | Persistence | SGF on disk, autosave. |
| 15 | Repo | git, spec committed in `docs/`. |

## Conflicts between the two documents

- **GUI:** Joseki Dojo chose Fyne (desktop). Superseded: web UI.
- **SGF library:** Joseki Dojo chose `toikarin/sgf`. Superseded by Easy Mode §3: own tree, standard SGF only at import/export.
- **Scoring:** Joseki Dojo listed territory counting as a non-goal. Superseded.
- **Kept from Joseki Dojo:** no transcription of the books' diagrams/commentary (copyright); SQLite via `modernc.org/sqlite`; the Pattern/MoveNode model; one `.sgf` per pattern.

## Choices made without asking

- **Live sync is SSE + POST**, not websockets. Server pushes, client posts moves. Needs no dependency and is enough for turn-based play.
- **The server owns the rules and the clock.** Clients only display. Timeouts are computed server-side.
- **AI assistance is off in human-vs-human networked games** unless both players opt in; it would be cheating otherwise.
- **KataGo OpenCL build, not CUDA.** CUDA needs a system-wide CUDA/cuDNN install. OpenCL runs on the GPU driver alone.
- **"Greyed out" (Q10)** means: disabled whenever KataGo is unavailable.
- **Google OAuth** needs a client ID/secret only the account owner can create. A dev-login stub sits behind the same interface until then.
