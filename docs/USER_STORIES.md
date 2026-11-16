# User stories: proposing, taking and running a firing

These stories describe the **current build**. The app is pre-production; the schema is
a single clean baseline and `make db-reset` is the supported way to rebuild a local
database. Where behaviour differs from the original spec, the difference is called out
at the end.

---

## The cast and the rules

| Person | Community | Role | Sign-in platform |
|---|---|---|---|
| Sam | ArtLab | Member | Discord |
| Carol | ArtLab | Member | Discord |
| Alice | BMS | Member | Slack |
| Bob | BMS | Member | Slack |
| Kim | ArtLab | Kiln master | Discord |
| Lee | ArtLab | Kiln master | Discord |

A **member** is in the ArtLab Discord or the BMS Slack workspace. A **kiln master**
has the Kiln Master role in ArtLab; admins can do everything a kiln master can.

### Firing profiles

Each kiln has a list of **firing profiles**:

- name (the firing's displayed name, e.g. “Cone 06 Bisque — fast”)
- firing type and cone
- rough firing time (start → kiln off)
- rough cooldown time (kiln off → unloadable)

A proposer picks a preset for the selected kiln, or **Other** for a one-off custom
firing. The firing snapshots all the values, so later profile edits do not rewrite it.
KMs/admins can save an “Other” firing as a new preset for that kiln.

### Notifications

| Kind | What it is | Who sees it |
|---|---|---|
| **Main message** | One message per firing per platform (Discord + Slack), edited in place | Everyone in the notification channel(s) |
| **Threaded reply** | A short reply under the main message (a Discord thread is created on the first one) | People following that thread |
| **DM** | A direct message from the bot | The named person, on the platform of their first account |

- The actor is never DMed about their own action.
- While a firing is **unclaimed**, routine kiln-master alerts go to **every KM**.
- Once a KM **takes** the firing, routine alerts go to **the owner only**. Other KMs
  can still act, but they are not pinged.
- Missed start/unload first goes to the owner; after `escalation_grace_minutes` it
  goes to every KM + admin.
- Changes are committed first; sends go through `notification_outbox`. Failures
  retry and never roll back the change.
- The main message is rendered at send time and only edited when its hash changes.

### Defaults

| Setting | Default |
|---|---|
| Studio timezone | `America/Denver` |
| Minimum notice | 48 hours |
| Requests close | 12 hours before the start of the scheduled day (noon the day before) |
| Under-filled alert threshold | 50% |
| Day-of reminder | 8:00am studio time |
| Missed-start grace | 120 minutes after the day ends |
| Missed-unload grace | 180 minutes after `unloadable_at` |
| Escalation grace | 120 minutes after the first missed reminder |

Firings are booked for a **day**. “Thursday” means the kiln runs sometime Thursday.

---

## Story 1 — Sam proposes a firing

> **As Sam**, I want to choose a firing preset for the kiln, see what it means, and add
> my own work, so that the firing appears in both communities and people can ask for
> space.

> **As Kim**, I want new proposals to appear in the channels without an announcement,
> and I want a custom option for something new.

### Flow

1. Sam opens **Propose a firing** and selects a kiln.
2. The **Firing profile** list shows that kiln's active presets plus **Other**.
3. If Sam picks a preset, the app shows its type, cone, firing time and cooldown.
4. If Sam picks **Other**, they enter a name, type, cone, firing time and cooldown.
   KMs/admins also see “Save this as a preset for this kiln.”
5. Sam picks a day, describes their work, picks a share, and optionally adds a clay
   body and notes.
6. The server checks the notice, blackouts, share and profile; snapshots the profile
   onto the firing; and creates it as `proposed`.

### What happens

- The firing appears on `/`, `/upcoming`, and `/calendar.ics`.
- **One new main message** posts to each configured Discord/Slack channel.
- **No DMs, no thread, no @mentions.** The card says **“Needs a kiln master.”**
- The public page shows the profile name, day, type, cone, status, fullness/room and
  participant count — no names or notes.

### Card example

```
🔥 Cone 06 Bisque — fast — Thu Nov 13
Status: Proposed · ~40% full · room for ~60%
Proposed by Sam · 1 participant
Needs a kiln master
Requests close Wed 12:00pm
https://kiln.example/firings/42
```

### Variants

| Situation | Result |
|---|---|
| Less than 48h notice, by a member | Rejected: ask a kiln master |
| Less than 48h notice, by a KM/admin | Allowed |
| Kiln blacked out that day | Rejected for everyone |
| Unknown profile, inactive profile, or a profile for another kiln | Rejected |
| Duplicate custom preset name for that kiln | Rejected |
| Short-notice firing created after its cutoff | It is never open for requests; later close alerts stay quiet |

---

## Story 2 — Kim takes the firing, and Alice asks to join

> **As Kim**, I want to claim the firing so I am the person responsible and the other
> KMs stop getting pinged.

> **As Alice**, I want to request space and get a DM when my request is approved or
> declined.

### Taking the firing

1. Kim opens the firing and taps **Take this firing**.
2. The server checks Kim is a KM/admin and the firing is unclaimed, sets
   `managed_by_user_id`, and moves `proposed → scheduled`.
3. The card shows **“Run by Kim.”** No DM goes to the other KMs.
4. If someone else already took it, a second claim returns a conflict. A KM/admin can
   choose **Take over from …**; the previous owner gets a DM.
5. Kim can **Drop this firing** before the kiln starts. It returns to `proposed` and
   every KM gets a DM that it needs a kiln master again.

### Requesting space

Alice scans the wheel QR, opens `/upcoming`, taps **Request space**, signs in if
needed, and submits her work/share/cone/glazed/clay body.

The server checks the request window, one-live-request rule, share, cone and
compatibility. It creates a `pending` request.

**Notifications:**

- The **owner** gets “📩 Alice requested 10% in …” (or every KM while unclaimed).
- The channel card normally does **not** change, because fullness and participant
  count only include approved requests.
- Alice sees any compatibility/overfill warning immediately.

### Kim decides

- **Approve:** the request counts toward fullness; Alice gets a DM with Kim's optional
  note; the card is edited in place to show the new fullness/participant count.
- **Decline:** Alice gets a DM with the note; the card usually stays the same.
- Members see live requests (name/work/share). Decision notes and compatibility flags
  are visible only to the requester and KMs. The timeline never contains notes.
- Editing an approved request sends it back to `pending` and needs approval again.
  Editing/withdrawing does **not** DM the KMs.

### Card after approval

```
Status: Scheduled · ~85% full · room for ~15%
Proposed by Sam · 3 participants
Run by Kim
```

---

## Story 3 — The firing happens

> **As Kim**, I want to start the kiln in one tap and have cooling happen on its own.

> **As Carol**, I want to unload when the kiln is actually cool, even if that is earlier
> than the estimate.

> **As Alice**, I want a DM when the work is ready for pickup.

### Flow

1. **Day-of reminder:** at 8am studio time, the owner gets “🌅 … is today.” If the
   firing is still unclaimed, every KM gets it with “still needs a kiln master.”
2. **Start:** the owner (or another KM/admin) taps **START FIRING**. No program or
   duration is typed — the firing uses its profile. The server sets
   `started_at`, `firing_ends_at = started_at + firing_minutes`, and
   `unloadable_at = firing_ends_at + cooldown_minutes`.
   - Card edits to “Firing now · started … · est. cool … · ready to unload …”
   - A thread reply posts. No DMs.
3. **Automatic cooling:** the 5-minute job sets `cooling` at `firing_ends_at`. The
   card edits to “Cooling · ready to unload ….” No DM.
4. **Ready to unload:** derived from `unloadable_at`; the status stays `cooling` but
   the UI says “ready to unload now” once the estimate has passed.
5. **Unload:** anyone signed in can tap **Unloaded: ready for pickup**, from `firing`
   or `cooling`. Early unload is allowed; the confirm screen warns if it is before the
   estimate. It records the actual `unloaded_at` and sets `completed`.
   - Card: “✅ Ready for pickup.”
   - Thread reply: “✅ Ready for pickup!”
   - DMs: proposer + every approved requester. Pending/declined/withdrawn are not
     DMed; the actor is skipped.
6. **Missed start/unload:** if a scheduled firing is not started after its day ends +
   grace, or a firing/cooling firing is not unloaded after `unloadable_at` + grace,
   the owner gets a DM. After the escalation grace, all KMs + admins get one.

### Who can do what

| Action | Who |
|---|---|
| Take / take over / drop a firing | KM/admin |
| Approve/decline requests | KM/admin |
| Hold / release / reschedule / cancel | KM/admin |
| Start the kiln | KM/admin |
| Unload | Anyone signed in |
| Correct a recorded time | KM/admin |

`on_hold` and `cancelled` remain exceptions. Hold/release/reschedule/cancel are not
part of the normal path.

### Fixing the record

- KMs can back-date a status change (up to 7 days, not before the previous step).
- Correcting `started_at` recomputes `firing_ends_at` and `unloadable_at`.
- Correcting `cooling_at` or `unloaded_at` only changes the recorded moment. The
  estimates stay as estimates.
- Corrections do not rewind status or DM anyone.

---

## Notification cheat sheet

| Event | Main message | Thread | DMs |
|---|---|---|---|
| Firing proposed | Posted | — | — |
| Taken | Edited (“Run by Kim”) | — | — |
| Takeover | Edited | — | Previous owner |
| Dropped | Edited (“Needs a kiln master”) | — | All KMs except actor |
| Request submitted | (usually no change) | — | Owner, or all KMs if unclaimed |
| Request approved | Edited (fullness/count) | — | Requester |
| Request declined | (usually no change) | — | Requester |
| Requests auto-closed | Edited | — | Owner (or all KMs) if under-filled/unowned |
| Day of firing | — | — | Owner (or all KMs if unclaimed) |
| Firing started | Edited | 🔥 Firing started … | — |
| Cooling (automatic) | Edited | — | — |
| Ready to unload | Edited (label) | — | — |
| Unloaded | Edited | ✅ Ready for pickup! | Proposer + approved requesters |
| Missed start/unload | — | — | Owner; then all KMs + admins |
| Hold / reschedule / cancel | Edited | Reschedule/cancel reply | Participants |

“Participants” means the proposer plus pending and approved requesters.

---

## Behaviours worth knowing

1. **A proposal does not DM kiln masters.** It posts the main message and shows
   “Needs a kiln master.” The cutoff alert and day-of reminder are the follow-ups.
2. **A pending request is invisible in the channel.** Only approved requests count
   toward fullness and the participant count; the owner's DM is the signal.
3. **Editing or withdrawing a request doesn't DM the KMs.** Only a new request does.
4. **One owner per firing is a single point of failure.** Requests can sit if the
   owner is away; missed start/unload escalates, but request alerts do not.
5. **Cooling is an estimate.** Auto-cooling uses the profile's firing time. If the kiln
   runs longer, correct the timestamps; the status is not rewound automatically.
6. **Unloading early is allowed.** The estimate is a warning, not a gate.
7. **The 5-minute job is the clock.** Auto-cooling can lag by up to a tick; the UI
   still derives “ready” from `unloadable_at`.
8. **A profile change before start re-checks every live request.** Warnings are
   recomputed; requests are not auto-declined. After start the profile is frozen.
9. **A short-notice firing created after its cutoff closes quietly.** No under-filled
   or needs-owner DM.
10. **Unload DMs exclude pending requesters.** Only approved work is expected in the
    kiln. The page still shows pending requests.
11. **The owner is skipped by the actor rule.** If a KM unloads their own firing, they
    are not DMed; the proposer and other approved requesters still are.
12. **Notification failure is silent to users.** Admins can inspect the outbox on
    `/admin`; changes still succeed.
