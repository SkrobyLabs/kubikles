package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var markdownLink = regexp.MustCompile(`\[[^]]+\]\(([^)]+)\)`)
var heading = regexp.MustCompile(`(?m)^#{1,6}\s+(.+?)\s*$`)

func validateMarkdown(root string, c contract) error {
	for _, rel := range append([]string{"README.md", "docs/README.md", "docs/server-mode.md"}, c.Pages...) {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if os.IsNotExist(err) && rel == "docs/accelerator/evidence.md" {
			continue
		}
		if err != nil {
			return fmt.Errorf("DOC-PAGE-MISSING: %s", rel)
		}
		text := string(b)
		anchors := map[string]bool{}
		for _, m := range heading.FindAllStringSubmatch(text, -1) {
			slug := githubSlug(m[1])
			if anchors[slug] {
				return fmt.Errorf("DOC-ANCHOR-DUPLICATE: %s", rel)
			}
			anchors[slug] = true
		}
		for _, m := range markdownLink.FindAllStringSubmatch(text, -1) {
			link := strings.TrimSpace(m[1])
			if strings.Contains(link, "://") || strings.HasPrefix(link, "mailto:") {
				continue
			}
			parts := strings.SplitN(link, "#", 2)
			target := parts[0]
			if target == "" {
				target = rel
			}
			clean := filepath.Clean(filepath.Join(filepath.Dir(rel), filepath.FromSlash(target)))
			if strings.HasPrefix(clean, "..") {
				return fmt.Errorf("DOC-LINK-TRAVERSAL: %s", rel)
			}
			absolute := filepath.Join(root, clean)
			info, err := os.Stat(absolute)
			if os.IsNotExist(err) && clean == "docs/accelerator/evidence.md" {
				continue
			}
			if err != nil {
				return fmt.Errorf("DOC-LINK-BROKEN: %s -> %s", rel, clean)
			}
			if info.IsDir() {
				if len(parts) == 2 {
					return fmt.Errorf("DOC-LINK-FRAGMENT: %s", rel)
				}
				continue
			}
			data, err := os.ReadFile(absolute)
			if err != nil {
				return fmt.Errorf("DOC-LINK-BROKEN: %s -> %s", rel, clean)
			}
			if len(parts) == 2 {
				found := false
				for _, h := range heading.FindAllStringSubmatch(string(data), -1) {
					if githubSlug(h[1]) == parts[1] {
						found = true
					}
				}
				if !found {
					return fmt.Errorf("DOC-LINK-FRAGMENT: %s", rel)
				}
			}
		}
	}
	server, _ := os.ReadFile(filepath.Join(root, "docs/server-mode.md"))
	s := string(server)
	if strings.Count(s, "<!-- accelerator-docs:begin distinction -->") != 1 || strings.Count(s, "<!-- accelerator-docs:end distinction -->") != 1 {
		return errors.New("DOC-SERVER-DISTINCTION")
	}
	return nil
}

func githubSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(s, "`", "")))
	var b strings.Builder
	dash := false
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			dash = false
		} else if (r == ' ' || r == '-') && !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}
