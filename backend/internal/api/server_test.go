package api

import "testing"

func TestAnonymousActorUsesStableOpaqueDeviceIdentity(t *testing.T) {
	first, ok := anonymousActor("c04de66e-bb74-4e77-b47f-613f23731f42")
	if !ok || first.Source != "anonymous" || first.Role != "user" || first.ID == "" || first.Username != first.ID {
		t.Fatalf("anonymousActor() = %#v, %v", first, ok)
	}
	again, ok := anonymousActor("c04de66e-bb74-4e77-b47f-613f23731f42")
	if !ok || again.ID != first.ID {
		t.Fatalf("stable identity = %#v, %v", again, ok)
	}
	other, ok := anonymousActor("8b1f56ef-3f6e-495d-b3c9-b77f68ddc92c")
	if !ok || other.ID == first.ID {
		t.Fatalf("different device identity = %#v, %v", other, ok)
	}
	for _, invalid := range []string{"", "too-short", "invalid device id with spaces"} {
		if actor, ok := anonymousActor(invalid); ok {
			t.Fatalf("anonymousActor(%q) = %#v, true", invalid, actor)
		}
	}
}
