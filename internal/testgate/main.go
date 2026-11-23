// Command testgate rejects failed, skipped, or empty offline test runs.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

type resultSummary struct {
	passed    int
	failed    int
	skipped   int
	completed int
}

func (s resultSummary) String() string {
	return fmt.Sprintf("Offline test results: %d passed, %d failed, %d skipped; %d packages completed", s.passed, s.failed, s.skipped, s.completed)
}

func validateResults(input io.Reader) (resultSummary, error) {
	var summary resultSummary
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	for scanner.Scan() {
		var event struct{ Action, Test, Package string }
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return summary, err
		}
		switch event.Action {
		case "pass":
			if event.Test != "" {
				summary.passed++
			} else {
				summary.completed++
			}
		case "fail":
			summary.failed++
		case "skip":
			if event.Test != "" {
				summary.skipped++
			} else {
				// Go reports packages with no test files as package-level skips.
				summary.completed++
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return summary, err
	}
	if summary.passed == 0 || summary.completed == 0 || summary.failed > 0 || summary.skipped > 0 {
		return summary, errors.New("offline tests must pass without skipped test cases")
	}
	return summary, nil
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: testgate test-results.jsonl")
		os.Exit(2)
	}
	input, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer input.Close()

	summary, err := validateResults(input)
	fmt.Println(summary)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
