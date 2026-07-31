package reconcile

import (
	"errors"
	"fmt"
)

type FlagCreateStep string

const (
	StepVariations FlagCreateStep = "variations"
	StepTargeting  FlagCreateStep = "targeting"
	StepTags       FlagCreateStep = "tags"
	StepVerify     FlagCreateStep = "canonical_read"
)

var flagCreateOrder = []FlagCreateStep{
	StepVariations,
	StepTargeting,
	StepTags,
	StepVerify,
}

type FlagCheckpoint struct {
	EnvironmentID   string
	Key             string
	BaseCreated     bool
	IdentityTracked bool
	Completed       map[FlagCreateStep]bool
}

type RecoveryAction string

const (
	RecoveryExactLookup RecoveryAction = "exact_lookup"
	RecoveryResume      RecoveryAction = "resume"
	RecoveryComplete    RecoveryAction = "complete"
)

type RecoveryPlan struct {
	Action        RecoveryAction
	NextStep      FlagCreateStep
	ImportID      string
	ImportCommand string
}

func (c FlagCheckpoint) RecoveryPlan(resourceAddress string) (RecoveryPlan, error) {
	if c.EnvironmentID == "" || c.Key == "" {
		return RecoveryPlan{}, errors.New("flag checkpoint requires environment ID and exact key")
	}
	importID := c.EnvironmentID + "/" + c.Key
	command := fmt.Sprintf("terraform import %s %s", resourceAddress, importID)
	if !c.BaseCreated {
		return RecoveryPlan{
			Action:        RecoveryExactLookup,
			ImportID:      importID,
			ImportCommand: command,
		}, nil
	}
	if !c.IdentityTracked {
		return RecoveryPlan{}, errors.New("base-created flag must be registered in the cleanup inventory")
	}
	for _, step := range flagCreateOrder {
		if !c.Completed[step] {
			return RecoveryPlan{
				Action:        RecoveryResume,
				NextStep:      step,
				ImportID:      importID,
				ImportCommand: command,
			}, nil
		}
	}
	return RecoveryPlan{
		Action:        RecoveryComplete,
		ImportID:      importID,
		ImportCommand: command,
	}, nil
}
