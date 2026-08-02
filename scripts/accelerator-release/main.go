package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fatal("command is required")
	}
	var err error
	switch os.Args[1] {
	case "verify-git":
		if len(os.Args) != 4 && len(os.Args) != 5 {
			fatal("usage: verify-git REPOSITORY TAG [EXPECTED_COMMIT]")
		}
		expected := ""
		if len(os.Args) == 5 {
			expected = os.Args[4]
		}
		var commit string
		commit, err = VerifyGitSource(os.Args[2], os.Args[3], expected)
		if err == nil {
			fmt.Println(commit)
		}
	case "normalize":
		if len(os.Args) != 3 {
			fatal("usage: normalize vMAJOR.MINOR.PATCH")
		}
		var v Version
		v, err = NormalizeStableReleaseTag(os.Args[2])
		if err == nil {
			err = writeCanonicalJSON("/dev/stdout", v)
		}
	case "descriptor":
		if len(os.Args) != 5 {
			fatal("usage: descriptor EVIDENCE_JSON DESCRIPTOR CHECKSUM")
		}
		var b []byte
		b, err = os.ReadFile(os.Args[2])
		if err != nil {
			break
		}
		var e Evidence
		if json.Unmarshal(b, &e) != nil {
			err = fmt.Errorf("invalid registry evidence")
			break
		}
		var d Descriptor
		d, err = NewDescriptor(e)
		if err != nil {
			break
		}
		b, err = CanonicalDescriptor(d)
		if err != nil {
			break
		}
		if err = os.WriteFile(os.Args[3], b, 0600); err == nil {
			err = os.WriteFile(os.Args[4], DescriptorChecksum(os.Args[3], b), 0600)
		}
	case "evidence":
		if len(os.Args) != 8 {
			fatal("usage: evidence IMAGE_JSON BUILD_VERSION COMMIT CHART_DIGEST EVIDENCE_JSON GIT_TAG")
		}
		var imageEvidence struct {
			ImageDigest string     `json:"imageDigest"`
			Platforms   []Platform `json:"platforms"`
		}
		var b []byte
		b, err = os.ReadFile(os.Args[2])
		if err != nil {
			break
		}
		if json.Unmarshal(b, &imageEvidence) != nil {
			err = fmt.Errorf("invalid image evidence")
			break
		}
		var v Version
		v, err = NormalizeStableReleaseTag(os.Args[3])
		if err == nil && os.Args[7] != v.GitTag {
			err = fmt.Errorf("Git tag and BuildVersion differ")
		}
		if err == nil {
			err = writeCanonicalJSON(os.Args[6], Evidence{BuildVersion: v.BuildVersion, Commit: os.Args[4], GitTag: os.Args[7], ImageDigest: imageEvidence.ImageDigest, Platforms: imageEvidence.Platforms, ChartDigest: os.Args[5], ChartVersion: v.ChartVersion, ChartAppVersion: v.BuildVersion})
		}
	case "verify-descriptor":
		if len(os.Args) != 4 {
			fatal("usage: verify-descriptor DESCRIPTOR CHECKSUM")
		}
		var b, c []byte
		b, err = os.ReadFile(os.Args[2])
		if err != nil {
			break
		}
		c, err = os.ReadFile(os.Args[3])
		if err != nil {
			break
		}
		_, err = DecodeStrictDescriptor(b)
		if err == nil {
			err = ensureEqual("descriptor checksum", DescriptorChecksum(os.Args[2], b), c)
		}
	case "verify-registry-evidence":
		if len(os.Args) != 7 {
			fatal("usage: verify-registry-evidence DESCRIPTOR IMAGE_EVIDENCE CHART_DIGEST BUILD_VERSION COMMIT")
		}
		err = VerifyRegistryEvidence(os.Args[2], os.Args[3], os.Args[4], os.Args[5], os.Args[6])
	case "verify-release-assets":
		if len(os.Args) != 4 {
			fatal("usage: verify-release-assets BUILD_VERSION ASSET_NAMES_FILE")
		}
		var b []byte
		b, err = os.ReadFile(os.Args[3])
		if err == nil {
			err = VerifyReleaseAssetNames(os.Args[2], strings.Split(strings.TrimSuffix(string(b), "\n"), "\n"))
		}
	case "inspect-chart-manifest":
		if len(os.Args) != 10 {
			fatal("usage: inspect-chart-manifest MANIFEST CONFIG PACKAGE SOURCE CHART_VERSION APP_VERSION EPOCH EXPECTED_DIGEST")
		}
		var epoch int64
		epoch, err = strconv.ParseInt(os.Args[8], 10, 64)
		if err == nil {
			err = InspectChartManifest(os.Args[2], os.Args[3], os.Args[4], os.Args[5], os.Args[6], os.Args[7], epoch, os.Args[9])
		}
	case "chart-config-digest":
		if len(os.Args) != 4 {
			fatal("usage: chart-config-digest MANIFEST_JSON EXPECTED_DIGEST")
		}
		var digest string
		digest, err = ChartConfigDigest(os.Args[2], os.Args[3])
		if err == nil {
			fmt.Println(digest)
		}
	case "package-chart-oci":
		if len(os.Args) != 8 {
			fatal("usage: package-chart-oci PACKAGE SOURCE OUTPUT CHART_VERSION APP_VERSION EPOCH")
		}
		var epoch int64
		epoch, err = strconv.ParseInt(os.Args[7], 10, 64)
		if err == nil {
			var digest string
			digest, err = PackageChartOCI(os.Args[2], os.Args[3], os.Args[4], os.Args[5], os.Args[6], epoch)
			if err == nil {
				fmt.Println(digest)
			}
		}
	case "package-chart":
		if len(os.Args) != 7 {
			fatal("usage: package-chart SOURCE OUTPUT CHART_VERSION APP_VERSION EPOCH")
		}
		var epoch int64
		epoch, err = strconv.ParseInt(os.Args[6], 10, 64)
		if err == nil {
			err = PackageChart(os.Args[2], os.Args[3], os.Args[4], os.Args[5], epoch)
		}
	case "inspect-chart":
		if len(os.Args) != 7 {
			fatal("usage: inspect-chart PACKAGE SOURCE CHART_VERSION APP_VERSION EPOCH")
		}
		var epoch int64
		epoch, err = strconv.ParseInt(os.Args[6], 10, 64)
		if err == nil {
			err = InspectChart(os.Args[2], os.Args[3], os.Args[4], os.Args[5], epoch)
		}
	case "inspect-oci":
		if len(os.Args) != 6 {
			fatal("usage: inspect-oci LAYOUT BUILD_VERSION COMMIT EVIDENCE_JSON")
		}
		var digest string
		var platforms []Platform
		digest, platforms, err = InspectOCIIndex(os.Args[2], os.Args[3], os.Args[4])
		if err == nil {
			err = writeCanonicalJSON(os.Args[5], map[string]any{"imageDigest": digest, "platforms": platforms})
		}
	case "field":
		if len(os.Args) != 4 {
			fatal("usage: field DESCRIPTOR image-reference|image-digest|chart-reference|chart-digest|chart-version|commit|build-version")
		}
		var b []byte
		b, err = os.ReadFile(os.Args[2])
		if err != nil {
			break
		}
		var d Descriptor
		d, err = DecodeStrictDescriptor(b)
		if err != nil {
			break
		}
		var value string
		switch os.Args[3] {
		case "image-reference":
			value = d.Image.Reference
		case "image-digest":
			value = d.Image.Digest
		case "chart-reference":
			value = d.Chart.Reference
		case "chart-digest":
			value = d.Chart.Digest
		case "chart-version":
			value = d.Chart.Version
		case "commit":
			value = d.Source.Commit
		case "build-version":
			value = d.BuildVersion
		default:
			err = fmt.Errorf("unknown descriptor field")
		}
		if err == nil {
			fmt.Println(value)
		}
	default:
		fatal("unknown command")
	}
	if err != nil {
		fatal("%v", err)
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "accelerator release: "+format+"\n", args...)
	os.Exit(1)
}
