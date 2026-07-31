package reconcile

import "testing"

func TestPartialFlagCreateRecoversExactIdentityAndNextStep(t *testing.T) {
	t.Parallel()

	checkpoint := FlagCheckpoint{
		EnvironmentID:   "11111111-1111-1111-1111-111111111111",
		Key:             "tfp0-complex-flag",
		BaseCreated:     true,
		IdentityTracked: true,
		Completed:       map[FlagCreateStep]bool{},
	}
	plan, err := checkpoint.RecoveryPlan("featbit_feature_flag.recovered")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Action != RecoveryResume || plan.NextStep != StepVariations {
		t.Fatalf("recovery plan = %+v", plan)
	}
	if plan.ImportID != "11111111-1111-1111-1111-111111111111/tfp0-complex-flag" {
		t.Fatalf("ImportID = %q", plan.ImportID)
	}
	wantCommand := "terraform import featbit_feature_flag.recovered 11111111-1111-1111-1111-111111111111/tfp0-complex-flag"
	if plan.ImportCommand != wantCommand {
		t.Fatalf("ImportCommand = %q, want %q", plan.ImportCommand, wantCommand)
	}

	checkpoint.Completed[StepVariations] = true
	checkpoint.Completed[StepTargeting] = true
	plan, err = checkpoint.RecoveryPlan("featbit_feature_flag.recovered")
	if err != nil {
		t.Fatal(err)
	}
	if plan.NextStep != StepTags {
		t.Fatalf("next step = %q, want %q", plan.NextStep, StepTags)
	}
}

func TestAmbiguousBaseCreateRequiresExactLookup(t *testing.T) {
	t.Parallel()

	checkpoint := FlagCheckpoint{
		EnvironmentID: "11111111-1111-1111-1111-111111111111",
		Key:           "tfp0-complex-flag",
	}
	plan, err := checkpoint.RecoveryPlan("featbit_feature_flag.recovered")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Action != RecoveryExactLookup {
		t.Fatalf("action = %q, want %q", plan.Action, RecoveryExactLookup)
	}
}

func TestCreatedIdentityMustBeTrackedBeforeSuboperations(t *testing.T) {
	t.Parallel()

	checkpoint := FlagCheckpoint{
		EnvironmentID: "11111111-1111-1111-1111-111111111111",
		Key:           "tfp0-complex-flag",
		BaseCreated:   true,
	}
	if _, err := checkpoint.RecoveryPlan("featbit_feature_flag.recovered"); err == nil {
		t.Fatal("recovery accepted an untracked created identity")
	}
}
