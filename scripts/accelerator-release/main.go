package main

import (
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
			fatal("usage: normalize vMAJOR.MINOR.PATCH[-PRERELEASE]")
		}
		var v Version
		v, err = NormalizeReleaseTag(os.Args[2])
		if err == nil {
			err = writeCanonicalJSON("/dev/stdout", v)
		}
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
