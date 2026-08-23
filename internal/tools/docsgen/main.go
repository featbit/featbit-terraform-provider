// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

// Command docsgen generates Registry documentation or verifies that the
// committed output is byte-for-byte current without rewriting the worktree.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/featbit/terraform-provider-featbit/internal/tools/toolenv"
)

const (
	// terraform-plugin-docs is HashiCorp's MPL-2.0 documentation generator.
	// Keep this exact version pinned so generated Registry pages are reviewable.
	terraformPluginDocsModule = "github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@v0.25.0"
	providerName              = "featbit"
	renderedProviderName      = "FeatBit"
)

func main() {
	write := flag.Bool("write", false, "write generated documentation to docs instead of checking drift")
	flag.Parse()

	root, err := repositoryRoot()
	if err != nil {
		fatal(err)
	}
	temporaryRoot, err := os.MkdirTemp("", "featbit-docs-tool-")
	if err != nil {
		fatal(fmt.Errorf("create documentation tool directory: %w", err))
	}
	defer os.RemoveAll(temporaryRoot)
	terraformBinary, err := exec.LookPath("terraform")
	if err != nil {
		fatal(fmt.Errorf("locate Terraform CLI: %w", err))
	}
	terraformDirectory := filepath.Dir(terraformBinary)
	staticGuidesSnapshot := filepath.Join(temporaryRoot, "static-guides")
	if err := copyOptionalDirectory(
		filepath.Join(root, "docs", "guides"),
		staticGuidesSnapshot,
	); err != nil {
		fatal(fmt.Errorf("snapshot static Registry guides: %w", err))
	}

	renderedDirectory := filepath.Join(root, "docs")
	if !*write {
		renderedDirectory = filepath.Join(temporaryRoot, "generated-docs")
	}

	generationErr := generate(root, renderedDirectory, terraformDirectory)
	restoreErr := copyOptionalDirectory(
		staticGuidesSnapshot,
		filepath.Join(renderedDirectory, "guides"),
	)
	if restoreErr != nil {
		restoreErr = fmt.Errorf("restore static Registry guides: %w", restoreErr)
	}
	if err := errors.Join(generationErr, restoreErr); err != nil {
		fatal(err)
	}
	if !*write {
		if err := compareDirectories(filepath.Join(root, "docs"), renderedDirectory); err != nil {
			fatal(err)
		}
	}
	if err := validate(root, terraformDirectory); err != nil {
		fatal(err)
	}

	if *write {
		fmt.Println("Registry documentation generated and validated.")
		return
	}
	fmt.Println("Registry documentation is current and valid.")
}

func repositoryRoot() (string, error) {
	root, err := filepath.Abs(".")
	if err != nil {
		return "", fmt.Errorf("resolve repository root: %w", err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return "", errors.New("run docsgen from the provider repository root")
	}
	return root, nil
}

func generate(root string, output string, terraformDirectory string) error {
	relativeOutput, err := filepath.Rel(root, output)
	if err != nil {
		return fmt.Errorf("resolve documentation output relative to provider: %w", err)
	}
	arguments := []string{
		"run",
		terraformPluginDocsModule,
		"generate",
		"--provider-dir", root,
		"--provider-name", providerName,
		"--rendered-provider-name", renderedProviderName,
		"--rendered-website-dir", relativeOutput,
	}
	return run(root, generatorEnvironment(terraformDirectory),
		"generate Registry documentation", "go", arguments...)
}

func validate(root string, terraformDirectory string) error {
	arguments := []string{
		"run",
		terraformPluginDocsModule,
		"validate",
		"--provider-dir", root,
		"--provider-name", providerName,
	}
	return run(root, generatorEnvironment(terraformDirectory),
		"validate Registry documentation", "go", arguments...)
}

func run(
	directory string,
	environment []string,
	action string,
	name string,
	arguments ...string,
) error {
	command := exec.Command(name, arguments...)
	command.Dir = directory
	command.Env = environment
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("%s: %w", action, err)
	}
	return nil
}

func generatorEnvironment(terraformDirectory string) []string {
	return toolenv.Sanitized(map[string]string{
		"CHECKPOINT_DISABLE": "1",
		"PATH":               terraformDirectory + string(os.PathListSeparator) + os.Getenv("PATH"),
		"TF_IN_AUTOMATION":   "1",
	})
}

func compareDirectories(committed string, generated string) error {
	committedFiles, err := regularFiles(committed)
	if err != nil {
		return fmt.Errorf("inspect committed documentation: %w", err)
	}
	generatedFiles, err := regularFiles(generated)
	if err != nil {
		return fmt.Errorf("inspect generated documentation: %w", err)
	}

	allPaths := make(map[string]struct{}, len(committedFiles)+len(generatedFiles))
	for path := range committedFiles {
		allPaths[path] = struct{}{}
	}
	for path := range generatedFiles {
		allPaths[path] = struct{}{}
	}
	paths := make([]string, 0, len(allPaths))
	for path := range allPaths {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	var drift []string
	for _, path := range paths {
		committedPath, committedExists := committedFiles[path]
		generatedPath, generatedExists := generatedFiles[path]
		switch {
		case !committedExists:
			drift = append(drift, "+ "+path)
		case !generatedExists:
			drift = append(drift, "- "+path)
		default:
			committedBytes, readErr := os.ReadFile(committedPath)
			if readErr != nil {
				return fmt.Errorf("read committed %s: %w", path, readErr)
			}
			generatedBytes, readErr := os.ReadFile(generatedPath)
			if readErr != nil {
				return fmt.Errorf("read generated %s: %w", path, readErr)
			}
			if !bytes.Equal(committedBytes, generatedBytes) {
				drift = append(drift, "~ "+path)
			}
		}
	}
	if len(drift) != 0 {
		return fmt.Errorf(
			"generated Registry documentation drifted; run `make docs` and review the result:\n  %s",
			joinLines(drift),
		)
	}
	return nil
}

func regularFiles(root string) (map[string]string, error) {
	files := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unexpected non-regular documentation entry %s", path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relative)] = path
		return nil
	})
	return files, err
}

// copyOptionalDirectory copies a static documentation directory when present.
// terraform-plugin-docs recreates the rendered docs tree during generation, so
// hand-authored Registry guides must be snapshotted and restored around it.
func copyOptionalDirectory(source string, destination string) error {
	info, err := os.Stat(source)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect source directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("source %s is not a directory", source)
	}

	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return fmt.Errorf("resolve static documentation path: %w", err)
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		entryInfo, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect static documentation file: %w", err)
		}
		if !entryInfo.Mode().IsRegular() {
			return fmt.Errorf("unexpected non-regular static documentation entry %s", path)
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read static documentation file: %w", err)
		}
		if err := os.WriteFile(target, contents, entryInfo.Mode().Perm()); err != nil {
			return fmt.Errorf("write static documentation file: %w", err)
		}
		return nil
	})
}

func joinLines(lines []string) string {
	var output bytes.Buffer
	for index, line := range lines {
		if index != 0 {
			output.WriteString("\n  ")
		}
		output.WriteString(line)
	}
	return output.String()
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
