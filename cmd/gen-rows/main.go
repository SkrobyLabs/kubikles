// cmd/gen-rows writes frontend/src/generated/rowSchemas.ts from the row spec registry in
// pkg/rows, so the frontend column schema can never drift from the Go specs.
//
// Usage: go run cmd/gen-rows/main.go [root]
package main

import (
	"log"
	"os"
	"path/filepath"

	"kubikles/pkg/rows"
)

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	out := filepath.Join(root, "frontend", "src", "generated", "rowSchemas.ts")
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(out, rows.GenerateTS(), 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %s for %d types", out, len(rows.Types()))
}
