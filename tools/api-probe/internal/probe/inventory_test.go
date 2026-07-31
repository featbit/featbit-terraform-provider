package probe

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestInventoryPersistsAndPlansDependencyOrderedCleanup(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 7, 30, 7, 0, 0, 0, time.UTC)
	inventory := Inventory{}
	entries := []InventoryEntry{
		{
			CreatedAt: base,
			Target:    TargetCloudCurrent,
			Type:      ResourceProject,
			Identity:  ResourceIdentity{ID: "project-test-id"},
			TODO:      "P0-040",
		},
		{
			CreatedAt: base.Add(time.Minute),
			Target:    TargetCloudCurrent,
			Type:      ResourceEnvironment,
			Identity:  ResourceIdentity{ID: "environment-test-id", ProjectID: "project-test-id"},
			TODO:      "P0-043",
		},
		{
			CreatedAt: base.Add(2 * time.Minute),
			Target:    TargetCloudCurrent,
			Type:      ResourceFlag,
			Identity:  ResourceIdentity{Key: "tfp0-flag", EnvironmentID: "environment-test-id"},
			TODO:      "P0-050",
		},
	}
	for _, entry := range entries {
		if err := inventory.Track(entry); err != nil {
			t.Fatal(err)
		}
	}

	path := filepath.Join(t.TempDir(), "cleanup.json")
	if err := inventory.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadInventory(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Pending() != 3 {
		t.Fatalf("Pending() = %d, want 3", loaded.Pending())
	}

	results := loaded.Cleanup(context.Background(), true, nil)
	var got []ResourceType
	for _, result := range results {
		got = append(got, result.Entry.Type)
		if !result.DryRun || result.Err != nil {
			t.Fatalf("unexpected dry-run result: %+v", result)
		}
	}
	want := []ResourceType{ResourceFlag, ResourceEnvironment, ResourceProject}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cleanup order = %v, want %v", got, want)
	}
	if loaded.Pending() != 3 {
		t.Fatalf("dry run changed inventory; Pending() = %d", loaded.Pending())
	}
}

func TestInventoryCleanupMarksOnlySuccessfulDeletes(t *testing.T) {
	t.Parallel()

	inventory := Inventory{}
	if err := inventory.Track(InventoryEntry{
		Target:   TargetSelfHostedMin,
		Type:     ResourceSegment,
		Identity: ResourceIdentity{ID: "segment-test-id"},
		TODO:     "P0-060",
	}); err != nil {
		t.Fatal(err)
	}
	results := inventory.Cleanup(context.Background(), false, func(context.Context, InventoryEntry) error {
		return nil
	})
	if len(results) != 1 || results[0].Err != nil {
		t.Fatalf("unexpected cleanup results: %+v", results)
	}
	if inventory.Pending() != 0 {
		t.Fatalf("Pending() = %d, want 0", inventory.Pending())
	}
}
