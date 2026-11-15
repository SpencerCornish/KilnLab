package notify

import (
	"strings"
	"testing"
	"time"

	"github.com/spencercornish/kilnlab/internal/db"
	"github.com/spencercornish/kilnlab/internal/firing"
)

var denver, _ = time.LoadLocation("America/Denver")

func sampleFiring() db.Firing {
	start := time.Date(2026, 11, 12, 16, 0, 0, 0, time.UTC) // Thu 9:00am Denver
	return db.Firing{
		ID: 7, FiringType: "glaze", Cone: "6", ScheduledDate: time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC), Status: firing.Proposed,
		ProposerSharePct: 30, RequestsCloseAt: start.Add(-12 * time.Hour),
	}
}

func render(f db.Firing, approved int, full firing.Fullness, now time.Time) Card {
	return RenderCard(CardInput{Firing: f, ProposerName: "Cody", ApprovedCount: approved, Fullness: full,
		Now: now, Loc: denver, BaseURL: "https://kiln.example"})
}

func TestRenderProposed(t *testing.T) {
	f := sampleFiring()
	c := render(f, 2, firing.Fullness{Display: 75, RoomLeft: 25}, f.RequestsCloseAt.Add(12*time.Hour).Add(-48*time.Hour))
	want := []string{
		"Status: Proposed · ~75% full · room for ~25%",
		"Proposed by Cody · 3 participants",
		"Requests close Wed Nov 11, 9:00pm",
	}
	if c.Title != "🔥 Cone 6 Glaze — Thu Nov 12" {
		t.Errorf("title = %q", c.Title)
	}
	if strings.Join(c.Lines, "\n") != strings.Join(want, "\n") {
		t.Errorf("lines =\n%s\nwant\n%s", strings.Join(c.Lines, "\n"), strings.Join(want, "\n"))
	}
	if c.URL != "https://kiln.example/firings/7" || c.Cancelled {
		t.Errorf("card = %+v", c)
	}
}

func TestRenderVariants(t *testing.T) {
	f := sampleFiring()
	now := f.RequestsCloseAt.Add(12 * time.Hour)
	reason := "not enough work"
	started := f.RequestsCloseAt.Add(12 * time.Hour).Add(15 * time.Minute)
	unload := started.Add(30 * time.Hour)
	cases := []struct {
		mut   func(*db.Firing)
		first string
	}{
		{func(f *db.Firing) { f.Status = firing.OnHold; f.HoldReason = &reason }, "⏸ Waiting for more work — not enough work"},
		{func(f *db.Firing) { f.Status = firing.Firing; f.StartedAt = &started; f.EstimatedUnloadAt = &unload },
			"Firing now · started 9:15am · est. unload Fri 3:15pm"},
		{func(f *db.Firing) { f.Status = firing.Completed }, "✅ Ready for pickup"},
		{func(f *db.Firing) { f.Status = firing.Cancelled; f.CancelReason = &reason }, "❌ Cancelled — not enough work"},
		{func(f *db.Firing) { f.Status = firing.Loading }, "Status: Loading"},
	}
	for _, c := range cases {
		ff := f
		c.mut(&ff)
		card := render(ff, 0, firing.Fullness{Display: 30, RoomLeft: 70}, now)
		if card.Lines[0] != c.first {
			t.Errorf("%s: first line = %q, want %q", ff.Status, card.Lines[0], c.first)
		}
		if !strings.Contains(strings.Join(card.Lines, "\n"), "Proposed by Cody · 1 participant") {
			t.Errorf("%s: %q", ff.Status, card.Lines)
		}
		if (ff.Status == firing.Cancelled) != card.Cancelled {
			t.Errorf("%s: cancelled flag", ff.Status)
		}
	}
	// After the cutoff a pre-firing card says requests are closed.
	closed := render(f, 0, firing.Fullness{}, f.RequestsCloseAt)
	if closed.Lines[len(closed.Lines)-1] != "Requests closed" {
		t.Errorf("closed lines = %v", closed.Lines)
	}
}

func TestCardHash(t *testing.T) {
	f := sampleFiring()
	now := f.RequestsCloseAt.Add(12 * time.Hour).Add(-48 * time.Hour)
	a := render(f, 1, firing.Fullness{Display: 50, RoomLeft: 50}, now)
	b := render(f, 1, firing.Fullness{Display: 50, RoomLeft: 50}, now.Add(time.Hour))
	c := render(f, 2, firing.Fullness{Display: 70, RoomLeft: 30}, now)
	if a.Hash() != b.Hash() {
		t.Error("same content should hash the same")
	}
	if a.Hash() == c.Hash() {
		t.Error("different content should hash differently")
	}
}

func TestTitleAndDescribe(t *testing.T) {
	f := sampleFiring()
	if Describe(f, denver) != "Cone 6 Glaze (Thu Nov 12)" {
		t.Errorf("describe = %q", Describe(f, denver))
	}
	title := "Holiday mugs"
	f.Title = &title
	if Title(f) != "Holiday mugs" {
		t.Errorf("title = %q", Title(f))
	}
}

func TestBackoff(t *testing.T) {
	want := []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute,
		16 * time.Minute, 32 * time.Minute, time.Hour, time.Hour}
	for i, w := range want {
		if got := Backoff(i + 1); got != w {
			t.Errorf("Backoff(%d) = %v, want %v", i+1, got, w)
		}
	}
}

// Slack only carries colour on legacy attachments, so the card's blocks ride
// inside one whose `color` is the status colour Discord already gets.
func TestSlackCardColor(t *testing.T) {
	c := Card{Title: "Cone 6 Glaze", Lines: []string{"Status: Confirmed"}, URL: "https://k/f/1", Color: statusColors[firing.Confirmed]}
	att := slackAttachments(c)[0].(map[string]any)
	if att["color"] != "#15803d" {
		t.Errorf("attachment color = %v", att["color"])
	}
	if blocks, ok := att["blocks"].([]any); !ok || len(blocks) != 2 {
		t.Errorf("attachment blocks = %v", att["blocks"])
	}
	// A card with no colour (never sent today) must not paint a black bar.
	if _, ok := slackAttachments(Card{Title: "x"})[0].(map[string]any)["color"]; ok {
		t.Error("uncoloured card should have no attachment color")
	}
}

func TestSlackEscaping(t *testing.T) {
	c := Card{Title: "A <b> & c", Lines: []string{"x > y"}, URL: "https://k/f/1"}
	text := slackBlocks(c)[0].(map[string]any)["text"].(map[string]string)["text"]
	if text != "*<https://k/f/1|A &lt;b&gt; &amp; c>*\nx &gt; y" {
		t.Errorf("slack text = %q", text)
	}
}

// Member-written text (titles, reasons) must not turn into disguised links.
func TestPlainTextIsEscaped(t *testing.T) {
	if got := mrkdwnEscape("❌ <https://evil.example|Sign in again> was cancelled"); got != "❌ &lt;https://evil.example|Sign in again&gt; was cancelled" {
		t.Errorf("slack = %q", got)
	}
	if got := discordEscape(`[Sign in](https://evil.example) a\b`); got != `\[Sign in\](https://evil.example) a\\b` {
		t.Errorf("discord = %q", got)
	}
	m := discordCardMessage(Card{Title: "[x](https://evil.example)", Lines: []string{"on hold — [y](https://evil.example)"}, URL: "https://k/f/1"})
	if e := m.Embeds[0]; e.Title != `\[x\](https://evil.example)` ||
		e.Description != `on hold — \[y\](https://evil.example)`+"\n\n[View / request space →](https://k/f/1)" {
		t.Errorf("discord card = %+v", e)
	}
}
