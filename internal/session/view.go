package session

import (
	"easygo/internal/board"
	"easygo/internal/clock"
	"easygo/internal/tree"
)

// The JSON shapes the browser receives. Colors are 0 empty, 1 black,
// 2 white everywhere, matching board.Color. Points are {x, y} with the
// origin at the top-left.

type XY struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type MarkView struct {
	X    int    `json:"x"`
	Y    int    `json:"y"`
	Kind string `json:"kind"`
	Text string `json:"text,omitempty"`
}

type NodeView struct {
	ID       int        `json:"id"`
	Parent   int        `json:"parent"` // 0 for the root
	Color    int        `json:"color"`  // 0 for the root / setup-only nodes
	Pass     bool       `json:"pass,omitempty"`
	X        int        `json:"x"`
	Y        int        `json:"y"`
	Comment  string     `json:"comment,omitempty"`
	Name     string     `json:"name,omitempty"`
	Judgment string     `json:"judgment,omitempty"`
	Marks    []MarkView `json:"marks,omitempty"`
	Children []int      `json:"children"`
}

type PlayerView struct {
	Name     string `json:"name"`
	Rank     string `json:"rank"`
	Location string `json:"location"`
	Kind     string `json:"kind"`
	You      bool   `json:"you"`
}

type YouView struct {
	// Seat is 1 (black), 2 (white), 3 (both, a local game) or 0 (watching).
	Seat    int  `json:"seat"`
	CanMove bool `json:"canMove"`
	Owner   bool `json:"owner"`
}

type GameView struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	Mode     Mode        `json:"mode"`
	Status   Status      `json:"status"`
	Result   string      `json:"result,omitempty"`
	Size     int         `json:"size"`
	Rules    board.Rules `json:"rules"`
	Handicap int         `json:"handicap"`
	Version  uint64      `json:"version"`

	ToPlay     int    `json:"toPlay"`
	MoveNumber int    `json:"moveNumber"`
	Board      []int  `json:"board"`
	Liberties  []int  `json:"liberties"`
	Captures   [2]int `json:"captures"` // [black's captures, white's captures]
	LastMove   *XY    `json:"lastMove,omitempty"`
	Cursor     int    `json:"cursor"`

	Nodes    []NodeView `json:"nodes"`
	MainLine []int      `json:"mainLine"`
	// Partial is true when Nodes is only a window of a very large tree;
	// ids in a node's children that are missing from Nodes are not loaded yet.
	Partial bool `json:"partial"`

	Players [2]PlayerView `json:"players"` // [black, white]
	Clock   [2]clock.View `json:"clock"`
	You     YouView       `json:"you"`

	// Settled-position territory: 0 neutral, 1 black, 2 white, with the
	// dead stones currently in force removed. Empty during ordinary play
	// of a live game is still meaningful ("what is closed off right now").
	Territory []int      `json:"territory"`
	Dead      []XY       `json:"dead"`
	Score     *ScoreView `json:"score,omitempty"`
	Accepted  [2]bool    `json:"accepted"`
	AIAssist  bool       `json:"aiAssist"`
	// AssistVotes is who has agreed to AI help: [black, white].
	AssistVotes [2]bool `json:"assistVotes"`
}

type ScoreView struct {
	Black          float64 `json:"black"`
	White          float64 `json:"white"`
	Margin         float64 `json:"margin"`
	BlackTerritory int     `json:"blackTerritory"`
	WhiteTerritory int     `json:"whiteTerritory"`
}

// View snapshots the game for one viewer (userID may be empty).
func (g *Game) View(userID string) GameView { return g.ViewAt(userID, 0) }

// ViewAt is View for the position after node nodeID (0 means the cursor), so a
// client reviewing history gets the real board there, handicap stones included.
// A historical view never lets anyone move.
func (g *Game) ViewAt(userID string, nodeID int) GameView {
	g.mu.Lock()
	defer g.mu.Unlock()
	t := g.Tree
	cur := g.Cursor
	if nodeID != 0 {
		if n := t.Find(nodeID); n != nil {
			cur = n
		}
	}
	historical := cur != g.Cursor
	v := GameView{
		ID: g.ID, Name: g.Name, Mode: g.Mode, Status: g.Status, Result: g.Result,
		Size: t.Size, Rules: t.Rules, Handicap: g.Handicap, Version: g.version,
		Cursor: cur.ID, AIAssist: g.AIAssist,
		AssistVotes: [2]bool{g.AssistVotes[board.Black], g.AssistVotes[board.White]},
		Accepted:    [2]bool{g.Accepted[board.Black], g.Accepted[board.White]},
		Dead:        []XY{},
	}

	if pos, err := t.Position(cur); err == nil {
		n := t.Size
		v.ToPlay = int(pos.ToPlay)
		v.MoveNumber = t.MoveNumber(cur)
		v.Board = make([]int, n*n)
		v.Liberties = make([]int, n*n)
		for _, gr := range pos.Board.Groups() {
			for _, s := range gr.Stones {
				v.Board[s.Y*n+s.X] = int(gr.Color)
				v.Liberties[s.Y*n+s.X] = len(gr.Liberties)
			}
		}
		v.Captures = [2]int{pos.Captures[board.Black], pos.Captures[board.White]}
		if !pos.LastMove.IsPass() {
			v.LastMove = &XY{pos.LastMove.X, pos.LastMove.Y}
		}
		dead := g.deadLocked()
		for p := range dead {
			v.Dead = append(v.Dead, XY{p.X, p.Y})
		}
		res := board.Score(pos, t.Rules, board.ScoreOpts{Dead: dead, Handicap: g.Handicap})
		v.Territory = make([]int, n*n)
		for i, c := range res.Territory {
			v.Territory[i] = int(c)
		}
		v.Score = &ScoreView{
			Black: res.Black, White: res.White, Margin: res.Margin,
			BlackTerritory: res.BlackTerr, WhiteTerritory: res.WhiteTerr,
		}
	}

	// Small trees go out whole. A huge tree (the joseki dictionary has
	// tens of thousands of nodes) is sent as a window: the path to the
	// cursor, each path node's children, and the cursor's children.
	total := 0
	t.Walk(func(*tree.Node) bool { total++; return true })
	windowed := total > maxFullNodes
	show := map[int]bool{}
	if windowed {
		v.Partial = true
		for _, p := range t.Path(cur) {
			show[p.ID] = true
			for _, c := range p.Children {
				show[c.ID] = true
			}
		}
	}

	t.Walk(func(n *tree.Node) bool {
		if windowed && !show[n.ID] {
			return true
		}
		nv := NodeView{
			ID: n.ID, Color: int(n.Color), Comment: n.Comment, Name: n.Name,
			Judgment: n.Judgment, Children: []int{},
		}
		if n.Parent != nil {
			nv.Parent = n.Parent.ID
		}
		if n.HasMove {
			if n.Move.IsPass() {
				nv.Pass = true
			} else {
				nv.X, nv.Y = n.Move.X, n.Move.Y
			}
		}
		for _, m := range n.Marks {
			nv.Marks = append(nv.Marks, MarkView{m.Pt.X, m.Pt.Y, m.Kind, m.Text})
		}
		for _, c := range n.Children {
			nv.Children = append(nv.Children, c.ID)
		}
		v.Nodes = append(v.Nodes, nv)
		return true
	})
	for _, n := range t.MainLine() {
		if windowed && len(v.MainLine) >= 200 {
			break
		}
		v.MainLine = append(v.MainLine, n.ID)
	}

	for i, c := range []board.Color{board.Black, board.White} {
		p := g.Players[c]
		v.Players[i] = PlayerView{Name: p.Name, Rank: p.Rank, Location: p.Location, Kind: p.Kind,
			You: p.Kind == "human" && p.UserID != "" && p.UserID == userID}
		v.Clock[i] = g.Clock.Snapshot(c)
	}

	seat := g.seatLocked(userID)
	you := YouView{Owner: g.OwnerID == "" || g.OwnerID == userID}
	switch {
	case g.Local && you.Owner:
		you.Seat = 3
	case seat != board.Empty:
		you.Seat = int(seat)
	}
	switch g.Mode {
	case ModeLive:
		you.CanMove = !historical && g.Status == StatusPlaying && g.atTipLocked() && seat != board.Empty && seat == board.Color(v.ToPlay)
	default:
		you.CanMove = you.Owner && !historical
	}
	v.You = you
	return v
}

// maxFullNodes is the largest tree sent whole.
const maxFullNodes = 3000
