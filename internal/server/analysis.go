package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"easygo/internal/board"
	"easygo/internal/katago"
	"easygo/internal/session"
	"easygo/internal/shapes"
	"easygo/internal/tree"
)

type markT = tree.Mark

type xy struct {
	X int `json:"x"`
	Y int `json:"y"`
}

func toXY(p board.Point) xy { return xy{p.X, p.Y} }

func toXYs(ps []board.Point) []xy {
	out := make([]xy, len(ps))
	for i, p := range ps {
		out[i] = toXY(p)
	}
	return out
}

// ---- who may ask the AI -------------------------------------------------

// canAssist decides whether userID may see KataGo's opinion of this game.
// In a game between two networked humans it would be cheating, so it needs
// both players' agreement and spectators are shut out while it is in play.
func canAssist(g *session.Game, userID string) (bool, string) {
	var ok bool
	var why string
	g.Lock(func() {
		switch {
		case g.Mode != session.ModeLive || g.Status == session.StatusFinished:
			ok = true
		case g.Local || g.Players[board.Black].Kind == "ai" || g.Players[board.White].Kind == "ai":
			ok = userID != "" && (g.OwnerID == userID || g.Local)
			if !ok {
				why = "AI analysis is only available to the players while a game is in progress"
			}
		case g.AIAssist:
			ok = g.Players[board.Black].UserID == userID || g.Players[board.White].UserID == userID
			if !ok {
				why = "AI analysis is only available to the players while a game is in progress"
			}
		default:
			why = "AI assistance is off: both players must agree to turn it on"
		}
	})
	return ok, why
}

// ---- result cache ---------------------------------------------------------

// cache is a small bounded map for per-position results. Positions never
// change once a node exists, so a node id is a safe key.
type cache[T any] struct {
	mu    sync.Mutex
	items map[string]T
	order []string
}

func newCache[T any]() *cache[T] { return &cache[T]{items: map[string]T{}} }

func (c *cache[T]) get(k string) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.items[k]
	return v, ok
}

func (c *cache[T]) put(k string, v T) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.items[k]; !ok {
		c.order = append(c.order, k)
		if len(c.order) > 256 {
			delete(c.items, c.order[0])
			c.order = c.order[1:]
		}
	}
	c.items[k] = v
}

// ---- analysis ---------------------------------------------------------

type Suggestion struct {
	X         int     `json:"x"`
	Y         int     `json:"y"`
	Pass      bool    `json:"pass,omitempty"`
	Winrate   float64 `json:"winrate"` // Black's
	ScoreLead float64 `json:"scoreLead"`
	Visits    int     `json:"visits"`
	Prior     float64 `json:"prior"`
	PV        []xy    `json:"pv"`
}

type GroupView struct {
	Stones   []xy    `json:"stones"`
	Color    int     `json:"color"`
	Status   string  `json:"status"`
	Strength string  `json:"strength"`
	Own      float64 `json:"own"`
}

type AnalysisResult struct {
	Available   bool         `json:"available"`
	ToPlay      int          `json:"toPlay"`
	Winrate     float64      `json:"winrate"`   // Black's
	ScoreLead   float64      `json:"scoreLead"` // Black's
	Visits      int          `json:"visits"`
	Suggestions []Suggestion `json:"suggestions"`
	Ownership   []float64    `json:"ownership"`
	Territory   []int        `json:"territory"`
	Groups      []GroupView  `json:"groups"`
}

// analyze runs KataGo on the position after nodeID (0 = the cursor).
func (s *Server) analyze(ctx context.Context, g *session.Game, nodeID, visits int) (*AnalysisResult, error) {
	kg := s.katago()
	if kg == nil {
		return nil, katago.ErrUnavailable
	}
	var (
		q    katago.Query
		pos  *board.Position
		size int
		qerr error
		key  string
	)
	g.Lock(func() {
		n := g.Cursor
		if nodeID != 0 {
			n = g.Tree.Find(nodeID)
		}
		if n == nil {
			qerr = session.ErrUnknownNode
			return
		}
		pos, qerr = g.Tree.Position(n)
		if qerr != nil {
			return
		}
		size = g.Tree.Size
		q, qerr = katago.FromTree(g.Tree, n)
		key = fmt.Sprintf("%s/%d/%d", g.ID, n.ID, visits)
	})
	if qerr != nil {
		return nil, qerr
	}
	if c, ok := s.analysis.get(key); ok {
		return c, nil
	}
	q.MaxVisits, q.Ownership = visits, true
	select {
	case s.gpu <- struct{}{}:
		defer func() { <-s.gpu }()
	default:
		return nil, errBusy
	}
	resp, err := kg.Analyze(ctx, q)
	if err != nil {
		return nil, err
	}
	res := &AnalysisResult{
		Available: true, ToPlay: int(pos.ToPlay), Winrate: resp.RootInfo.Winrate,
		ScoreLead: resp.RootInfo.ScoreLead, Visits: resp.RootInfo.Visits, Ownership: resp.Ownership,
		Suggestions: []Suggestion{},
	}
	for i, m := range resp.MoveInfos {
		if i >= 6 {
			break
		}
		sg := Suggestion{Winrate: m.Winrate, ScoreLead: m.ScoreLead, Visits: m.Visits, Prior: m.Prior}
		p, err := board.ParseGTP(m.Move, size)
		if err != nil {
			continue
		}
		if p.IsPass() {
			sg.Pass = true
		} else {
			sg.X, sg.Y = p.X, p.Y
		}
		for _, mv := range m.PV {
			if pp, err := board.ParseGTP(mv, size); err == nil && !pp.IsPass() {
				sg.PV = append(sg.PV, toXY(pp))
			}
		}
		res.Suggestions = append(res.Suggestions, sg)
	}
	for _, c := range katago.Territory(resp.Ownership, 0.7) {
		res.Territory = append(res.Territory, int(c))
	}
	for _, j := range katago.Judge(pos.Board, resp.Ownership) {
		res.Groups = append(res.Groups, GroupView{
			Stones: toXYs(j.Stones), Color: int(j.Color), Status: string(j.Status),
			Strength: string(j.Strength), Own: j.Own,
		})
	}
	s.analysis.put(key, res)
	return res, nil
}

func (s *Server) handleAnalysis(w http.ResponseWriter, r *http.Request) {
	g, ok := s.game(w, r)
	if !ok {
		return
	}
	if allowed, why := canAssist(g, s.auth.UserID(r)); !allowed {
		writeJSON(w, 403, apiError{why})
		return
	}
	visits := 200
	if v, err := strconv.Atoi(r.URL.Query().Get("visits")); err == nil && v >= 10 && v <= 5000 {
		visits = v
	}
	node, _ := strconv.Atoi(r.URL.Query().Get("node"))
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	res, err := s.analyze(ctx, g, node, visits)
	if err != nil {
		if err == katago.ErrUnavailable {
			writeJSON(w, 200, AnalysisResult{Available: false, Suggestions: []Suggestion{}})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, res)
}

// ---- shapes & joseki ----------------------------------------------------

type shapeView struct {
	Kind       string `json:"kind"`
	Result     string `json:"result,omitempty"`
	Color      int    `json:"color"`
	Move       *xy    `json:"move,omitempty"`
	Points     []xy   `json:"points"`
	Breaker    *xy    `json:"breaker,omitempty"`
	Confidence string `json:"confidence"`
	Text       string `json:"text"`
}

func (s *Server) handleShapes(w http.ResponseWriter, r *http.Request) {
	g, ok := s.game(w, r)
	if !ok {
		return
	}
	// The ladder reader is computer help like any other: same fair-play rule.
	if allowed, why := canAssist(g, s.auth.UserID(r)); !allowed {
		writeJSON(w, 403, apiError{why})
		return
	}
	var pos *board.Position
	var rules board.Rules
	var perr error
	var key string
	g.Lock(func() {
		n := g.Cursor
		if id, _ := strconv.Atoi(r.URL.Query().Get("node")); id != 0 {
			if f := g.Tree.Find(id); f != nil {
				n = f
			}
		}
		pos, perr = g.Tree.Position(n)
		rules = g.Tree.Rules
		key = fmt.Sprintf("%s/%d", g.ID, n.ID)
	})
	if perr != nil {
		writeErr(w, perr)
		return
	}
	if cached, ok := s.shapes.get(key); ok {
		writeJSON(w, 200, map[string]any{"findings": cached})
		return
	}
	out := []shapeView{}
	for _, f := range shapes.Detect(pos, rules) {
		v := shapeView{Kind: f.Kind, Result: f.Result, Color: int(f.Color), Points: toXYs(f.Points), Confidence: f.Confidence, Text: f.Text}
		if f.Move != nil {
			m := toXY(*f.Move)
			v.Move = &m
		}
		if f.Breaker != nil {
			b := toXY(*f.Breaker)
			v.Breaker = &b
		}
		out = append(out, v)
	}
	s.shapes.put(key, out)
	writeJSON(w, 200, map[string]any{"findings": out})
}

type josekiSugg struct {
	Color   int    `json:"color"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
	Comment string `json:"comment,omitempty"`
}

type josekiCorner struct {
	Corner     string       `json:"corner"`
	Status     string       `json:"status"`
	Stones     int          `json:"stones"`
	Comment    string       `json:"comment,omitempty"`
	Next       []josekiSugg `json:"next"`
	DeviatedAt int          `json:"deviatedAt,omitempty"`
	Swapped    bool         `json:"swapped,omitempty"`
}

func (s *Server) handleJosekiMatch(w http.ResponseWriter, r *http.Request) {
	g, ok := s.game(w, r)
	if !ok {
		return
	}
	if allowed, why := canAssist(g, s.auth.UserID(r)); !allowed {
		writeJSON(w, 403, apiError{why})
		return
	}
	db := s.josekiDB()
	if db == nil {
		writeJSON(w, 200, map[string]any{"loaded": false, "corners": []josekiCorner{}})
		return
	}
	var ms []josekiCorner
	g.Lock(func() {
		n := g.Cursor
		if id, _ := strconv.Atoi(r.URL.Query().Get("node")); id != 0 {
			if f := g.Tree.Find(id); f != nil {
				n = f
			}
		}
		for _, m := range db.Analyze(g.Tree, n) {
			c := josekiCorner{
				Corner: string(m.Corner), Status: m.Status, Stones: m.Stones, Comment: m.Comment,
				DeviatedAt: m.DeviatedAt, Swapped: m.Swapped, Next: []josekiSugg{},
			}
			for _, sg := range m.Next {
				c.Next = append(c.Next, josekiSugg{int(sg.Color), sg.Point.X, sg.Point.Y, sg.Comment})
			}
			ms = append(ms, c)
		}
	})
	if ms == nil {
		ms = []josekiCorner{}
	}
	writeJSON(w, 200, map[string]any{"loaded": true, "corners": ms})
}

// ---- follow-up work: the computer opponent and scoring judgment ----------

// followUp runs KataGo work a state change calls for. At most one runs per game.
func (s *Server) followUp(g *session.Game) {
	kg := s.katago()
	if kg == nil {
		return
	}
	s.mu.Lock()
	if s.busy == nil {
		s.busy = map[string]bool{}
	}
	if s.busy[g.ID] {
		s.mu.Unlock()
		return
	}
	s.busy[g.ID] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.busy, g.ID)
		s.mu.Unlock()
	}()

	for step := 0; step < 4; step++ { // a few rounds, in case state moved while we worked
		var (
			status   session.Status
			needDead bool
			aiTurn   bool
			aiColor  board.Color
			aiVisits int
			version  uint64
		)
		g.Lock(func() {
			status = g.Status
			if status == session.StatusScoring && g.AutoDead == nil {
				needDead = true
			}
			if status == session.StatusPlaying && g.Mode == session.ModeLive && g.Cursor.Children == nil {
				if pos, err := g.Tree.Position(g.Cursor); err == nil && g.Players[pos.ToPlay].Kind == "ai" {
					aiTurn, aiColor, aiVisits = true, pos.ToPlay, g.Players[pos.ToPlay].AIVisits
				}
			}
		})
		version = g.Version()
		switch {
		case needDead:
			s.judgeDead(g)
			s.touchNoFollow(g)
		case aiTurn:
			if !s.aiMove(g, aiColor, aiVisits, version) {
				return
			}
			s.touchNoFollow(g)
		default:
			return
		}
	}
}

// touchNoFollow saves and re-arms timers without spawning another followUp.
func (s *Server) touchNoFollow(g *session.Game) {
	if err := s.store.SaveGame(g); err != nil {
		log.Printf("save game %s: %v", g.ID, err)
	}
	s.armTimer(g)
}

// judgeDead asks KataGo which stones are dead and records it.
func (s *Server) judgeDead(g *session.Game) {
	ctx, cancel := context.WithTimeout(s.ctx, 90*time.Second)
	defer cancel()
	res, err := s.analyze(ctx, g, 0, 400)
	// A busy engine is temporary; scoring must not quietly fall back to
	// "nothing is dead" just because other analyses were running.
	for try := 0; try < 20 && errors.Is(err, errBusy); try++ {
		time.Sleep(500 * time.Millisecond)
		res, err = s.analyze(ctx, g, 0, 400)
	}
	if err != nil {
		log.Printf("judge dead %s: %v", g.ID, err)
		g.SetAutoDead(map[board.Point]bool{}) // fall back to "nothing dead"; players can mark by hand
		return
	}
	dead := map[board.Point]bool{}
	for _, gr := range res.Groups {
		if gr.Status == string(katago.Dead) {
			for _, st := range gr.Stones {
				dead[board.Point{X: st.X, Y: st.Y}] = true
			}
		}
	}
	g.SetAutoDead(dead)
}

// aiMove plays KataGo's choice for color. It reports whether a move was made.
func (s *Server) aiMove(g *session.Game, color board.Color, visits int, version uint64) bool {
	kg := s.katago()
	if kg == nil {
		return false
	}
	if visits <= 0 {
		visits = 200
	}
	var q katago.Query
	var size int
	var qerr error
	var aiID, rank string
	var pos *board.Position
	var rules board.Rules
	g.Lock(func() {
		q, qerr = katago.FromTree(g.Tree, g.Cursor)
		size = g.Tree.Size
		aiID = g.Players[color].UserID
		rank = g.Players[color].AIRank
		rules = g.Tree.Rules
		pos, _ = g.Tree.Position(g.Cursor)
	})
	if qerr != nil || pos == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(s.ctx, 60*time.Second)
	defer cancel()

	if rank != "" {
		// A human-style opponent: sample the move a player of this rank would make.
		p, err := kg.HumanMove(ctx, q, rank, func(p board.Point) bool { return pos.Legal(rules, color, p) }, nil)
		if err != nil {
			log.Printf("ai move (%s) %s: %v", rank, g.ID, err)
			return false
		}
		if g.Version() != version {
			return true
		}
		if _, err := g.Play(aiID, p, 0); err != nil {
			log.Printf("ai move %s: %v", g.ID, err)
			return false
		}
		return true
	}

	q.MaxVisits = visits
	resp, err := kg.Analyze(ctx, q)
	if err != nil {
		log.Printf("ai move %s: %v", g.ID, err)
		return false
	}
	if g.Version() != version {
		return true // the position changed under us; let the next round decide
	}
	// Try KataGo's choices in order until one is legal under our rules.
	for _, m := range resp.MoveInfos {
		p, err := board.ParseGTP(m.Move, size)
		if err != nil {
			continue
		}
		if _, err := g.Play(aiID, p, 0); err == nil {
			return true
		}
	}
	// Nothing legal suggested: pass rather than stall the game.
	if _, err := g.Play(aiID, board.Pass, 0); err != nil {
		log.Printf("ai pass %s: %v", g.ID, err)
		return false
	}
	return true
}
