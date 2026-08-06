// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

// Command examplecheck formats and validates every public example against a
// locally installed provider at its frozen Registry source address. It uses a
// temporary plugin directory, not a Terraform development override.
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/featbit/terraform-provider-featbit/internal/tools/toolenv"
)

const validationProviderVersion = "0.1.0"

func main() {
	root, err := repositoryRoot()
	if err != nil {
		fatal(err)
	}
	terraform := os.Getenv("FEATBIT_TERRAFORM_BINARY")
	if terraform == "" {
		terraform = "terraform"
	}
	if err := run(root, toolenv.Sanitized(nil), "check Terraform example formatting", terraform,
		"fmt", "-check", "-recursive", "examples"); err != nil {
		fatal(err)
	}

	temporaryRoot, err := os.MkdirTemp("", "featbit-example-check-")
	if err != nil {
		fatal(fmt.Errorf("create example check directory: %w", err))
	}
	defer os.RemoveAll(temporaryRoot)

	pluginDirectory := filepath.Join(temporaryRoot, "plugins")
	providerPackageDirectory := filepath.Join(
		pluginDirectory,
		"registry.terraform.io",
		"featbit",
		"featbit",
		validationProviderVersion,
		runtime.GOOS+"_"+runtime.GOARCH,
	)
	if err := os.MkdirAll(providerPackageDirectory, 0o755); err != nil {
		fatal(fmt.Errorf("create temporary plugin directory: %w", err))
	}
	binaryName := "terraform-provider-featbit_v" + validationProviderVersion
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(providerPackageDirectory, binaryName)
	if err := run(root, toolenv.Sanitized(nil), "build example-validation provider", "go",
		"build", "-trimpath", "-ldflags", "-X main.version="+validationProviderVersion,
		"-o", binaryPath, "."); err != nil {
		fatal(err)
	}

	targets, err := validationTargets(root)
	if err != nil {
		fatal(err)
	}
	providerExample := filepath.Join(root, "examples", "provider")
	for _, target := range targets {
		relativeTarget, err := filepath.Rel(root, target)
		if err != nil {
			fatal(fmt.Errorf("resolve example path %s: %w", target, err))
		}
		fixtureName := strings.ReplaceAll(filepath.ToSlash(relativeTarget), "/", "-")
		fixtureDirectory := filepath.Join(temporaryRoot, "fixtures", fixtureName)
		if err := os.MkdirAll(fixtureDirectory, 0o755); err != nil {
			fatal(fmt.Errorf("create fixture for %s: %w", target, err))
		}
		if err := copyTerraformFiles(providerExample, fixtureDirectory); err != nil {
			fatal(err)
		}
		if target != providerExample {
			if err := copyTerraformFiles(target, fixtureDirectory); err != nil {
				fatal(err)
			}
		}

		dataDirectory := filepath.Join(fixtureDirectory, ".terraform-data")
		environment := toolenv.Sanitized(map[string]string{
			"CHECKPOINT_DISABLE": "1",
			"TF_DATA_DIR":        dataDirectory,
			"TF_IN_AUTOMATION":   "1",
		})
		if err := run(fixtureDirectory, environment, "initialize "+target, terraform,
			"init", "-backend=false", "-input=false", "-no-color",
			"-plugin-dir="+pluginDirectory); err != nil {
			fatal(err)
		}
		if err := run(fixtureDirectory, environment, "validate "+target, terraform,
			"validate", "-no-color"); err != nil {
			fatal(err)
		}
	}

	fmt.Printf("Validated %d credential-free Terraform example sets.\n", len(targets))
}

func repositoryRoot() (string, error) {
	root, err := filepath.Abs(".")
	if err != nil {
		return "", fmt.Errorf("resolve repository root: %w", err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return "", errors.New("run examplecheck from the provider repository root")
	}
	return root, nil
}

func validationTargets(root string) ([]string, error) {
	targets := []string{filepath.Join(root, "examples", "provider")}
	for _, category := range []string{"resources", "data-sources"} {
		categoryPath := filepath.Join(root, "examples", category)
		entries, err := os.ReadDir(categoryPath)
		if err != nil {
			return nil, fmt.Errorf("read %s examples: %w", category, err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				targets = append(targets, filepath.Join(categoryPath, entry.Name()))
			}
		}
	}
	sort.Strings(targets[1:])
	return targets, nil
}

func copyTerraformFiles(source string, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".tf" {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read example %s: %w", path, err)
		}
		target := filepath.Join(destination, filepath.Base(path))
		if _, err := os.Stat(target); err == nil {
			return fmt.Errorf("example composition would overwrite %s", target)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect example target %s: %w", target, err)
		}
		if err := os.WriteFile(target, contents, 0o600); err != nil {
			return fmt.Errorf("copy example to %s: %w", target, err)
		}
		return nil
	})
}

func run(directory string, environment []string, action string, name string, arguments ...string) error {
	command := exec.Command(name, arguments...)
	command.Dir = directory
	command.Env = environment
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w\n%s", action, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
