package shapes

import (
	"fmt"

	"easygo/internal/board"
)

// ---- Snapback ----------------------------------------------------------
//
// A snapback is a sacrifice: the mover plays a stone that has exactly one
// liberty, the opponent captures it, and the mover then recaptures a larger
// group by replaying on the vacated point. It is checked by playing the
// three plies out on the real board.

type snapbackDetector struct{}

func (snapbackDetector) Name() string { return "snapback" }

func (snapbackDetector) Detect(pos *board.Position, r board.Rules) []Finding {
	b := pos.Board
	size := b.Size
	me := pos.ToPlay
	var out []Finding
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			m := board.Point{X: x, Y: y}
			if b.At(m) != board.Empty {
				continue
			}
			if gain, ok := snapbackGain(pos, r, me, m); ok {
				mv := m
				out = append(out, Finding{
					Kind: "snapback", Result: "works", Color: me, Move: &mv,
					Points: []board.Point{m}, Confidence: Bounded,
					Text: fmt.Sprintf("Snapback: sacrifice at %s, then recapture %d more stone(s) than you lose.", m.GTP(size), gain),
				})
			}
		}
	}
	return out
}

// snapbackGain plays me at m, the opponent captures, me retakes. It
// returns the net stone gain if it is positive.
func snapbackGain(pos *board.Position, r board.Rules, me board.Color, m board.Point) (int, bool) {
	p1, err := pos.Play(r, me, m)
	if err != nil {
		return 0, false
	}
	// The sacrificed stone group must be in atari and must not capture anything,
	// otherwise this is an ordinary capture.
	if p1.Captures[me] != pos.Captures[me] {
		return 0, false
	}
	stones, libs := p1.Board.Group(m)
	if len(libs) != 1 {
		return 0, false
	}
	opp := me.Opp()
	p2, err := p1.Play(r, opp, libs[0])
	if err != nil || p2.Board.At(m) != board.Empty {
		return 0, false // opponent cannot capture the sacrifice
	}
	lost := p2.Captures[opp] - p1.Captures[opp]
	if lost != len(stones) {
		return 0, false
	}
	// The capturing stone's group should now be in atari at a vacated point.
	cg, cl := p2.Board.Group(libs[0])
	if len(cl) != 1 {
		return 0, false
	}
	p3, err := p2.Play(r, me, cl[0])
	if err != nil {
		return 0, false
	}
	won := p3.Captures[me] - p2.Captures[me]
	_ = cg
	if won > lost {
		return won - lost, true
	}
	return 0, false
}

// ---- Net (geta) --------------------------------------------------------
//
// A net is a quiet move that does not atari anything yet leaves a target
// group unable to escape. It is found by a depth-limited AND/OR search for
// the capture. The search is local and bounded, so a "works" here means no
// escape was found within the horizon; ko and seki are not read out, and
// the finding is flagged Bounded.

const (
	netDepth    = 7
	netNodeCap  = 60000
	netMaxStone = 4
	netMaxLibs  = 6
)

type netDetector struct{}

func (netDetector) Name() string { return "net" }

type netSearch struct {
	r      board.Rules
	att    board.Color
	def    board.Color
	target board.Point
	nodes  int
}

func (netDetector) Detect(pos *board.Position, r board.Rules) []Finding {
	b := pos.Board
	size := b.Size
	me := pos.ToPlay
	var out []Finding
	budget := 4 * netNodeCap
	for _, g := range b.Groups() {
		if g.Color == me || len(g.Stones) > netMaxStone || len(g.Liberties) < 3 || len(g.Liberties) > netMaxLibs {
			continue
		}
		if budget <= 0 {
			break
		}
		// Candidate nets: empty points near the group.
		cands := map[board.Point]bool{}
		for _, s := range g.Stones {
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					q := board.Point{X: s.X + dx, Y: s.Y + dy}
					if b.InBounds(q) && b.At(q) == board.Empty {
						cands[q] = true
					}
				}
			}
		}
		for m := range cands {
			p1, err := pos.Play(r, me, m)
			if err != nil || p1.Board.At(g.Stones[0]) != g.Color {
				continue
			}
			if p1.Board.Liberties(g.Stones[0]) < 2 {
				continue // that is an atari, not a net
			}
			s := &netSearch{r: r, att: me, def: g.Color, target: g.Stones[0]}
			escapes := s.defend(p1, netDepth)
			budget -= s.nodes
			if !escapes && s.nodes < netNodeCap {
				mv := m
				out = append(out, Finding{
					Kind: "net", Result: "works", Color: me, Move: &mv,
					Points: g.Stones, Confidence: Bounded,
					Text: fmt.Sprintf("Net: %s traps the %s group at %s with no escape within %d plies.",
						m.GTP(size), g.Color, g.Stones[0].GTP(size), netDepth),
				})
			}
		}
	}
	return out
}

// attack reports whether the attacker (to move) captures the target within
// depth plies.
func (s *netSearch) attack(pos *board.Position, depth int) bool {
	s.nodes++
	if s.nodes > netNodeCap {
		return false
	}
	if pos.Board.At(s.target) != s.def {
		return true
	}
	if depth == 0 {
		return false
	}
	for _, m := range s.moves(pos, s.att) {
		np, err := pos.Play(s.r, s.att, m)
		if err != nil {
			continue
		}
		if !s.defend(np, depth-1) {
			return true
		}
	}
	return false
}

// defend reports whether the defender (to move) can avoid capture
// within the horizon. Unproven positions count as escapes.
func (s *netSearch) defend(pos *board.Position, depth int) bool {
	s.nodes++
	if s.nodes > netNodeCap {
		return true
	}
	if pos.Board.At(s.target) != s.def {
		return false
	}
	_, libs := pos.Board.Group(s.target)
	if len(libs) >= netMaxLibs {
		return true
	}
	if depth == 0 {
		return true
	}
	for _, m := range s.moves(pos, s.def) {
		np, err := pos.Play(s.r, s.def, m)
		if err != nil {
			continue
		}
		if !s.attack(np, depth-1) {
			return true
		}
	}
	return false
}

// moves are the local candidates: liberties of the target, empty points
// next to them, and the liberty of any adjacent enemy group in atari.
func (s *netSearch) moves(pos *board.Position, color board.Color) []board.Point {
	b := pos.Board
	stones, libs := b.Group(s.target)
	seen := map[board.Point]bool{}
	var out []board.Point
	add := func(p board.Point) {
		if !seen[p] && b.At(p) == board.Empty {
			seen[p] = true
			out = append(out, p)
		}
	}
	for _, l := range libs {
		add(l)
		for _, n := range b.Neighbors(l) {
			add(n)
		}
	}
	for _, st := range stones {
		for _, n := range b.Neighbors(st) {
			if b.At(n) == board.Empty || b.At(n) == s.def {
				continue
			}
			if _, al := b.Group(n); len(al) == 1 {
				add(al[0])
			}
		}
	}
	return out
}

func init() {
	Register(snapbackDetector{})
	Register(netDetector{})
}
