// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyOptionalDirectory(t *testing.T) {
	t.Parallel()

	t.Run("missing source is ignored", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		destination := filepath.Join(root, "destination")
		if err := copyOptionalDirectory(filepath.Join(root, "missing"), destination); err != nil {
			t.Fatalf("copy missing directory: %v", err)
		}
		if _, err := os.Stat(destination); !os.IsNotExist(err) {
			t.Fatalf("destination exists after missing source copy: %v", err)
		}
	})

	t.Run("copies nested regular files", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		source := filepath.Join(root, "source")
		destination := filepath.Join(root, "destination")
		nested := filepath.Join(source, "nested")
		if err := os.MkdirAll(nested, 0o755); err != nil {
			t.Fatalf("create source: %v", err)
		}
		want := []byte("guide contents\n")
		if err := os.WriteFile(filepath.Join(nested, "guide.md"), want, 0o644); err != nil {
			t.Fatalf("write source guide: %v", err)
		}

		if err := copyOptionalDirectory(source, destination); err != nil {
			t.Fatalf("copy directory: %v", err)
		}
		got, err := os.ReadFile(filepath.Join(destination, "nested", "guide.md"))
		if err != nil {
			t.Fatalf("read copied guide: %v", err)
		}
		if string(got) != string(want) {
			t.Fatalf("copied guide = %q, want %q", got, want)
		}
	})
}
