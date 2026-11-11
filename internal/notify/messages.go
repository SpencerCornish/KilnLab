package notify

import (
	"fmt"
	"time"

	"github.com/spencercornish/kilnlab/internal/db"
)

// Texts for threaded replies and DMs. Plain text with a link; both platforms
// render URLs.

// ThreadEvents is open question 11: which moments get a threaded reply.
// Default: started, ready for pickup, cancelled, rescheduled.

func ThreadStarted(f db.Firing, loc *time.Location) string {
	s := "🔥 Firing started"
	if f.StartedAt != nil {
		s += " " + f.StartedAt.In(loc).Format(timeLayout)
	}
	if f.EstimatedUnloadAt != nil {
		s += " · est. unload " + f.EstimatedUnloadAt.In(loc).Format(whenLayout)
	}
	return s
}

func ThreadReady() string { return "✅ Ready for pickup!" }

func ThreadCancelled(reason string) string { return "❌ Cancelled — " + reason }

func ThreadRescheduled(newStart time.Time, loc *time.Location) string {
	return "📅 Moved to " + newStart.In(loc).Format(whenLayout)
}

func withNote(s string, note *string) string {
	if note != nil && *note != "" {
		s += fmt.Sprintf("\nNote from the kiln master: “%s”", *note)
	}
	return s
}

func DMApproved(f db.Firing, loc *time.Location, note *string, url string) string {
	return withNote(fmt.Sprintf("✅ Your request in %s was approved.", Describe(f, loc)), note) + "\n" + url
}

func DMDeclined(f db.Firing, loc *time.Location, note *string, url string) string {
	return withNote(fmt.Sprintf("Your request in %s was declined.", Describe(f, loc)), note) + "\n" + url
}

func DMRescheduled(f db.Firing, newStart time.Time, loc *time.Location, url string) string {
	return fmt.Sprintf("📅 %s was moved from %s to %s.\n%s", Title(f),
		f.ScheduledStart.In(loc).Format(whenLayout), newStart.In(loc).Format(whenLayout), url)
}

func DMCancelled(f db.Firing, loc *time.Location, reason, url string) string {
	return fmt.Sprintf("❌ %s was cancelled: %s\n%s", Describe(f, loc), reason, url)
}

func DMOnHold(f db.Firing, loc *time.Location, reason, url string) string {
	return fmt.Sprintf("⏸ %s is on hold: %s\n%s", Describe(f, loc), reason, url)
}

func DMReady(f db.Firing, loc *time.Location, url string) string {
	return fmt.Sprintf("✅ %s is ready for pickup!\n%s", Describe(f, loc), url)
}

// ── Kiln-master alerts (M4 jobs) ────────────────────────────────────────────

func AlertUnderfilled(f db.Firing, loc *time.Location, pct int, unconfirmed bool, url string) string {
	s := fmt.Sprintf("⚠️ %s is only %d%% full and requests are closed", Describe(f, loc), pct)
	if unconfirmed {
		s += " (not confirmed yet)"
	}
	return s + ".\nRun it anyway, postpone it, reopen requests or cancel:\n" + url
}

func AlertNeedsConfirmation(f db.Firing, loc *time.Location, pct int, url string) string {
	return fmt.Sprintf("📝 Requests are closed for %s (~%d%% full) and it still needs confirming.\n%s",
		Describe(f, loc), pct, url)
}

func AlertMissedStart(f db.Firing, loc *time.Location, url string) string {
	return fmt.Sprintf("⏰ %s hasn't been marked started yet.\n%s", Describe(f, loc), url)
}

func AlertMissedUnload(f db.Firing, loc *time.Location, url string) string {
	when := ""
	if f.EstimatedUnloadAt != nil {
		when = " (est. unload " + f.EstimatedUnloadAt.In(loc).Format(whenLayout) + ")"
	}
	return fmt.Sprintf("⏰ %s%s hasn't been marked unloaded yet.\n%s", Title(f), when, url)
}
