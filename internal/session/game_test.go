package session

import (
	"testing"
	"time"

	"easygo/internal/board"
	"easygo/internal/clock"
)

func rules(t *testing.T) board.Rules {
	t.Helper()
	r, err := board.Preset("japanese")
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func networked(t *testing.T) *Game {
	t.Helper()
	g, err := New(Options{
		Mode: ModeLive, Size: 9, Rules: rules(t),
		Black: Player{Name: "Ann", Kind: "human", UserID: "u1"},
		White: Player{Name: "Bo", Kind: "human", UserID: "u2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func pt(x, y int) board.Point { return board.Point{X: x, Y: y} }

func TestTurnsAreEnforced(t *testing.T) {
	g := networked(t)
	if _, err := g.Play("u2", pt(4, 4), 0); err != ErrNotYourTurn {
		t.Errorf("white moving first: %v", err)
	}
	if _, err := g.Play("stranger", pt(4, 4), 0); err != ErrNotYourTurn {
		t.Errorf("spectator moving: %v", err)
	}
	if _, err := g.Play("u1", pt(4, 4), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Play("u1", pt(3, 3), 0); err != ErrNotYourTurn {
		t.Errorf("black twice in a row: %v", err)
	}
	if _, err := g.Play("u2", pt(4, 4), 0); err == nil {
		t.Error("occupied point accepted")
	}
}

func TestTwoPassesStartScoring(t *testing.T) {
	g := networked(t)
	g.Play("u1", board.Pass, 0)
	g.Play("u2", board.Pass, 0)
	if g.Status != StatusScoring {
		t.Fatalf("status %s", g.Status)
	}
	if _, err := g.Play("u1", pt(1, 1), 0); err != ErrNotPlaying {
		t.Errorf("move during scoring: %v", err)
	}
	// Both must accept.
	if err := g.AcceptScore("u1"); err != nil || g.Status != StatusScoring {
		t.Fatalf("one acceptance must not end it: %v %s", err, g.Status)
	}
	if err := g.AcceptScore("u2"); err != nil || g.Status != StatusFinished {
		t.Fatalf("two acceptances end it: %v %s", err, g.Status)
	}
	if g.Result != "W+6.5" {
		t.Errorf("empty board result %q", g.Result)
	}
}

func TestDeadToggleResetsAgreement(t *testing.T) {
	g := networked(t)
	g.Play("u1", pt(2, 2), 0)
	g.Play("u2", pt(6, 6), 0)
	g.Play("u1", board.Pass, 0)
	g.Play("u2", board.Pass, 0)
	g.AcceptScore("u1")
	if err := g.ToggleDead("u2", pt(2, 2), true); err != nil {
		t.Fatal(err)
	}
	if g.Accepted[board.Black] {
		t.Error("changing the dead stones must cancel earlier agreement")
	}
	if !g.Dead()[pt(2, 2)] {
		t.Error("override not applied")
	}
}

func TestResign(t *testing.T) {
	g := networked(t)
	if err := g.Resign("stranger"); err != ErrNotYours {
		t.Errorf("stranger resigning: %v", err)
	}
	if err := g.Resign("u2"); err != nil || g.Result != "B+R" {
		t.Errorf("resign: %v %q", err, g.Result)
	}
}

func TestOpenSeatAndJoin(t *testing.T) {
	g, _ := New(Options{
		Mode: ModeLive, Size: 9, Rules: rules(t),
		Black: Player{Name: "Ann", Kind: "human", UserID: "u1"},
		White: Player{Kind: "open"},
	})
	if g.Status != StatusWaiting {
		t.Fatalf("status %s", g.Status)
	}
	if err := g.Join("u1", "Ann", "", "", board.White); err != ErrSeatTaken && err != nil {
		// u1 may join the open white seat; but a taken seat must be refused
		t.Log("owner joined their own game:", err)
	}
	g2, _ := New(Options{Mode: ModeLive, Size: 9, Rules: rules(t),
		Black: Player{Name: "Ann", Kind: "human", UserID: "u1"}, White: Player{Kind: "open"}})
	if err := g2.Join("u2", "Bo", "5k", "Oslo", board.Black); err != ErrSeatTaken {
		t.Errorf("joining a taken seat: %v", err)
	}
	if err := g2.Join("u2", "Bo", "5k", "Oslo", board.White); err != nil || g2.Status != StatusPlaying {
		t.Errorf("join: %v status %s", err, g2.Status)
	}
}

func TestTimeoutEndsGame(t *testing.T) {
	g, _ := New(Options{
		Mode: ModeLive, Size: 9, Rules: rules(t),
		Black: Player{Kind: "human", UserID: "u1"}, White: Player{Kind: "human", UserID: "u2"},
		Clock: clock.Config{Enabled: true, MainSeconds: 0, Periods: 1, PeriodSeconds: 1},
	})
	time.Sleep(1100 * time.Millisecond)
	if !g.CheckTimeout() || g.Result != "W+T" {
		t.Errorf("timeout: %q", g.Result)
	}
}

func TestClockCanBeDisabled(t *testing.T) {
	g, _ := New(Options{Mode: ModeLive, Size: 9, Rules: rules(t),
		Black: Player{Kind: "human", UserID: "u1"}, White: Player{Kind: "human", UserID: "u2"},
		Clock: clock.Config{Enabled: false, MainSeconds: 1}})
	time.Sleep(1100 * time.Millisecond)
	if g.CheckTimeout() {
		t.Error("a disabled clock must never time out")
	}
	v := g.View("u1")
	if v.Clock[0].Enabled {
		t.Error("view should report the clock as off")
	}
}

func TestTeachingBranchesAndNavigation(t *testing.T) {
	g, _ := New(Options{Mode: ModeTeaching, Size: 9, Rules: rules(t), OwnerID: "teacher",
		Black: Player{Kind: "human"}, White: Player{Kind: "human"}})
	a, err := g.Play("teacher", pt(4, 4), 0)
	if err != nil {
		t.Fatal(err)
	}
	g.Goto("teacher", g.Tree.Root.ID)
	b, err := g.Play("teacher", pt(2, 2), 0)
	if err != nil || b == a {
		t.Fatalf("second branch: %v", err)
	}
	if len(g.Tree.Root.Children) != 2 {
		t.Errorf("branches = %d", len(g.Tree.Root.Children))
	}
	if _, err := g.Play("student", pt(6, 6), 0); err != ErrNotYours {
		t.Errorf("student editing: %v", err)
	}
	// Either color can be forced in teaching.
	if _, err := g.Play("teacher", pt(6, 6), board.Black); err != nil {
		t.Errorf("forced color: %v", err)
	}
}

func TestLiveGameCannotBeNavigatedOrBranched(t *testing.T) {
	g := networked(t)
	g.Play("u1", pt(4, 4), 0)
	if err := g.Goto("u1", g.Tree.Root.ID); err != ErrBadMode {
		t.Errorf("goto in live: %v", err)
	}
	if err := g.Undo("u1"); err != ErrBadMode {
		t.Errorf("undo between humans: %v", err)
	}
}

func TestAnnotate(t *testing.T) {
	g, _ := New(Options{Mode: ModeTeaching, Size: 9, Rules: rules(t),
		Black: Player{Kind: "human"}, White: Player{Kind: "human"}})
	n, _ := g.Play("", pt(4, 4), 0)
	c := "tengen"
	j := "correct"
	if err := g.Annotate("", n.ID, Annotation{Comment: &c, Judgment: &j}); err != nil {
		t.Fatal(err)
	}
	bad := "bogus"
	if err := g.Annotate("", n.ID, Annotation{Judgment: &bad}); err == nil {
		t.Error("bad judgment accepted")
	}
	v := g.View("")
	found := false
	for _, nv := range v.Nodes {
		if nv.ID == n.ID && nv.Comment == "tengen" && nv.Judgment == "correct" {
			found = true
		}
	}
	if !found {
		t.Error("annotation missing from the view")
	}
}

func TestHandicapGame(t *testing.T) {
	g, err := New(Options{Mode: ModeLive, Size: 19, Rules: rules(t), Handicap: 4, Local: true,
		Black: Player{Kind: "human"}, White: Player{Kind: "human"}})
	if err != nil {
		t.Fatal(err)
	}
	v := g.View("")
	if v.ToPlay != int(board.White) {
		t.Errorf("white should move first, toPlay=%d", v.ToPlay)
	}
	stones := 0
	for _, c := range v.Board {
		if c == int(board.Black) {
			stones++
		}
	}
	if stones != 4 {
		t.Errorf("handicap stones = %d", stones)
	}
}

func TestViewHasLibertiesAndSubscribers(t *testing.T) {
	g := networked(t)
	ch, cancel := g.Subscribe()
	defer cancel()
	g.Play("u1", pt(0, 0), 0)
	select {
	case <-ch:
	default:
		t.Error("subscriber not notified")
	}
	v := g.View("u1")
	if v.Liberties[0] != 2 || v.Board[0] != int(board.Black) {
		t.Errorf("corner stone: board=%d libs=%d", v.Board[0], v.Liberties[0])
	}
	if v.You.Seat != int(board.Black) || v.You.CanMove {
		t.Errorf("you: %+v (black just moved)", v.You)
	}
	if !g.View("u2").You.CanMove {
		t.Error("white should be able to move")
	}
}

func TestBadSize(t *testing.T) {
	if _, err := New(Options{Size: 17, Rules: rules(t)}); err != ErrBadSize {
		t.Errorf("size 17: %v", err)
	}
}
