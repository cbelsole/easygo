// Package session holds a game's live state: the move tree, who may move,
// the clock, and the mode. The server owns rules and time; clients only
// display what a Game reports.
package session

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"easygo/internal/board"
	"easygo/internal/clock"
	"easygo/internal/tree"
)

type Mode string

const (
	ModeLive     Mode = "live"
	ModeTeaching Mode = "teaching"
	ModeJoseki   Mode = "joseki"
)

type Status string

const (
	StatusWaiting  Status = "waiting" // an open seat is still empty
	StatusPlaying  Status = "playing"
	StatusScoring  Status = "scoring" // two passes: mark dead stones and agree
	StatusFinished Status = "finished"
)

var (
	ErrNotYourTurn = errors.New("it is not your turn")
	ErrNotYours    = errors.New("you cannot change this game")
	ErrNotPlaying  = errors.New("the game is not in progress")
	ErrNotAtTip    = errors.New("moves can only be played at the end of a live game")
	ErrSeatTaken   = errors.New("that seat is taken")
	ErrBadMode     = errors.New("not allowed in this mode")
	ErrUnknownNode = errors.New("no such move")
	ErrBadSize     = errors.New("board size must be 9, 13 or 19")
)

// Player is one side. Kind is human, ai or open (an empty seat).
type Player struct {
	Name     string `json:"name"`
	Rank     string `json:"rank"`
	Location string `json:"location"`
	Kind     string `json:"kind"`
	UserID   string `json:"-"`
	// AILevel is the visit budget a KataGo opponent searches with.
	AIVisits int `json:"-"`
}

// Options configures a new game.
type Options struct {
	Name     string
	Mode     Mode
	Size     int
	Rules    board.Rules
	Handicap int
	Black    Player
	White    Player
	Clock    clock.Config
	// Local means one person controls both seats (hot-seat or teaching).
	Local   bool
	OwnerID string
	// AIAssist enables KataGo help. Off by default in games between two humans.
	AIAssist bool
}

// Game is one game in progress or finished.
type Game struct {
	mu sync.Mutex

	ID       string
	Name     string
	Mode     Mode
	Tree     *tree.Tree
	Cursor   *tree.Node
	Players  [3]Player
	Local    bool
	OwnerID  string
	Clock    *clock.Clock
	ClockCfg clock.Config

	Status   Status
	Result   string
	Created  time.Time
	Updated  time.Time
	Handicap int
	AIAssist bool
	// AssistVotes records who has agreed to AI help; assistDefault is true
	// for games where only one human is involved.
	AssistVotes   [3]bool
	assistDefault bool

	// Scoring phase.
	AutoDead  map[board.Point]bool // stones the AI judged dead
	Overrides map[board.Point]bool // the players' corrections
	Accepted  [3]bool

	version uint64
	subs    map[chan struct{}]struct{}
}

func newID() string {
	b := make([]byte, 5)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// New creates a game from options.
func New(o Options) (*Game, error) {
	switch o.Size {
	case 9, 13, 19:
	default:
		return nil, ErrBadSize
	}
	if err := o.Rules.Validate(); err != nil {
		return nil, err
	}
	if o.Mode == "" {
		o.Mode = ModeLive
	}
	t := tree.New(o.Size, o.Rules)
	if o.Handicap > 1 {
		pos, err := board.Handicap(o.Size, o.Handicap)
		if err != nil {
			return nil, err
		}
		t.Root.Setup = map[board.Point]board.Color{}
		for _, g := range pos.Board.Groups() {
			for _, s := range g.Stones {
				t.Root.Setup[s] = board.Black
			}
		}
		t.Root.ToPlay = board.White
		t.Info.Handicap = o.Handicap
	}
	t.Info.BlackName, t.Info.BlackRank = o.Black.Name, o.Black.Rank
	t.Info.WhiteName, t.Info.WhiteRank = o.White.Name, o.White.Rank
	t.Info.GameName = o.Name
	t.Info.Date = time.Now().Format("2006-01-02")

	g := &Game{
		ID: newID(), Name: o.Name, Mode: o.Mode, Tree: t, Cursor: t.Root,
		Local: o.Local, OwnerID: o.OwnerID, ClockCfg: o.Clock,
		Created: time.Now(), Handicap: o.Handicap, AIAssist: o.AIAssist,
		Overrides: map[board.Point]bool{}, subs: map[chan struct{}]struct{}{},
	}
	g.Players[board.Black], g.Players[board.White] = o.Black, o.White
	g.Status = StatusPlaying
	if o.Mode == ModeLive && (o.Black.Kind == "open" || o.White.Kind == "open") {
		g.Status = StatusWaiting
	}
	if o.Mode != ModeLive {
		g.Local = true // teaching and joseki are driven by one person
	}
	g.Updated = g.Created
	// Between two networked humans AI help is off until both agree; with a
	// single human (hot-seat, teaching, KataGo opponent) it is on.
	g.assistDefault = g.Local || o.Black.Kind == "ai" || o.White.Kind == "ai"
	g.recomputeAssistLocked()
	g.Clock = clock.New(o.Clock, nil)
	if g.Status == StatusPlaying && o.Mode == ModeLive {
		g.Clock.Start(g.toPlayLocked())
	}
	return g, nil
}

// FromTree wraps an imported tree (SGF import, joseki example sets).
func FromTree(t *tree.Tree, mode Mode, owner string) *Game {
	g := &Game{
		ID: newID(), Name: t.Info.GameName, Mode: mode, Tree: t, Cursor: t.Root,
		Local: true, OwnerID: owner, Status: StatusPlaying,
		Created: time.Now(), Overrides: map[board.Point]bool{}, subs: map[chan struct{}]struct{}{},
		Handicap: t.Info.Handicap,
	}
	g.Updated = g.Created
	g.assistDefault, g.AIAssist = true, true
	g.Players[board.Black] = Player{Name: orDefault(t.Info.BlackName, "Black"), Rank: t.Info.BlackRank, Kind: "human"}
	g.Players[board.White] = Player{Name: orDefault(t.Info.WhiteName, "White"), Rank: t.Info.WhiteRank, Kind: "human"}
	g.Clock = clock.New(clock.Config{}, nil)
	return g
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

// Lock runs fn with the game locked. Readers outside this package use it
// to take a consistent snapshot.
func (g *Game) Lock(fn func()) {
	g.mu.Lock()
	defer g.mu.Unlock()
	fn()
}

// Subscribe returns a channel pinged on every change and a cancel func.
func (g *Game) Subscribe() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	g.mu.Lock()
	g.subs[ch] = struct{}{}
	g.mu.Unlock()
	return ch, func() {
		g.mu.Lock()
		delete(g.subs, ch)
		g.mu.Unlock()
	}
}

// changedLocked bumps the version and wakes subscribers. Caller holds mu.
func (g *Game) changedLocked() {
	g.version++
	g.Updated = time.Now()
	for ch := range g.subs {
		select {
		case ch <- struct{}{}:
		default: // already pending
		}
	}
}

// Version increments on every change; handy for caching.
func (g *Game) Version() uint64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.version
}

func (g *Game) positionLocked() *board.Position {
	pos, err := g.Tree.Position(g.Cursor)
	if err != nil {
		return nil
	}
	return pos
}

func (g *Game) toPlayLocked() board.Color {
	if pos := g.positionLocked(); pos != nil {
		return pos.ToPlay
	}
	return board.Black
}

// SeatOf returns the color userID controls: Black, White, or Empty for a
// spectator. In a local game everyone with access controls the side to move.
func (g *Game) seatLocked(userID string) board.Color {
	if g.Local {
		if g.OwnerID == "" || userID == g.OwnerID {
			return g.toPlayLocked()
		}
		return board.Empty
	}
	for _, c := range []board.Color{board.Black, board.White} {
		p := g.Players[c]
		if (p.Kind == "human" || p.Kind == "ai") && p.UserID != "" && p.UserID == userID {
			return c
		}
	}
	return board.Empty
}

// SetAssist records a player's vote on AI assistance. In a game between two
// networked humans it is on only when both have said yes; anywhere else a
// single player's choice stands.
func (g *Game) SetAssist(userID string, on bool) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	seat := g.seatLocked(userID)
	if seat == board.Empty {
		return ErrNotYours
	}
	g.AssistVotes[seat] = on
	g.recomputeAssistLocked()
	g.changedLocked()
	return nil
}

func (g *Game) recomputeAssistLocked() {
	if g.Local || g.Players[board.Black].Kind == "ai" || g.Players[board.White].Kind == "ai" {
		g.AIAssist = g.AssistVotes[board.Black] || g.AssistVotes[board.White] || g.assistDefault
		return
	}
	g.AIAssist = g.AssistVotes[board.Black] && g.AssistVotes[board.White]
}

// Seat is the exported form of seatLocked.
func (g *Game) Seat(userID string) board.Color {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.seatLocked(userID)
}

// IsOwner reports whether userID may edit a teaching/joseki game.
func (g *Game) IsOwner(userID string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.OwnerID == "" || g.OwnerID == userID
}

// Join fills an open seat.
func (g *Game) Join(userID, name, rank, location string, color board.Color) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if color != board.Black && color != board.White {
		return fmt.Errorf("choose black or white")
	}
	if g.Players[color].Kind != "open" {
		return ErrSeatTaken
	}
	g.Players[color] = Player{Name: name, Rank: rank, Location: location, Kind: "human", UserID: userID}
	g.setNamesLocked()
	if g.Players[board.Black].Kind != "open" && g.Players[board.White].Kind != "open" {
		g.Status = StatusPlaying
		g.Clock.Start(g.toPlayLocked())
	}
	g.changedLocked()
	return nil
}

func (g *Game) setNamesLocked() {
	g.Tree.Info.BlackName, g.Tree.Info.BlackRank = g.Players[board.Black].Name, g.Players[board.Black].Rank
	g.Tree.Info.WhiteName, g.Tree.Info.WhiteRank = g.Players[board.White].Name, g.Players[board.White].Rank
}

// atTipLocked reports whether the cursor has no children.
func (g *Game) atTipLocked() bool { return len(g.Cursor.Children) == 0 }

// Play places a stone (or passes, if pt is board.Pass) for userID.
// override lets teaching and joseki games play either color; it is ignored
// in live games.
func (g *Game) Play(userID string, pt board.Point, override board.Color) (*tree.Node, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	pos := g.positionLocked()
	if pos == nil {
		return nil, fmt.Errorf("the game position is invalid")
	}
	color := pos.ToPlay

	if g.Mode == ModeLive {
		if g.Status != StatusPlaying {
			return nil, ErrNotPlaying
		}
		if !g.atTipLocked() {
			return nil, ErrNotAtTip
		}
		if seat := g.seatLocked(userID); seat != color {
			return nil, ErrNotYourTurn
		}
		if g.Clock.Flagged() != board.Empty {
			g.timeoutLocked()
			return nil, ErrNotPlaying
		}
	} else {
		if g.OwnerID != "" && userID != g.OwnerID {
			return nil, ErrNotYours
		}
		if override != board.Empty {
			color = override
		}
	}

	n, err := g.Tree.AddMove(g.Cursor, color, pt)
	if err != nil {
		return nil, err
	}
	g.Cursor = n
	if g.Mode == ModeLive {
		g.Clock.Switch()
		if p, err := g.Tree.Position(n); err == nil && p.Over() {
			g.Status = StatusScoring
			g.Clock.Stop()
			g.AutoDead, g.Overrides, g.Accepted = nil, map[board.Point]bool{}, [3]bool{}
		}
	}
	g.changedLocked()
	return n, nil
}

// Resign ends a live game in the opponent's favor.
func (g *Game) Resign(userID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.Mode != ModeLive || (g.Status != StatusPlaying && g.Status != StatusScoring) {
		return ErrNotPlaying
	}
	seat := g.seatLocked(userID)
	if g.Local {
		seat = g.toPlayLocked()
	}
	if seat == board.Empty {
		return ErrNotYours
	}
	g.finishLocked(fmt.Sprintf("%s+R", seat.Opp()))
	return nil
}

func (g *Game) finishLocked(result string) {
	g.Status = StatusFinished
	g.Result = result
	g.Tree.Info.Result = result
	g.Clock.Stop()
	g.changedLocked()
}

// timeoutLocked ends the game if a side has flagged.
func (g *Game) timeoutLocked() {
	if g.Status == StatusFinished {
		return
	}
	if f := g.Clock.Flagged(); f != board.Empty {
		g.finishLocked(fmt.Sprintf("%s+T", f.Opp()))
	}
}

// CheckTimeout applies a flag if the clock has run out. The server calls it
// from a timer scheduled at Clock.Deadline.
func (g *Game) CheckTimeout() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.Status != StatusPlaying {
		return false
	}
	before := g.Status
	g.timeoutLocked()
	return g.Status != before
}

// Goto moves the cursor. Live games may only be browsed, never branched, so
// navigation there is read-only per viewer on the client; the server cursor
// stays at the tip.
func (g *Game) Goto(userID string, nodeID int) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.Mode == ModeLive {
		return ErrBadMode
	}
	if g.OwnerID != "" && userID != g.OwnerID {
		return ErrNotYours
	}
	n := g.Tree.Find(nodeID)
	if n == nil {
		return ErrUnknownNode
	}
	g.Cursor = n
	g.changedLocked()
	return nil
}

// Undo removes the last move. Allowed in teaching/joseki games and in
// local or AI live games (the caller decides how many plies).
func (g *Game) Undo(userID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.Mode == ModeLive {
		humanOnly := g.Players[board.Black].Kind == "human" && g.Players[board.White].Kind == "human"
		if !g.Local && humanOnly {
			return ErrBadMode // no takebacks between two networked humans
		}
		if g.Status == StatusFinished {
			return ErrNotPlaying
		}
	}
	if g.OwnerID != "" && userID != g.OwnerID && g.Mode != ModeLive {
		return ErrNotYours
	}
	if g.Cursor.Parent == nil {
		return nil
	}
	old := g.Cursor
	g.Cursor = old.Parent
	g.Tree.Delete(old)
	if g.Mode == ModeLive {
		g.Status = StatusPlaying
		g.Clock.Stop()
		if g.ClockCfg.Enabled {
			g.Clock.Start(g.toPlayLocked())
		}
	}
	g.changedLocked()
	return nil
}

// Annotation is an edit to a node's notes.
type Annotation struct {
	Comment  *string      `json:"comment"`
	Name     *string      `json:"name"`
	Judgment *string      `json:"judgment"`
	Marks    *[]tree.Mark `json:"marks"`
}

// Annotate edits comments and marks. Allowed in teaching and joseki games.
func (g *Game) Annotate(userID string, nodeID int, a Annotation) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.Mode == ModeLive && g.Status != StatusFinished {
		return ErrBadMode
	}
	if g.OwnerID != "" && userID != g.OwnerID {
		return ErrNotYours
	}
	n := g.Tree.Find(nodeID)
	if n == nil {
		return ErrUnknownNode
	}
	if a.Comment != nil {
		n.Comment = *a.Comment
	}
	if a.Name != nil {
		n.Name = *a.Name
	}
	if a.Judgment != nil {
		switch *a.Judgment {
		case "", "correct", "playable", "mistake":
			n.Judgment = *a.Judgment
		default:
			return fmt.Errorf("unknown judgment %q", *a.Judgment)
		}
	}
	if a.Marks != nil {
		n.Marks = *a.Marks
	}
	g.changedLocked()
	return nil
}

// DeleteNode removes a branch; Promote moves it toward the main line.
func (g *Game) DeleteNode(userID string, nodeID int) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.Mode == ModeLive {
		return ErrBadMode
	}
	if g.OwnerID != "" && userID != g.OwnerID {
		return ErrNotYours
	}
	n := g.Tree.Find(nodeID)
	if n == nil {
		return ErrUnknownNode
	}
	// If the cursor is inside the deleted branch, move it to the parent.
	for c := g.Cursor; c != nil; c = c.Parent {
		if c == n {
			g.Cursor = n.Parent
			break
		}
	}
	if err := g.Tree.Delete(n); err != nil {
		return err
	}
	g.changedLocked()
	return nil
}

func (g *Game) Promote(userID string, nodeID int) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.Mode == ModeLive {
		return ErrBadMode
	}
	if g.OwnerID != "" && userID != g.OwnerID {
		return ErrNotYours
	}
	n := g.Tree.Find(nodeID)
	if n == nil {
		return ErrUnknownNode
	}
	g.Tree.Promote(n)
	g.changedLocked()
	return nil
}

// SetAutoDead stores the AI's life/death verdict for the scoring phase.
func (g *Game) SetAutoDead(dead map[board.Point]bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.AutoDead = dead
	g.changedLocked()
}

// ToggleDead flips a group's status during scoring. The override is keyed
// by any stone of the group.
func (g *Game) ToggleDead(userID string, p board.Point, dead bool) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.Mode == ModeLive && g.Status != StatusScoring {
		return ErrBadMode
	}
	if g.Mode == ModeLive && g.seatLocked(userID) == board.Empty && !g.Local {
		return ErrNotYours
	}
	pos := g.positionLocked()
	if pos == nil || pos.Board.At(p) == board.Empty {
		return fmt.Errorf("no stone there")
	}
	stones, _ := pos.Board.Group(p)
	for _, s := range stones {
		g.Overrides[s] = dead
	}
	g.Accepted = [3]bool{} // any change cancels earlier agreement
	g.changedLocked()
	return nil
}

// DeadSet is the AI's verdict with the players' overrides applied.
func (g *Game) deadLocked() map[board.Point]bool {
	out := map[board.Point]bool{}
	for p := range g.AutoDead {
		out[p] = true
	}
	for p, v := range g.Overrides {
		if v {
			out[p] = true
		} else {
			delete(out, p)
		}
	}
	return out
}

// Dead returns the dead-stone set currently in force.
func (g *Game) Dead() map[board.Point]bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.deadLocked()
}

// AcceptScore records agreement; when both sides agree (or the game is
// local) it computes the result and ends the game.
func (g *Game) AcceptScore(userID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.Status != StatusScoring {
		return ErrNotPlaying
	}
	seat := g.seatLocked(userID)
	if g.Local {
		g.Accepted[board.Black], g.Accepted[board.White] = true, true
	} else {
		if seat == board.Empty {
			return ErrNotYours
		}
		g.Accepted[seat] = true
		// An AI opponent always agrees.
		for _, c := range []board.Color{board.Black, board.White} {
			if g.Players[c].Kind == "ai" {
				g.Accepted[c] = true
			}
		}
	}
	if g.Accepted[board.Black] && g.Accepted[board.White] {
		pos := g.positionLocked()
		res := board.Score(pos, g.Tree.Rules, board.ScoreOpts{Dead: g.deadLocked(), Handicap: g.Handicap})
		g.finishLocked(FormatResult(res))
		return nil
	}
	g.changedLocked()
	return nil
}

// FormatResult renders a score as an SGF result string.
func FormatResult(r board.ScoreResult) string {
	switch {
	case r.Margin > 0:
		return fmt.Sprintf("B+%g", r.Margin)
	case r.Margin < 0:
		return fmt.Sprintf("W+%g", -r.Margin)
	}
	return "0"
}
