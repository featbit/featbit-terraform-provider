package identity

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

type Kind string

const (
	Project      Kind = "project"
	Environment  Kind = "environment"
	FeatureFlag  Kind = "feature_flag"
	Segment      Kind = "segment"
	Group        Kind = "group"
	Policy       Kind = "policy"
	Member       Kind = "member"
	GroupMember  Kind = "group_member"
	GroupPolicy  Kind = "group_policy"
	MemberPolicy Kind = "member_policy"
)

var (
	uuidPattern    = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	flagKeyPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

type Parsed struct {
	Kind          Kind
	ProjectID     string
	EnvironmentID string
	ResourceID    string
	Key           string
	LeftID        string
	RightID       string
}

func Parse(kind Kind, value string) (Parsed, error) {
	if value == "" || strings.TrimSpace(value) != value {
		return Parsed{}, errors.New("Import ID must be non-empty and contain no surrounding whitespace")
	}
	parts := strings.Split(value, "/")
	parsed := Parsed{Kind: kind}
	switch kind {
	case Project, Group, Policy, Member:
		if len(parts) != 1 || !validUUID(parts[0]) {
			return Parsed{}, fmt.Errorf("%s Import ID must be one UUID", kind)
		}
		parsed.ResourceID = parts[0]
	case Environment:
		if err := requireUUIDPair(parts); err != nil {
			return Parsed{}, fmt.Errorf("environment Import ID: %w", err)
		}
		parsed.ProjectID, parsed.EnvironmentID = parts[0], parts[1]
	case FeatureFlag:
		if len(parts) != 2 || !validUUID(parts[0]) || !flagKeyPattern.MatchString(parts[1]) {
			return Parsed{}, errors.New("feature_flag Import ID must be environment_uuid/exact_flag_key")
		}
		parsed.EnvironmentID, parsed.Key = parts[0], parts[1]
	case Segment:
		if err := requireUUIDPair(parts); err != nil {
			return Parsed{}, fmt.Errorf("segment Import ID: %w", err)
		}
		parsed.EnvironmentID, parsed.ResourceID = parts[0], parts[1]
	case GroupMember, GroupPolicy, MemberPolicy:
		if err := requireUUIDPair(parts); err != nil {
			return Parsed{}, fmt.Errorf("%s Import ID: %w", kind, err)
		}
		parsed.LeftID, parsed.RightID = parts[0], parts[1]
	default:
		return Parsed{}, fmt.Errorf("unsupported Import identity kind %q", kind)
	}
	return parsed, nil
}

func requireUUIDPair(parts []string) error {
	if len(parts) != 2 || !validUUID(parts[0]) || !validUUID(parts[1]) {
		return errors.New("must contain exactly two UUIDs separated by one slash")
	}
	return nil
}

func validUUID(value string) bool {
	return uuidPattern.MatchString(value)
}
