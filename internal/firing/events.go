package firing

// Event types written to firing_events.type. Every change to a firing or its
// requests writes one (spec §6). M3 notifications key off these too.
const (
	EventCreated            = "created"
	EventClaimed            = "claimed"
	EventUnclaimed          = "unclaimed"
	EventProfileChanged     = "profile_changed"
	EventStatusChange       = "status_change"
	EventRescheduled        = "rescheduled"
	EventRequestSubmitted   = "request_submitted"
	EventRequestUpdated     = "request_updated"
	EventRequestWithdrawn   = "request_withdrawn"
	EventRequestApproved    = "request_approved"
	EventRequestDeclined    = "request_declined"
	EventFullnessOverride   = "fullness_override"
	EventRequestsClosed     = "requests_closed"
	EventRequestsReopened   = "requests_reopened"
	EventTimestampCorrected = "timestamp_corrected"
)

// EventAlertSent records a kiln-master alert or reminder from the scheduled
// jobs (source = system); the note says which.
const EventAlertSent = "alert_sent"
