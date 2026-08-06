package supplychain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

type CreationInfo struct {
	Created            string   `json:"created"`
	Creators           []string `json:"creators"`
	LicenseListVersion string   `json:"licenseListVersion,omitempty"`
}

type ExternalRef struct {
	Category string `json:"referenceCategory"`
	Type     string `json:"referenceType"`
	Locator  string `json:"referenceLocator"`
}

type SPDXPackage struct {
	SPDXID                string          `json:"SPDXID"`
	Name                  string          `json:"name"`
	VersionInfo           string          `json:"versionInfo,omitempty"`
	DownloadLocation      string          `json:"downloadLocation"`
	FilesAnalyzed         bool            `json:"filesAnalyzed"`
	LicenseConcluded      string          `json:"licenseConcluded,omitempty"`
	LicenseDeclared       string          `json:"licenseDeclared,omitempty"`
	CopyrightText         string          `json:"copyrightText,omitempty"`
	ExternalRefs          []ExternalRef   `json:"externalRefs,omitempty"`
	PackageVerification   json.RawMessage `json:"packageVerificationCode,omitempty"`
	Checksums             json.RawMessage `json:"checksums,omitempty"`
	Supplier              string          `json:"supplier,omitempty"`
	Originator            string          `json:"originator,omitempty"`
	PrimaryPackagePurpose string          `json:"primaryPackagePurpose,omitempty"`
	SourceInfo            string          `json:"sourceInfo,omitempty"`
}

type SPDXFile struct {
	SPDXID           string          `json:"SPDXID"`
	FileName         string          `json:"fileName"`
	Checksums        json.RawMessage `json:"checksums,omitempty"`
	LicenseConcluded string          `json:"licenseConcluded,omitempty"`
	CopyrightText    string          `json:"copyrightText,omitempty"`
	Comment          string          `json:"comment,omitempty"`
	FileTypes        []string        `json:"fileTypes,omitempty"`
	LicenseInfo      []string        `json:"licenseInfoInFiles,omitempty"`
}

type SPDXRelationship struct {
	Element string `json:"spdxElementId"`
	Type    string `json:"relationshipType"`
	Related string `json:"relatedSpdxElement"`
	Comment string `json:"comment,omitempty"`
}

type SPDXDocument struct {
	SPDXVersion       string             `json:"spdxVersion"`
	DataLicense       string             `json:"dataLicense"`
	SPDXID            string             `json:"SPDXID"`
	Name              string             `json:"name"`
	DocumentNamespace string             `json:"documentNamespace"`
	CreationInfo      CreationInfo       `json:"creationInfo"`
	DocumentDescribes []string           `json:"documentDescribes,omitempty"`
	Packages          []SPDXPackage      `json:"packages"`
	Files             []SPDXFile         `json:"files,omitempty"`
	Relationships     []SPDXRelationship `json:"relationships"`
	ExtractedLicenses json.RawMessage    `json:"hasExtractedLicensingInfos,omitempty"`
}

var spdxIDPattern = regexp.MustCompile(`^SPDXRef-[A-Za-z0-9.-]+$`)

func NormalizeSPDX(input []byte, name, subjectDigest, sourceCommit string, sourceEpoch int64) ([]byte, error) {
	if name == "" || !strings.HasPrefix(subjectDigest, "sha256:") || !hex64Pattern.MatchString(strings.TrimPrefix(subjectDigest, "sha256:")) || !hex40Pattern.MatchString(sourceCommit) || sourceEpoch <= 0 {
		return nil, errors.New("invalid SPDX identity")
	}
	var document SPDXDocument
	if decodeStrict(bytes.NewReader(input), &document) != nil {
		return nil, errors.New("invalid SPDX document")
	}
	if document.SPDXVersion != "SPDX-2.3" || document.DataLicense != "CC0-1.0" || document.SPDXID != "SPDXRef-DOCUMENT" || len(document.Packages) == 0 || len(document.Relationships) == 0 {
		return nil, errors.New("invalid SPDX contract")
	}
	document.Name = name
	document.DocumentNamespace = "https://github.com/SkrobyLabs/kubikles/sbom/" + strings.TrimPrefix(subjectDigest, "sha256:")
	licenseListVersion := document.CreationInfo.LicenseListVersion
	document.CreationInfo = CreationInfo{
		Created:            time.Unix(sourceEpoch, 0).UTC().Format(time.RFC3339),
		Creators:           []string{"Tool: syft-1.44.0"},
		LicenseListVersion: licenseListVersion,
	}
	seenIDs := map[string]struct{}{"SPDXRef-DOCUMENT": {}}
	for index := range document.Packages {
		item := &document.Packages[index]
		if !spdxIDPattern.MatchString(item.SPDXID) || item.Name == "" || unsafeEvidence(item.Name) || unsafeEvidence(item.DownloadLocation) {
			return nil, errors.New("invalid SPDX package")
		}
		if _, exists := seenIDs[item.SPDXID]; exists {
			return nil, errors.New("duplicate SPDX ID")
		}
		seenIDs[item.SPDXID] = struct{}{}
		for _, reference := range item.ExternalRefs {
			if reference.Category == "PACKAGE-MANAGER" && reference.Type == "purl" && (!strings.HasPrefix(reference.Locator, "pkg:") || strings.ContainsAny(reference.Locator, "*[]{}<>") || unsafeEvidence(reference.Locator)) {
				return nil, errors.New("invalid SPDX purl")
			}
		}
		sort.Slice(item.ExternalRefs, func(a, b int) bool {
			left, right := item.ExternalRefs[a], item.ExternalRefs[b]
			return left.Category+"\x00"+left.Type+"\x00"+left.Locator < right.Category+"\x00"+right.Type+"\x00"+right.Locator
		})
	}
	for index := range document.Files {
		item := &document.Files[index]
		if !spdxIDPattern.MatchString(item.SPDXID) || item.FileName == "" || unsafeEvidence(item.FileName) {
			return nil, errors.New("invalid SPDX file")
		}
		if _, exists := seenIDs[item.SPDXID]; exists {
			return nil, errors.New("duplicate SPDX ID")
		}
		seenIDs[item.SPDXID] = struct{}{}
	}
	sort.Slice(document.Packages, func(i, j int) bool { return document.Packages[i].SPDXID < document.Packages[j].SPDXID })
	sort.Slice(document.Files, func(i, j int) bool { return document.Files[i].SPDXID < document.Files[j].SPDXID })
	sort.Slice(document.Relationships, func(i, j int) bool {
		left, right := document.Relationships[i], document.Relationships[j]
		return left.Element+"\x00"+left.Type+"\x00"+left.Related < right.Element+"\x00"+right.Type+"\x00"+right.Related
	})
	for _, relationship := range document.Relationships {
		if _, ok := seenIDs[relationship.Element]; !ok || relationship.Type == "" {
			return nil, errors.New("invalid SPDX relationship")
		}
		if _, ok := seenIDs[relationship.Related]; !ok && relationship.Related != "NONE" && relationship.Related != "NOASSERTION" {
			return nil, errors.New("invalid SPDX relationship")
		}
	}
	if len(document.DocumentDescribes) > 0 {
		sort.Strings(document.DocumentDescribes)
		if !SortedUnique(document.DocumentDescribes) {
			return nil, errors.New("invalid SPDX subjects")
		}
	}
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil || sensitiveEvidence(string(encoded)) {
		return nil, errors.New("invalid SPDX output")
	}
	return append(encoded, '\n'), nil
}

func unsafeEvidence(value string) bool {
	if sensitiveEvidence(value) {
		return true
	}
	if strings.Contains(value, `\\`) || strings.HasPrefix(value, "/") || regexp.MustCompile(`^[A-Za-z]:[/\\]`).MatchString(value) {
		return true
	}
	return false
}

func sensitiveEvidence(value string) bool {
	lower := strings.ToLower(value)
	if strings.Contains(lower, "runner_temp") || strings.Contains(lower, "github_workspace") || strings.Contains(lower, "%2ftmp%2f") || strings.Contains(lower, "%2fhome%2f") || strings.Contains(lower, "%2fvar%2f") || strings.Contains(lower, "authorization:") || strings.Contains(lower, "bearer ") || strings.Contains(lower, "signedurl") || strings.Contains(lower, "password=") || strings.Contains(lower, "token=") {
		return true
	}
	if strings.Contains(value, "://") {
		if parsed := regexp.MustCompile(`://[^/@:]+:[^/@]+@`).FindString(value); parsed != "" {
			return true
		}
	}
	return false
}

func SPDXAssetName(artifact, version string) (string, error) {
	if _, err := ValidateReleaseIdentity(version, strings.Repeat("a", 40)); err != nil {
		return "", err
	}
	switch artifact {
	case "image-linux-amd64":
		return fmt.Sprintf("kubikles-accelerator-%s-%s.spdx.json", artifact, version), nil
	case "chart":
		return "kubikles-accelerator-chart-" + version + ".spdx.json", nil
	default:
		return "", errors.New("invalid SPDX artifact")
	}
}
