// Package policy holds KilnLab's role and permission rules (spec §4). It is
// pure logic with no I/O so it can be unit tested exhaustively.
package policy

// Roles a signed-in user holds. Member is implied by having a session at all.
type Roles struct {
	SelfLoader bool `json:"self_loader"`
	KilnMaster bool `json:"kiln_master"`
	Admin      bool `json:"admin"`
}

// CanManageFirings: confirm/hold/release/reschedule/cancel, override fullness,
// approve requests, bypass minimum notice. Admins can act as kiln masters.
func (r Roles) CanManageFirings() bool { return r.KilnMaster || r.Admin }

// CanManageSelfLoaders: grant/revoke the self-loader flag.
func (r Roles) CanManageSelfLoaders() bool { return r.KilnMaster || r.Admin }

// CanEditSettings: admin settings page and admin flag.
func (r Roles) CanEditSettings() bool { return r.Admin }
