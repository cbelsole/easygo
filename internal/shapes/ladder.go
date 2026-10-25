package shapes

import (
	"fmt"

	"easygo/internal/board"
)

// LadderResult is the outcome of reading out a chase.
type LadderResult struct {
	Outcome string        // works (attacker captures), broken (defender escapes), unknown
	Start   board.Point   // attacker's first atari
	Path    []board.Point // principal line, alternating attacker/defender
	Breaker *board.Point  // defender stone that joined the chased group, if any
}

const ladderNodeCap = 40000

type ladderReader struct {
	r        board.Rules
	att, def board.Color
	target   board.Point
	original map[board.Point]bool // defender stones on the board at the start
	nodes    int
	capped   bool
}

// ReadLadder plays out the chase of the group at target by attacker, on the
// real board. Because the forced sequence is played with real captures and
// real liberties, ladder breakers, escapes by capture and counter-ataris
// need no special cases: a breaker is just a friendly stone the group
// connects to, and the liberty count shows it.
//
// The group must have exactly two liberties. Anything else returns unknown.
func ReadLadder(pos *board.Position, r board.Rules, target board.Point, attacker board.Color) LadderResult {
	b := pos.Board
	if b.At(target) == board.Empty || b.At(target) == attacker {
		return LadderResult{Outcome: "unknown"}
	}
	if b.Liberties(target) != 2 {
		return LadderResult{Outcome: "unknown"}
	}
	rd := &ladderReader{
		r: r, att: attacker, def: attacker.Opp(), target: target,
		original: map[board.Point]bool{},
	}
	for _, g := range b.Groups() {
		if g.Color == rd.def {
			for _, s := range g.Stones {
				rd.original[s] = true
			}
		}
	}
	start := pos.WithSetup(nil, attacker)
	captured, line := rd.attack(start, 0)
	res := LadderResult{Path: line}
	if len(line) > 0 {
		res.Start = line[0]
	}
	switch {
	case captured:
		res.Outcome = "works"
	case rd.capped:
		res.Outcome = "unknown"
	default:
		res.Outcome = "broken"
		res.Breaker = rd.findBreaker(start, line)
	}
	return res
}

// attack: attacker to move with the target group on two liberties.
func (rd *ladderReader) attack(pos *board.Position, depth int) (bool, []board.Point) {
	rd.nodes++
	if rd.nodes > ladderNodeCap || depth > pos.Board.Size*6 {
		rd.capped = true
		return false, nil
	}
	if pos.Board.At(rd.target) != rd.def {
		return true, nil // already captured
	}
	stones, libs := pos.Board.Group(rd.target)
	_ = stones
	switch {
	case len(libs) == 1:
		np, err := pos.Play(rd.r, rd.att, libs[0])
		if err == nil && np.Board.At(rd.target) == board.Empty {
			return true, []board.Point{libs[0]}
		}
		return false, nil
	case len(libs) != 2:
		return false, nil // escaped (3+ liberties)
	}

	var bestFail []board.Point
	for _, l := range libs {
		np, err := pos.Play(rd.r, rd.att, l)
		if err != nil {
			continue
		}
		won, line := rd.defend(np, depth+1)
		if won {
			return true, append([]board.Point{l}, line...)
		}
		if len(line)+1 > len(bestFail) {
			bestFail = append([]board.Point{l}, line...)
		}
	}
	return false, bestFail
}

// defend: defender to move with the target in atari. Returns whether the
// attacker still captures.
func (rd *ladderReader) defend(pos *board.Position, depth int) (bool, []board.Point) {
	rd.nodes++
	if rd.nodes > ladderNodeCap {
		rd.capped = true
		return false, nil
	}
	if pos.Board.At(rd.target) != rd.def {
		return true, nil
	}
	stones, libs := pos.Board.Group(rd.target)
	if len(libs) != 1 {
		return false, nil
	}
	// Defender's options: extend, or capture an attacker group in atari
	// that touches the chased group.
	cands := []board.Point{libs[0]}
	seen := map[board.Point]bool{libs[0]: true}
	for _, s := range stones {
		for _, n := range pos.Board.Neighbors(s) {
			if pos.Board.At(n) != rd.att {
				continue
			}
			_, al := pos.Board.Group(n)
			if len(al) == 1 && !seen[al[0]] {
				seen[al[0]] = true
				cands = append(cands, al[0])
			}
		}
	}

	var bestFail []board.Point
	anyLegal := false
	for _, c := range cands {
		np, err := pos.Play(rd.r, rd.def, c)
		if err != nil {
			continue
		}
		anyLegal = true
		won, line := rd.attack(np, depth+1)
		if !won {
			return false, append([]board.Point{c}, line...) // defender found an escape
		}
		if len(line)+1 > len(bestFail) {
			bestFail = append([]board.Point{c}, line...)
		}
	}
	if !anyLegal {
		return true, nil // no legal reply: the group dies
	}
	return true, bestFail
}

// findBreaker replays the failing line and reports the defender stone that
// the chased group connected to.
func (rd *ladderReader) findBreaker(start *board.Position, line []board.Point) *board.Point {
	pos := start
	color := rd.att
	for _, p := range line {
		np, err := pos.Play(rd.r, color, p)
		if err != nil {
			return nil
		}
		pos = np
		color = color.Opp()
	}
	if pos.Board.At(rd.target) != rd.def {
		return nil
	}
	stones, _ := pos.Board.Group(rd.target)
	// Stones of the original group, found on the starting board.
	orig, _ := start.Board.Group(rd.target)
	inOrig := map[board.Point]bool{}
	for _, s := range orig {
		inOrig[s] = true
	}
	for _, s := range stones {
		if rd.original[s] && !inOrig[s] {
			return ptr(s)
		}
	}
	return nil
}

type ladderDetector struct{}

func (ladderDetector) Name() string { return "ladder" }

// Detect reports every two-liberty group that a chase could pursue for at
// least two ataris. Single-step captures are plain ataris, not ladders.
func (ladderDetector) Detect(pos *board.Position, r board.Rules) []Finding {
	var out []Finding
	for _, g := range pos.Board.Groups() {
		if len(g.Liberties) != 2 {
			continue
		}
		att := g.Color.Opp()
		res := ReadLadder(pos, r, g.Stones[0], att)
		if res.Outcome == "unknown" || len(res.Path) < 4 {
			continue
		}
		size := pos.Board.Size
		f := Finding{
			Kind: "ladder", Result: res.Outcome, Color: att,
			Move: ptr(res.Start), Points: g.Stones, Breaker: res.Breaker,
			Confidence: Exact,
		}
		switch res.Outcome {
		case "works":
			f.Text = fmt.Sprintf("Ladder: atari at %s captures the %s group.", res.Start.GTP(size), g.Color)
		default:
			f.Text = fmt.Sprintf("Ladder is broken: the %s group escapes.", g.Color)
			if res.Breaker != nil {
				f.Text = fmt.Sprintf("Ladder is broken by the %s stone at %s.", g.Color, res.Breaker.GTP(size))
			}
		}
		out = append(out, f)
	}
	return out
}

func init() { Register(ladderDetector{}) }
