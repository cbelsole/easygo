package clock

import (
	"testing"
	"time"

	"easygo/internal/board"
)

type fake struct{ t time.Time }

func (f *fake) now() time.Time          { return f.t }
func (f *fake) advance(d time.Duration) { f.t = f.t.Add(d) }

func newFake() *fake { return &fake{t: time.Unix(1_000_000, 0)} }

func TestMainTimeCountsDown(t *testing.T) {
	f := newFake()
	c := New(Config{Enabled: true, MainSeconds: 60}, f.now)
	c.Start(board.Black)
	f.advance(20 * time.Second)
	if v := c.Snapshot(board.Black); v.MainMs != 40_000 || v.InByo {
		t.Errorf("black view %+v", v)
	}
	if v := c.Snapshot(board.White); v.MainMs != 60_000 {
		t.Errorf("white should be untouched: %+v", v)
	}
}

func TestSwitchHandsOverTheClock(t *testing.T) {
	f := newFake()
	c := New(Config{Enabled: true, MainSeconds: 60}, f.now)
	c.Start(board.Black)
	f.advance(10 * time.Second)
	c.Switch()
	f.advance(5 * time.Second)
	if c.Snapshot(board.Black).MainMs != 50_000 || c.Snapshot(board.White).MainMs != 55_000 {
		t.Errorf("black=%d white=%d", c.Snapshot(board.Black).MainMs, c.Snapshot(board.White).MainMs)
	}
}

func TestEntersByoYomiAfterMain(t *testing.T) {
	f := newFake()
	c := New(Config{Enabled: true, MainSeconds: 10, Periods: 3, PeriodSeconds: 30}, f.now)
	c.Start(board.Black)
	f.advance(15 * time.Second)
	v := c.Snapshot(board.Black)
	if !v.InByo || v.MainMs != 0 || v.PeriodMs != 25_000 || v.Periods != 3 {
		t.Errorf("view %+v", v)
	}
}

func TestPeriodOnlyConsumedIfItExpires(t *testing.T) {
	f := newFake()
	c := New(Config{Enabled: true, MainSeconds: 0, Periods: 3, PeriodSeconds: 30}, f.now)
	c.Start(board.Black)
	f.advance(29 * time.Second)
	c.Switch() // moved in time: period resets, none consumed
	v := c.Snapshot(board.Black)
	if v.Periods != 3 || v.PeriodMs != 30_000 {
		t.Errorf("period should reset untouched: %+v", v)
	}
	c.Switch() // white's turn ends instantly, back to black
	f.advance(31 * time.Second)
	v = c.Snapshot(board.Black)
	if v.Periods != 2 || v.PeriodMs != 29_000 {
		t.Errorf("one period should have expired: %+v", v)
	}
}

func TestFlagsWhenAllPeriodsGone(t *testing.T) {
	f := newFake()
	c := New(Config{Enabled: true, MainSeconds: 5, Periods: 2, PeriodSeconds: 10}, f.now)
	c.Start(board.White)
	f.advance(5*time.Second + 10*time.Second)
	if c.Flagged() != board.Empty {
		t.Fatal("one period still left")
	}
	f.advance(10 * time.Second)
	if c.Flagged() != board.White {
		t.Error("white should have flagged")
	}
}

func TestSuddenDeathWithoutByo(t *testing.T) {
	f := newFake()
	c := New(Config{Enabled: true, MainSeconds: 30}, f.now)
	c.Start(board.Black)
	f.advance(31 * time.Second)
	if c.Flagged() != board.Black {
		t.Error("no byo-yomi: running out of main time loses")
	}
}

func TestDisabledClockNeverFlags(t *testing.T) {
	f := newFake()
	c := New(Config{Enabled: false, MainSeconds: 1}, f.now)
	c.Start(board.Black)
	f.advance(time.Hour)
	if c.Flagged() != board.Empty || c.Snapshot(board.Black).Enabled {
		t.Error("a disabled clock must be inert")
	}
	if _, ok := c.Deadline(); ok {
		t.Error("disabled clock has no deadline")
	}
}

func TestByoOnlyWithMainDisabled(t *testing.T) {
	f := newFake()
	c := New(Config{Enabled: true, Periods: 1, PeriodSeconds: 20}, f.now)
	if v := c.Snapshot(board.Black); !v.InByo || v.PeriodMs != 20_000 {
		t.Errorf("should start directly in byo-yomi: %+v", v)
	}
}

func TestDeadline(t *testing.T) {
	f := newFake()
	c := New(Config{Enabled: true, MainSeconds: 10, Periods: 2, PeriodSeconds: 30}, f.now)
	c.Start(board.Black)
	d, ok := c.Deadline()
	if !ok {
		t.Fatal("no deadline")
	}
	if want := f.t.Add(10*time.Second + 2*30*time.Second); !d.Equal(want) {
		t.Errorf("deadline %v want %v", d, want)
	}
	f.advance(15 * time.Second) // 5s into the first period
	d, _ = c.Deadline()
	if want := f.t.Add(25*time.Second + 30*time.Second); !d.Equal(want) {
		t.Errorf("deadline in byo %v want %v", d, want)
	}
}

func TestExportImportRoundTrip(t *testing.T) {
	f := newFake()
	c := New(Config{Enabled: true, MainSeconds: 10, Periods: 3, PeriodSeconds: 30}, f.now)
	c.Start(board.Black)
	f.advance(25 * time.Second) // into byo-yomi, 15s of the first period used
	st := c.Export()

	d := New(Config{Enabled: true, MainSeconds: 10, Periods: 3, PeriodSeconds: 30}, f.now)
	d.Import(st)
	v := d.Snapshot(board.Black)
	if !v.InByo || v.PeriodMs != 15_000 || v.Periods != 3 || v.Running {
		t.Errorf("imported view %+v", v)
	}
}
