package helm

import (
	"bufio"
	"strings"

	"sigs.k8s.io/yaml"
)

// parseManifestResources extracts resource references from a Helm manifest
func parseManifestResources(manifest, defaultNamespace string) ([]ResourceReference, error) {
	var resources []ResourceReference

	// Split manifest into individual documents
	scanner := bufio.NewScanner(strings.NewReader(manifest))
	scanner.Split(splitYAMLDocuments)

	for scanner.Scan() {
		doc := strings.TrimSpace(scanner.Text())
		if doc == "" || doc == "---" {
			continue
		}

		// Parse the YAML document to extract metadata
		var obj struct {
			Kind     string `json:"kind"`
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
		}

		if err := yaml.Unmarshal([]byte(doc), &obj); err != nil {
			continue // Skip invalid documents
		}

		if obj.Kind == "" || obj.Metadata.Name == "" {
			continue
		}

		ns := obj.Metadata.Namespace
		if ns == "" {
			ns = defaultNamespace
		}

		resources = append(resources, ResourceReference{
			Kind:      obj.Kind,
			Name:      obj.Metadata.Name,
			Namespace: ns,
		})
	}

	return resources, nil
}

// splitYAMLDocuments is a split function for Scanner that splits on YAML document separators
func splitYAMLDocuments(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}

	// Look for document separator
	sep := []byte("\n---")
	if i := strings.Index(string(data), string(sep)); i >= 0 {
		return i + len(sep), data[0:i], nil
	}

	// If at EOF, return what's left
	if atEOF {
		return len(data), data, nil
	}

	// Request more data
	return 0, nil, nil
}
