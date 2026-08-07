package contract

import (
	"reflect"
	"testing"

	"github.com/RaniduNethma/vedoc/internal/models"
)

func TestDiffReportsResolvedAdditionsAndRemovals(t *testing.T) {
	before := BuildSnapshot([]models.Endpoint{
		{Method: "GET", Path: "/users/:id", Resolution: models.ResolutionResolved},
		{Method: "POST", Path: "/users", Resolution: models.ResolutionResolved},
	})
	after := BuildSnapshot([]models.Endpoint{
		{Method: "GET", Path: "/users/:id", Resolution: models.ResolutionResolved},
		{Method: "PATCH", Path: "/users/:id", Resolution: models.ResolutionResolved},
	})

	got := Diff(before, after)
	wantAdded := []EndpointRef{{Method: "PATCH", Path: "/users/:id"}}
	wantRemoved := []EndpointRef{{Method: "POST", Path: "/users"}}
	if !reflect.DeepEqual(got.Added, wantAdded) {
		t.Fatalf("Added = %#v, want %#v", got.Added, wantAdded)
	}
	if !reflect.DeepEqual(got.Removed, wantRemoved) {
		t.Fatalf("Removed = %#v, want %#v", got.Removed, wantRemoved)
	}
	if !got.Breaking {
		t.Fatal("Breaking = false, want true")
	}
}

func TestDiffIgnoresUnresolvedEndpoints(t *testing.T) {
	before := BuildSnapshot([]models.Endpoint{
		{Method: "GET", LocalPath: "/:id", Resolution: models.ResolutionUnresolved},
	})
	after := BuildSnapshot(nil)

	got := Diff(before, after)
	if got.Breaking || len(got.Removed) != 0 {
		t.Fatalf("Diff() treated unresolved endpoint as a contract removal: %#v", got)
	}
	if got.IgnoredUnresolvedBefore != 1 {
		t.Fatalf("IgnoredUnresolvedBefore = %d, want 1", got.IgnoredUnresolvedBefore)
	}
}

func TestBuildSnapshotDropsAIOnlyFields(t *testing.T) {
	snapshot := BuildSnapshot([]models.Endpoint{{
		Method:      "POST",
		Path:        "/users",
		Resolution:  models.ResolutionResolved,
		Description: "generated description",
		Payload:     `{"name":"example"}`,
	}})

	if snapshot.Endpoints[0].Description != "" || snapshot.Endpoints[0].Payload != "" {
		t.Fatalf("snapshot retained AI-only fields: %#v", snapshot.Endpoints[0])
	}
}
