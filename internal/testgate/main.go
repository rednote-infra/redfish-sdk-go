// Command testgate rejects skipped or incomplete offline test runs.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

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
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	completed, passed, failed, skipped := 0, 0, 0, 0
	for scanner.Scan() {
		var event struct{ Action, Test, Package string }
		if err = json.Unmarshal(scanner.Bytes(), &event); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		switch event.Action {
		case "pass":
			if event.Test != "" {
				passed++
			} else {
				completed++
			}
		case "fail":
			failed++
		case "skip":
			skipped++
		}
	}
	if err = scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("Offline test results: %d passed, %d failed, %d skipped; %d packages completed\n", passed, failed, skipped, completed)
	if passed == 0 || completed == 0 || failed > 0 || skipped > 0 {
		os.Exit(1)
	}
}
