// Package notify posts firing updates to Discord and Slack (spec §11).
//
// Every change enqueues rows in notification_outbox inside the same
// transaction as the change itself; the Worker delivers them with retries. A
// failed send therefore never blocks or rolls back a state change.
//
// One main message per firing per platform is created on first send and then
// edited in place. Important moments also get a threaded reply, and some
// changes DM the people involved.
package notify

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spencercornish/kilnlab/internal/db"
	"github.com/spencercornish/kilnlab/internal/firing"
)

// Card is the platform-neutral main message for a firing.
type Card struct {
	Title     string   `json:"title"`
	Lines     []string `json:"lines"`
	URL       string   `json:"url"`
	Color     int      `json:"color"`
	Cancelled bool     `json:"cancelled"`
}

// CardInput is everything the main message shows.
type CardInput struct {
	Firing        db.Firing
	ProposerName  string
	KilnName      string // only set when there are several kilns
	ApprovedCount int
	Fullness      firing.Fullness
	Now           time.Time
	Loc           *time.Location
	BaseURL       string
}

const (
	whenLayout = "Mon Jan 2, 3:04pm"
	timeLayout = "3:04pm"
)

// Title is the firing's name: its title, or "Cone 6 Glaze".
func Title(f db.Firing) string {
	if f.Title != nil && *f.Title != "" {
		return *f.Title
	}
	t := f.FiringType
	if t != "" {
		t = strings.ToUpper(t[:1]) + t[1:]
	}
	return fmt.Sprintf("Cone %s %s", f.Cone, t)
}

// Describe is "Cone 6 Glaze (Thu Nov 13)", for DMs. Firings are booked by day.
func Describe(f db.Firing, loc *time.Location) string {
	return fmt.Sprintf("%s (%s)", Title(f), firing.DayLabel(f.ScheduledDate))
}

// FiringURL links to the firing page.
func FiringURL(baseURL string, id int64) string {
	return fmt.Sprintf("%s/firings/%d", baseURL, id)
}

var statusColors = map[db.FiringStatus]int{
	firing.Proposed: 0x0284c7, firing.OnHold: 0xd97706, firing.Confirmed: 0x15803d,
	firing.Loading: 0x4f46e5, firing.Loaded: 0x4f46e5, firing.Firing: 0xdc2626, firing.Cooling: 0x2563eb,
	firing.Completed: 0x0d9488, firing.Cancelled: 0xa8a29e,
}

// RenderCard builds the main message (§11 "Main message content").
func RenderCard(in CardInput) Card {
	f := in.Firing
	loc := in.Loc
	c := Card{
		Title: fmt.Sprintf("🔥 %s — %s", Title(f), firing.DayLabel(f.ScheduledDate)),
		URL:   FiringURL(in.BaseURL, f.ID),
		Color: statusColors[f.Status],
	}
	people := in.ApprovedCount + 1
	participants := fmt.Sprintf("%d participants", people)
	if people == 1 {
		participants = "1 participant"
	}
	full := fmt.Sprintf("~%d%% full · room for ~%d%%", in.Fullness.Display, in.Fullness.RoomLeft)

	switch f.Status {
	case firing.OnHold:
		reason := ""
		if f.HoldReason != nil {
			reason = " — " + *f.HoldReason
		}
		c.Lines = append(c.Lines, "⏸ Waiting for more work"+reason, full)
	case firing.Firing:
		line := "Firing now"
		if f.StartedAt != nil {
			line += " · started " + f.StartedAt.In(loc).Format(timeLayout)
		}
		if f.EstimatedUnloadAt != nil {
			line += " · est. unload " + f.EstimatedUnloadAt.In(loc).Format("Mon 3:04pm")
		}
		c.Lines = append(c.Lines, line)
	case firing.Completed:
		c.Lines = append(c.Lines, "✅ Ready for pickup")
	case firing.Cancelled:
		reason := ""
		if f.CancelReason != nil {
			reason = " — " + *f.CancelReason
		}
		c.Lines = append(c.Lines, "❌ Cancelled"+reason)
		c.Cancelled = true
	case firing.Proposed, firing.Confirmed:
		c.Lines = append(c.Lines, fmt.Sprintf("Status: %s · %s", statusName(f.Status), full))
	default:
		c.Lines = append(c.Lines, "Status: "+statusName(f.Status))
	}

	by := fmt.Sprintf("Proposed by %s · %s", in.ProposerName, participants)
	if in.KilnName != "" {
		by += " · " + in.KilnName
	}
	c.Lines = append(c.Lines, by)

	if firing.RequestsOpen(firing.Window{Status: f.Status, CloseAt: f.RequestsCloseAt, ClosedManually: f.RequestsClosedManually}, in.Now) {
		c.Lines = append(c.Lines, "Requests close "+f.RequestsCloseAt.In(loc).Format(whenLayout))
	} else if f.Status == firing.Proposed || f.Status == firing.OnHold || f.Status == firing.Confirmed {
		c.Lines = append(c.Lines, "Requests closed")
	}
	return c
}

func statusName(s db.FiringStatus) string {
	l := firing.Label(s)
	return strings.ToUpper(l[:1]) + l[1:]
}

// Hash identifies a rendered card, to skip edits that change nothing.
func (c Card) Hash() string {
	b, _ := json.Marshal(c)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Text is a plain-text rendering (Slack fallback text, dry-run logs).
func (c Card) Text() string {
	return c.Title + "\n" + strings.Join(c.Lines, "\n") + "\n" + c.URL
}

// ── Backoff ─────────────────────────────────────────────────────────────────

// MaxAttempts before an outbox row is marked failed.
const MaxAttempts = 8

// Backoff is the wait after the given (1-based) failed attempt: 30s, 1m, 2m … capped at 1h.
func Backoff(attempt int) time.Duration {
	d := 30 * time.Second
	for i := 1; i < attempt && d < time.Hour; i++ {
		d *= 2
	}
	return min(d, time.Hour)
}
