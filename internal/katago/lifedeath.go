package katago

import (
	"math"

	"easygo/internal/board"
)

// Status is the life-and-death verdict for one group.
type Status string

const (
	Alive     Status = "alive"
	Dead      Status = "dead"
	Seki      Status = "seki"      // both sides' stones stay on, ownership near zero
	Unsettled Status = "unsettled" // ownership is in between: the fight is not over
)

// Strength grades a living group, the "strong / weak" indicator.
type Strength string

const (
	Strong   Strength = "strong"
	Moderate Strength = "moderate"
	Weak     Strength = "weak"
)

// GroupJudgment is the verdict for one chain.
type GroupJudgment struct {
	Stones   []board.Point `json:"stones"`
	Color    board.Color   `json:"color"`
	Status   Status        `json:"status"`
	Strength Strength      `json:"strength"`
	// Own is the group's mean ownership from its own side's view:
	// +1 certainly its own area, -1 certainly the opponent's.
	Own float64 `json:"own"`
}

// Thresholds on mean own-side ownership. A group whose area KataGo gives to
// the opponent is dead; one it keeps is alive; the band between is a fight.
const (
	aliveAt  = 0.65
	deadAt   = -0.65
	strongAt = 0.90
	sekiBand = 0.25
)

// Judge classifies every group on the board from an ownership map
// (+1 Black, -1 White, row-major from the top). It aggregates per group
// instead of looking at single points, because one point inside a living
// shape can briefly read as contested.
func Judge(b *board.Board, ownership []float64) []GroupJudgment {
	if len(ownership) != b.Size*b.Size {
		return nil
	}
	var out []GroupJudgment
	for _, g := range b.Groups() {
		sign := 1.0
		if g.Color == board.White {
			sign = -1.0
		}
		var sum float64
		for _, s := range g.Stones {
			sum += ownership[s.Y*b.Size+s.X] * sign
		}
		own := sum / float64(len(g.Stones))
		j := GroupJudgment{Stones: g.Stones, Color: g.Color, Own: own}
		switch {
		case own >= aliveAt:
			j.Status = Alive
		case own <= deadAt:
			j.Status = Dead
		case math.Abs(own) <= sekiBand && sharesLibertyWithEnemy(b, g):
			j.Status = Seki
		default:
			j.Status = Unsettled
		}
		if j.Status == Alive || j.Status == Seki {
			switch {
			case own >= strongAt:
				j.Strength = Strong
			case own >= aliveAt+0.1:
				j.Strength = Moderate
			default:
				j.Strength = Weak
			}
		} else {
			j.Strength = Weak
		}
		out = append(out, j)
	}
	return out
}

// sharesLibertyWithEnemy is the structural half of the seki test: a seki
// group shares a liberty with an enemy group. Ownership alone cannot tell a
// seki from a coin-flip fight.
func sharesLibertyWithEnemy(b *board.Board, g board.GroupInfo) bool {
	for _, l := range g.Liberties {
		for _, n := range b.Neighbors(l) {
			if c := b.At(n); c != board.Empty && c != g.Color {
				return true
			}
		}
	}
	return false
}

// DeadStones lists the stones of every group judged dead, as input for
// board.Score. overrides flips a group's verdict: key is any stone of the
// group, value true forces dead and false forces alive.
func DeadStones(js []GroupJudgment, overrides map[board.Point]bool) map[board.Point]bool {
	dead := map[board.Point]bool{}
	for _, j := range js {
		isDead := j.Status == Dead
		for _, s := range j.Stones {
			if v, ok := overrides[s]; ok {
				isDead = v
				break
			}
		}
		if isDead {
			for _, s := range j.Stones {
				dead[s] = true
			}
		}
	}
	return dead
}

// Territory turns ownership into a per-point owner for the overlay.
// Points below the confidence threshold are left neutral.
func Territory(ownership []float64, confidence float64) []board.Color {
	out := make([]board.Color, len(ownership))
	for i, o := range ownership {
		switch {
		case o >= confidence:
			out[i] = board.Black
		case o <= -confidence:
			out[i] = board.White
		}
	}
	return out
}
