//go:build ignore

// Command generate writes the benchmark fixtures:
//
//	go run bench/fixtures/generate.go bench/fixtures
package main

import (
	"fmt"
	"os"

	"github.com/asaf-shitrit/pdfcmp/internal/fixtures"
)

func main() {
	dir := "."
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	if err := fixtures.Generate(dir, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
