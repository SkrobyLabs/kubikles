package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"debug/buildinfo"
	"debug/elf"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"sigs.k8s.io/yaml"
)

const (
	sourceRepository = "https://github.com/SkrobyLabs/kubikles"
	imageRepository  = "ghcr.io/skrobylabs/kubikles-accelerator"
	chartRepository  = "oci://ghcr.io/skrobylabs/helm/kubikles-accelerator"
)

var (
	stableTagRE  = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	releaseTagRE = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-(?:0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*))*)?$`)
	commitRE     = regexp.MustCompile(`^[0-9a-f]{40}$`)
	digestRE     = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

var chartSourceFiles = []string{
	"Chart.yaml",
	"templates/_helpers.tpl",
	"templates/clusterrole.yaml",
	"templates/clusterrolebinding.yaml",
	"templates/creator-verifier-secret.yaml",
	"templates/job.yaml",
	"templates/serviceaccount.yaml",
	"values.schema.json",
	"values.yaml",
}

func gitOutput(repository string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", repository}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", errors.New("Git source validation failed")
	}
	return strings.TrimSpace(string(out)), nil
}

// VerifyGitSource makes the checked-out release commit the sole source of
// authority. expectedCommit is empty during the first preflight and is the
// previously observed commit during every pre-write/read-back recheck.
func VerifyGitSource(repository, tag, expectedCommit string) (string, error) {
	if _, err := NormalizeReleaseTag(tag); err != nil {
		return "", err
	}
	if expectedCommit != "" && !commitRE.MatchString(expectedCommit) {
		return "", errors.New("expected release commit is invalid")
	}
	shallow, err := gitOutput(repository, "rev-parse", "--is-shallow-repository")
	if err != nil || shallow != "false" {
		return "", errors.New("release checkout must be a complete Git repository")
	}
	dirty, err := gitOutput(repository, "status", "--porcelain=v1", "--untracked-files=normal")
	if err != nil || dirty != "" {
		return "", errors.New("release checkout must be clean")
	}
	head, err := gitOutput(repository, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil || !commitRE.MatchString(head) {
		return "", errors.New("release HEAD is not a full canonical commit")
	}
	ref, err := gitOutput(repository, "show-ref", "--verify", "--hash", "refs/tags/"+tag)
	if err != nil || ref == "" || strings.Contains(ref, "\n") {
		return "", errors.New("release tag is absent or ambiguous")
	}
	tagCommit, err := gitOutput(repository, "rev-parse", "--verify", "refs/tags/"+tag+"^{commit}")
	if err != nil || !commitRE.MatchString(tagCommit) || tagCommit != head {
		return "", errors.New("release tag does not resolve to checked-out HEAD")
	}
	if expectedCommit != "" && head != expectedCommit {
		return "", errors.New("release tag moved after preflight")
	}
	return head, nil
}

type Version struct {
	GitTag       string `json:"gitTag"`
	BuildVersion string `json:"buildVersion"`
	ChartVersion string `json:"chartVersion"`
	Prerelease   bool   `json:"prerelease"`
}

func NormalizeStableReleaseTag(tag string) (Version, error) {
	if !stableTagRE.MatchString(tag) {
		return Version{}, errors.New("release tag must be canonical vMAJOR.MINOR.PATCH")
	}
	return Version{GitTag: tag, BuildVersion: tag, ChartVersion: strings.TrimPrefix(tag, "v")}, nil
}

func NormalizeReleaseTag(tag string) (Version, error) {
	if !releaseTagRE.MatchString(tag) {
		return Version{}, errors.New("release tag must be canonical vMAJOR.MINOR.PATCH or vMAJOR.MINOR.PATCH-PRERELEASE")
	}
	return Version{GitTag: tag, BuildVersion: tag, ChartVersion: strings.TrimPrefix(tag, "v"), Prerelease: !stableTagRE.MatchString(tag)}, nil
}

type Platform struct {
	OS             string `json:"os"`
	Architecture   string `json:"architecture"`
	ManifestDigest string `json:"manifestDigest"`
}

type Source struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
	GitTag     string `json:"gitTag"`
}

type Compatibility struct {
	Mode                    string `json:"mode"`
	DesktopBuildVersion     string `json:"desktopBuildVersion"`
	AcceleratorBuildVersion string `json:"acceleratorBuildVersion"`
}

type Image struct {
	Repository string     `json:"repository"`
	Digest     string     `json:"digest"`
	Reference  string     `json:"reference"`
	Platforms  []Platform `json:"platforms"`
}

type Chart struct {
	Repository string `json:"repository"`
	Digest     string `json:"digest"`
	Reference  string `json:"reference"`
	Version    string `json:"version"`
	AppVersion string `json:"appVersion"`
}

type Descriptor struct {
	SchemaVersion int           `json:"schemaVersion"`
	Schema        string        `json:"$schema"`
	BuildVersion  string        `json:"buildVersion"`
	Source        Source        `json:"source"`
	Compatibility Compatibility `json:"compatibility"`
	Image         Image         `json:"image"`
	Chart         Chart         `json:"chart"`
}

type Evidence struct {
	BuildVersion    string     `json:"buildVersion"`
	Commit          string     `json:"commit"`
	GitTag          string     `json:"gitTag"`
	ImageDigest     string     `json:"imageDigest"`
	Platforms       []Platform `json:"platforms"`
	ChartDigest     string     `json:"chartDigest"`
	ChartVersion    string     `json:"chartVersion"`
	ChartAppVersion string     `json:"chartAppVersion"`
}

func NewDescriptor(e Evidence) (Descriptor, error) {
	v, err := NormalizeReleaseTag(e.BuildVersion)
	if err != nil || e.GitTag != e.BuildVersion || e.ChartVersion != v.ChartVersion || e.ChartAppVersion != e.BuildVersion {
		return Descriptor{}, errors.New("release evidence has inconsistent exact version identity")
	}
	if !commitRE.MatchString(e.Commit) || !digestRE.MatchString(e.ImageDigest) || !digestRE.MatchString(e.ChartDigest) {
		return Descriptor{}, errors.New("release evidence has invalid commit or digest identity")
	}
	platforms := append([]Platform(nil), e.Platforms...)
	sort.Slice(platforms, func(i, j int) bool { return platforms[i].Architecture < platforms[j].Architecture })
	if len(platforms) != 2 || platforms[0].OS != "linux" || platforms[0].Architecture != "amd64" || platforms[1].OS != "linux" || platforms[1].Architecture != "arm64" || platforms[0].ManifestDigest == platforms[1].ManifestDigest {
		return Descriptor{}, errors.New("release evidence must contain exactly linux/amd64 and linux/arm64")
	}
	for _, p := range platforms {
		if !digestRE.MatchString(p.ManifestDigest) {
			return Descriptor{}, errors.New("release evidence has invalid platform digest")
		}
	}
	d := Descriptor{
		SchemaVersion: 1,
		Schema:        "https://raw.githubusercontent.com/SkrobyLabs/kubikles/" + e.Commit + "/release/accelerator-release.schema.json",
		BuildVersion:  e.BuildVersion,
		Source:        Source{Repository: sourceRepository, Commit: e.Commit, GitTag: e.GitTag},
		Compatibility: Compatibility{Mode: "exact-build-version", DesktopBuildVersion: e.BuildVersion, AcceleratorBuildVersion: e.BuildVersion},
		Image:         Image{Repository: imageRepository, Digest: e.ImageDigest, Reference: imageRepository + "@" + e.ImageDigest, Platforms: platforms},
		Chart:         Chart{Repository: chartRepository, Digest: e.ChartDigest, Reference: chartRepository + "@" + e.ChartDigest, Version: e.ChartVersion, AppVersion: e.ChartAppVersion},
	}
	return d, ValidateDescriptor(d)
}

func ValidateDescriptor(d Descriptor) error {
	v, err := NormalizeReleaseTag(d.BuildVersion)
	if err != nil || d.SchemaVersion != 1 || !commitRE.MatchString(d.Source.Commit) {
		return errors.New("descriptor identity is invalid")
	}
	if d.Schema != "https://raw.githubusercontent.com/SkrobyLabs/kubikles/"+d.Source.Commit+"/release/accelerator-release.schema.json" || d.Source.Repository != sourceRepository || d.Source.GitTag != d.BuildVersion {
		return errors.New("descriptor source identity is inconsistent")
	}
	if d.Compatibility.Mode != "exact-build-version" || d.Compatibility.DesktopBuildVersion != d.BuildVersion || d.Compatibility.AcceleratorBuildVersion != d.BuildVersion {
		return errors.New("descriptor compatibility must be exact BuildVersion equality")
	}
	if d.Image.Repository != imageRepository || !digestRE.MatchString(d.Image.Digest) || d.Image.Reference != d.Image.Repository+"@"+d.Image.Digest {
		return errors.New("descriptor image reference is not immutable")
	}
	if d.Chart.Repository != chartRepository || !digestRE.MatchString(d.Chart.Digest) || d.Chart.Reference != d.Chart.Repository+"@"+d.Chart.Digest || d.Chart.Version != v.ChartVersion || d.Chart.AppVersion != d.BuildVersion {
		return errors.New("descriptor chart reference or version is inconsistent")
	}
	if len(d.Image.Platforms) != 2 || d.Image.Platforms[0].OS != "linux" || d.Image.Platforms[0].Architecture != "amd64" || d.Image.Platforms[1].OS != "linux" || d.Image.Platforms[1].Architecture != "arm64" {
		return errors.New("descriptor platform set or order is invalid")
	}
	for _, p := range d.Image.Platforms {
		if !digestRE.MatchString(p.ManifestDigest) {
			return errors.New("descriptor platform digest is invalid")
		}
	}
	if d.Image.Platforms[0].ManifestDigest == d.Image.Platforms[1].ManifestDigest {
		return errors.New("descriptor platform manifests must be distinct")
	}
	return nil
}

func CanonicalDescriptor(d Descriptor) ([]byte, error) {
	if err := ValidateDescriptor(d); err != nil {
		return nil, err
	}
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return nil, errors.New("encode descriptor")
	}
	return append(b, '\n'), nil
}

func DescriptorChecksum(name string, b []byte) []byte {
	sum := sha256.Sum256(b)
	return []byte(hex.EncodeToString(sum[:]) + "  " + filepath.Base(name) + "\n")
}

type chartMetadata struct {
	APIVersion  string `yaml:"apiVersion"`
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	Type        string `yaml:"type,omitempty"`
	Version     string `yaml:"version"`
	AppVersion  string `yaml:"appVersion"`
}

func PackageChart(source, output, chartVersion, appVersion string, epoch int64) error {
	if _, err := NormalizeReleaseTag(appVersion); err != nil || strings.TrimPrefix(appVersion, "v") != chartVersion || epoch <= 0 {
		return errors.New("invalid deterministic chart identity")
	}
	root, err := filepath.Abs(source)
	if err != nil {
		return errors.New("resolve chart source")
	}
	var files []string
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if entry.IsDir() {
			if rel == "tests" || strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() || strings.HasPrefix(entry.Name(), ".") {
			return nil
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return errors.New("read chart source")
	}
	sort.Strings(files)
	if strings.Join(files, "\n") != strings.Join(chartSourceFiles, "\n") {
		return errors.New("chart source file set is not exact")
	}
	var buf bytes.Buffer
	gz, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	gz.Header.ModTime = time.Unix(epoch, 0).UTC()
	gz.Header.OS = 255
	gz.Header.Name = ""
	tw := tar.NewWriter(gz)
	for _, rel := range files {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return errors.New("read chart file")
		}
		if rel == "Chart.yaml" {
			var meta chartMetadata
			if err := yaml.Unmarshal(b, &meta); err != nil || meta.Name != "kubikles-accelerator" || meta.APIVersion != "v2" {
				return errors.New("invalid source Chart.yaml")
			}
			meta.Version, meta.AppVersion = chartVersion, appVersion
			b, err = yaml.Marshal(meta)
			if err != nil {
				return errors.New("stage Chart.yaml")
			}
		}
		name := "kubikles-accelerator/" + rel
		h := &tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0644, Size: int64(len(b)), ModTime: time.Unix(epoch, 0).UTC(), AccessTime: time.Time{}, ChangeTime: time.Time{}, Uid: 0, Gid: 0, Uname: "", Gname: "", Format: tar.FormatPAX}
		if err := tw.WriteHeader(h); err != nil {
			return errors.New("write chart header")
		}
		if _, err := tw.Write(b); err != nil {
			return errors.New("write chart content")
		}
	}
	if err := tw.Close(); err != nil {
		return errors.New("finish chart archive")
	}
	if err := gz.Close(); err != nil {
		return errors.New("finish chart compression")
	}
	if err := os.WriteFile(output, buf.Bytes(), 0644); err != nil {
		return errors.New("write chart package")
	}
	return nil
}

func InspectChart(path, source, version, appVersion string, epoch int64) error {
	if _, err := NormalizeReleaseTag(appVersion); err != nil || strings.TrimPrefix(appVersion, "v") != version || epoch <= 0 {
		return errors.New("invalid chart inspection identity")
	}
	f, err := os.Open(path)
	if err != nil {
		return errors.New("open chart package")
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return errors.New("open chart gzip")
	}
	defer gz.Close()
	canonicalTime := time.Unix(epoch, 0).UTC()
	if !gz.ModTime.Equal(canonicalTime) || gz.Name != "" || gz.Comment != "" || gz.Extra != nil || gz.OS != 255 {
		return errors.New("chart gzip header is not canonical")
	}
	tr := tar.NewReader(gz)
	seen := map[string]bool{}
	var meta chartMetadata
	var names []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.New("read chart package")
		}
		if h.Typeflag != tar.TypeReg || h.Uid != 0 || h.Gid != 0 || h.Uname != "" || h.Gname != "" || h.Mode != 0644 || !h.ModTime.Equal(canonicalTime) || !h.AccessTime.IsZero() || !h.ChangeTime.IsZero() || h.Name == "" || seen[h.Name] || !strings.HasPrefix(h.Name, "kubikles-accelerator/") {
			return errors.New("chart archive metadata is not canonical")
		}
		seen[h.Name] = true
		rel := strings.TrimPrefix(h.Name, "kubikles-accelerator/")
		names = append(names, rel)
		b, readErr := io.ReadAll(tr)
		if readErr != nil || int64(len(b)) != h.Size {
			return errors.New("read chart package content")
		}
		if h.Name == "kubikles-accelerator/Chart.yaml" {
			if yaml.Unmarshal(b, &meta) != nil {
				return errors.New("decode packaged Chart.yaml")
			}
			var sourceMeta chartMetadata
			sourceBytes, sourceErr := os.ReadFile(filepath.Join(source, "Chart.yaml"))
			if sourceErr != nil || yaml.Unmarshal(sourceBytes, &sourceMeta) != nil {
				return errors.New("read source Chart.yaml")
			}
			sourceMeta.Version, sourceMeta.AppVersion = version, appVersion
			want, marshalErr := yaml.Marshal(sourceMeta)
			if marshalErr != nil || !bytes.Equal(b, want) {
				return errors.New("packaged Chart.yaml differs from staged source")
			}
		} else {
			want, sourceErr := os.ReadFile(filepath.Join(source, filepath.FromSlash(rel)))
			if sourceErr != nil || !bytes.Equal(b, want) {
				return errors.New("packaged chart content differs from source")
			}
		}
	}
	if strings.Join(names, "\n") != strings.Join(chartSourceFiles, "\n") || meta.Name != "kubikles-accelerator" || meta.Version != version || meta.AppVersion != appVersion {
		return errors.New("packaged chart identity is invalid")
	}
	return validateRenderedChart(path, version, appVersion)
}

func chartNested(value any, keys ...string) any {
	for _, key := range keys {
		object, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		value = object[key]
	}
	return value
}

func chartStrings(value any) string {
	items, ok := value.([]any)
	if !ok {
		return ""
	}
	result := make([]string, len(items))
	for i, item := range items {
		result[i], _ = item.(string)
	}
	return strings.Join(result, ",")
}

func validateRenderedChart(path, version, appVersion string) error {
	if _, err := exec.LookPath("helm"); err != nil {
		return errors.New("Helm is required to inspect the Accelerator chart")
	}
	tmp, err := os.MkdirTemp("", "accelerator-chart-inspect-")
	if err != nil {
		return errors.New("create chart inspection directory")
	}
	defer os.RemoveAll(tmp)
	valuesPath := filepath.Join(tmp, "values.yaml")
	values := "image:\n  repository: example.invalid/kubikles-accelerator\n  digest: sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\n  version: " + appVersion + "\naccelerator:\n  workloadSessionId: release-contract\nauth:\n  creatorVerifier: w2gLrXNNILGDLmRDyzm2sAmvRsdu_fbQpzmryPK-hlM\n"
	if err := os.WriteFile(valuesPath, []byte(values), 0600); err != nil {
		return errors.New("write chart inspection values")
	}
	if err := exec.Command("helm", "lint", "--strict", path, "--values", valuesPath).Run(); err != nil {
		return errors.New("packaged Accelerator chart failed strict schema validation")
	}
	out, err := exec.Command("helm", "template", "release", path, "--namespace", "release-contract", "--values", valuesPath).Output()
	if err != nil {
		return errors.New("packaged Accelerator chart failed rendering")
	}
	objects := map[string]map[string]any{}
	for _, document := range bytes.Split(out, []byte("\n---")) {
		if len(bytes.TrimSpace(document)) == 0 {
			continue
		}
		var object map[string]any
		if yaml.Unmarshal(document, &object) != nil {
			return errors.New("packaged Accelerator chart rendered invalid YAML")
		}
		kind, _ := object["kind"].(string)
		if kind == "" || objects[kind] != nil {
			return errors.New("packaged Accelerator chart object set is not exact")
		}
		objects[kind] = object
	}
	for _, kind := range []string{"Job", "Secret", "ServiceAccount", "ClusterRole", "ClusterRoleBinding"} {
		if objects[kind] == nil {
			return errors.New("packaged Accelerator chart object set is not exact")
		}
	}
	if len(objects) != 5 {
		return errors.New("packaged Accelerator chart object set is not exact")
	}
	job := objects["Job"]
	spec, _ := chartNested(job, "spec").(map[string]any)
	pod, _ := chartNested(job, "spec", "template", "spec").(map[string]any)
	containers, _ := pod["containers"].([]any)
	if len(spec) != 5 || len(pod) != 7 || len(containers) != 1 || spec["completions"] != float64(1) || spec["parallelism"] != float64(1) || spec["backoffLimit"] != float64(0) || spec["ttlSecondsAfterFinished"] != float64(3600) || pod["restartPolicy"] != "Never" || pod["serviceAccountName"] != "release-kubikles-accelerator" || pod["automountServiceAccountToken"] != false {
		return errors.New("packaged Accelerator Job lifecycle contract differs")
	}
	container, _ := containers[0].(map[string]any)
	if len(container) != 7 || container["name"] != "accelerator" || container["image"] != "example.invalid/kubikles-accelerator@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" || container["imagePullPolicy"] != "IfNotPresent" || chartNested(container, "securityContext", "readOnlyRootFilesystem") != true || chartNested(container, "securityContext", "allowPrivilegeEscalation") != false || chartStrings(chartNested(container, "securityContext", "capabilities", "drop")) != "ALL" {
		return errors.New("packaged Accelerator container security contract differs")
	}
	if chartNested(container, "resources", "requests", "cpu") != "100m" || chartNested(container, "resources", "requests", "memory") != "128Mi" || chartNested(container, "resources", "limits", "cpu") != "1" || chartNested(container, "resources", "limits", "memory") != "512Mi" {
		return errors.New("packaged Accelerator resource contract differs")
	}
	environment, _ := container["env"].([]any)
	mounts, _ := container["volumeMounts"].([]any)
	volumes, _ := pod["volumes"].([]any)
	if len(environment) != 1 || chartNested(environment[0], "name") != "KUBIKLES_ACCELERATOR_CREATOR_VERIFIER" || chartNested(environment[0], "valueFrom", "secretKeyRef", "name") != "release-kubikles-accelerator-verifier" || chartNested(environment[0], "valueFrom", "secretKeyRef", "key") != "creatorVerifier" || len(mounts) != 1 || chartNested(mounts[0], "name") != "serviceaccount" || chartNested(mounts[0], "mountPath") != "/var/run/secrets/kubernetes.io/serviceaccount" || chartNested(mounts[0], "readOnly") != true || len(volumes) != 1 || chartNested(volumes[0], "name") != "serviceaccount" || chartNested(volumes[0], "projected", "defaultMode") != float64(292) {
		return errors.New("packaged Accelerator credential projection contract differs")
	}
	sources, _ := chartNested(volumes[0], "projected", "sources").([]any)
	if len(sources) != 3 || chartNested(sources[0], "serviceAccountToken", "path") != "token" || chartNested(sources[0], "serviceAccountToken", "expirationSeconds") != float64(3600) || chartNested(sources[1], "configMap", "name") != "kube-root-ca.crt" || chartNested(sources[2], "downwardAPI", "items") == nil {
		return errors.New("packaged Accelerator service account projection differs")
	}
	if chartNested(job, "spec", "template", "metadata", "annotations", "kubikles.io/build-version") != appVersion || chartNested(job, "spec", "template", "spec", "securityContext", "runAsUser") != float64(65532) || chartNested(job, "spec", "template", "spec", "securityContext", "runAsGroup") != float64(65532) || chartNested(job, "spec", "template", "spec", "securityContext", "runAsNonRoot") != true || chartNested(job, "spec", "template", "spec", "securityContext", "seccompProfile", "type") != "RuntimeDefault" {
		return errors.New("packaged Accelerator Pod identity contract differs")
	}
	role := objects["ClusterRole"]
	rules, _ := chartNested(role, "rules").([]any)
	if len(rules) != 1 || chartStrings(chartNested(rules[0], "apiGroups")) != "" || chartStrings(chartNested(rules[0], "resources")) != "secrets" || chartStrings(chartNested(rules[0], "verbs")) != "get,list,watch" {
		return errors.New("packaged Accelerator RBAC contract differs")
	}
	binding := objects["ClusterRoleBinding"]
	subjects, _ := chartNested(binding, "subjects").([]any)
	if len(subjects) != 1 || chartNested(binding, "roleRef", "kind") != "ClusterRole" || chartNested(binding, "roleRef", "name") != chartNested(role, "metadata", "name") || chartNested(subjects[0], "kind") != "ServiceAccount" || chartNested(subjects[0], "name") != "release-kubikles-accelerator" || chartNested(subjects[0], "namespace") != "release-contract" {
		return errors.New("packaged Accelerator RBAC binding contract differs")
	}
	if chartNested(objects["Secret"], "immutable") != true || chartNested(objects["Secret"], "type") != "Opaque" {
		return errors.New("packaged Accelerator credential contract differs")
	}
	for kind, object := range objects {
		labels, _ := chartNested(object, "metadata", "labels").(map[string]any)
		if labels["app.kubernetes.io/name"] != "kubikles-accelerator" || labels["app.kubernetes.io/instance"] != "release" || labels["app.kubernetes.io/component"] != "accelerator" || labels["app.kubernetes.io/part-of"] != "kubikles" || labels["app.kubernetes.io/managed-by"] != "Helm" {
			return fmt.Errorf("packaged Accelerator %s ownership contract differs", kind)
		}
	}
	return nil
}

type ociIndex struct {
	SchemaVersion int             `json:"schemaVersion"`
	Manifests     []ociDescriptor `json:"manifests"`
}
type ociDescriptor struct {
	MediaType   string            `json:"mediaType"`
	Digest      string            `json:"digest"`
	Size        int64             `json:"size"`
	Platform    *ociPlatform      `json:"platform,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}
type ociPlatform struct {
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
}
type ociManifest struct {
	SchemaVersion int               `json:"schemaVersion"`
	MediaType     string            `json:"mediaType"`
	Config        ociDescriptor     `json:"config"`
	Layers        []ociDescriptor   `json:"layers"`
	Annotations   map[string]string `json:"annotations,omitempty"`
}
type ociConfig struct {
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
	Config       struct {
		User         string            `json:"User"`
		Entrypoint   []string          `json:"Entrypoint"`
		Cmd          []string          `json:"Cmd"`
		Env          []string          `json:"Env"`
		ExposedPorts map[string]any    `json:"ExposedPorts"`
		Volumes      map[string]any    `json:"Volumes"`
		Healthcheck  any               `json:"Healthcheck"`
		Labels       map[string]string `json:"Labels"`
	} `json:"config"`
	RootFS struct {
		Type    string   `json:"type"`
		DiffIDs []string `json:"diff_ids"`
	} `json:"rootfs"`
}

const (
	ociIndexMediaType    = "application/vnd.oci.image.index.v1+json"
	ociManifestMediaType = "application/vnd.oci.image.manifest.v1+json"
	ociConfigMediaType   = "application/vnd.oci.image.config.v1+json"
	ociLayerMediaType    = "application/vnd.oci.image.layer.v1.tar+gzip"
	helmConfigMediaType  = "application/vnd.cncf.helm.config.v1+json"
	helmLayerMediaType   = "application/vnd.cncf.helm.chart.content.v1.tar+gzip"
)

func readBlob(layout, digest string) ([]byte, error) {
	if !digestRE.MatchString(digest) {
		return nil, errors.New("invalid OCI digest")
	}
	b, err := os.ReadFile(filepath.Join(layout, "blobs", "sha256", strings.TrimPrefix(digest, "sha256:")))
	if err != nil {
		return nil, errors.New("missing OCI blob")
	}
	s := sha256.Sum256(b)
	if "sha256:"+hex.EncodeToString(s[:]) != digest {
		return nil, errors.New("OCI blob digest mismatch")
	}
	return b, nil
}

func validateDescriptorBlob(layout string, descriptor ociDescriptor, mediaType string) ([]byte, error) {
	if descriptor.MediaType != mediaType || descriptor.Size <= 0 {
		return nil, errors.New("OCI descriptor media type or size is invalid")
	}
	b, err := readBlob(layout, descriptor.Digest)
	if err != nil {
		return nil, err
	}
	if int64(len(b)) != descriptor.Size {
		return nil, errors.New("OCI descriptor size mismatch")
	}
	return b, nil
}

func inspectAcceleratorBinary(binary []byte, architecture, version, commit string) error {
	reader := bytes.NewReader(binary)
	f, err := elf.NewFile(reader)
	if err != nil {
		return errors.New("Accelerator layer does not contain an ELF executable")
	}
	defer f.Close()
	wantMachine := elf.EM_X86_64
	if architecture == "arm64" {
		wantMachine = elf.EM_AARCH64
	}
	if (f.Type != elf.ET_EXEC && f.Type != elf.ET_DYN) || f.Machine != wantMachine {
		return errors.New("Accelerator ELF platform identity differs")
	}
	for _, program := range f.Progs {
		if program.Type == elf.PT_INTERP {
			return errors.New("Accelerator ELF has a dynamic interpreter")
		}
	}
	for _, section := range f.Sections {
		if section.Name == ".dynamic" || section.Name == ".note.go.buildid" || strings.HasPrefix(section.Name, ".debug") || section.Name == ".symtab" {
			return errors.New("Accelerator ELF is not the stripped static release binary")
		}
	}
	info, err := buildinfo.Read(reader)
	if err != nil {
		return errors.New("Accelerator Go build identity is unreadable")
	}
	settings := map[string]string{}
	for _, setting := range info.Settings {
		settings[setting.Key] = setting.Value
	}
	if settings["-trimpath"] != "true" || settings["GOOS"] != "linux" || settings["GOARCH"] != architecture || settings["CGO_ENABLED"] != "0" || settings["-tags"] != "headless,accelerator" {
		return errors.New("Accelerator Go build settings differ from the release contract")
	}
	for key := range settings {
		if strings.HasPrefix(key, "vcs.") {
			return errors.New("Accelerator binary contains unapproved VCS build settings")
		}
	}
	text := string(binary)
	for _, forbidden := range []string{"github.com/wailsapp/wails", "kubikles/pkg/helm", "kubikles/pkg/terminal", "kubikles/pkg/ai"} {
		if strings.Contains(text, forbidden) {
			return errors.New("Accelerator binary contains a forbidden desktop dependency")
		}
	}
	for _, required := range []string{
		"kubikles-accelerator-build-identity:" + version + "|" + commit + "|false",
		"frontend/dist/index.html",
	} {
		if !strings.Contains(text, required) {
			return errors.New("Accelerator binary is missing release identity or embedded frontend")
		}
	}
	return nil
}

func inspectAcceleratorLayer(blob []byte, architecture, version, commit string) error {
	gz, err := gzip.NewReader(bytes.NewReader(blob))
	if err != nil {
		return errors.New("open Accelerator image layer")
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	h, err := tr.Next()
	if err != nil || h.Typeflag != tar.TypeReg || strings.TrimPrefix(h.Name, "./") != "kubikles-accelerator" || h.Mode&0111 == 0 || h.Uid != 65532 || h.Gid != 65532 {
		return errors.New("Accelerator image must add only its owned non-root executable")
	}
	binary, err := io.ReadAll(tr)
	if err != nil || int64(len(binary)) != h.Size {
		return errors.New("read Accelerator executable layer")
	}
	if _, err := tr.Next(); err != io.EOF {
		return errors.New("Accelerator image adds an unexpected rootfs entry")
	}
	return inspectAcceleratorBinary(binary, architecture, version, commit)
}

func InspectOCIIndex(layout, version, commit string) (string, []Platform, error) {
	if _, err := NormalizeReleaseTag(version); err != nil || !commitRE.MatchString(commit) {
		return "", nil, errors.New("invalid OCI expected identity")
	}
	layoutBytes, err := os.ReadFile(filepath.Join(layout, "oci-layout"))
	if err != nil || string(layoutBytes) != `{"imageLayoutVersion":"1.0.0"}` {
		return "", nil, errors.New("OCI layout marker is not canonical")
	}
	b, err := os.ReadFile(filepath.Join(layout, "index.json"))
	if err != nil {
		return "", nil, errors.New("read OCI index")
	}
	var root ociIndex
	if json.Unmarshal(b, &root) != nil || root.SchemaVersion != 2 {
		return "", nil, errors.New("OCI layout must contain one image index")
	}
	var indexRoot *ociDescriptor
	var copiedManifests []ociDescriptor
	for i := range root.Manifests {
		descriptor := &root.Manifests[i]
		if descriptor.MediaType == ociIndexMediaType {
			if indexRoot != nil || descriptor.Platform != nil {
				return "", nil, errors.New("OCI layout image index root is ambiguous")
			}
			indexRoot = descriptor
		} else {
			copiedManifests = append(copiedManifests, *descriptor)
		}
	}
	if indexRoot == nil {
		return "", nil, errors.New("OCI layout must contain one image index")
	}
	indexDigest := indexRoot.Digest
	ib, err := validateDescriptorBlob(layout, *indexRoot, ociIndexMediaType)
	if err != nil {
		return "", nil, err
	}
	var index ociIndex
	if json.Unmarshal(ib, &index) != nil {
		return "", nil, errors.New("decode image index")
	}
	if index.SchemaVersion != 2 || len(index.Manifests) != 2 {
		return "", nil, errors.New("image index must contain exactly two manifests")
	}
	// ORAS digest copies catalog the selected index plus its copied child
	// manifests in index.json. Accept only that exact expansion; no unrelated
	// root artifact is allowed to hide beside the release index.
	if len(copiedManifests) != 0 {
		if len(copiedManifests) != len(index.Manifests) {
			return "", nil, errors.New("OCI layout root contains unrelated manifests")
		}
		for _, copied := range copiedManifests {
			matched := false
			for _, nested := range index.Manifests {
				if copied.Digest == nested.Digest && copied.MediaType == nested.MediaType && copied.Size == nested.Size {
					matched = true
					break
				}
			}
			if !matched {
				return "", nil, errors.New("OCI layout root contains unrelated manifests")
			}
		}
	}
	platforms := make([]Platform, 0, 2)
	for _, d := range index.Manifests {
		if d.Platform == nil || d.Platform.OS != "linux" || (d.Platform.Architecture != "amd64" && d.Platform.Architecture != "arm64") || d.MediaType != ociManifestMediaType {
			return "", nil, errors.New("unexpected image platform")
		}
		mb, err := validateDescriptorBlob(layout, d, ociManifestMediaType)
		if err != nil {
			return "", nil, err
		}
		var m ociManifest
		if json.Unmarshal(mb, &m) != nil || m.SchemaVersion != 2 || m.MediaType != ociManifestMediaType || len(m.Layers) == 0 || m.Config.MediaType != ociConfigMediaType {
			return "", nil, errors.New("invalid image manifest")
		}
		cb, err := validateDescriptorBlob(layout, m.Config, ociConfigMediaType)
		if err != nil {
			return "", nil, err
		}
		var c ociConfig
		if json.Unmarshal(cb, &c) != nil || c.OS != "linux" || c.Architecture != d.Platform.Architecture || c.Config.User != "65532:65532" || len(c.Config.Entrypoint) != 1 || c.Config.Entrypoint[0] != "/kubikles-accelerator" || len(c.Config.Cmd) != 0 || len(c.Config.ExposedPorts) != 0 || len(c.Config.Volumes) != 0 || c.Config.Healthcheck != nil || c.Config.Labels["org.opencontainers.image.title"] != "kubikles-accelerator" || c.Config.Labels["org.opencontainers.image.version"] != version || c.Config.Labels["org.opencontainers.image.revision"] != commit || c.RootFS.Type != "layers" || len(c.RootFS.DiffIDs) != len(m.Layers) {
			return "", nil, errors.New("image config identity or hardening mismatch")
		}
		for _, environment := range c.Config.Env {
			if strings.HasPrefix(environment, "HOME=") || strings.HasPrefix(environment, "XDG_") || strings.HasPrefix(environment, "TMPDIR=") {
				return "", nil, errors.New("image config declares runtime storage environment")
			}
		}
		for i, layer := range m.Layers {
			layerBytes, err := validateDescriptorBlob(layout, layer, ociLayerMediaType)
			if err != nil {
				return "", nil, err
			}
			gz, err := gzip.NewReader(bytes.NewReader(layerBytes))
			if err != nil {
				return "", nil, errors.New("open OCI filesystem layer")
			}
			uncompressed, readErr := io.ReadAll(gz)
			closeErr := gz.Close()
			if readErr != nil || closeErr != nil {
				return "", nil, errors.New("read OCI filesystem layer")
			}
			diffID := sha256.Sum256(uncompressed)
			if "sha256:"+hex.EncodeToString(diffID[:]) != c.RootFS.DiffIDs[i] {
				return "", nil, errors.New("OCI filesystem layer DiffID mismatch")
			}
		}
		lastLayer := m.Layers[len(m.Layers)-1]
		layerBytes, err := validateDescriptorBlob(layout, lastLayer, ociLayerMediaType)
		if err != nil {
			return "", nil, err
		}
		if err := inspectAcceleratorLayer(layerBytes, d.Platform.Architecture, version, commit); err != nil {
			return "", nil, err
		}
		platforms = append(platforms, Platform{OS: "linux", Architecture: d.Platform.Architecture, ManifestDigest: d.Digest})
	}
	sort.Slice(platforms, func(i, j int) bool { return platforms[i].Architecture < platforms[j].Architecture })
	if platforms[0].Architecture != "amd64" || platforms[1].Architecture != "arm64" {
		return "", nil, errors.New("image platform cardinality mismatch")
	}
	return indexDigest, platforms, nil
}

func consumeUniqueJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]struct{}{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("JSON object key is invalid")
			}
			if _, duplicate := seen[key]; duplicate {
				return errors.New("JSON object contains a duplicate key")
			}
			seen[key] = struct{}{}
			if err := consumeUniqueJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return errors.New("JSON object is incomplete")
		}
	case '[':
		for decoder.More() {
			if err := consumeUniqueJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return errors.New("JSON array is incomplete")
		}
	default:
		return errors.New("JSON delimiter is invalid")
	}
	return nil
}

func validateUniqueJSONKeys(b []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(b))
	if err := consumeUniqueJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("JSON has trailing content")
	}
	return nil
}

func DecodeStrictDescriptor(b []byte) (Descriptor, error) {
	if err := validateUniqueJSONKeys(b); err != nil {
		return Descriptor{}, errors.New("descriptor JSON is invalid")
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var d Descriptor
	if err := dec.Decode(&d); err != nil {
		return d, errors.New("descriptor JSON is invalid")
	}
	if dec.Decode(new(any)) != io.EOF {
		return d, errors.New("descriptor has trailing content")
	}
	if err := ValidateDescriptor(d); err != nil {
		return d, err
	}
	canonical, err := CanonicalDescriptor(d)
	if err != nil || !bytes.Equal(b, canonical) {
		return d, errors.New("descriptor bytes are not canonical")
	}
	return d, nil
}

func writeCanonicalJSON(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0600)
}

func ensureEqual(label string, expected, actual []byte) error {
	if !bytes.Equal(expected, actual) {
		return fmt.Errorf("%s conflicts with immutable release content", label)
	}
	return nil
}

// The canonical Helm-compatible OCI artifact uses the release epoch instead of
// Helm's wall clock so reruns have one exact manifest digest.
type helmOCIConfig struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	APIVersion  string `json:"apiVersion"`
	AppVersion  string `json:"appVersion"`
	Type        string `json:"type"`
}

func canonicalChartArtifact(packagePath, source, version, appVersion string, epoch int64) ([]byte, []byte, string, error) {
	if _, err := NormalizeReleaseTag(appVersion); err != nil || strings.TrimPrefix(appVersion, "v") != version || epoch <= 0 {
		return nil, nil, "", errors.New("invalid chart OCI identity")
	}
	metadataBytes, err := os.ReadFile(filepath.Join(source, "Chart.yaml"))
	if err != nil {
		return nil, nil, "", errors.New("read chart metadata")
	}
	var metadata chartMetadata
	if yaml.Unmarshal(metadataBytes, &metadata) != nil || metadata.Name != "kubikles-accelerator" || metadata.APIVersion != "v2" || metadata.Type != "application" {
		return nil, nil, "", errors.New("chart metadata is not canonical")
	}
	configBytes, err := json.Marshal(helmOCIConfig{Name: metadata.Name, Version: version, Description: metadata.Description, APIVersion: metadata.APIVersion, AppVersion: appVersion, Type: metadata.Type})
	if err != nil {
		return nil, nil, "", errors.New("encode chart OCI config")
	}
	packageBytes, err := os.ReadFile(packagePath)
	if err != nil {
		return nil, nil, "", errors.New("read chart package")
	}
	configSum := sha256.Sum256(configBytes)
	packageSum := sha256.Sum256(packageBytes)
	manifest := ociManifest{
		SchemaVersion: 2,
		MediaType:     ociManifestMediaType,
		Config:        ociDescriptor{MediaType: helmConfigMediaType, Digest: "sha256:" + hex.EncodeToString(configSum[:]), Size: int64(len(configBytes))},
		Layers:        []ociDescriptor{{MediaType: helmLayerMediaType, Digest: "sha256:" + hex.EncodeToString(packageSum[:]), Size: int64(len(packageBytes))}},
		Annotations: map[string]string{
			"org.opencontainers.image.created":     time.Unix(epoch, 0).UTC().Format(time.RFC3339),
			"org.opencontainers.image.description": metadata.Description,
			"org.opencontainers.image.title":       metadata.Name,
			"org.opencontainers.image.version":     version,
		},
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return nil, nil, "", errors.New("encode chart OCI manifest")
	}
	manifestSum := sha256.Sum256(manifestBytes)
	return configBytes, manifestBytes, "sha256:" + hex.EncodeToString(manifestSum[:]), nil
}

func writeChartBlob(layout string, content []byte) (ociDescriptor, error) {
	sum := sha256.Sum256(content)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	if err := os.WriteFile(filepath.Join(layout, "blobs", "sha256", strings.TrimPrefix(digest, "sha256:")), content, 0600); err != nil {
		return ociDescriptor{}, errors.New("write chart OCI blob")
	}
	return ociDescriptor{Digest: digest, Size: int64(len(content))}, nil
}

func PackageChartOCI(packagePath, source, layout, version, appVersion string, epoch int64) (string, error) {
	configBytes, manifestBytes, manifestDigest, err := canonicalChartArtifact(packagePath, source, version, appVersion, epoch)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Join(layout, "blobs", "sha256"), 0700); err != nil {
		return "", errors.New("create chart OCI layout")
	}
	if _, err := writeChartBlob(layout, configBytes); err != nil {
		return "", err
	}
	packageBytes, err := os.ReadFile(packagePath)
	if err != nil {
		return "", errors.New("read chart package")
	}
	if _, err := writeChartBlob(layout, packageBytes); err != nil {
		return "", err
	}
	manifestDescriptor, err := writeChartBlob(layout, manifestBytes)
	if err != nil {
		return "", err
	}
	manifestDescriptor.MediaType = ociManifestMediaType
	manifestDescriptor.Annotations = map[string]string{"org.opencontainers.image.ref.name": version}
	rootBytes, err := json.Marshal(ociIndex{SchemaVersion: 2, Manifests: []ociDescriptor{manifestDescriptor}})
	if err != nil {
		return "", errors.New("encode chart OCI index")
	}
	if err := os.WriteFile(filepath.Join(layout, "oci-layout"), []byte(`{"imageLayoutVersion":"1.0.0"}`), 0600); err != nil {
		return "", errors.New("write chart OCI marker")
	}
	if err := os.WriteFile(filepath.Join(layout, "index.json"), rootBytes, 0600); err != nil {
		return "", errors.New("write chart OCI index")
	}
	return manifestDigest, nil
}

func ChartConfigDigest(path, expectedDigest string) (string, error) {
	if !digestRE.MatchString(expectedDigest) {
		return "", errors.New("registry chart digest is invalid")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", errors.New("read registry chart manifest")
	}
	sum := sha256.Sum256(b)
	if "sha256:"+hex.EncodeToString(sum[:]) != expectedDigest {
		return "", errors.New("registry chart manifest digest differs")
	}
	var manifest ociManifest
	if err := validateUniqueJSONKeys(b); err != nil || json.Unmarshal(b, &manifest) != nil || manifest.SchemaVersion != 2 || manifest.MediaType != ociManifestMediaType || manifest.Config.MediaType != helmConfigMediaType || len(manifest.Layers) != 1 || manifest.Layers[0].MediaType != helmLayerMediaType {
		return "", errors.New("registry chart manifest media contract differs")
	}
	if !digestRE.MatchString(manifest.Config.Digest) || manifest.Config.Size <= 0 || !digestRE.MatchString(manifest.Layers[0].Digest) || manifest.Layers[0].Size <= 0 {
		return "", errors.New("registry chart manifest descriptors are invalid")
	}
	return manifest.Config.Digest, nil
}

func InspectChartManifest(manifestPath, configPath, packagePath, source, version, appVersion string, epoch int64, expectedDigest string) error {
	configBytes, manifestBytes, canonicalDigest, err := canonicalChartArtifact(packagePath, source, version, appVersion, epoch)
	if err != nil {
		return err
	}
	if canonicalDigest != expectedDigest {
		return errors.New("registry chart digest differs from canonical release evidence")
	}
	observedManifest, err := os.ReadFile(manifestPath)
	if err != nil || !bytes.Equal(observedManifest, manifestBytes) {
		return errors.New("registry chart manifest differs from canonical release evidence")
	}
	observedConfig, err := os.ReadFile(configPath)
	if err != nil || !bytes.Equal(observedConfig, configBytes) {
		return errors.New("registry chart config differs from canonical release evidence")
	}
	return nil
}

func VerifyRegistryEvidence(descriptorPath, imageEvidencePath, chartDigest, buildVersion, commit string) error {
	descriptorBytes, err := os.ReadFile(descriptorPath)
	if err != nil {
		return errors.New("read release descriptor")
	}
	d, err := DecodeStrictDescriptor(descriptorBytes)
	if err != nil {
		return err
	}
	if d.BuildVersion != buildVersion || d.Source.GitTag != buildVersion || d.Source.Commit != commit || d.Chart.Digest != chartDigest {
		return errors.New("descriptor source, version, or chart evidence differs from registry authority")
	}
	var observed struct {
		ImageDigest string     `json:"imageDigest"`
		Platforms   []Platform `json:"platforms"`
	}
	b, err := os.ReadFile(imageEvidencePath)
	if err != nil || json.Unmarshal(b, &observed) != nil {
		return errors.New("read registry image evidence")
	}
	if d.Image.Digest != observed.ImageDigest || len(d.Image.Platforms) != len(observed.Platforms) {
		return errors.New("descriptor image index evidence differs from registry authority")
	}
	for i := range d.Image.Platforms {
		if d.Image.Platforms[i] != observed.Platforms[i] {
			return errors.New("descriptor platform manifest evidence differs from registry authority")
		}
	}
	return nil
}

func VerifyReleaseAssetNames(buildVersion string, names []string) error {
	if _, err := NormalizeReleaseTag(buildVersion); err != nil {
		return err
	}
	want := []string{
		"Kubikles-linux-amd64.zip",
		"Kubikles-macos-amd64.zip",
		"Kubikles-macos-arm64.zip",
		"Kubikles-windows-amd64.zip",
		"Kubikles-windows-arm64.zip",
		"kubikles-accelerator-release-" + buildVersion + ".json",
		"kubikles-accelerator-release-" + buildVersion + ".json.sha256",
		"kubikles-accelerator-image-linux-amd64-" + buildVersion + ".spdx.json",
		"kubikles-accelerator-image-linux-arm64-" + buildVersion + ".spdx.json",
		"kubikles-accelerator-chart-" + buildVersion + ".spdx.json",
		"kubikles-accelerator-attestations-" + buildVersion + ".jsonl",
	}
	sort.Strings(want)
	got := append([]string(nil), names...)
	sort.Strings(got)
	if len(got) != len(want) {
		return errors.New("finalized GitHub Release asset set is not exact")
	}
	for i := range want {
		if got[i] != want[i] {
			return errors.New("finalized GitHub Release asset set is not exact")
		}
	}
	return nil
}
