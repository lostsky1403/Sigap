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
