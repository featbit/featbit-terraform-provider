package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/featbit/terraform-provider-featbit/tools/api-probe/internal/probe"
)

const defaultInventoryPath = ".featbit-api-probe-cleanup.json"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "config":
		err = runConfig()
	case "projects-list":
		err = runProjectsList(os.Args[2:])
	case "auth-negative":
		err = runAuthNegative(os.Args[2:])
	case "cleanup":
		err = runCleanup(os.Args[2:])
	case "project-env-lifecycle":
		err = runProjectEnvironmentLifecycle(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, probe.RedactText(err.Error()))
		os.Exit(1)
	}
}

func runConfig() error {
	cfg, presence, err := probe.LoadConfig(os.LookupEnv)
	printPresence(presence)
	if err != nil {
		fmt.Println("read_ready=false")
		fmt.Println("mutation_ready=false")
		return err
	}

	readErr := cfg.ValidateReadOnly()
	mutationErr := cfg.ValidateMutation()
	fmt.Printf("read_ready=%t\n", readErr == nil)
	fmt.Printf("mutation_ready=%t\n", mutationErr == nil)
	return nil
}

func printPresence(p probe.Presence) {
	fmt.Printf("%s=%s\n", probe.EnvAPIURL, presenceWord(p.APIURL))
	fmt.Printf("%s=%s\n", probe.EnvServiceToken, presenceWord(p.ServiceToken))
	fmt.Printf("%s=%s\n", probe.EnvPersonalToken, presenceWord(p.PersonalToken))
	fmt.Printf("%s=%s\n", probe.EnvTarget, presenceWord(p.Target))
	fmt.Printf("%s=%s\n", probe.EnvResourcePrefix, presenceWord(p.ResourcePrefix))
}

func runCleanup(args []string) error {
	flags := flag.NewFlagSet("cleanup", flag.ContinueOnError)
	dryRun := flags.Bool("dry-run", false, "plan cleanup without deleting resources")
	execute := flags.Bool("execute", false, "execute cleanup against an approved mutation target")
	inventoryPath := flags.String("inventory", defaultInventoryPath, "cleanup inventory path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *dryRun == *execute {
		return fmt.Errorf("select exactly one of --dry-run or --execute")
	}

	inventory, err := probe.LoadInventory(*inventoryPath)
	if err != nil {
		return err
	}
	if *dryRun {
		results := inventory.Cleanup(context.Background(), true, nil)
		fmt.Printf("pending=%d\n", len(results))
		for _, result := range results {
			fmt.Printf("plan resource_type=%s identity=%s\n", result.Entry.Type, printableIdentity(result.Entry.Identity))
		}
		return nil
	}

	cfg, _, err := probe.LoadConfig(os.LookupEnv)
	if err != nil {
		return err
	}
	if err := cfg.ValidateMutation(); err != nil {
		return err
	}
	client, err := probe.NewClient(cfg, probe.TokenService, 30*time.Second, nil)
	if err != nil {
		return err
	}
	results := inventory.Cleanup(context.Background(), false, client.DeleteInventoryEntry)
	if err := inventory.Save(*inventoryPath); err != nil {
		return err
	}
	fmt.Printf("pending=%d\n", inventory.Pending())
	failures := 0
	for _, result := range results {
		status := "deleted"
		if result.Err != nil {
			status = "failed"
			failures++
		}
		fmt.Printf("cleanup resource_type=%s identity=%s status=%s\n",
			result.Entry.Type,
			printableIdentity(result.Entry.Identity),
			status,
		)
	}
	for _, workaround := range client.CleanupWorkarounds() {
		fmt.Printf("workaround=%s\n", workaround)
	}
	if failures > 0 {
		return fmt.Errorf("%d cleanup operations failed; inventory entries remain pending", failures)
	}
	return nil
}

func runProjectsList(args []string) error {
	flags := flag.NewFlagSet("projects-list", flag.ContinueOnError)
	tokenKind := flags.String("token-kind", string(probe.TokenService), "service or personal")
	timeout := flags.Duration("timeout", 30*time.Second, "request timeout")
	if err := flags.Parse(args); err != nil {
		return err
	}
	kind := probe.TokenKind(*tokenKind)
	if kind != probe.TokenService && kind != probe.TokenPersonal {
		return fmt.Errorf("token-kind must be service or personal")
	}
	cfg, _, err := probe.LoadConfig(os.LookupEnv)
	if err != nil {
		return err
	}
	client, err := probe.NewClient(cfg, kind, *timeout, nil)
	if err != nil {
		return err
	}
	result, err := client.DoJSON(context.Background(), http.MethodGet, "/api/v1/projects", nil)
	if err != nil {
		return err
	}
	output, err := probe.MarshalObservation(result.Observation)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(output)
	return err
}

func runProjectEnvironmentLifecycle(args []string) error {
	flags := flag.NewFlagSet("project-env-lifecycle", flag.ContinueOnError)
	execute := flags.Bool("execute", false, "execute one isolated project/environment lifecycle")
	compatibilityChecks := flags.Bool(
		"compatibility-checks",
		false,
		"also probe validation, duplicate-key, and post-delete behavior",
	)
	featureFlagChecks := flags.Bool(
		"feature-flag-checks",
		false,
		"also run flag CRUD plus duplicate-create and stale-revision checks under the new environment",
	)
	featureFlagCRUDChecks := flags.Bool(
		"feature-flag-crud-checks",
		false,
		"also run exactly one flag create, normal narrow update, and delete under the new environment",
	)
	featureFlagTypeMatrixChecks := flags.Bool(
		"feature-flag-type-matrix-checks",
		false,
		"also run String, Number, and JSON flag lifecycles sequentially under the new environment",
	)
	segmentChecks := flags.Bool(
		"segment-checks",
		false,
		"also run an environment-specific segment lifecycle plus a duplicate-create check under the new environment",
	)
	segmentCRUDChecks := flags.Bool(
		"segment-crud-checks",
		false,
		"also run exactly one environment-specific segment lifecycle under the new environment",
	)
	childReadChecks := flags.Bool(
		"child-read-checks",
		false,
		"also read exact missing flag/segment identities under the new environment without child writes",
	)
	timeout := flags.Duration("timeout", 30*time.Second, "request timeout")
	inventoryPath := flags.String("inventory", defaultInventoryPath, "cleanup inventory path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if !*execute {
		return fmt.Errorf("project-env-lifecycle requires explicit --execute")
	}
	selectedModes := 0
	for _, selected := range []bool{
		*compatibilityChecks,
		*featureFlagChecks,
		*featureFlagCRUDChecks,
		*featureFlagTypeMatrixChecks,
		*segmentChecks,
		*segmentCRUDChecks,
		*childReadChecks,
	} {
		if selected {
			selectedModes++
		}
	}
	if selectedModes > 1 {
		return fmt.Errorf(
			"select at most one lifecycle extension mode",
		)
	}

	cfg, _, err := probe.LoadConfig(os.LookupEnv)
	if err != nil {
		return err
	}
	if err := cfg.ValidateMutation(); err != nil {
		return err
	}
	client, err := probe.NewClient(cfg, probe.TokenService, *timeout, nil)
	if err != nil {
		return err
	}
	runLifecycle := probe.RunProjectEnvironmentLifecycle
	if *compatibilityChecks {
		runLifecycle = probe.RunProjectEnvironmentCompatibilityLifecycle
	}
	if *featureFlagChecks {
		runLifecycle = probe.RunFeatureFlagLifecycle
	}
	if *featureFlagCRUDChecks {
		runLifecycle = probe.RunFeatureFlagCRUDLifecycle
	}
	if *featureFlagTypeMatrixChecks {
		runLifecycle = probe.RunFeatureFlagTypeMatrixLifecycle
	}
	if *segmentChecks {
		runLifecycle = probe.RunSegmentLifecycle
	}
	if *segmentCRUDChecks {
		runLifecycle = probe.RunSegmentCRUDLifecycle
	}
	if *childReadChecks {
		runLifecycle = probe.RunChildCollectionReadLifecycle
	}
	report, lifecycleErr := runLifecycle(context.Background(), cfg, client, *inventoryPath)
	output, marshalErr := probe.MarshalProjectEnvironmentLifecycleReport(report)
	if marshalErr != nil {
		return marshalErr
	}
	if _, err := os.Stdout.Write(output); err != nil {
		return err
	}
	return lifecycleErr
}

func runAuthNegative(args []string) error {
	flags := flag.NewFlagSet("auth-negative", flag.ContinueOnError)
	testCase := flags.String("case", "", "missing or malformed")
	timeout := flags.Duration("timeout", 30*time.Second, "request timeout")
	if err := flags.Parse(args); err != nil {
		return err
	}
	client, err := probe.NewCloudNegativeAuthClient(probe.NegativeAuthCase(*testCase), *timeout, nil)
	if err != nil {
		return err
	}
	result, err := client.DoJSON(context.Background(), http.MethodGet, "/api/v1/projects", nil)
	if err != nil {
		return err
	}
	output, err := probe.MarshalObservation(result.Observation)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(output)
	return err
}

func printableIdentity(identity probe.ResourceIdentity) string {
	switch {
	case identity.ID != "":
		return "<EXACT_ID_REDACTED>"
	case identity.Key != "":
		return "<EXACT_KEY_REDACTED>"
	default:
		return "<MISSING>"
	}
}

func presenceWord(present bool) string {
	if present {
		return "present"
	}
	return "absent"
}

func usage() {
	fmt.Fprintln(
		os.Stderr,
		"usage: featbit-api-probe <config|projects-list|auth-negative|cleanup|project-env-lifecycle>",
	)
}
