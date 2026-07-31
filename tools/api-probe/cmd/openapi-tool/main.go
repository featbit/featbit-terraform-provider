package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	phaseopenapi "github.com/featbit/terraform-provider-featbit/tools/api-probe/internal/openapi"
)

const (
	lockName      = "openapi.lock.json"
	snapshotName  = "featbit.openapi.json"
	overlayName   = "overlay.json"
	appliedName   = "featbit.overlayed.openapi.json"
	inventoryName = "inventory.json"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	flags := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	directory := flags.String("dir", filepath.FromSlash("../../internal/client/openapi"), "OpenAPI input directory")
	if err := flags.Parse(os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "pin":
		err = pin(*directory)
	case "generate":
		err = generate(*directory, false)
	case "check":
		err = generate(*directory, true)
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func pin(directory string) error {
	lock, err := phaseopenapi.LoadLock(filepath.Join(directory, lockName))
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := phaseopenapi.Pin(ctx, client, lock, filepath.Join(directory, snapshotName)); err != nil {
		return err
	}
	fmt.Printf("snapshot_sha256=%s\n", lock.SHA256)
	fmt.Printf("snapshot_bytes=%d\n", lock.Bytes)
	return nil
}

func generate(directory string, check bool) error {
	lock, err := phaseopenapi.LoadLock(filepath.Join(directory, lockName))
	if err != nil {
		return err
	}
	snapshot, err := os.ReadFile(filepath.Join(directory, snapshotName))
	if err != nil {
		return fmt.Errorf("read OpenAPI snapshot: %w", err)
	}
	if err := phaseopenapi.VerifySnapshot(snapshot, lock); err != nil {
		return err
	}
	overlay, err := os.ReadFile(filepath.Join(directory, overlayName))
	if err != nil {
		return fmt.Errorf("read OpenAPI overlay: %w", err)
	}
	applied, inventoryContent, inventory, err := phaseopenapi.Generate(snapshot, overlay)
	if err != nil {
		return err
	}

	outputs := map[string][]byte{
		filepath.Join(directory, appliedName):   applied,
		filepath.Join(directory, inventoryName): inventoryContent,
	}
	for path, content := range outputs {
		if check {
			existing, readErr := os.ReadFile(path)
			if readErr != nil {
				return fmt.Errorf("read generated OpenAPI artifact: %w", readErr)
			}
			if !bytes.Equal(existing, content) {
				return fmt.Errorf("generated OpenAPI artifact is stale: %s", filepath.Base(path))
			}
			continue
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return fmt.Errorf("write generated OpenAPI artifact: %w", err)
		}
	}

	fmt.Printf("snapshot_sha256=%s\n", phaseopenapi.SHA256(snapshot))
	fmt.Printf("overlayed_sha256=%s\n", phaseopenapi.SHA256(applied))
	fmt.Printf("paths=%d operations=%d schemas=%d properties=%d missing_operation_ids=%d\n",
		inventory.PathCount,
		inventory.OperationCount,
		inventory.SchemaCount,
		inventory.SchemaPropertyCount,
		inventory.MissingOperationIDCount,
	)
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: openapi-tool <pin|generate|check> [-dir path]")
}
