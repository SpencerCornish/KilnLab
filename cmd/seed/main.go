// Command seed fills the dev database with sample users and firings in
// various states. Users match the dev login form, so you can sign in as any
// of them by name: Kim (kiln master), Sam, Alice, Bob (BMS), Carol.
//
//	make seed           # refuses if firings already exist
//	go run ./cmd/seed -force
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/spencercornish/kilnlab/internal/db"
	"github.com/spencercornish/kilnlab/internal/firing"
)

type seedUser struct {
	name       string
	community  string // artlab | bms
	kilnMaster bool
}

var users = []seedUser{
	{"Kim", "artlab", true},
	{"Sam", "artlab", false},
	{"Alice", "artlab", false},
	{"Bob", "bms", false},
	{"Carol", "artlab", false},
}

type seedProfile struct {
	name     string
	typ      string
	cone     string
	firing   time.Duration
	cooldown time.Duration
}

var profiles = []seedProfile{
	{"Cone 06 Bisque — fast", "bisque", "06", 8 * time.Hour, 12 * time.Hour},
	{"Cone 6 Glaze — standard", "glaze", "6", 10 * time.Hour, 14 * time.Hour},
	{"Cone 10 Glaze — slow", "glaze", "10", 14 * time.Hour, 24 * time.Hour},
}

type seedRequest struct {
	user   string
	share  int32
	cone   string
	glazed bool
	desc   string
	status db.RequestStatus
}

type seedFiring struct {
	proposer   string
	profile    string
	in         time.Duration // from now
	share      int32
	desc       string
	status     db.FiringStatus
	owner      string // empty = unclaimed
	holdReason string
	cancel     string
	startedAgo time.Duration // 0 = not started
	requests   []seedRequest
}

var firings = []seedFiring{
	{
		proposer: "Alice", profile: "Cone 6 Glaze — standard", in: -4 * time.Hour, share: 60, desc: "Dinner plates",
		status: firing.Firing, owner: "Kim", startedAgo: 3 * time.Hour,
		requests: []seedRequest{{"Carol", 30, "6", true, "Mugs", db.RequestStatusApproved}},
	},
	{
		proposer: "Kim", profile: "Cone 06 Bisque — fast", in: 50 * time.Hour, share: 40, desc: "Greenware from the Tuesday class",
		status: firing.Scheduled, owner: "Kim",
		requests: []seedRequest{
			{"Alice", 30, "06", false, "6 bowls, 2 plates", db.RequestStatusApproved},
			{"Bob", 15, "06", false, "Tiles", db.RequestStatusPending},
		},
	},
	{
		proposer: "Alice", profile: "Cone 6 Glaze — standard", in: 4 * 24 * time.Hour, share: 30, desc: "4 mugs, 2 medium bowls",
		status: firing.Proposed,
		requests: []seedRequest{
			{"Bob", 25, "6", true, "A planter and 3 cups", db.RequestStatusPending},
			{"Carol", 20, "5", true, "Small vases", db.RequestStatusPending}, // cone mismatch
		},
	},
	{
		proposer: "Bob", profile: "Cone 10 Glaze — slow", in: 6 * 24 * time.Hour, share: 15, desc: "Test tiles",
		status: firing.OnHold, holdReason: "Not enough cone 10 work yet",
	},
	{
		proposer: "Carol", profile: "Cone 6 Glaze — standard", in: 30 * time.Hour, share: 20, desc: "Bowls",
		status: firing.Cancelled, cancel: "Kiln element being replaced",
	},
	{
		proposer: "Sam", profile: "Cone 6 Glaze — standard", in: 10 * 24 * time.Hour, share: 50, desc: "Dinnerware set",
		status: firing.Proposed,
	},
}

func main() {
	force := flag.Bool("force", false, "seed even if firings already exist")
	flag.Parse()
	_ = godotenv.Load()
	ctx := context.Background()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is not set (see .env)")
	}
	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM firings`).Scan(&n); err != nil {
		log.Fatal(err)
	}
	if n > 0 && !*force {
		log.Fatalf("%d firings already exist; run `make db-reset` first or pass -force", n)
	}

	ids := map[string]int64{}
	for _, u := range users {
		id, err := upsertUser(ctx, pool, u)
		if err != nil {
			log.Fatal(err)
		}
		ids[u.name] = id
	}

	err = db.InTx(ctx, pool, func(q *db.Queries) error {
		st, err := q.GetSettings(ctx)
		if err != nil {
			return err
		}
		kilns, err := q.ListKilns(ctx) // active first
		if err != nil {
			return err
		}
		kiln := kilns[0]
		profileIDs, err := ensureProfiles(ctx, q, kiln.ID)
		if err != nil {
			return err
		}
		loc, _ := time.LoadLocation(st.Timezone)
		for _, sf := range firings {
			if err := seedOne(ctx, q, ids, profileIDs, kiln.ID, st.RequestCutoffHours, loc, sf); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Seeded %d users and %d firings. Sign in with the dev login as: %s\n",
		len(users), len(firings), names())
}

func names() string {
	var out []string
	for _, u := range users {
		out = append(out, u.name)
	}
	return strings.Join(out, ", ")
}

// upsertUser creates the same user/account the dev login form would.
func upsertUser(ctx context.Context, pool *pgxpool.Pool, u seedUser) (int64, error) {
	provider := "discord"
	if u.community == "bms" {
		provider = "slack"
	}
	pid := "dev-" + strings.ToLower(u.name)
	var id int64
	err := pool.QueryRow(ctx, `SELECT user_id FROM accounts WHERE provider = $1 AND provider_user_id = $2`, provider, pid).Scan(&id)
	if !errors.Is(err, pgx.ErrNoRows) {
		return id, err
	}
	err = pool.QueryRow(ctx, `
		WITH u AS (
			INSERT INTO users (display_name, community) VALUES ($1, $2) RETURNING id
		)
		INSERT INTO accounts (user_id, provider, provider_user_id, provider_team_or_guild_id, is_kiln_master)
		SELECT id, $3, $4, '', $5 FROM u
		RETURNING user_id`,
		u.name, u.community, provider, pid, u.kilnMaster).Scan(&id)
	return id, err
}

func ensureProfiles(ctx context.Context, q *db.Queries, kilnID int64) (map[string]db.FiringProfile, error) {
	existing, err := q.ListProfiles(ctx)
	if err != nil {
		return nil, err
	}
	byName := map[string]db.FiringProfile{}
	for _, p := range existing {
		if p.KilnID == kilnID {
			byName[p.Name] = p
		}
	}
	for _, sp := range profiles {
		if _, ok := byName[sp.name]; ok {
			continue
		}
		p, err := q.CreateProfile(ctx, db.CreateProfileParams{
			KilnID: kilnID, Name: sp.name, FiringType: sp.typ, Cone: sp.cone,
			FiringMinutes: int32(sp.firing / time.Minute), CooldownMinutes: int32(sp.cooldown / time.Minute), Active: true,
		})
		if err != nil {
			return nil, err
		}
		byName[sp.name] = p
	}
	return byName, nil
}

func seedOne(ctx context.Context, q *db.Queries, ids map[string]int64, profiles map[string]db.FiringProfile, kilnID int64, cutoff int32, loc *time.Location, sf seedFiring) error {
	profile, ok := profiles[sf.profile]
	if !ok {
		return fmt.Errorf("unknown seed profile %q", sf.profile)
	}
	// Firings are booked by day: the studio-local day sf.in from now.
	day := firing.DayOf(time.Now().In(loc).Add(sf.in), loc)
	created := time.Now().Add(-24 * time.Hour)
	f, err := q.CreateFiring(ctx, db.CreateFiringParams{
		KilnID: kilnID, ProposedByUserID: ids[sf.proposer], ProfileID: &profile.ID,
		FiringType: profile.FiringType, Cone: profile.Cone, Title: profile.Name,
		ScheduledDate: day, FiringMinutes: profile.FiringMinutes, CooldownMinutes: profile.CooldownMinutes,
		ProposerDescription: sf.desc, ProposerSharePct: sf.share, ProposerCone: profile.Cone,
		RequestsCloseAt: firing.RequestsCloseAt(firing.DayStart(day, loc), cutoff),
	})
	if err != nil {
		return err
	}
	event := func(actor string, typ string, from, to *db.FiringStatus, note string, at time.Time) error {
		var n *string
		if note != "" {
			n = &note
		}
		id := ids[actor]
		return q.InsertEvent(ctx, db.InsertEventParams{FiringID: f.ID, ActorUserID: &id, Type: typ,
			FromStatus: from, ToStatus: to, OccurredAt: at, Note: n, Source: db.EventSourceWeb})
	}
	proposed := firing.Proposed
	if err := event(sf.proposer, firing.EventCreated, nil, &proposed, "", created); err != nil {
		return err
	}

	for i, r := range sf.requests {
		at := created.Add(time.Duration(i+1) * time.Hour)
		var compat *string
		if c := firing.Compatibility(profile.FiringType, profile.Cone, r.cone, r.glazed); c != "" {
			compat = &c
		}
		rq, err := q.CreateRequest(ctx, db.CreateRequestParams{
			FiringID: f.ID, UserID: ids[r.user], Description: r.desc, EstimatedSharePct: r.share,
			Cone: r.cone, Glazed: r.glazed, CompatibilityWarning: compat,
		})
		if err != nil {
			return err
		}
		if err := event(r.user, firing.EventRequestSubmitted, nil, nil, fmt.Sprintf("%s requested %d%%", r.user, r.share), at); err != nil {
			return err
		}
		if r.status == db.RequestStatusApproved {
			km := ids["Kim"]
			if _, err := q.DecideRequest(ctx, db.DecideRequestParams{ID: rq.ID, Status: r.status, DecidedByUserID: &km}); err != nil {
				return err
			}
			if err := event("Kim", firing.EventRequestApproved, nil, nil,
				fmt.Sprintf("Approved %s's request (%d%%)", r.user, r.share), at.Add(30*time.Minute)); err != nil {
				return err
			}
		}
	}

	at := created.Add(6 * time.Hour)
	if sf.owner != "" {
		owner := ids[sf.owner]
		if _, err := q.ClaimFiring(ctx, db.ClaimFiringParams{ID: f.ID, UserID: &owner}); err != nil {
			return err
		}
		if err := event(sf.owner, firing.EventClaimed, &proposed, nil, fmt.Sprintf("%s took this firing", sf.owner), at); err != nil {
			return err
		}
		at = at.Add(time.Hour)
	}

	setStatus := func(to db.FiringStatus, wasScheduled bool, hold, cancel *string, note string) error {
		from := f.Status
		var err error
		f, err = q.SetFiringStatus(ctx, db.SetFiringStatusParams{ID: f.ID, Status: to, WasScheduled: wasScheduled, HoldReason: hold, CancelReason: cancel})
		if err != nil {
			return err
		}
		return event("Kim", firing.EventStatusChange, &from, &to, note, at)
	}
	switch sf.status {
	case firing.Scheduled, firing.Firing:
		if sf.owner == "" {
			return fmt.Errorf("%s firing needs an owner", sf.status)
		}
	case firing.OnHold:
		if err := setStatus(firing.OnHold, false, &sf.holdReason, nil, sf.holdReason); err != nil {
			return err
		}
	case firing.Cancelled:
		if err := setStatus(firing.Cancelled, false, nil, &sf.cancel, sf.cancel); err != nil {
			return err
		}
	}
	if sf.status == firing.Firing {
		started := time.Now().Add(-sf.startedAgo).Truncate(time.Minute)
		firingEnds, unloadable := firing.PlannedTimings(started, profile.FiringMinutes, profile.CooldownMinutes)
		coolingAt := (*time.Time)(nil)
		if f.Status == firing.Cooling {
			coolingAt = &firingEnds
		}
		if _, err = q.AdvanceFiring(ctx, db.AdvanceFiringParams{ID: f.ID, Status: firing.Firing,
			StartedAt: &started, FiringEndsAt: &firingEnds, UnloadableAt: &unloadable, CoolingAt: coolingAt}); err != nil {
			return err
		}
		from, to := firing.Scheduled, firing.Firing
		if err := q.InsertEvent(ctx, db.InsertEventParams{FiringID: f.ID, ActorUserID: ptr(ids["Kim"]), Type: firing.EventStatusChange,
			FromStatus: &from, ToStatus: &to, OccurredAt: started,
			Note:   ptr(fmt.Sprintf("%s firing, %s cooldown", formatDuration(profile.FiringMinutes), formatDuration(profile.CooldownMinutes))),
			Source: db.EventSourceKilnQr}); err != nil {
			return err
		}
	}
	return nil
}

func formatDuration(min int32) string {
	h, m := min/60, min%60
	switch {
	case h == 0:
		return fmt.Sprintf("%dm", m)
	case m == 0:
		return fmt.Sprintf("%dh", h)
	default:
		return fmt.Sprintf("%dh %dm", h, m)
	}
}

func ptr[T any](v T) *T { return &v }
