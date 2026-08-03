package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func authorityDigest(root string, paths []string) (string, error) {
	if !equal(paths, expectedAuthorityInputs) {
		return "", errors.New("DOC-AUTHORITY-SET")
	}
	h := sha256.New()
	for _, rel := range paths {
		if filepath.IsAbs(rel) || filepath.Clean(rel) != filepath.FromSlash(rel) {
			return "", errors.New("DOC-AUTHORITY-PATH")
		}
		path := filepath.Join(root, filepath.FromSlash(rel))
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("DOC-AUTHORITY-FILE: %s", rel)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("DOC-AUTHORITY-READ: %s", rel)
		}
		mode := "0644"
		if info.Mode().Perm()&0o111 != 0 {
			mode = "0755"
		}
		io.WriteString(h, rel)
		h.Write([]byte{0})
		io.WriteString(h, mode)
		h.Write([]byte{0})
		io.WriteString(h, fmt.Sprintf("%d", len(b)))
		h.Write([]byte{0})
		h.Write(b)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
