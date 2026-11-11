package policy

import "testing"

// every combination of the three role flags (plus plain member).
func allRoles() []Roles {
	var out []Roles
	for i := range 8 {
		out = append(out, Roles{SelfLoader: i&1 != 0, KilnMaster: i&2 != 0, Admin: i&4 != 0})
	}
	return out
}

func TestPermissionMatrix(t *testing.T) {
	for _, r := range allRoles() {
		km := r.KilnMaster || r.Admin
		checks := map[string][2]bool{
			"manage firings":        {r.CanManageFirings(), km},
			"bypass min notice":     {r.CanBypassMinNotice(), km},
			"decide (non-proposer)": {r.CanDecideRequests(false), km},
			"decide (proposer)":     {r.CanDecideRequests(true), km || ProposerCanDecideRequests},
			"see request details":   {r.CanSeeRequestDetails(), km},
			"start firing":          {r.CanStartFiring(), km},
			"correct timestamps":    {r.CanCorrectTimestamps(), km},
			"manage self-loaders":   {r.CanManageSelfLoaders(), km},
			"edit settings":         {r.CanEditSettings(), r.Admin},
			"be loader":             {r.CanBeLoader(), km || r.SelfLoader},
		}
		for name, c := range checks {
			if c[0] != c[1] {
				t.Errorf("%+v: %s = %v, want %v", r, name, c[0], c[1])
			}
		}
	}
}

func TestSelfLoaderCannotStartKiln(t *testing.T) {
	if (Roles{SelfLoader: true}).CanStartFiring() {
		t.Fatal("self-loader must never start the kiln")
	}
}
