package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/spencercornish/kilnlab/internal/db"
	"github.com/spencercornish/kilnlab/internal/firing"
	"github.com/spencercornish/kilnlab/internal/ical"
	"github.com/spencercornish/kilnlab/internal/notify"
)

// Public views: the wheel-QR page (§9A) and the iCal feed. No login, so no
// names or notes: participant counts only (open question 4).

type publicFiringDTO struct {
	ID               int64           `json:"id"`
	KilnName         string          `json:"kiln_name,omitempty"` // only with several active kilns
	Title            string          `json:"title"`
	FiringType       string          `json:"firing_type"`
	Cone             string          `json:"cone"`
	ScheduledDate    Day             `json:"scheduled_date"`
	Status           db.FiringStatus `json:"status"`
	FullnessPct      int             `json:"fullness_pct"`
	RoomLeftPct      int             `json:"room_left_pct"`
	RequestsCloseAt  time.Time       `json:"requests_close_at"`
	RequestsOpen     bool            `json:"requests_open"`
	ParticipantCount int             `json:"participant_count"` // proposer + approved requests
	StartedAt        *time.Time      `json:"started_at"`
	UnloadableAt     *time.Time      `json:"unloadable_at"`
}

type upcomingDTO struct {
	Timezone  string            `json:"timezone"`
	DaysAhead int32             `json:"days_ahead"`
	Firings   []publicFiringDTO `json:"firings"`
}

var (
	upcomingStatuses = []string{"proposed", "on_hold", "scheduled", "firing", "cooling"}
	calendarStatuses = []string{"proposed", "on_hold", "scheduled", "firing", "cooling", "completed"}
)

// upcomingGrace keeps a not-yet-started firing listed a while after its start time.
const upcomingGrace = 12 * time.Hour

// kilnNames maps kiln id → name when there are several active kilns, else nil.
func kilnNames(ctx context.Context, q *db.Queries) (map[int64]string, error) {
	kilns, err := q.ListKilns(ctx)
	if err != nil {
		return nil, err
	}
	names, active := map[int64]string{}, 0
	for _, k := range kilns {
		names[k.ID] = k.Name
		if k.Active {
			active++
		}
	}
	if active < 2 {
		return nil, nil
	}
	return names, nil
}

func (s *Server) handleUpcoming(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st, err := s.queries.GetSettings(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	now := s.now()
	loc := s.studioLocation(ctx, s.queries)
	rows, err := s.queries.ListPublicFirings(ctx, db.ListPublicFiringsParams{
		Statuses: upcomingStatuses, Since: firing.DayOf(now.Add(-upcomingGrace), loc),
		Until: firing.DayOf(now.AddDate(0, 0, int(st.PublicPageDaysAhead)), loc),
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	names, err := kilnNames(ctx, s.queries)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := upcomingDTO{Timezone: st.Timezone, DaysAhead: st.PublicPageDaysAhead, Firings: make([]publicFiringDTO, 0, len(rows))}
	for _, row := range rows {
		f := row.Firing
		full := firing.ComputeFullness(f.ProposerSharePct, []int32{row.ApprovedSharePct}, f.FullnessOverridePct)
		out.Firings = append(out.Firings, publicFiringDTO{
			ID: f.ID, KilnName: names[f.KilnID], Title: f.Title, FiringType: f.FiringType, Cone: f.Cone,
			ScheduledDate: Day(f.ScheduledDate), Status: f.Status, FullnessPct: full.Display, RoomLeftPct: full.RoomLeft,
			RequestsCloseAt: f.RequestsCloseAt, RequestsOpen: firing.RequestsOpen(window(f), now),
			ParticipantCount: int(row.ApprovedCount) + 1, StartedAt: f.StartedAt, UnloadableAt: f.UnloadableAt,
		})
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

// calendarPast is how far back the feed keeps firings, so recent ones don't vanish from calendars.
const calendarPast = 30 * 24 * time.Hour

// calendarEvent maps a firing to a VEVENT. It spans start → unload when a time
// is known (actual, then the profile's unloadable estimate); otherwise it's a
// one-hour marker at the scheduled start.
func calendarEvent(f db.Firing, full firing.Fullness, kiln, baseURL, host string, loc *time.Location) ical.Event {
	// Until it has started, a firing is only booked for its day: an all-day event.
	var start, end time.Time
	allDay := f.StartedAt == nil
	if allDay {
		start, end = f.ScheduledDate, f.ScheduledDate.AddDate(0, 0, 1)
	} else {
		start = *f.StartedAt
		end = start.Add(time.Hour)
		switch {
		case f.UnloadedAt != nil && f.UnloadedAt.After(start):
			end = *f.UnloadedAt
		case f.UnloadableAt != nil && f.UnloadableAt.After(start):
			end = *f.UnloadableAt
		}
	}
	link := notify.FiringURL(baseURL, f.ID)
	label := firing.Label(f.Status)
	desc := "Status: " + strings.ToUpper(label[:1]) + label[1:]
	if tentativeOrConfirmed(f.Status) {
		desc += fmt.Sprintf("\nAbout %d%% full, room for ~%d%%", full.Display, full.RoomLeft)
		desc += "\nRequests close " + f.RequestsCloseAt.In(loc).Format("Mon Jan 2, 3:04pm")
	}
	desc += "\n" + link
	return ical.Event{
		UID:         ical.UID("firing", f.ID, host),
		Summary:     fmt.Sprintf("%s (%d%% full)", notify.Title(f), full.Display),
		Description: desc, URL: link, Location: kiln, Start: start, End: end, AllDay: allDay, Modified: f.UpdatedAt,
		Tentative: f.Status == firing.Proposed || f.Status == firing.OnHold,
	}
}

func tentativeOrConfirmed(st db.FiringStatus) bool {
	return st == firing.Proposed || st == firing.OnHold || st == firing.Scheduled
}

func (s *Server) handleCalendar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st, err := s.queries.GetSettings(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	now := s.now()
	loc := s.studioLocation(ctx, s.queries)
	rows, err := s.queries.ListPublicFirings(ctx, db.ListPublicFiringsParams{
		Statuses: calendarStatuses, Since: firing.DayOf(now.Add(-calendarPast), loc),
		Until: firing.DayOf(now.AddDate(0, 0, int(st.PublicPageDaysAhead)), loc),
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	names, err := kilnNames(ctx, s.queries)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	host := "kilnlab"
	if u, err := url.Parse(s.cfg.AppBaseURL); err == nil && u.Hostname() != "" {
		host = u.Hostname()
	}
	cal := ical.Calendar{Name: "Kiln firings"}
	for _, row := range rows {
		f := row.Firing
		full := firing.ComputeFullness(f.ProposerSharePct, []int32{row.ApprovedSharePct}, f.FullnessOverridePct)
		cal.Events = append(cal.Events, calendarEvent(f, full, names[f.KilnID], s.cfg.AppBaseURL, host, loc))
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `inline; filename="kiln-firings.ics"`)
	w.Header().Set("Cache-Control", "public, max-age=300")
	if err := ical.Write(w, cal); err != nil {
		s.logger.Warn("calendar write failed", "error", err)
	}
}
