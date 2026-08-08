package main

import (
	"fmt"
	"os"

	"kubikles/internal/acceleratoracceptance"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "accelerator-e2e-shared-fixture: invalid")
		os.Exit(1)
	}
	if _, err := acceleratoracceptance.LoadSharedFixture(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "accelerator-e2e-shared-fixture: invalid")
		os.Exit(1)
	}
}
