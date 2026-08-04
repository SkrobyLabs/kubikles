package main

import (
	"debug/buildinfo"
	"debug/elf"
	"fmt"
	"os"
	"runtime"
	"strings"
)

func main() {
	if len(os.Args) != 6 {
		fail("usage: inspect-accelerator-binary BINARY ARCH VERSION COMMIT DIRTY")
	}
	if err := inspect(os.Args[1], os.Args[2], os.Args[3], os.Args[4], os.Args[5]); err != nil {
		fail("%v", err)
	}
}

func inspect(path, arch, version, commit, dirty string) error {
	f, err := elf.Open(path)
	if err != nil {
		return fmt.Errorf("open ELF: %w", err)
	}
	defer f.Close()
	if f.FileHeader.Type != elf.ET_EXEC && f.FileHeader.Type != elf.ET_DYN {
		return fmt.Errorf("not executable ELF")
	}
	if want := elfMachine(arch); want == elf.EM_NONE || f.FileHeader.Machine != want {
		return fmt.Errorf("ELF machine=%s, want %s", f.FileHeader.Machine, want)
	}
	for _, p := range f.Progs {
		if p.Type == elf.PT_INTERP {
			return fmt.Errorf("dynamic interpreter present")
		}
	}
	for _, s := range f.Sections {
		if s.Name == ".dynamic" || s.Name == ".note.go.buildid" || strings.HasPrefix(s.Name, ".debug") || s.Name == ".symtab" {
			return fmt.Errorf("unexpected section %s", s.Name)
		}
	}
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read build info: %w", err)
	}
	seen := map[string]string{}
	for _, s := range info.Settings {
		seen[s.Key] = s.Value
	}
	if seen["-trimpath"] != "true" {
		return fmt.Errorf("-trimpath=%q", seen["-trimpath"])
	}
	for key := range seen {
		if strings.HasPrefix(key, "vcs.") {
			return fmt.Errorf("unexpected VCS build setting %s", key)
		}
	}
	if seen["GOOS"] != "linux" {
		return fmt.Errorf("GOOS=%q", seen["GOOS"])
	}
	if seen["GOARCH"] != arch {
		return fmt.Errorf("GOARCH=%q, want %q", seen["GOARCH"], arch)
	}
	if seen["CGO_ENABLED"] != "0" {
		return fmt.Errorf("CGO_ENABLED=%q", seen["CGO_ENABLED"])
	}
	if seen["-tags"] != "headless,accelerator" {
		return fmt.Errorf("tags=%q", seen["-tags"])
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read binary: %w", err)
	}
	text := string(b)
	for _, forbidden := range []string{"github.com/wailsapp/wails", "kubikles/pkg/helm", "kubikles/pkg/terminal", "kubikles/pkg/ai"} {
		if strings.Contains(text, forbidden) {
			return fmt.Errorf("forbidden dependency %s", forbidden)
		}
	}
	identity := "kubikles-accelerator-build-identity:" + version + "|" + commit + "|" + dirty
	if !strings.Contains(text, identity) {
		return fmt.Errorf("missing exact build identity %q", identity)
	}
	for _, required := range []string{"frontend/dist/index.html"} {
		if !strings.Contains(text, required) {
			return fmt.Errorf("missing embedded asset identity %q", required)
		}
	}
	return nil
}
func elfMachine(arch string) elf.Machine {
	if arch == "" {
		arch = runtime.GOARCH
	}
	switch arch {
	case "amd64":
		return elf.EM_X86_64
	case "arm64":
		return elf.EM_AARCH64
	default:
		return elf.EM_NONE
	}
}
func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "accelerator binary inspection failed: "+format+"\n", args...)
	os.Exit(1)
}
