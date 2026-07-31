package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type rule struct {
	name    string
	pattern *regexp.Regexp
	allow   func(string) bool
}

var rules = []rule{
	{
		name:    "featbit_api_token",
		pattern: regexp.MustCompile(`\bapi-[A-Za-z0-9_-]{20,}\b`),
		allow:   allowSyntheticSecret,
	},
	{
		name:    "jwt",
		pattern: regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{4,}\.[A-Za-z0-9_-]{4,}\b`),
		allow:   allowSyntheticSecret,
	},
	{
		name:    "private_key",
		pattern: regexp.MustCompile(`-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----`),
	},
	{
		name:    "test_token_assignment",
		pattern: regexp.MustCompile(`(?i)FEATBIT_TEST_(?:SERVICE|PERSONAL)_TOKEN\s*[:=]\s*["']?([^\s"'` + "`" + `]+)`),
		allow:   allowSyntheticSecret,
	},
	{
		name:    "member_email",
		pattern: regexp.MustCompile(`(?i)\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b`),
		allow:   allowExampleEmail,
	},
}

func main() {
	root := flag.String("root", "../..", "repository root")
	flag.Parse()

	files := 0
	findings := 0
	err := filepath.Walk(*root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			if info.Name() == ".git" || info.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		files++
		fileFindings, err := scanFile(path)
		if err != nil {
			return err
		}
		findings += fileFindings
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "secret scan failed without printing file content")
		os.Exit(1)
	}
	fmt.Printf("scanned_files=%d findings=%d\n", files, findings)
	if findings != 0 {
		os.Exit(1)
	}
}

func scanFile(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	lineNumber := 0
	findings := 0
	for {
		line, readErr := reader.ReadString('\n')
		if len(line) > 0 {
			lineNumber++
			for _, candidate := range rules {
				for _, match := range candidate.pattern.FindAllString(line, -1) {
					if candidate.allow != nil && candidate.allow(match) {
						continue
					}
					// Report location and category only. Never print the match.
					fmt.Printf("%s:%d category=%s\n", filepath.ToSlash(path), lineNumber, candidate.name)
					findings++
				}
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return findings, readErr
		}
	}
	return findings, nil
}

func allowSyntheticSecret(value string) bool {
	value = strings.ToLower(value)
	for _, marker := range []string{"synthetic", "example", "placeholder", "<token>", "{token}", "abcdefgh"} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func allowExampleEmail(value string) bool {
	value = strings.ToLower(value)
	return strings.HasSuffix(value, "@example.test") ||
		strings.HasSuffix(value, "@example.com") ||
		strings.HasSuffix(value, "@example.org")
}
