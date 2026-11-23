package identity

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestActor_IsZero(t *testing.T) {
	t.Run("zero actor", func(t *testing.T) {
		var a Actor
		if !a.IsZero() {
			t.Error("expected zero Actor to be zero")
		}
	})
	t.Run("non zero actor", func(t *testing.T) {
		a := Actor{UserID: "u1", Type: ActorUser}
		if a.IsZero() {
			t.Error("expected non-zero Actor to not be zero")
		}
	})
}

func TestActor_HasPermission(t *testing.T) {
	a := Actor{Permissions: []string{"queue.generate", "medical_records:read"}}

	t.Run("has permission", func(t *testing.T) {
		if !a.HasPermission("queue.generate") {
			t.Error("expected HasPermission(queue.generate) == true")
		}
	})
	t.Run("missing permission", func(t *testing.T) {
		if a.HasPermission("admin.delete") {
			t.Error("expected HasPermission(admin.delete) == false")
		}
	})
	t.Run("empty permissions", func(t *testing.T) {
		var empty Actor
		if empty.HasPermission("anything") {
			t.Error("expected zero Actor to have no permissions")
		}
	})
}

// TestActor_HasPermissionAtFacility pins the facility-provenance semantics
// that close the cross-facility privilege gap: a flat key union is not enough.
func TestActor_HasPermissionAtFacility(t *testing.T) {
	facA := uuid.New()
	facB := uuid.New()
	scoped := func(key string, id uuid.UUID) FacilityGrant {
		return FacilityGrant{Key: key, FacilityID: &id}
	}

	t.Run("grant scoped to another facility does not apply here", func(t *testing.T) {
		a := Actor{
			// The flat union contains the key, as the old resolver produced.
			Permissions: []string{"queue.manage"},
			FacilityGrants: []FacilityGrant{
				scoped("queue.manage", facB),
			},
		}
		if !a.HasPermission("queue.manage") {
			t.Fatal("precondition: the flat permission set should contain queue.manage")
		}
		if a.HasPermissionAtFacility("queue.manage", facA) {
			t.Error("a grant scoped to B must not apply at A")
		}
		if !a.HasPermissionAtFacility("queue.manage", facB) {
			t.Error("a grant scoped to B must apply at B")
		}
	})

	t.Run("global grant applies at every facility", func(t *testing.T) {
		a := Actor{FacilityGrants: []FacilityGrant{{Key: "queue.manage", Unrestricted: true}}}
		if !a.HasPermissionAtFacility("queue.manage", facA) {
			t.Error("a global grant must apply at A")
		}
		if !a.HasPermissionAtFacility("queue.manage", facB) {
			t.Error("a global grant must apply at B")
		}
	})

	t.Run("provenance-free grant applies nowhere", func(t *testing.T) {
		a := Actor{Permissions: []string{"queue.manage"}}
		if a.HasPermissionAtFacility("queue.manage", facA) {
			t.Error("a flat key with no provenance must not apply at any facility")
		}
	})

	t.Run("zero scope grant with nil facility applies nowhere", func(t *testing.T) {
		a := Actor{
			Permissions:    []string{"queue.manage"},
			FacilityGrants: []FacilityGrant{{Key: "queue.manage", FacilityID: nil, Unrestricted: false}},
		}
		if a.HasPermissionAtFacility("queue.manage", facA) {
			t.Error("a nil-facility, non-unrestricted grant must not apply at any facility")
		}
	})

	t.Run("nil facility id is never authorized", func(t *testing.T) {
		a := Actor{FacilityGrants: []FacilityGrant{{Key: "queue.manage", Unrestricted: true}}}
		if a.HasPermissionAtFacility("queue.manage", uuid.Nil) {
			t.Error("the nil facility must fail closed even for a global grant")
		}
	})
}

func TestContextWithActor_RoundTrip(t *testing.T) {
	ctx := context.Background()

	// Empty context should return zero actor
	if got := ActorFromContext(ctx); !got.IsZero() {
		t.Errorf("expected zero actor from empty context, got %+v", got)
	}

	actor := Actor{UserID: "dev-001", Type: ActorDev, IsDev: true, Permissions: []string{"*"}}
	ctx = ContextWithActor(ctx, actor)
	got := ActorFromContext(ctx)

	if got.UserID != actor.UserID {
		t.Errorf("UserID mismatch: got %q want %q", got.UserID, actor.UserID)
	}
	if got.Type != actor.Type {
		t.Errorf("Type mismatch: got %q want %q", got.Type, actor.Type)
	}
	if !got.IsDev {
		t.Error("expected IsDev == true")
	}
	if !got.HasPermission("*") {
		t.Error("expected HasPermission('*') == true")
	}
}

// The audit actor id must be the resolved app_users.id, never the external
// subject.
//
// `audit_events.actor_user_id` is a uuid with a foreign key to app_users.id.
// Handlers used to pass UserID, which is the EXTERNAL subject: a Supabase `sub`
// in production, and a plain string like "local-facility-admin" under the local
// test identity. Writing that string into a uuid column fails with SQLSTATE
// 22P02, and the audit service only retries without the column when the actor
// type is "dev" — so every admin read and write under the local test identity
// silently produced no audit row at all.
//
// The audit trail is the one record that is supposed to survive a compromise.
// Losing it quietly is worse than losing a screen.
func TestActor_AuditUserID_UsesResolvedAppUserID(t *testing.T) {
	appUserID := uuid.NewString()

	t.Run("prefers the resolved app user id over the subject", func(t *testing.T) {
		a := Actor{
			UserID:    "local-facility-admin",
			Type:      ActorUser,
			AppUserID: appUserID,
		}
		if got := a.AuditUserID(); got != appUserID {
			t.Errorf("expected the app user id %q, got %q", appUserID, got)
		}
	})

	t.Run("a jwt subject that is uuid-shaped is still not the app user id", func(t *testing.T) {
		// Both values are valid uuids here, so nothing about the shape reveals
		// the mistake. The FK is what would fail, and only sometimes — which is
		// exactly why the wrong field survived this long.
		subject := uuid.NewString()
		a := Actor{UserID: subject, Type: ActorUser, AppUserID: appUserID}
		if got := a.AuditUserID(); got == subject {
			t.Error("AuditUserID returned the external subject; the audit FK requires app_users.id")
		}
		if got := a.AuditUserID(); got != appUserID {
			t.Errorf("expected %q, got %q", appUserID, got)
		}
	})

	t.Run("an unresolvable actor yields empty, not the subject", func(t *testing.T) {
		// Empty is written as NULL: an audit row with no actor is still worth
		// keeping. Returning the subject here would fail the insert instead and
		// discard the whole event.
		a := Actor{UserID: "some-external-sub", Type: ActorUser}
		if got := a.AuditUserID(); got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})
}
