// Package shapes detects named tactical shapes on a position. Each shape is
// a Detector registered by name, so adding one touches nothing else.
package shapes

import (
	"sort"
	"sync"

	"easygo/internal/board"
)

// Confidence says how far a result can be trusted.
const (
	Exact   = "exact"   // the whole claim was read out on the real board
	Bounded = "bounded" // a depth-limited local search; ko and seki can change it
)

// Finding is one shape the sensor noticed.
type Finding struct {
	Kind       string        `json:"kind"`   // ladder, net, snapback, knight, large-knight
	Result     string        `json:"result"` // works, broken, unknown, or "" for pure geometry
	Color      board.Color   `json:"color"`  // the side the shape belongs to / favors
	Move       *board.Point  `json:"move,omitempty"`
	Points     []board.Point `json:"points"`            // stones or points to highlight
	Breaker    *board.Point  `json:"breaker,omitempty"` // ladder: the stone that saves the defender
	Confidence string        `json:"confidence"`
	Text       string        `json:"text"`
}

// Detector finds one kind of shape in a position.
type Detector interface {
	Name() string
	Detect(pos *board.Position, r board.Rules) []Finding
}

var (
	mu       sync.RWMutex
	registry = map[string]Detector{}
)

// Register adds a detector. A later registration under the same name wins.
func Register(d Detector) {
	mu.Lock()
	defer mu.Unlock()
	registry[d.Name()] = d
}

// Names lists registered detectors alphabetically.
func Names() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(registry))
	for n := range registry {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Detect runs the named detectors, or all of them if names is empty.
func Detect(pos *board.Position, r board.Rules, names ...string) []Finding {
	mu.RLock()
	var ds []Detector
	if len(names) == 0 {
		for _, d := range registry {
			ds = append(ds, d)
		}
	} else {
		for _, n := range names {
			if d, ok := registry[n]; ok {
				ds = append(ds, d)
			}
		}
	}
	mu.RUnlock()
	sort.Slice(ds, func(i, j int) bool { return ds[i].Name() < ds[j].Name() })
	var out []Finding
	for _, d := range ds {
		out = append(out, d.Detect(pos, r)...)
	}
	return out
}

func ptr(p board.Point) *board.Point { return &p }
