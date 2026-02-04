package certviewer

import (
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"strings"
	"time"
)

// SubjectInfo contains parsed subject/issuer fields
type SubjectInfo struct {
	CommonName         string `json:"commonName"`
	Organization       string `json:"organization"`
	OrganizationalUnit string `json:"organizationalUnit"`
	Country            string `json:"country"`
	Province           string `json:"province"`
	Locality           string `json:"locality"`
}

// KeyInfo contains public key information
type KeyInfo struct {
	Algorithm string `json:"algorithm"`
	Size      int    `json:"size"`
}

// CertInfo contains parsed certificate information
type CertInfo struct {
	// Subject and Issuer
	Subject    SubjectInfo `json:"subject"`
	SubjectRaw string      `json:"subjectRaw"`
	Issuer     SubjectInfo `json:"issuer"`
	IssuerRaw  string      `json:"issuerRaw"`

	// Validity
	NotBefore          string `json:"notBefore"`
	NotAfter           string `json:"notAfter"`
	IsExpired          bool   `json:"isExpired"`
	IsNotYetValid      bool   `json:"isNotYetValid"`
	DaysUntilExpiry    int    `json:"daysUntilExpiry"`
	ValidityPercentage int    `json:"validityPercentage"`

	// SANs
	DNSNames       []string `json:"dnsNames"`
	IPAddresses    []string `json:"ipAddresses"`
	EmailAddresses []string `json:"emailAddresses"`

	// Key Info
	PublicKey          KeyInfo  `json:"publicKey"`
	SignatureAlgorithm string   `json:"signatureAlgorithm"`
	KeyUsage           []string `json:"keyUsage"`
	ExtKeyUsage        []string `json:"extKeyUsage"`

	// Identifiers
	SerialNumber string `json:"serialNumber"`
	Version      int    `json:"version"`

	// Fingerprints
	FingerprintSHA256 string `json:"fingerprintSHA256"`
	FingerprintSHA1   string `json:"fingerprintSHA1"`
}

// IsPEMCertificate checks if the data contains a PEM-encoded certificate
func IsPEMCertificate(data string) bool {
	return strings.Contains(data, "-----BEGIN CERTIFICATE-----")
}

// formatHexFingerprint formats bytes as colon-separated hex
func formatHexFingerprint(data []byte) string {
	hex := hex.EncodeToString(data)
	var parts []string
	for i := 0; i < len(hex); i += 2 {
		parts = append(parts, strings.ToUpper(hex[i:i+2]))
	}
	return strings.Join(parts, ":")
}

// formatSerialNumber formats a serial number as colon-separated hex
func formatSerialNumber(data []byte) string {
	var parts []string
	for _, b := range data {
		parts = append(parts, fmt.Sprintf("%02X", b))
	}
	return strings.Join(parts, ":")
}

// getKeyUsages returns human-readable key usage strings
func getKeyUsages(ku x509.KeyUsage) []string {
	var usages []string
	if ku&x509.KeyUsageDigitalSignature != 0 {
		usages = append(usages, "Digital Signature")
	}
	if ku&x509.KeyUsageKeyEncipherment != 0 {
		usages = append(usages, "Key Encipherment")
	}
	if ku&x509.KeyUsageContentCommitment != 0 {
		usages = append(usages, "Content Commitment")
	}
	if ku&x509.KeyUsageDataEncipherment != 0 {
		usages = append(usages, "Data Encipherment")
	}
	if ku&x509.KeyUsageKeyAgreement != 0 {
		usages = append(usages, "Key Agreement")
	}
	if ku&x509.KeyUsageCertSign != 0 {
		usages = append(usages, "Certificate Sign")
	}
	if ku&x509.KeyUsageCRLSign != 0 {
		usages = append(usages, "CRL Sign")
	}
	return usages
}

// getExtKeyUsages returns human-readable extended key usage strings
func getExtKeyUsages(ekus []x509.ExtKeyUsage) []string {
	var usages []string
	for _, eku := range ekus {
		switch eku {
		case x509.ExtKeyUsageServerAuth:
			usages = append(usages, "Server Authentication")
		case x509.ExtKeyUsageClientAuth:
			usages = append(usages, "Client Authentication")
		case x509.ExtKeyUsageCodeSigning:
			usages = append(usages, "Code Signing")
		case x509.ExtKeyUsageEmailProtection:
			usages = append(usages, "Email Protection")
		case x509.ExtKeyUsageTimeStamping:
			usages = append(usages, "Time Stamping")
		case x509.ExtKeyUsageOCSPSigning:
			usages = append(usages, "OCSP Signing")
		default:
			usages = append(usages, "Unknown")
		}
	}
	return usages
}

// getPublicKeyInfo extracts public key algorithm and size
func getPublicKeyInfo(cert *x509.Certificate) KeyInfo {
	info := KeyInfo{}

	switch cert.PublicKeyAlgorithm {
	case x509.RSA:
		info.Algorithm = "RSA"
	case x509.ECDSA:
		info.Algorithm = "ECDSA"
	case x509.Ed25519:
		info.Algorithm = "Ed25519"
	case x509.DSA:
		info.Algorithm = "DSA"
	default:
		info.Algorithm = "Unknown"
	}

	// Get key size from the public key
	switch pub := cert.PublicKey.(type) {
	case interface{ Size() int }:
		info.Size = pub.Size() * 8
	default:
		// For Ed25519 and others, use a default
		if cert.SignatureAlgorithm.String() == "Ed25519" {
			info.Size = 256
		}
	}

	return info
}

// parseSingleCert parses a single x509.Certificate into CertInfo
func parseSingleCert(cert *x509.Certificate) *CertInfo {
	now := time.Now()
	daysUntilExpiry := int(cert.NotAfter.Sub(now).Hours() / 24)
	isExpired := now.After(cert.NotAfter)
	isNotYetValid := now.Before(cert.NotBefore)

	// Calculate validity percentage
	totalDuration := cert.NotAfter.Sub(cert.NotBefore).Hours()
	elapsedDuration := now.Sub(cert.NotBefore).Hours()
	validityPercentage := 0
	if totalDuration > 0 {
		validityPercentage = int((elapsedDuration / totalDuration) * 100)
		if validityPercentage < 0 {
			validityPercentage = 0
		}
		if validityPercentage > 100 {
			validityPercentage = 100
		}
	}

	// Calculate fingerprints
	sha256Sum := sha256.Sum256(cert.Raw)
	sha1Sum := sha1.Sum(cert.Raw)

	// Extract IP addresses as strings
	var ipAddresses []string
	for _, ip := range cert.IPAddresses {
		ipAddresses = append(ipAddresses, ip.String())
	}

	// Get first values for subject/issuer fields
	getFirst := func(arr []string) string {
		if len(arr) > 0 {
			return arr[0]
		}
		return ""
	}

	return &CertInfo{
		Subject: SubjectInfo{
			CommonName:         cert.Subject.CommonName,
			Organization:       getFirst(cert.Subject.Organization),
			OrganizationalUnit: getFirst(cert.Subject.OrganizationalUnit),
			Country:            getFirst(cert.Subject.Country),
			Province:           getFirst(cert.Subject.Province),
			Locality:           getFirst(cert.Subject.Locality),
		},
		SubjectRaw: cert.Subject.String(),
		Issuer: SubjectInfo{
			CommonName:         cert.Issuer.CommonName,
			Organization:       getFirst(cert.Issuer.Organization),
			OrganizationalUnit: getFirst(cert.Issuer.OrganizationalUnit),
			Country:            getFirst(cert.Issuer.Country),
			Province:           getFirst(cert.Issuer.Province),
			Locality:           getFirst(cert.Issuer.Locality),
		},
		IssuerRaw:          cert.Issuer.String(),
		NotBefore:          cert.NotBefore.Format(time.RFC3339),
		NotAfter:           cert.NotAfter.Format(time.RFC3339),
		IsExpired:          isExpired,
		IsNotYetValid:      isNotYetValid,
		DaysUntilExpiry:    daysUntilExpiry,
		ValidityPercentage: validityPercentage,
		DNSNames:           cert.DNSNames,
		IPAddresses:        ipAddresses,
		EmailAddresses:     cert.EmailAddresses,
		PublicKey:          getPublicKeyInfo(cert),
		SignatureAlgorithm: cert.SignatureAlgorithm.String(),
		KeyUsage:           getKeyUsages(cert.KeyUsage),
		ExtKeyUsage:        getExtKeyUsages(cert.ExtKeyUsage),
		SerialNumber:       formatSerialNumber(cert.SerialNumber.Bytes()),
		Version:            cert.Version,
		FingerprintSHA256:  formatHexFingerprint(sha256Sum[:]),
		FingerprintSHA1:    formatHexFingerprint(sha1Sum[:]),
	}
}

// ParseCertInfo extracts certificate information from PEM data (first cert only)
func ParseCertInfo(pemData string) (*CertInfo, error) {
	certs, err := ParseAllCertInfo(pemData)
	if err != nil {
		return nil, err
	}
	if len(certs) == 0 {
		return nil, fmt.Errorf("no certificates found in PEM data")
	}
	return certs[0], nil
}

// ParseAllCertInfo extracts all certificates from PEM data (for certificate chains)
func ParseAllCertInfo(pemData string) ([]*CertInfo, error) {
	var certs []*CertInfo
	data := []byte(pemData)

	for {
		block, rest := pem.Decode(data)
		if block == nil {
			break
		}

		if block.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				// Skip malformed certificates, continue with next
				data = rest
				continue
			}
			certs = append(certs, parseSingleCert(cert))
		}

		data = rest
	}

	if len(certs) == 0 {
		return nil, fmt.Errorf("no valid certificates found in PEM data")
	}

	return certs, nil
}
