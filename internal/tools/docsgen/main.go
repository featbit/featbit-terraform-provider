// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

// Command docsgen generates Registry documentation or verifies that the
// committed output is byte-for-byte current without rewriting the worktree.
package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"github.com/featbit/terraform-provider-featbit/internal/tools/toolenv"
)

const (
	// terraform-plugin-docs is HashiCorp's MPL-2.0 documentation generator.
	// Keep this exact version pinned so generated Registry pages are reviewable.
	terraformPluginDocsModule = "github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@v0.25.0"
	docsTerraformVersion      = "1.15.8"
	providerName              = "featbit"
	renderedProviderName      = "FeatBit"
)

var terraformArchiveSHA256 = map[string]string{
	"darwin_amd64":  "e2e812e783771159bf758fd4e55d6dc9bb08f63e2af2c63d212721807a02c5dc",
	"darwin_arm64":  "f210110c5698b94d803a7a63cdb0251b5455c150841478808e2bbb343f95ed68",
	"linux_amd64":   "d25ce7b6902013ad905db3d2eab0be4cd905887fe88b81a6171b8d5503c31f3d",
	"linux_arm64":   "8891e9dcedc9e3b8950bc6af9d4d8af1f4cfade3062f53b9dc403a89f6ce8c9c",
	"windows_amd64": "2ff41d2129afb1982733c132c61a8d6ef038f879f3aeede7fc28b8b8b24acf02",
}

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
	terraformDirectory, err := installPinnedTerraform(temporaryRoot)
	if err != nil {
		fatal(err)
	}

	renderedDirectory := filepath.Join(root, "docs")
	if !*write {
		renderedDirectory = filepath.Join(temporaryRoot, "generated-docs")
	}

	if err := generate(root, renderedDirectory, terraformDirectory); err != nil {
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

func installPinnedTerraform(temporaryRoot string) (string, error) {
	platform := runtime.GOOS + "_" + runtime.GOARCH
	expectedHash, supported := terraformArchiveSHA256[platform]
	if !supported {
		return "", fmt.Errorf(
			"Terraform %s documentation generation is not pinned for %s",
			docsTerraformVersion,
			platform,
		)
	}
	archiveName := fmt.Sprintf(
		"terraform_%s_%s.zip",
		docsTerraformVersion,
		platform,
	)
	archivePath := filepath.Join(temporaryRoot, archiveName)
	url := fmt.Sprintf(
		"https://releases.hashicorp.com/terraform/%s/%s",
		docsTerraformVersion,
		archiveName,
	)
	if err := downloadTerraformArchive(url, archivePath, expectedHash); err != nil {
		return "", err
	}

	binaryName := "terraform"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(temporaryRoot, "terraform", binaryName)
	if err := extractTerraformBinary(archivePath, binaryName, binaryPath); err != nil {
		return "", err
	}
	if err := verifyTerraformVersion(binaryPath); err != nil {
		return "", err
	}
	return filepath.Dir(binaryPath), nil
}

func downloadTerraformArchive(url string, destination string, expectedHash string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create Terraform download request: %w", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("download Terraform %s: %w", docsTerraformVersion, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download Terraform %s: unexpected HTTP status %s",
			docsTerraformVersion, response.Status)
	}

	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create Terraform archive: %w", err)
	}
	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, 256<<20))
	closeErr := file.Close()
	if copyErr != nil {
		return fmt.Errorf("download Terraform archive: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close Terraform archive: %w", closeErr)
	}
	actualHash := hex.EncodeToString(hash.Sum(nil))
	if actualHash != expectedHash {
		return fmt.Errorf(
			"Terraform %s archive SHA-256 is %s, want %s",
			docsTerraformVersion,
			actualHash,
			expectedHash,
		)
	}
	return nil
}

func extractTerraformBinary(archivePath string, binaryName string, destination string) error {
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open Terraform archive: %w", err)
	}
	defer archive.Close()

	var binary *zip.File
	for _, entry := range archive.File {
		if filepath.Base(entry.Name) == binaryName && !entry.FileInfo().IsDir() {
			if binary != nil {
				return errors.New("Terraform archive contains duplicate binaries")
			}
			binary = entry
		}
	}
	if binary == nil {
		return fmt.Errorf("Terraform archive does not contain %s", binaryName)
	}
	if binary.UncompressedSize64 > 256<<20 {
		return errors.New("Terraform binary exceeds the extraction limit")
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("create Terraform binary directory: %w", err)
	}
	reader, err := binary.Open()
	if err != nil {
		return fmt.Errorf("open Terraform binary in archive: %w", err)
	}
	defer reader.Close()
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
	if err != nil {
		return fmt.Errorf("create Terraform binary: %w", err)
	}
	_, copyErr := io.Copy(file, io.LimitReader(reader, 256<<20))
	closeErr := file.Close()
	if copyErr != nil {
		return fmt.Errorf("extract Terraform binary: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close Terraform binary: %w", closeErr)
	}
	if err := os.Chmod(destination, 0o755); err != nil {
		return fmt.Errorf("make Terraform binary executable: %w", err)
	}
	return nil
}

func verifyTerraformVersion(binaryPath string) error {
	command := exec.Command(binaryPath, "version", "-json")
	command.Env = toolenv.Sanitized(map[string]string{
		"CHECKPOINT_DISABLE": "1",
	})
	output, err := command.Output()
	if err != nil {
		return fmt.Errorf("start pinned Terraform binary: %w", err)
	}
	var version struct {
		TerraformVersion string `json:"terraform_version"`
	}
	decoder := json.NewDecoder(bytes.NewReader(output))
	if err := decoder.Decode(&version); err != nil {
		return fmt.Errorf("decode pinned Terraform version: %w", err)
	}
	if version.TerraformVersion != docsTerraformVersion {
		return fmt.Errorf("Terraform binary version is %q, want %q",
			version.TerraformVersion, docsTerraformVersion)
	}
	return nil
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
