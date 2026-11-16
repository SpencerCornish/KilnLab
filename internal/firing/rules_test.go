package firing

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 20, 16, 0, 0, 0, time.UTC)

func TestRequestsWindow(t *testing.T) {
	closeAt := RequestsCloseAt(t0, 12)
	if !closeAt.Equal(t0.Add(-12 * time.Hour)) {
		t.Fatalf("close_at = %v", closeAt)
	}
	before, after := closeAt.Add(-time.Minute), closeAt
	for _, s := range AllStatuses {
		open := s == Proposed || s == OnHold || s == Scheduled
		if got := RequestsOpen(Window{Status: s, CloseAt: closeAt}, before); got != open {
			t.Errorf("%s before cutoff: open = %v", s, got)
		}
		if RequestsOpen(Window{Status: s, CloseAt: closeAt}, after) {
			t.Errorf("%s at cutoff should be closed", s)
		}
		if RequestsOpen(Window{Status: s, CloseAt: closeAt, ClosedManually: true}, before) {
			t.Errorf("%s closed manually should be closed", s)
		}
	}
}

func TestReopenCloseAt(t *testing.T) {
	closeAt := RequestsCloseAt(t0, 12)
	if got := ReopenCloseAt(closeAt, t0, closeAt.Add(-time.Hour)); !got.Equal(closeAt) {
		t.Errorf("reopen before cutoff should keep close_at, got %v", got)
	}
	if got := ReopenCloseAt(closeAt, t0, closeAt.Add(time.Hour)); !got.Equal(t0) {
		t.Errorf("reopen after cutoff should extend to start, got %v", got)
	}
}

func TestMinNotice(t *testing.T) {
	now := t0.Add(-48 * time.Hour)
	if err := CheckMinNotice(t0, now, 48); err != nil {
		t.Errorf("exactly 48h: %v", err)
	}
	if err := CheckMinNotice(t0, now.Add(time.Minute), 48); err == nil {
		t.Error("47h59m should fail")
	}
	if err := CheckMinNotice(t0, t0, 0); err != nil {
		t.Errorf("0h notice: %v", err)
	}
}

func p(v int32) *int32 { return &v }

func TestFullness(t *testing.T) {
	cases := []struct {
		name     string
		proposer int32
		approved []int32
		override *int32
		want     Fullness
	}{
		{"proposer only", 30, nil, nil, Fullness{30, 30, false, 70}},
		{"with approved", 30, []int32{20, 25}, nil, Fullness{75, 75, false, 25}},
		{"exactly full", 50, []int32{50}, nil, Fullness{100, 100, false, 0}},
		{"over full capped", 60, []int32{30, 40}, nil, Fullness{130, 100, false, 0}},
		{"override wins", 30, []int32{20}, p(90), Fullness{50, 90, true, 10}},
		{"override zero", 30, nil, p(0), Fullness{30, 0, true, 100}},
		{"override lower", 80, []int32{40}, p(70), Fullness{120, 70, true, 30}},
	}
	for _, c := range cases {
		if got := ComputeFullness(c.proposer, c.approved, c.override); got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestWouldOverfill(t *testing.T) {
	if WouldOverfill(80, 20) {
		t.Error("80+20 = 100 should fit")
	}
	if !WouldOverfill(80, 21) {
		t.Error("80+21 should overfill")
	}
}

func TestCompatibility(t *testing.T) {
	cases := []struct {
		typ, cone, work string
		glazed          bool
		warn            bool
	}{
		{"glaze", "6", "6", true, false},
		{"glaze", "6", "5", true, true},
		{"bisque", "04", "04", false, false},
		{"bisque", "04", "04", true, true},
		{"Bisque", "04", "06", true, true},
		{"other", "6", "6", true, false},
	}
	for _, c := range cases {
		got := Compatibility(c.typ, c.cone, c.work, c.glazed)
		if (got != "") != c.warn {
			t.Errorf("%+v: warning = %q", c, got)
		}
	}
	both := Compatibility("bisque", "04", "6", true)
	if both != "Work is for cone 6 but this is a cone 04 firing. Glazed work in a bisque firing." {
		t.Errorf("combined warning = %q", both)
	}
}
