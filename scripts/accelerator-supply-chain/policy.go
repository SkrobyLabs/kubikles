package supplychain

import (
	"bytes"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Artifact string

const (
	ArtifactImageAMD64 Artifact = "image-linux-amd64"
	ArtifactChart      Artifact = "chart"
	ArtifactSourceGo   Artifact = "source-go"
	ArtifactSourceNPM  Artifact = "source-npm"
)

type VulnerabilityException struct {
	ID            string   `json:"id"`
	PURL          string   `json:"purl"`
	Artifact      Artifact `json:"artifact"`
	Owner         string   `json:"owner"`
	Justification string   `json:"justification"`
	ExpiresOn     string   `json:"expiresOn"`
}

type Finding struct {
	ID       string   `json:"id"`
	PURL     string   `json:"purl"`
	Artifact Artifact `json:"artifact"`
	Severity string   `json:"severity"`
}

type ScanReport struct {
	SchemaVersion int        `json:"schemaVersion"`
	DBUpdatedAt   string     `json:"dbUpdatedAt"`
	Artifacts     []Artifact `json:"artifacts"`
	Findings      []Finding  `json:"findings"`
}

type PolicyResult struct {
	Critical int      `json:"critical"`
	High     int      `json:"high"`
	Medium   int      `json:"medium"`
	Low      int      `json:"low"`
	Excepted []string `json:"excepted"`
}

var (
	vulnerabilityIDPattern = regexp.MustCompile(`^(CVE-[0-9]{4}-[0-9]{4,}|GHSA-[23456789cfghjmpqrvwx]{4}-[23456789cfghjmpqrvwx]{4}-[23456789cfghjmpqrvwx]{4})$`)
	ownerPattern           = regexp.MustCompile(`^@[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$`)
)

func ParseExceptions(data []byte) ([]VulnerabilityException, error) {
	var exceptions []VulnerabilityException
	if decodeStrict(bytes.NewReader(data), &exceptions) != nil || ValidateExceptions(exceptions) != nil {
		return nil, errors.New("vulnerability exceptions invalid")
	}
	return exceptions, nil
}

func ValidateExceptions(exceptions []VulnerabilityException) error {
	seen := make(map[string]struct{}, len(exceptions))
	for _, exception := range exceptions {
		if !vulnerabilityIDPattern.MatchString(exception.ID) || !strings.HasPrefix(exception.PURL, "pkg:") || strings.ContainsAny(exception.PURL, "*?[]{}<>") || !validArtifact(exception.Artifact) || !ownerPattern.MatchString(exception.Owner) || strings.TrimSpace(exception.Justification) == "" || strings.TrimSpace(exception.Justification) != exception.Justification {
			return errors.New("invalid vulnerability exception")
		}
		date, err := time.Parse("2006-01-02", exception.ExpiresOn)
		if err != nil || date.Format("2006-01-02") != exception.ExpiresOn {
			return errors.New("invalid vulnerability expiry")
		}
		key := findingKey(exception.ID, exception.PURL, exception.Artifact)
		if _, duplicate := seen[key]; duplicate {
			return errors.New("duplicate vulnerability exception")
		}
		seen[key] = struct{}{}
	}
	return nil
}

func EvaluatePolicy(report ScanReport, exceptions []VulnerabilityException, now time.Time, requireFreshDB bool) (PolicyResult, error) {
	if report.SchemaVersion != 1 || ValidateExceptions(exceptions) != nil || now.Location() != time.UTC {
		return PolicyResult{}, errors.New("vulnerability policy preflight failed")
	}
	dbTime, err := time.Parse(time.RFC3339, report.DBUpdatedAt)
	if err != nil || dbTime.After(now.Add(time.Minute)) || requireFreshDB && now.Sub(dbTime) > 24*time.Hour {
		return PolicyResult{}, errors.New("vulnerability database invalid")
	}
	wantArtifacts := []Artifact{ArtifactImageAMD64, ArtifactChart, ArtifactSourceGo, ArtifactSourceNPM}
	if len(report.Artifacts) != len(wantArtifacts) {
		return PolicyResult{}, errors.New("vulnerability coverage incomplete")
	}
	seenArtifacts := map[Artifact]struct{}{}
	for _, artifact := range report.Artifacts {
		if !validArtifact(artifact) {
			return PolicyResult{}, errors.New("vulnerability coverage invalid")
		}
		seenArtifacts[artifact] = struct{}{}
	}
	if len(seenArtifacts) != len(wantArtifacts) {
		return PolicyResult{}, errors.New("vulnerability coverage incomplete")
	}
	exceptionByKey := make(map[string]VulnerabilityException, len(exceptions))
	used := make(map[string]struct{}, len(exceptions))
	for _, exception := range exceptions {
		exceptionByKey[findingKey(exception.ID, exception.PURL, exception.Artifact)] = exception
	}
	seenFindings := make(map[string]struct{}, len(report.Findings))
	result := PolicyResult{}
	for _, finding := range report.Findings {
		if !vulnerabilityIDPattern.MatchString(finding.ID) || !strings.HasPrefix(finding.PURL, "pkg:") || !validArtifact(finding.Artifact) {
			return PolicyResult{}, errors.New("vulnerability finding invalid")
		}
		key := findingKey(finding.ID, finding.PURL, finding.Artifact)
		if _, duplicate := seenFindings[key]; duplicate {
			return PolicyResult{}, errors.New("duplicate vulnerability finding")
		}
		seenFindings[key] = struct{}{}
		switch strings.ToUpper(finding.Severity) {
		case "CRITICAL":
			result.Critical++
		case "HIGH":
			result.High++
		case "MEDIUM":
			result.Medium++
			continue
		case "LOW":
			result.Low++
			continue
		default:
			return PolicyResult{}, errors.New("vulnerability severity invalid")
		}
		exception, ok := exceptionByKey[key]
		if !ok {
			return PolicyResult{}, errors.New("unexcepted high vulnerability")
		}
		expires, _ := time.Parse("2006-01-02", exception.ExpiresOn)
		if !now.Before(expires) {
			return PolicyResult{}, errors.New("vulnerability exception expired")
		}
		used[key] = struct{}{}
		result.Excepted = append(result.Excepted, finding.ID)
	}
	if len(used) != len(exceptions) {
		return PolicyResult{}, errors.New("unused vulnerability exception")
	}
	sort.Strings(result.Excepted)
	return result, nil
}

func validArtifact(value Artifact) bool {
	switch value {
	case ArtifactImageAMD64, ArtifactChart, ArtifactSourceGo, ArtifactSourceNPM:
		return true
	default:
		return false
	}
}

func findingKey(id, purl string, artifact Artifact) string {
	return id + "\x00" + purl + "\x00" + string(artifact)
}
