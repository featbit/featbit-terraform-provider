package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	linkPattern     = regexp.MustCompile(`\[[^\]]*\]\(([^)]+)\)`)
	todoPattern     = regexp.MustCompile(`(?m)^- \[([ x])\] \*\*(P0-[0-9]{3})\*\*`)
	progressPattern = regexp.MustCompile(`TODO progress: \*\*([0-9]+) / ([0-9]+) completed\*\*`)
)

func main() {
	phase := flag.String("phase", "../../.context-wiki/plan-execution-phase-0", "Phase 0 context directory")
	flag.Parse()

	markdownFiles, evidenceFiles, acceptedADRs, findings, err := checkPhase(*phase)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	todoContent, err := os.ReadFile(filepath.Join(*phase, "todo.md"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "read todo.md")
		os.Exit(1)
	}
	todoCount, checkedCount, todoFindings := checkTODO(todoContent)
	findings += todoFindings
	statusContent, err := os.ReadFile(filepath.Join(*phase, "status.md"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "read status.md")
		os.Exit(1)
	}
	findings += checkProgress(statusContent, checkedCount, todoCount)

	fmt.Printf("markdown_files=%d evidence_files=%d todo_ids=%d checked_todos=%d accepted_adrs=%d findings=%d\n",
		markdownFiles,
		evidenceFiles,
		todoCount,
		checkedCount,
		acceptedADRs,
		findings,
	)
	if findings != 0 {
		os.Exit(1)
	}
}

func checkPhase(phase string) (int, int, int, int, error) {
	markdownFiles := 0
	evidenceFiles := 0
	acceptedADRs := 0
	findings := 0
	err := filepath.Walk(phase, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		markdownFiles++
		if filepath.Base(filepath.Dir(path)) == "evidence" && filepath.Base(path) != "README.md" {
			evidenceFiles++
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytesCountFence(content)%2 != 0 {
			fmt.Printf("%s category=unbalanced_fence\n", filepath.ToSlash(path))
			findings++
		}
		if strings.HasPrefix(filepath.Base(path), "ADR-") &&
			strings.Contains(string(content), "- Status: Accepted") {
			acceptedADRs++
		}
		for _, match := range linkPattern.FindAllSubmatch(content, -1) {
			target := string(match[1])
			if strings.HasPrefix(target, "http://") ||
				strings.HasPrefix(target, "https://") ||
				strings.HasPrefix(target, "#") ||
				strings.HasPrefix(target, "mailto:") {
				continue
			}
			target = strings.Trim(target, "<>")
			if index := strings.IndexByte(target, '#'); index >= 0 {
				target = target[:index]
			}
			if target == "" {
				continue
			}
			resolved := filepath.Clean(filepath.Join(filepath.Dir(path), filepath.FromSlash(target)))
			if _, err := os.Stat(resolved); err != nil {
				fmt.Printf("%s category=broken_link\n", filepath.ToSlash(path))
				findings++
			}
		}
		return nil
	})
	return markdownFiles, evidenceFiles, acceptedADRs, findings, err
}

func checkTODO(content []byte) (int, int, int) {
	matches := todoPattern.FindAllSubmatchIndex(content, -1)
	seen := map[string]struct{}{}
	checked := 0
	findings := 0
	for index, match := range matches {
		state := string(content[match[2]:match[3]])
		id := string(content[match[4]:match[5]])
		if _, duplicate := seen[id]; duplicate {
			fmt.Printf("todo.md category=duplicate_todo_id\n")
			findings++
		}
		seen[id] = struct{}{}
		if state == "x" {
			checked++
			end := len(content)
			if index+1 < len(matches) {
				end = matches[index+1][0]
			}
			if !strings.Contains(string(content[match[0]:end]), "Evidence:") {
				fmt.Printf("todo.md category=checked_todo_without_evidence\n")
				findings++
			}
		}
	}
	return len(seen), checked, findings
}

func checkProgress(content []byte, checked, total int) int {
	match := progressPattern.FindSubmatch(content)
	if match == nil {
		fmt.Printf("status.md category=missing_progress\n")
		return 1
	}
	gotChecked, _ := strconv.Atoi(string(match[1]))
	gotTotal, _ := strconv.Atoi(string(match[2]))
	if gotChecked != checked || gotTotal != total {
		fmt.Printf("status.md category=progress_mismatch\n")
		return 1
	}
	return 0
}

func bytesCountFence(content []byte) int {
	count := 0
	for _, line := range strings.Split(string(content), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			count++
		}
	}
	return count
}
