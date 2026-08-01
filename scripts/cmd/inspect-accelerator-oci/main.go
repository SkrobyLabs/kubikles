package main

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

type manifest struct {
	Config string   `json:"Config"`
	Layers []string `json:"Layers"`
}
type archive struct {
	manifest manifest
	blobs    map[string][]byte
}

func main() {
	if len(os.Args) != 3 {
		fail("usage: inspect-accelerator-oci FIRST SECOND")
	}
	a, b := read(os.Args[1]), read(os.Args[2])
	if descriptor(a.manifest) != descriptor(b.manifest) {
		fail("selected OCI manifest descriptors differ")
	}
	if a.manifest.Config != b.manifest.Config || !bytes.Equal(a.blobs[a.manifest.Config], b.blobs[b.manifest.Config]) {
		fail("config descriptor or blob differs")
	}
	if len(a.manifest.Layers) != len(b.manifest.Layers) {
		fail("compressed layer counts differ")
	}
	for i := range a.manifest.Layers {
		if a.manifest.Layers[i] != b.manifest.Layers[i] {
			fail("compressed layer descriptor %d differs", i)
		}
		if !bytes.Equal(a.blobs[a.manifest.Layers[i]], b.blobs[b.manifest.Layers[i]]) {
			fail("compressed layer blob %d differs", i)
		}
	}
}
func read(path string) archive {
	f, err := os.Open(path)
	if err != nil {
		fail("open %s: %v", path, err)
	}
	defer f.Close()
	files := map[string][]byte{}
	tr := tar.NewReader(f)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			fail("read %s: %v", path, err)
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			fail("read %s: %v", path, err)
		}
		files[h.Name] = b
	}
	var manifests []manifest
	if err := json.Unmarshal(files["manifest.json"], &manifests); err != nil || len(manifests) != 1 {
		fail("invalid exported OCI manifest in %s", path)
	}
	m := manifests[0]
	blobs := map[string][]byte{}
	for _, name := range append([]string{m.Config}, m.Layers...) {
		blobs[name] = files[name]
		if blobs[name] == nil {
			fail("missing blob %s", name)
		}
	}
	return archive{m, blobs}
}
func descriptor(m manifest) string {
	h := sha256.New()
	io.WriteString(h, m.Config)
	for _, layer := range m.Layers {
		io.WriteString(h, "\n")
		io.WriteString(h, layer)
	}
	return "sha256:" + fmt.Sprintf("%x", h.Sum(nil))
}
func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "accelerator OCI inspection failed: "+strings.TrimSpace(format)+"\n", args...)
	os.Exit(1)
}
