package supplychain

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func ValidateWorkflows(root string, toolchain Toolchain) error {
	if ValidateToolchain(toolchain) != nil {
		return errors.New("workflow toolchain invalid")
	}
	paths := []string{"build.yml", "pull-request.yml", "main.yml", "release.yml"}
	combined := ""
	for _, name := range paths {
		data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", name))
		if err != nil {
			return errors.New("workflow unavailable")
		}
		text := string(data)
		combined += "\n" + text
		for _, forbidden := range []string{"pull_request_target", "workflow_run", "issue_comment", "security-events: write"} {
			if strings.Contains(text, forbidden) {
				return errors.New("workflow authority invalid")
			}
		}
	}
	for _, action := range toolchain.Actions {
		needle := "uses: " + action.Name + "@" + action.Revision
		if !strings.Contains(combined, needle) {
			return errors.New("workflow action missing")
		}
	}
	uses := regexp.MustCompile(`(?m)^\s*-?\s*uses:\s*([^\s#]+)`).FindAllStringSubmatch(combined, -1)
	allowed := make(map[string]struct{}, len(toolchain.Actions)+1)
	for _, action := range toolchain.Actions {
		allowed[action.Name+"@"+action.Revision] = struct{}{}
	}
	allowed["./.github/workflows/build.yml"] = struct{}{}
	for _, match := range uses {
		if _, ok := allowed[match[1]]; !ok {
			return errors.New("workflow action is not pinned")
		}
	}
	pull, _ := os.ReadFile(filepath.Join(root, ".github", "workflows", "pull-request.yml"))
	main, _ := os.ReadFile(filepath.Join(root, ".github", "workflows", "main.yml"))
	release, _ := os.ReadFile(filepath.Join(root, ".github", "workflows", "release.yml"))
	if !strings.Contains(string(pull), "pull_request:") || !strings.Contains(string(pull), "workflow_dispatch:") || strings.Contains(string(pull), "packages: write") || strings.Contains(string(pull), "id-token: write") || !strings.Contains(string(pull), "build_version: v0.0.0") {
		return errors.New("pull request authority invalid")
	}
	if !strings.Contains(string(main), "branches: [main]") || !strings.Contains(string(main), "make test-accelerator-e2e") || !strings.Contains(string(main), "make test-accelerator-supply-chain BUILD_VERSION=v0.0.0") || strings.Contains(string(main), "packages: write") || strings.Contains(string(main), "id-token: write") {
		return errors.New("main authority invalid")
	}
	if !strings.Contains(string(release), "cancel-in-progress: false") || !strings.Contains(string(release), "release-acceptance:") || !strings.Contains(string(release), "accelerator-prepare:") || !strings.Contains(string(release), "accelerator-publish:") || !strings.Contains(string(release), "accelerator-attest:") || strings.Count(string(release), "actions/attest@508db95dd578ae2727ebd6217d5ba78e4fbda05d") != 6 {
		return errors.New("release authority invalid")
	}
	return nil
}
