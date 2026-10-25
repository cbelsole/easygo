package katago

import (
	"fmt"

	"easygo/internal/board"
	"easygo/internal/tree"
)

// Query is one analysis request. Values are reported from Black's point of
// view because the config sets reportAnalysisWinratesAs = BLACK; that is
// what the Response types below assume.
type Query struct {
	ID            string
	Size          int
	Rules         board.Rules
	InitialStones [][2]string // ["B","D4"]
	InitialPlayer string
	Moves         [][2]string
	MaxVisits     int
	Ownership     bool
	Policy        bool
	// HumanProfile, e.g. "rank_16k", makes KataGo report what a human of that
	// rank would play (needs the human model). HumanExplore is the share of
	// the search spent examining those human moves, which lets their values
	// be judged without biasing the evaluation of the position.
	HumanProfile string
	HumanExplore float64
	// ReportEvery asks for partial results every n seconds (0 = off).
	ReportEvery float64
}

func (q Query) toMap() map[string]any {
	m := map[string]any{
		"moves":        q.Moves,
		"rules":        RulesJSON(q.Rules),
		"komi":         q.Rules.Komi,
		"boardXSize":   q.Size,
		"boardYSize":   q.Size,
		"analyzeTurns": []int{len(q.Moves)},
	}
	if q.ID != "" {
		m["id"] = q.ID
	}
	if len(q.InitialStones) > 0 {
		m["initialStones"] = q.InitialStones
	}
	if q.InitialPlayer != "" {
		m["initialPlayer"] = q.InitialPlayer
	}
	if q.MaxVisits > 0 {
		m["maxVisits"] = q.MaxVisits
	}
	if q.Ownership {
		m["includeOwnership"] = true
	}
	if q.Policy {
		m["includePolicy"] = true
	}
	if q.HumanProfile != "" {
		os := map[string]any{"humanSLProfile": q.HumanProfile, "ignorePreRootHistory": false, "rootNumSymmetriesToSample": 2}
		if q.HumanExplore > 0 {
			os["humanSLRootExploreProbWeightless"] = q.HumanExplore
		}
		m["overrideSettings"] = os
	}
	if q.ReportEvery > 0 {
		m["reportDuringSearchEvery"] = q.ReportEvery
	}
	return m
}

// RulesJSON converts engine rules to KataGo's rules object.
func RulesJSON(r board.Rules) map[string]any {
	return map[string]any{
		"ko":                 string(r.Ko),
		"scoring":            string(r.Scoring),
		"tax":                string(r.Tax),
		"suicide":            r.MultiStoneSuicide,
		"hasButton":          r.HasButton,
		"whiteHandicapBonus": r.WhiteHandicapBonus,
		"friendlyPassOk":     true,
	}
}

func colorLetter(c board.Color) string {
	if c == board.White {
		return "W"
	}
	return "B"
}

// FromTree builds the query for the position after node n. Only root setup
// stones can be sent (KataGo's initialStones); setup on later nodes is an
// error, since silently dropping it would analyze the wrong position.
func FromTree(t *tree.Tree, n *tree.Node) (Query, error) {
	q := Query{Size: t.Size, Rules: t.Rules, Moves: [][2]string{}}
	for i, node := range t.Path(n) {
		if len(node.Setup) > 0 {
			if i != 0 {
				return q, fmt.Errorf("katago: setup stones after the root are not supported")
			}
			for p, c := range node.Setup {
				if c != board.Empty {
					q.InitialStones = append(q.InitialStones, [2]string{colorLetter(c), p.GTP(t.Size)})
				}
			}
		}
		if i == 0 && (node.ToPlay != board.Empty) {
			q.InitialPlayer = colorLetter(node.ToPlay)
		}
		if i == 0 && t.Info.Handicap > 1 && q.InitialPlayer == "" {
			q.InitialPlayer = "W"
		}
		if node.HasMove {
			q.Moves = append(q.Moves, [2]string{colorLetter(node.Color), node.Move.GTP(t.Size)})
		}
	}
	return q, nil
}

// Response is one analysis answer. Winrate and score are Black's.
type Response struct {
	ID             string     `json:"id"`
	IsDuringSearch bool       `json:"isDuringSearch"`
	TurnNumber     int        `json:"turnNumber"`
	MoveInfos      []MoveInfo `json:"moveInfos"`
	RootInfo       RootInfo   `json:"rootInfo"`
	Ownership      []float64  `json:"ownership"`
	Policy         []float64  `json:"policy"`
	// HumanPolicy is the human-imitation policy: size*size points then pass,
	// -1 for illegal moves. Present when the query asked for a human profile.
	HumanPolicy []float64 `json:"humanPolicy"`
	Error       string    `json:"error"`
	Version     string    `json:"version"`
}

type MoveInfo struct {
	Move       string   `json:"move"`
	Order      int      `json:"order"`
	Visits     int      `json:"visits"`
	Winrate    float64  `json:"winrate"`
	ScoreLead  float64  `json:"scoreLead"`
	Prior      float64  `json:"prior"`
	HumanPrior float64  `json:"humanPrior"`
	Utility    float64  `json:"utility"`
	PV         []string `json:"pv"`
}

type RootInfo struct {
	CurrentPlayer string  `json:"currentPlayer"`
	Winrate       float64 `json:"winrate"`
	ScoreLead     float64 `json:"scoreLead"`
	Visits        int     `json:"visits"`
}

// OwnershipAt returns ownership at p: +1 is Black's, -1 White's.
func (r *Response) OwnershipAt(size int, p board.Point) float64 {
	i := p.Y*size + p.X
	if i < 0 || i >= len(r.Ownership) {
		return 0
	}
	return r.Ownership[i]
}
