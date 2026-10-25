package session

import (
	"time"

	"easygo/internal/board"
	"easygo/internal/clock"
	"easygo/internal/sgf"
)

// PlayerMeta is a Player including the fields hidden from browsers.
type PlayerMeta struct {
	Name     string `json:"name"`
	Rank     string `json:"rank"`
	Location string `json:"location"`
	Kind     string `json:"kind"`
	UserID   string `json:"userId"`
	AIVisits int    `json:"aiVisits"`
	AIRank   string `json:"aiRank,omitempty"`
}

// Meta is everything about a game that the SGF record does not carry.
// Together with the SGF it is enough to resume the game after a restart.
type Meta struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Mode     Mode           `json:"mode"`
	Status   Status         `json:"status"`
	Result   string         `json:"result"`
	Created  time.Time      `json:"created"`
	Updated  time.Time      `json:"updated"`
	Handicap int            `json:"handicap"`
	Size     int            `json:"size"`
	AIAssist bool           `json:"aiAssist"`
	Local    bool           `json:"local"`
	OwnerID  string         `json:"ownerId"`
	Rules    board.Rules    `json:"rules"`
	Players  [2]PlayerMeta  `json:"players"`
	Clock    clock.Config   `json:"clock"`
	ClockNow [2]clock.State `json:"clockState"`
	// Cursor is the path of child indexes from the root to the cursor.
	Cursor    []int          `json:"cursor"`
	Dead      []board.Point  `json:"autoDead"`
	Overrides []overrideMeta `json:"overrides"`
	Accepted  [2]bool        `json:"accepted"`
	// AssistVotes is who has agreed to AI help: [black, white].
	AssistVotes [2]bool `json:"assistVotes"`
}

type overrideMeta struct {
	Pt   board.Point `json:"pt"`
	Dead bool        `json:"dead"`
}

// Snapshot returns the metadata and the SGF record.
func (g *Game) Snapshot() (Meta, string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	m := Meta{
		ID: g.ID, Name: g.Name, Mode: g.Mode, Status: g.Status, Result: g.Result,
		Created: g.Created, Updated: g.Updated, Handicap: g.Handicap, Size: g.Tree.Size, AIAssist: g.AIAssist,
		Local: g.Local, OwnerID: g.OwnerID, Rules: g.Tree.Rules, Clock: g.ClockCfg,
		ClockNow:    g.Clock.Export(),
		Accepted:    [2]bool{g.Accepted[board.Black], g.Accepted[board.White]},
		AssistVotes: [2]bool{g.AssistVotes[board.Black], g.AssistVotes[board.White]},
	}
	for i, c := range []board.Color{board.Black, board.White} {
		p := g.Players[c]
		m.Players[i] = PlayerMeta{p.Name, p.Rank, p.Location, p.Kind, p.UserID, p.AIVisits, p.AIRank}
	}
	// The cursor as a path survives re-parsing, where node IDs would not.
	var rev []int
	for n := g.Cursor; n.Parent != nil; n = n.Parent {
		for i, c := range n.Parent.Children {
			if c == n {
				rev = append(rev, i)
			}
		}
	}
	for i := len(rev) - 1; i >= 0; i-- {
		m.Cursor = append(m.Cursor, rev[i])
	}
	for p := range g.AutoDead {
		m.Dead = append(m.Dead, p)
	}
	for p, v := range g.Overrides {
		m.Overrides = append(m.Overrides, overrideMeta{p, v})
	}
	return m, sgf.Write(g.Tree)
}

// Restore rebuilds a game from a snapshot.
func Restore(m Meta, record string) (*Game, error) {
	t, err := sgf.Parse(record)
	if err != nil {
		return nil, err
	}
	t.Rules = m.Rules
	g := &Game{
		ID: m.ID, Name: m.Name, Mode: m.Mode, Tree: t, Cursor: t.Root,
		Status: m.Status, Result: m.Result, Created: m.Created, Updated: m.Updated,
		Handicap: m.Handicap, AIAssist: m.AIAssist, Local: m.Local, OwnerID: m.OwnerID,
		ClockCfg: m.Clock, Overrides: map[board.Point]bool{}, subs: map[chan struct{}]struct{}{},
	}
	for i, c := range []board.Color{board.Black, board.White} {
		p := m.Players[i]
		g.Players[c] = Player{Name: p.Name, Rank: p.Rank, Location: p.Location, Kind: p.Kind, UserID: p.UserID, AIVisits: p.AIVisits, AIRank: p.AIRank}
	}
	for _, i := range m.Cursor {
		if i >= 0 && i < len(g.Cursor.Children) {
			g.Cursor = g.Cursor.Children[i]
		}
	}
	if len(m.Dead) > 0 {
		g.AutoDead = map[board.Point]bool{}
		for _, p := range m.Dead {
			g.AutoDead[p] = true
		}
	}
	for _, o := range m.Overrides {
		g.Overrides[o.Pt] = o.Dead
	}
	g.Accepted[board.Black], g.Accepted[board.White] = m.Accepted[0], m.Accepted[1]
	g.AssistVotes[board.Black], g.AssistVotes[board.White] = m.AssistVotes[0], m.AssistVotes[1]
	g.assistDefault = g.Local || g.Players[board.Black].Kind == "ai" || g.Players[board.White].Kind == "ai"
	g.recomputeAssistLocked()
	g.Clock = clock.New(m.Clock, nil)
	g.Clock.Import(m.ClockNow)
	if g.Mode == ModeLive && g.Status == StatusPlaying {
		g.Clock.Start(g.toPlayLocked())
	}
	return g, nil
}
