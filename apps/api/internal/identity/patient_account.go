// Internal patient identity: an app_users row that represents a citizen, NOT a
// staff role. The account-ownership resolution contract for patient
// appointments lives here; see design/patient-identity-model.md.
package identity

// PatientAccountID returns the app_users.id that may OWN patient appointments
// for this actor, or "" when no such account exists.
//
// Ownership exists exactly when ALL of the following hold:
//
//   - the actor is a real authenticated user (ActorUser). System, dev, and
//     unauthenticated actors never own appointments;
//   - the authentication subject resolved to a live server-side app_users.id
//     (AppUserID), which the RBAC resolver only does for active, non-deleted
//     app_users rows;
//   - the actor holds NO staff authorization — no permission keys and no
//     facility/org grants. A citizen is an app_user with no staff role
//     assignment, so it resolves grant-free (empty Permissions and empty
//     FacilityGrants).
//
// The result fails closed in every ambiguous case: staff, zero-scope
// assignments (a grant with no facility provenance), dev identities, system
// actors, and authenticated-but-unmapped subjects all return "". There is no
// "safe default to allow" branch: an empty result means the caller must treat
// the appointment as anonymous (NULL) or reject.
func (a Actor) PatientAccountID() string {
	if a.Type != ActorUser || a.AppUserID == "" {
		return ""
	}
	if len(a.Permissions) > 0 || len(a.FacilityGrants) > 0 {
		return ""
	}
	return a.AppUserID
}
