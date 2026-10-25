// Package clock is the per-game timer: main time plus Japanese byo-yomi.
// Time is injected so tests need no sleeping, and the server owns the clock;
// clients only display snapshots.
package clock

import (
	"sync"
	"time"

	"easygo/internal/board"
)

// Config is set per game. Enabled=false turns the whole clock off.
// MainSeconds=0 means no main time; Periods=0 means no byo-yomi. With both
// zero (or Enabled false) the game is untimed.
type Config struct {
	Enabled       bool `json:"enabled"`
	MainSeconds   int  `json:"mainSeconds"`
	Periods       int  `json:"periods"`
	PeriodSeconds int  `json:"periodSeconds"`
}

func (c Config) timed() bool {
	return c.Enabled && (c.MainSeconds > 0 || (c.Periods > 0 && c.PeriodSeconds > 0))
}

// Side is one player's remaining time.
type Side struct {
	Main    time.Duration `json:"-"`
	Periods int           `json:"periods"` // periods left, counting the one in progress
	Period  time.Duration `json:"-"`       // time left in the current period
	InByo   bool          `json:"inByo"`
	Flagged bool          `json:"flagged"`
}

// View is the JSON a client displays.
type View struct {
	Enabled   bool    `json:"enabled"`
	MainMs    int64   `json:"mainMs"`
	PeriodMs  int64   `json:"periodMs"`
	Periods   int     `json:"periods"`
	InByo     bool    `json:"inByo"`
	Flagged   bool    `json:"flagged"`
	Running   bool    `json:"running"`
	ByoActive bool    `json:"byoConfigured"`
	Config    *Config `json:"config,omitempty"`
}

type Clock struct {
	mu      sync.Mutex
	cfg     Config
	sides   [3]Side
	running board.Color // Empty when stopped
	since   time.Time
	now     func() time.Time
}

// New creates a clock. Pass nil for now to use the wall clock.
func New(cfg Config, now func() time.Time) *Clock {
	if now == nil {
		now = time.Now
	}
	c := &Clock{cfg: cfg, now: now}
	for _, col := range []board.Color{board.Black, board.White} {
		s := Side{Main: time.Duration(cfg.MainSeconds) * time.Second}
		if cfg.Periods > 0 && cfg.PeriodSeconds > 0 {
			s.Periods = cfg.Periods
		}
		c.sides[col] = s
		if cfg.timed() && cfg.MainSeconds == 0 {
			c.enterByo(&c.sides[col])
		}
	}
	return c
}

func (c *Clock) byoConfigured() bool { return c.cfg.Periods > 0 && c.cfg.PeriodSeconds > 0 }

func (c *Clock) enterByo(s *Side) {
	if !c.byoConfigured() {
		s.Flagged = true
		return
	}
	s.InByo = true
	s.Period = time.Duration(c.cfg.PeriodSeconds) * time.Second
}

// burn spends elapsed time on a side, following byo-yomi rules.
func (c *Clock) burn(s *Side, e time.Duration) {
	if s.Flagged || e <= 0 {
		return
	}
	if !s.InByo {
		if e <= s.Main {
			s.Main -= e
			return
		}
		e -= s.Main
		s.Main = 0
		c.enterByo(s)
		if s.Flagged {
			return
		}
	}
	full := time.Duration(c.cfg.PeriodSeconds) * time.Second
	for e > 0 {
		if e < s.Period {
			s.Period -= e
			return
		}
		e -= s.Period
		s.Periods--
		if s.Periods <= 0 {
			s.Periods = 0
			s.Period = 0
			s.Flagged = true
			return
		}
		s.Period = full
	}
}

// settle applies time elapsed since the running side started.
func (c *Clock) settle() {
	if c.running == board.Empty {
		return
	}
	t := c.now()
	c.burn(&c.sides[c.running], t.Sub(c.since))
	c.since = t
}

// Start begins counting down color's time. A no-op if untimed.
func (c *Clock) Start(color board.Color) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.cfg.timed() {
		return
	}
	c.settle()
	c.running = color
	c.since = c.now()
}

// Switch ends the running side's turn and starts the other's. A side in
// byo-yomi gets its full period back; the period is only consumed if it
// actually expired.
func (c *Clock) Switch() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.cfg.timed() || c.running == board.Empty {
		return
	}
	c.settle()
	s := &c.sides[c.running]
	if s.InByo && !s.Flagged {
		s.Period = time.Duration(c.cfg.PeriodSeconds) * time.Second
	}
	c.running = c.running.Opp()
	c.since = c.now()
}

// Stop freezes the clock (game over or paused).
func (c *Clock) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.settle()
	c.running = board.Empty
}

// Flagged returns the color that ran out of time, or Empty.
func (c *Clock) Flagged() board.Color {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.settle()
	for _, col := range []board.Color{board.Black, board.White} {
		if c.sides[col].Flagged {
			return col
		}
	}
	return board.Empty
}

// Deadline is when the running side will flag if it does not move, so the
// server can schedule a timeout. ok is false when nothing is counting down.
func (c *Clock) Deadline() (t time.Time, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.cfg.timed() || c.running == board.Empty {
		return time.Time{}, false
	}
	c.settle()
	s := c.sides[c.running]
	if s.Flagged {
		return c.now(), true
	}
	left := s.Main
	if s.InByo {
		left = s.Period + time.Duration(s.Periods-1)*time.Duration(c.cfg.PeriodSeconds)*time.Second
	} else if c.byoConfigured() {
		left += time.Duration(s.Periods) * time.Duration(c.cfg.PeriodSeconds) * time.Second
	}
	return c.now().Add(left), true
}

// Snapshot is the displayable state of one side.
func (c *Clock) Snapshot(color board.Color) View {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.settle()
	s := c.sides[color]
	cfg := c.cfg
	return View{
		Enabled: c.cfg.timed(), MainMs: s.Main.Milliseconds(), PeriodMs: s.Period.Milliseconds(),
		Periods: s.Periods, InByo: s.InByo, Flagged: s.Flagged,
		Running: c.running == color, ByoActive: c.byoConfigured(), Config: &cfg,
	}
}

// State is a clock's persisted per-side remainder, in milliseconds.
type State struct {
	MainMs   int64 `json:"mainMs"`
	PeriodMs int64 `json:"periodMs"`
	Periods  int   `json:"periods"`
	InByo    bool  `json:"inByo"`
	Flagged  bool  `json:"flagged"`
}

// Export captures both sides so a restart can resume the game.
func (c *Clock) Export() [2]State {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.settle()
	var out [2]State
	for i, col := range []board.Color{board.Black, board.White} {
		s := c.sides[col]
		out[i] = State{s.Main.Milliseconds(), s.Period.Milliseconds(), s.Periods, s.InByo, s.Flagged}
	}
	return out
}

// Import restores remainders saved by Export. The clock stays stopped.
func (c *Clock) Import(st [2]State) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.running = board.Empty
	for i, col := range []board.Color{board.Black, board.White} {
		s := st[i]
		c.sides[col] = Side{
			Main: time.Duration(s.MainMs) * time.Millisecond, Period: time.Duration(s.PeriodMs) * time.Millisecond,
			Periods: s.Periods, InByo: s.InByo, Flagged: s.Flagged,
		}
	}
}
