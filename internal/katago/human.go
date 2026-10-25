package katago

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"regexp"
	"strconv"

	"easygo/internal/board"
)

// ErrNoHumanModel means a rank-based opponent was requested but KataGo was
// started without the human-imitation model.
var ErrNoHumanModel = errors.New("the human-style model is not installed")

var rankRE = regexp.MustCompile(`^(20|1[0-9]|[1-9])k$|^[1-9]d$`)

// ValidRank reports whether r is a rank the human model can imitate:
// 20k down to 1k, then 1d up to 9d.
func ValidRank(r string) bool { return rankRE.MatchString(r) }

// Ranks lists every supported rank, weakest first.
func Ranks() []string {
	var out []string
	for k := 20; k >= 1; k-- {
		out = append(out, strconv.Itoa(k)+"k")
	}
	for d := 1; d <= 9; d++ {
		out = append(out, strconv.Itoa(d)+"d")
	}
	return out
}

// IsDan reports whether r is a dan rank.
func IsDan(r string) bool { return len(r) > 0 && r[len(r)-1] == 'd' }

// The human policy alone plays like the imitated rank for kyu players. For
// dan ranks the raw policy plays weaker than the players it imitates (KataGo's
// own docs say so), so the search's judgment is mixed in: moves are weighted
// by humanPrior * exp(utility/utilityScale), which keeps human-looking moves
// but suppresses the ones that lose a lot.
const (
	kyuVisits    = 50
	danVisits    = 150
	danExplore   = 0.5
	utilityScale = 0.5
)

// HumanMove picks a move the way a human of the given rank would play it.
// legal filters candidates under the game's own rules, since KataGo's idea
// of legality can differ in corner cases. A nil rng uses the global source.
func (c *Client) HumanMove(ctx context.Context, q Query, rank string, legal func(board.Point) bool, rng *rand.Rand) (board.Point, error) {
	if !ValidRank(rank) {
		return board.Pass, fmt.Errorf("unknown rank %q", rank)
	}
	if !c.HasHuman {
		return board.Pass, ErrNoHumanModel
	}
	q.HumanProfile = "rank_" + rank
	q.Policy = true
	q.MaxVisits = kyuVisits
	if IsDan(rank) {
		q.MaxVisits, q.HumanExplore = danVisits, danExplore
	}
	resp, err := c.Analyze(ctx, q)
	if err != nil {
		return board.Pass, err
	}
	return PickHumanMove(resp, q.Size, IsDan(rank), legal, rng), nil
}

// PickHumanMove chooses from a response. It is separate from the KataGo call
// so the choice itself can be tested.
func PickHumanMove(resp *Response, size int, weighted bool, legal func(board.Point) bool, rng *rand.Rand) board.Point {
	// The engine's own top choice decides passing: a human-style policy is not
	// reliable about when a game is over.
	if len(resp.MoveInfos) > 0 && resp.MoveInfos[0].Move == "pass" {
		return board.Pass
	}
	if rng == nil {
		rng = rand.New(rand.NewSource(rand.Int63()))
	}
	type cand struct {
		p board.Point
		w float64
	}
	var cands []cand

	if weighted && len(resp.MoveInfos) > 0 {
		best := math.Inf(-1)
		for _, m := range resp.MoveInfos {
			best = math.Max(best, m.Utility)
		}
		for _, m := range resp.MoveInfos {
			p, err := board.ParseGTP(m.Move, size)
			if err != nil || p.IsPass() || m.HumanPrior <= 0 || (legal != nil && !legal(p)) {
				continue
			}
			// Utility is measured from the best move so the exponent never overflows.
			cands = append(cands, cand{p, m.HumanPrior * math.Exp((m.Utility-best)/utilityScale)})
		}
	}
	if len(cands) == 0 {
		for i := 0; i < size*size && i < len(resp.HumanPolicy); i++ {
			w := resp.HumanPolicy[i]
			if w <= 0 { // -1 marks an illegal move
				continue
			}
			p := board.Point{X: i % size, Y: i / size}
			if legal != nil && !legal(p) {
				continue
			}
			cands = append(cands, cand{p, w})
		}
	}
	if len(cands) == 0 {
		// Nothing the human model likes is playable: fall back to the engine's choice.
		for _, m := range resp.MoveInfos {
			if p, err := board.ParseGTP(m.Move, size); err == nil && !p.IsPass() && (legal == nil || legal(p)) {
				return p
			}
		}
		return board.Pass
	}

	var total float64
	for _, c := range cands {
		total += c.w
	}
	r := rng.Float64() * total
	for _, c := range cands {
		if r < c.w {
			return c.p
		}
		r -= c.w
	}
	return cands[len(cands)-1].p
}
