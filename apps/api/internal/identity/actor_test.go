package identity

import (
	"context"
	"testing"
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
