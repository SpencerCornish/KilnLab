package notify

import (
	"fmt"
	"time"

	"github.com/spencercornish/kilnlab/internal/db"
	"github.com/spencercornish/kilnlab/internal/firing"
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
	if f.FiringEndsAt != nil {
		s += " · est. cool " + f.FiringEndsAt.In(loc).Format(whenLayout)
	}
	if f.UnloadableAt != nil {
		s += " · ready to unload " + f.UnloadableAt.In(loc).Format(whenLayout)
	}
	return s
}

func ThreadReady() string { return "✅ Ready for pickup!" }

func ThreadCancelled(reason string) string { return "❌ Cancelled — " + reason }

func ThreadRescheduled(newDay time.Time) string {
	return "📅 Moved to " + firing.DayLabel(newDay)
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

func DMRescheduled(f db.Firing, newDay time.Time, url string) string {
	return fmt.Sprintf("📅 %s was moved from %s to %s.\n%s", Title(f),
		firing.DayLabel(f.ScheduledDate), firing.DayLabel(newDay), url)
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

func DMTakenOver(f db.Firing, loc *time.Location, actorName, url string) string {
	return fmt.Sprintf("🙋 %s is now being run by %s.\n%s", Describe(f, loc), actorName, url)
}

func AlertUnclaimed(f db.Firing, loc *time.Location, actorName, url string) string {
	return fmt.Sprintf("🙋 %s was dropped by %s and needs a kiln master.\n%s", Describe(f, loc), actorName, url)
}

// ── Kiln-master alerts ──────────────────────────────────────────────────────

// AlertRequestReceived tells the responsible kiln master(s) someone asked to join.
func AlertRequestReceived(f db.Firing, loc *time.Location, requester string, share int32, url string) string {
	return fmt.Sprintf("📩 %s requested %d%% in %s.\n%s", requester, share, Describe(f, loc), url)
}

func AlertUnderfilled(f db.Firing, loc *time.Location, pct int, needsOwner bool, url string) string {
	s := fmt.Sprintf("⚠️ %s is only %d%% full and requests are closed", Describe(f, loc), pct)
	if needsOwner {
		s += " and no one has taken it"
	}
	return s + ".\nRun it anyway, postpone it, reopen requests or cancel:\n" + url
}

func AlertNeedsOwner(f db.Firing, loc *time.Location, pct int, url string) string {
	return fmt.Sprintf("🙋 %s is ~%d%% full and no kiln master has taken it yet.\n%s",
		Describe(f, loc), pct, url)
}

// AlertMorning is the day-of reminder: the firing is on today.
func AlertMorning(f db.Firing, loc *time.Location, needsOwner bool, url string) string {
	s := fmt.Sprintf("🌅 %s is today", Describe(f, loc))
	if needsOwner {
		s += " and still needs a kiln master"
	}
	return s + ".\n" + url
}

func AlertMissedStart(f db.Firing, loc *time.Location, url string) string {
	return fmt.Sprintf("⏰ %s hasn't been marked started yet.\n%s", Describe(f, loc), url)
}

func AlertMissedStartEscalation(f db.Firing, loc *time.Location, ownerName, url string) string {
	return fmt.Sprintf("⏰ %s still hasn't been marked started (run by %s).\n%s", Describe(f, loc), ownerName, url)
}

func AlertMissedUnload(f db.Firing, loc *time.Location, url string) string {
	when := ""
	if f.UnloadableAt != nil {
		when = " (ready to unload " + f.UnloadableAt.In(loc).Format(whenLayout) + ")"
	}
	return fmt.Sprintf("⏰ %s%s hasn't been unloaded yet.\n%s", Title(f), when, url)
}

func AlertMissedUnloadEscalation(f db.Firing, loc *time.Location, ownerName, url string) string {
	when := ""
	if f.UnloadableAt != nil {
		when = " (ready to unload " + f.UnloadableAt.In(loc).Format(whenLayout) + ")"
	}
	return fmt.Sprintf("⏰ %s%s still hasn't been unloaded (run by %s).\n%s", Title(f), when, ownerName, url)
}
