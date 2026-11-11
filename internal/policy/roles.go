// Package policy holds KilnLab's role and permission rules (spec §4). It is
// pure logic with no I/O so it can be unit tested exhaustively.
//
// Status-change permissions (who may move a firing to which status) live with
// the state machine in internal/firing, which calls into these predicates.
package policy

// Roles a signed-in user holds. Member is implied by having a session at all.
type Roles struct {
	SelfLoader bool `json:"self_loader"`
	KilnMaster bool `json:"kiln_master"`
	Admin      bool `json:"admin"`
}

// ProposerCanDecideRequests is open question 1 (DECISIONS.md). Default: only
// kiln masters approve/decline; the proposer is just notified.
const ProposerCanDecideRequests = false

// CanManageFirings: confirm/hold/release/reschedule/cancel, override fullness,
// close/reopen requests, assign a loader. Admins can act as kiln masters.
func (r Roles) CanManageFirings() bool { return r.KilnMaster || r.Admin }

// CanBypassMinNotice: propose a firing sooner than settings.min_notice_hours.
func (r Roles) CanBypassMinNotice() bool { return r.CanManageFirings() }

// CanDecideRequests: approve/decline requests in a firing. isProposer is
// whether the user proposed that firing.
func (r Roles) CanDecideRequests(isProposer bool) bool {
	return r.CanManageFirings() || (ProposerCanDecideRequests && isProposer)
}

// CanSeeRequestDetails: compatibility flags and decision notes on other
// people's requests.
func (r Roles) CanSeeRequestDetails() bool { return r.CanManageFirings() }

// CanStartFiring: only kiln masters (and admins) ever start the kiln.
func (r Roles) CanStartFiring() bool { return r.KilnMaster || r.Admin }

// CanCorrectTimestamps: back-date status changes.
func (r Roles) CanCorrectTimestamps() bool { return r.CanManageFirings() }

// CanManageSelfLoaders: grant/revoke the self-loader flag.
func (r Roles) CanManageSelfLoaders() bool { return r.KilnMaster || r.Admin }

// CanEditSettings: admin settings page and admin flag.
func (r Roles) CanEditSettings() bool { return r.Admin }

// CanBeLoader: may be assigned as a firing's loader.
func (r Roles) CanBeLoader() bool { return r.SelfLoader || r.CanManageFirings() }
