package main

import (
	"os"

	"kubikles/internal/acceleratoracceptance"
)

func main() {
	if len(os.Args) != 2 || acceleratoracceptance.CanonicalizeAcceptanceOCIImageLayout(os.Args[1], acceleratoracceptance.BuildIdentity) != nil {
		os.Exit(1)
	}
}
