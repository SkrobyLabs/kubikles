package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) != 2 {
		fatal("usage: accelerator-docs <validate|render-evidence|capture-evidence>")
	}
	root, err := repositoryRoot()
	if err != nil {
		fatal(err.Error())
	}
	switch os.Args[1] {
	case "validate":
		err = validateAll(root)
	case "render-evidence":
		var m evidenceManifest
		var r map[string]evidenceRecord
		m, r, err = loadEvidence(root)
		if err == nil {
			_, err = os.Stdout.Write(renderEvidence(m, r))
		}
	case "capture-evidence":
		err = captureEvidence(root)
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fatal(err.Error())
	}
}
func fatal(s string) { fmt.Fprintln(os.Stderr, "accelerator-docs:", s); os.Exit(1) }
func repositoryRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if info, e := os.Stat(filepath.Join(dir, "go.mod")); e == nil && info.Mode().IsRegular() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("repository root not found")
		}
		dir = parent
	}
}
