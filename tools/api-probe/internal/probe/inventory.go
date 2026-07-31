package probe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

type ResourceType string

const (
	ResourceProject     ResourceType = "project"
	ResourceEnvironment ResourceType = "environment"
	ResourceFlag        ResourceType = "feature-flag"
	ResourceSegment     ResourceType = "segment"
	ResourceGroup       ResourceType = "group"
	ResourcePolicy      ResourceType = "policy"
	ResourceMember      ResourceType = "member"
)

type ResourceIdentity struct {
	ID            string `json:"id,omitempty"`
	Key           string `json:"key,omitempty"`
	ProjectID     string `json:"project_id,omitempty"`
	EnvironmentID string `json:"environment_id,omitempty"`
}

type InventoryEntry struct {
	CreatedAt time.Time        `json:"created_at"`
	Target    Target           `json:"target"`
	Type      ResourceType     `json:"resource_type"`
	Identity  ResourceIdentity `json:"identity"`
	TODO      string           `json:"todo"`
	CleanedAt *time.Time       `json:"cleaned_at,omitempty"`
}

type Inventory struct {
	Entries []InventoryEntry `json:"entries"`
}

type CleanupResult struct {
	Entry  InventoryEntry
	DryRun bool
	Err    error
}

type DeleteResource func(context.Context, InventoryEntry) error

func (i *Inventory) Track(entry InventoryEntry) error {
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	if entry.Target != TargetCloudCurrent && entry.Target != TargetSelfHostedMin {
		return errors.New("cleanup entry has an unrecognized target")
	}
	if entry.Type == "" || !strings.HasPrefix(entry.TODO, "P0-") {
		return errors.New("cleanup entry requires a resource type and Phase 0 TODO")
	}
	if entry.Identity.ID == "" && entry.Identity.Key == "" {
		return errors.New("cleanup entry requires an exact ID or key")
	}
	for _, existing := range i.Entries {
		if existing.CleanedAt == nil &&
			existing.Target == entry.Target &&
			existing.Type == entry.Type &&
			existing.Identity == entry.Identity {
			return errors.New("cleanup entry is already pending")
		}
	}
	i.Entries = append(i.Entries, entry)
	return nil
}

// MarkCleaned closes exactly one pending identity after a completed absence
// verification. It never selects by name, fuzzy key, or list position.
func (i *Inventory) MarkCleaned(target Target, resourceType ResourceType, identity ResourceIdentity) error {
	match := -1
	for index, entry := range i.Entries {
		if entry.CleanedAt == nil &&
			entry.Target == target &&
			entry.Type == resourceType &&
			entry.Identity == identity {
			if match != -1 {
				return errors.New("multiple pending cleanup entries match one exact identity")
			}
			match = index
		}
	}
	if match == -1 {
		return errors.New("pending cleanup identity was not found")
	}
	now := time.Now().UTC()
	i.Entries[match].CleanedAt = &now
	return nil
}

func LoadInventory(path string) (Inventory, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Inventory{}, nil
	}
	if err != nil {
		return Inventory{}, fmt.Errorf("read cleanup inventory: %w", err)
	}
	var inventory Inventory
	if err := json.Unmarshal(content, &inventory); err != nil {
		return Inventory{}, fmt.Errorf("decode cleanup inventory: %w", err)
	}
	return inventory, nil
}

func (i Inventory) Save(path string) error {
	content, err := json.MarshalIndent(i, "", "  ")
	if err != nil {
		return fmt.Errorf("encode cleanup inventory: %w", err)
	}
	content = append(content, '\n')
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return fmt.Errorf("write cleanup inventory: %w", err)
	}
	return nil
}

// Cleanup plans or executes deletion in deterministic dependency order.
func (i *Inventory) Cleanup(ctx context.Context, dryRun bool, deleteResource DeleteResource) []CleanupResult {
	indexes := make([]int, 0, len(i.Entries))
	for index := range i.Entries {
		if i.Entries[index].CleanedAt == nil {
			indexes = append(indexes, index)
		}
	}
	sort.SliceStable(indexes, func(left, right int) bool {
		leftEntry := i.Entries[indexes[left]]
		rightEntry := i.Entries[indexes[right]]
		leftRank := cleanupRank(leftEntry.Type)
		rightRank := cleanupRank(rightEntry.Type)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return leftEntry.CreatedAt.After(rightEntry.CreatedAt)
	})

	results := make([]CleanupResult, 0, len(indexes))
	for _, index := range indexes {
		entry := i.Entries[index]
		result := CleanupResult{Entry: entry, DryRun: dryRun}
		if !dryRun {
			if deleteResource == nil {
				result.Err = errors.New("cleanup executor is required")
			} else {
				result.Err = deleteResource(ctx, entry)
				if result.Err == nil {
					now := time.Now().UTC()
					i.Entries[index].CleanedAt = &now
				}
			}
		}
		results = append(results, result)
	}
	return results
}

func (i Inventory) Pending() int {
	count := 0
	for _, entry := range i.Entries {
		if entry.CleanedAt == nil {
			count++
		}
	}
	return count
}

func cleanupRank(resourceType ResourceType) int {
	switch resourceType {
	case ResourceFlag, ResourceSegment:
		return 0
	case ResourceEnvironment:
		return 1
	case ResourceGroup, ResourcePolicy, ResourceMember:
		return 2
	case ResourceProject:
		return 3
	default:
		return 4
	}
}
