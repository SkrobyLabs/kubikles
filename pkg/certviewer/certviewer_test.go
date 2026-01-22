package certviewer

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"
)

// generateTestCertificate creates a test certificate with the given options
func generateTestCertificate(opts testCertOptions) (string, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", err
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", err
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:         opts.CommonName,
			Organization:       opts.Organization,
			OrganizationalUnit: opts.OrganizationalUnit,
			Country:            opts.Country,
			Province:           opts.Province,
			Locality:           opts.Locality,
		},
		NotBefore:             opts.NotBefore,
		NotAfter:              opts.NotAfter,
		KeyUsage:              opts.KeyUsage,
		ExtKeyUsage:           opts.ExtKeyUsage,
		BasicConstraintsValid: true,
		DNSNames:              opts.DNSNames,
		IPAddresses:           opts.IPAddresses,
		EmailAddresses:        opts.EmailAddresses,
	}

	if opts.IsCA {
		template.IsCA = true
		template.KeyUsage |= x509.KeyUsageCertSign | x509.KeyUsageCRLSign
	}

	// If issuer cert/key provided, use those; otherwise self-sign
	issuerCert := template
	issuerKey := priv
	if opts.IssuerCert != nil && opts.IssuerKey != nil {
		issuerCert = opts.IssuerCert
		issuerKey = opts.IssuerKey
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, issuerCert, &priv.PublicKey, issuerKey)
	if err != nil {
		return "", err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	return string(certPEM), nil
}

type testCertOptions struct {
	CommonName         string
	Organization       []string
	OrganizationalUnit []string
	Country            []string
	Province           []string
	Locality           []string
	NotBefore          time.Time
	NotAfter           time.Time
	KeyUsage           x509.KeyUsage
	ExtKeyUsage        []x509.ExtKeyUsage
	DNSNames           []string
	IPAddresses        []net.IP
	EmailAddresses     []string
	IsCA               bool
	IssuerCert         *x509.Certificate
	IssuerKey          *rsa.PrivateKey
}

func defaultTestCertOptions() testCertOptions {
	return testCertOptions{
		CommonName:   "test.example.com",
		Organization: []string{"Test Organization"},
		NotBefore:    time.Now().Add(-24 * time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
}

// TestIsPEMCertificate tests the IsPEMCertificate function
func TestIsPEMCertificate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "valid certificate header",
			input:    "-----BEGIN CERTIFICATE-----\nMIIBkTCB...",
			expected: true,
		},
		{
			name:     "certificate header with content before",
			input:    "some text\n-----BEGIN CERTIFICATE-----\nMIIBkTCB...",
			expected: true,
		},
		{
			name:     "private key only",
			input:    "-----BEGIN PRIVATE KEY-----\nMIIEvg...",
			expected: false,
		},
		{
			name:     "RSA private key only",
			input:    "-----BEGIN RSA PRIVATE KEY-----\nMIIEvg...",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "random text",
			input:    "hello world this is not a certificate",
			expected: false,
		},
		{
			name:     "partial header",
			input:    "-----BEGIN CERT",
			expected: false,
		},
		{
			name:     "certificate request",
			input:    "-----BEGIN CERTIFICATE REQUEST-----\nMIIC...",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsPEMCertificate(tt.input)
			if result != tt.expected {
				t.Errorf("IsPEMCertificate(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

// TestParseCertInfo tests the ParseCertInfo function with a valid certificate
func TestParseCertInfo(t *testing.T) {
	opts := defaultTestCertOptions()
	opts.CommonName = "test.example.com"
	opts.Organization = []string{"Test Org"}
	opts.OrganizationalUnit = []string{"Test Unit"}
	opts.Country = []string{"US"}
	opts.Province = []string{"California"}
	opts.Locality = []string{"San Francisco"}
	opts.DNSNames = []string{"test.example.com", "www.example.com"}
	opts.IPAddresses = []net.IP{net.ParseIP("192.168.1.1"), net.ParseIP("10.0.0.1")}
	opts.EmailAddresses = []string{"admin@example.com"}
	opts.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}

	pemData, err := generateTestCertificate(opts)
	if err != nil {
		t.Fatalf("Failed to generate test certificate: %v", err)
	}

	certInfo, err := ParseCertInfo(pemData)
	if err != nil {
		t.Fatalf("ParseCertInfo failed: %v", err)
	}

	// Test subject fields
	if certInfo.Subject.CommonName != "test.example.com" {
		t.Errorf("Expected CommonName 'test.example.com', got '%s'", certInfo.Subject.CommonName)
	}
	if certInfo.Subject.Organization != "Test Org" {
		t.Errorf("Expected Organization 'Test Org', got '%s'", certInfo.Subject.Organization)
	}
	if certInfo.Subject.OrganizationalUnit != "Test Unit" {
		t.Errorf("Expected OrganizationalUnit 'Test Unit', got '%s'", certInfo.Subject.OrganizationalUnit)
	}
	if certInfo.Subject.Country != "US" {
		t.Errorf("Expected Country 'US', got '%s'", certInfo.Subject.Country)
	}
	if certInfo.Subject.Province != "California" {
		t.Errorf("Expected Province 'California', got '%s'", certInfo.Subject.Province)
	}
	if certInfo.Subject.Locality != "San Francisco" {
		t.Errorf("Expected Locality 'San Francisco', got '%s'", certInfo.Subject.Locality)
	}

	// Test DNS names
	if len(certInfo.DNSNames) != 2 {
		t.Errorf("Expected 2 DNS names, got %d", len(certInfo.DNSNames))
	}

	// Test IP addresses
	if len(certInfo.IPAddresses) != 2 {
		t.Errorf("Expected 2 IP addresses, got %d", len(certInfo.IPAddresses))
	}

	// Test email addresses
	if len(certInfo.EmailAddresses) != 1 || certInfo.EmailAddresses[0] != "admin@example.com" {
		t.Errorf("Expected email 'admin@example.com', got %v", certInfo.EmailAddresses)
	}

	// Test extended key usage
	if len(certInfo.ExtKeyUsage) != 2 {
		t.Errorf("Expected 2 extended key usages, got %d", len(certInfo.ExtKeyUsage))
	}

	// Test public key info
	if certInfo.PublicKey.Algorithm != "RSA" {
		t.Errorf("Expected RSA algorithm, got '%s'", certInfo.PublicKey.Algorithm)
	}
	if certInfo.PublicKey.Size != 2048 {
		t.Errorf("Expected key size 2048, got %d", certInfo.PublicKey.Size)
	}

	// Test version
	if certInfo.Version != 3 {
		t.Errorf("Expected version 3, got %d", certInfo.Version)
	}

	// Test fingerprints are not empty
	if certInfo.FingerprintSHA256 == "" {
		t.Error("Expected non-empty SHA256 fingerprint")
	}
	if certInfo.FingerprintSHA1 == "" {
		t.Error("Expected non-empty SHA1 fingerprint")
	}

	// Test fingerprint format (colon-separated hex)
	if !strings.Contains(certInfo.FingerprintSHA256, ":") {
		t.Error("Expected colon-separated SHA256 fingerprint")
	}
}

// TestParseCertInfoEmptyInput tests ParseCertInfo with empty input
func TestParseCertInfoEmptyInput(t *testing.T) {
	_, err := ParseCertInfo("")
	if err == nil {
		t.Error("Expected error for empty input, got nil")
	}
}

// TestParseCertInfoInvalidPEM tests ParseCertInfo with invalid PEM data
func TestParseCertInfoInvalidPEM(t *testing.T) {
	invalidInputs := []struct {
		name  string
		input string
	}{
		{"random text", "this is not a PEM certificate"},
		{"malformed PEM", "-----BEGIN CERTIFICATE-----\ninvalid base64\n-----END CERTIFICATE-----"},
		{"truncated PEM", "-----BEGIN CERTIFICATE-----\nMIIB"},
	}

	for _, tt := range invalidInputs {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseCertInfo(tt.input)
			if err == nil {
				t.Errorf("Expected error for %s, got nil", tt.name)
			}
		})
	}
}

// TestParseCertInfoPrivateKeyOnly tests ParseCertInfo with private key PEM (no cert)
func TestParseCertInfoPrivateKeyOnly(t *testing.T) {
	// Generate a private key PEM (no certificate)
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}

	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})

	_, err = ParseCertInfo(string(privPEM))
	if err == nil {
		t.Error("Expected error when parsing private key only, got nil")
	}
}

// TestParseAllCertInfoCertificateChain tests parsing multiple certificates
func TestParseAllCertInfoCertificateChain(t *testing.T) {
	// Generate root CA
	rootKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate root key: %v", err)
	}

	rootSerial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	rootTemplate := &x509.Certificate{
		SerialNumber: rootSerial,
		Subject: pkix.Name{
			CommonName:   "Test Root CA",
			Organization: []string{"Test CA Org"},
		},
		NotBefore:             time.Now().Add(-24 * time.Hour),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	rootDER, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	if err != nil {
		t.Fatalf("Failed to create root certificate: %v", err)
	}

	rootCert, err := x509.ParseCertificate(rootDER)
	if err != nil {
		t.Fatalf("Failed to parse root certificate: %v", err)
	}

	rootPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER})

	// Generate leaf certificate signed by root
	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate leaf key: %v", err)
	}

	leafSerial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	leafTemplate := &x509.Certificate{
		SerialNumber: leafSerial,
		Subject: pkix.Name{
			CommonName:   "leaf.example.com",
			Organization: []string{"Leaf Org"},
		},
		NotBefore:   time.Now().Add(-24 * time.Hour),
		NotAfter:    time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    []string{"leaf.example.com"},
	}

	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, rootCert, &leafKey.PublicKey, rootKey)
	if err != nil {
		t.Fatalf("Failed to create leaf certificate: %v", err)
	}

	leafPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})

	// Combine into chain (leaf first, then root)
	chainPEM := string(leafPEM) + string(rootPEM)

	certs, err := ParseAllCertInfo(chainPEM)
	if err != nil {
		t.Fatalf("ParseAllCertInfo failed: %v", err)
	}

	if len(certs) != 2 {
		t.Fatalf("Expected 2 certificates, got %d", len(certs))
	}

	// Verify first cert is leaf
	if certs[0].Subject.CommonName != "leaf.example.com" {
		t.Errorf("Expected first cert CN 'leaf.example.com', got '%s'", certs[0].Subject.CommonName)
	}

	// Verify second cert is root
	if certs[1].Subject.CommonName != "Test Root CA" {
		t.Errorf("Expected second cert CN 'Test Root CA', got '%s'", certs[1].Subject.CommonName)
	}

	// Verify issuer relationship
	if certs[0].Issuer.CommonName != "Test Root CA" {
		t.Errorf("Expected leaf issuer CN 'Test Root CA', got '%s'", certs[0].Issuer.CommonName)
	}
}

// TestParseCertInfoExpiredCertificate tests expired certificate detection
func TestParseCertInfoExpiredCertificate(t *testing.T) {
	opts := defaultTestCertOptions()
	opts.NotBefore = time.Now().Add(-365 * 24 * time.Hour)
	opts.NotAfter = time.Now().Add(-48 * time.Hour) // Expired 2 days ago

	pemData, err := generateTestCertificate(opts)
	if err != nil {
		t.Fatalf("Failed to generate test certificate: %v", err)
	}

	certInfo, err := ParseCertInfo(pemData)
	if err != nil {
		t.Fatalf("ParseCertInfo failed: %v", err)
	}

	if !certInfo.IsExpired {
		t.Error("Expected IsExpired to be true for expired certificate")
	}

	if certInfo.DaysUntilExpiry > -1 {
		t.Errorf("Expected DaysUntilExpiry <= -1 for expired cert, got %d", certInfo.DaysUntilExpiry)
	}

	if certInfo.ValidityPercentage != 100 {
		t.Errorf("Expected ValidityPercentage to be 100 for expired cert, got %d", certInfo.ValidityPercentage)
	}
}

// TestParseCertInfoNotYetValidCertificate tests not-yet-valid certificate detection
func TestParseCertInfoNotYetValidCertificate(t *testing.T) {
	opts := defaultTestCertOptions()
	opts.NotBefore = time.Now().Add(24 * time.Hour) // Valid starting tomorrow
	opts.NotAfter = time.Now().Add(365 * 24 * time.Hour)

	pemData, err := generateTestCertificate(opts)
	if err != nil {
		t.Fatalf("Failed to generate test certificate: %v", err)
	}

	certInfo, err := ParseCertInfo(pemData)
	if err != nil {
		t.Fatalf("ParseCertInfo failed: %v", err)
	}

	if !certInfo.IsNotYetValid {
		t.Error("Expected IsNotYetValid to be true for not-yet-valid certificate")
	}

	if certInfo.ValidityPercentage != 0 {
		t.Errorf("Expected ValidityPercentage to be 0 for not-yet-valid cert, got %d", certInfo.ValidityPercentage)
	}
}

// TestValidityPercentageCalculation tests the validity percentage at various points
func TestValidityPercentageCalculation(t *testing.T) {
	tests := []struct {
		name           string
		notBefore      time.Time
		notAfter       time.Time
		expectedMinPct int
		expectedMaxPct int
	}{
		{
			name:           "certificate at start of validity",
			notBefore:      time.Now().Add(-1 * time.Hour),
			notAfter:       time.Now().Add(99 * time.Hour),
			expectedMinPct: 0,
			expectedMaxPct: 5,
		},
		{
			name:           "certificate at 50% validity",
			notBefore:      time.Now().Add(-50 * 24 * time.Hour),
			notAfter:       time.Now().Add(50 * 24 * time.Hour),
			expectedMinPct: 45,
			expectedMaxPct: 55,
		},
		{
			name:           "certificate near end of validity",
			notBefore:      time.Now().Add(-99 * time.Hour),
			notAfter:       time.Now().Add(1 * time.Hour),
			expectedMinPct: 95,
			expectedMaxPct: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := defaultTestCertOptions()
			opts.NotBefore = tt.notBefore
			opts.NotAfter = tt.notAfter

			pemData, err := generateTestCertificate(opts)
			if err != nil {
				t.Fatalf("Failed to generate test certificate: %v", err)
			}

			certInfo, err := ParseCertInfo(pemData)
			if err != nil {
				t.Fatalf("ParseCertInfo failed: %v", err)
			}

			if certInfo.ValidityPercentage < tt.expectedMinPct || certInfo.ValidityPercentage > tt.expectedMaxPct {
				t.Errorf("Expected ValidityPercentage between %d and %d, got %d",
					tt.expectedMinPct, tt.expectedMaxPct, certInfo.ValidityPercentage)
			}
		})
	}
}

// TestParseCertInfoKeyUsages tests various key usage flags
func TestParseCertInfoKeyUsages(t *testing.T) {
	opts := defaultTestCertOptions()
	opts.KeyUsage = x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign | x509.KeyUsageCRLSign
	opts.IsCA = true

	pemData, err := generateTestCertificate(opts)
	if err != nil {
		t.Fatalf("Failed to generate test certificate: %v", err)
	}

	certInfo, err := ParseCertInfo(pemData)
	if err != nil {
		t.Fatalf("ParseCertInfo failed: %v", err)
	}

	expectedUsages := []string{"Digital Signature", "Key Encipherment", "Certificate Sign", "CRL Sign"}
	for _, expected := range expectedUsages {
		found := false
		for _, usage := range certInfo.KeyUsage {
			if usage == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected key usage '%s' not found in %v", expected, certInfo.KeyUsage)
		}
	}
}

// TestParseCertInfoExtKeyUsages tests various extended key usage flags
func TestParseCertInfoExtKeyUsages(t *testing.T) {
	opts := defaultTestCertOptions()
	opts.ExtKeyUsage = []x509.ExtKeyUsage{
		x509.ExtKeyUsageServerAuth,
		x509.ExtKeyUsageClientAuth,
		x509.ExtKeyUsageCodeSigning,
		x509.ExtKeyUsageEmailProtection,
	}

	pemData, err := generateTestCertificate(opts)
	if err != nil {
		t.Fatalf("Failed to generate test certificate: %v", err)
	}

	certInfo, err := ParseCertInfo(pemData)
	if err != nil {
		t.Fatalf("ParseCertInfo failed: %v", err)
	}

	expectedUsages := []string{"Server Authentication", "Client Authentication", "Code Signing", "Email Protection"}
	if len(certInfo.ExtKeyUsage) != len(expectedUsages) {
		t.Errorf("Expected %d extended key usages, got %d", len(expectedUsages), len(certInfo.ExtKeyUsage))
	}

	for i, expected := range expectedUsages {
		if i < len(certInfo.ExtKeyUsage) && certInfo.ExtKeyUsage[i] != expected {
			t.Errorf("Expected extended key usage '%s' at index %d, got '%s'", expected, i, certInfo.ExtKeyUsage[i])
		}
	}
}

// TestParseCertInfoMixedPEMBlocks tests parsing PEM with mixed block types
func TestParseCertInfoMixedPEMBlocks(t *testing.T) {
	// Generate a certificate
	opts := defaultTestCertOptions()
	certPEM, err := generateTestCertificate(opts)
	if err != nil {
		t.Fatalf("Failed to generate test certificate: %v", err)
	}

	// Generate a private key
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}

	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})

	// Combine: private key + certificate + random block
	mixedPEM := string(privPEM) + certPEM + "-----BEGIN UNKNOWN-----\ndata\n-----END UNKNOWN-----\n"

	certs, err := ParseAllCertInfo(mixedPEM)
	if err != nil {
		t.Fatalf("ParseAllCertInfo failed: %v", err)
	}

	// Should only find the certificate, not the private key or unknown block
	if len(certs) != 1 {
		t.Errorf("Expected 1 certificate, got %d", len(certs))
	}

	if certs[0].Subject.CommonName != "test.example.com" {
		t.Errorf("Expected CommonName 'test.example.com', got '%s'", certs[0].Subject.CommonName)
	}
}

// TestFormatHexFingerprint tests the fingerprint formatting
func TestFormatHexFingerprint(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "simple bytes",
			input:    []byte{0x01, 0x02, 0x03},
			expected: "01:02:03",
		},
		{
			name:     "bytes with leading zeros",
			input:    []byte{0x00, 0x0A, 0xFF},
			expected: "00:0A:FF",
		},
		{
			name:     "single byte",
			input:    []byte{0xAB},
			expected: "AB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatHexFingerprint(tt.input)
			if result != tt.expected {
				t.Errorf("formatHexFingerprint(%v) = %s, expected %s", tt.input, result, tt.expected)
			}
		})
	}
}




// TestECDSACertificate tests parsing an ECDSA certificate
func TestECDSACertificate(t *testing.T) {
	// Generate ECDSA key
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate ECDSA key: %v", err)
	}

	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   "ecdsa.example.com",
			Organization: []string{"ECDSA Org"},
		},
		NotBefore:             time.Now().Add(-24 * time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("Failed to create ECDSA certificate: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	certInfo, err := ParseCertInfo(string(certPEM))
	if err != nil {
		t.Fatalf("ParseCertInfo failed: %v", err)
	}

	if certInfo.PublicKey.Algorithm != "ECDSA" {
		t.Errorf("Expected ECDSA algorithm, got '%s'", certInfo.PublicKey.Algorithm)
	}

	if certInfo.Subject.CommonName != "ecdsa.example.com" {
		t.Errorf("Expected CommonName 'ecdsa.example.com', got '%s'", certInfo.Subject.CommonName)
	}
}

// TestDaysUntilExpiryCalculation tests the days until expiry calculation
func TestDaysUntilExpiryCalculation(t *testing.T) {
	tests := []struct {
		name        string
		notAfter    time.Duration
		expectedMin int
		expectedMax int
	}{
		{
			name:        "expires in ~30 days",
			notAfter:    30 * 24 * time.Hour,
			expectedMin: 29,
			expectedMax: 31,
		},
		{
			name:        "expires in ~1 day",
			notAfter:    24 * time.Hour,
			expectedMin: 0,
			expectedMax: 2,
		},
		{
			name:        "expires in ~365 days",
			notAfter:    365 * 24 * time.Hour,
			expectedMin: 364,
			expectedMax: 366,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := defaultTestCertOptions()
			opts.NotBefore = time.Now().Add(-24 * time.Hour)
			opts.NotAfter = time.Now().Add(tt.notAfter)

			pemData, err := generateTestCertificate(opts)
			if err != nil {
				t.Fatalf("Failed to generate test certificate: %v", err)
			}

			certInfo, err := ParseCertInfo(pemData)
			if err != nil {
				t.Fatalf("ParseCertInfo failed: %v", err)
			}

			if certInfo.DaysUntilExpiry < tt.expectedMin || certInfo.DaysUntilExpiry > tt.expectedMax {
				t.Errorf("Expected DaysUntilExpiry between %d and %d, got %d",
					tt.expectedMin, tt.expectedMax, certInfo.DaysUntilExpiry)
			}
		})
	}
}

// TestSubjectRawAndIssuerRaw tests that raw subject/issuer strings are populated
func TestSubjectRawAndIssuerRaw(t *testing.T) {
	opts := defaultTestCertOptions()
	opts.CommonName = "test.example.com"
	opts.Organization = []string{"Test Org"}
	opts.Country = []string{"US"}

	pemData, err := generateTestCertificate(opts)
	if err != nil {
		t.Fatalf("Failed to generate test certificate: %v", err)
	}

	certInfo, err := ParseCertInfo(pemData)
	if err != nil {
		t.Fatalf("ParseCertInfo failed: %v", err)
	}

	// Check SubjectRaw contains expected components
	if !strings.Contains(certInfo.SubjectRaw, "CN=test.example.com") {
		t.Errorf("Expected SubjectRaw to contain 'CN=test.example.com', got '%s'", certInfo.SubjectRaw)
	}
	if !strings.Contains(certInfo.SubjectRaw, "O=Test Org") {
		t.Errorf("Expected SubjectRaw to contain 'O=Test Org', got '%s'", certInfo.SubjectRaw)
	}

	// For self-signed cert, issuer should match subject
	if certInfo.SubjectRaw != certInfo.IssuerRaw {
		t.Errorf("For self-signed cert, expected SubjectRaw to equal IssuerRaw")
	}
}

// TestParseAllCertInfoEmptyChain tests ParseAllCertInfo with no valid certs
func TestParseAllCertInfoEmptyChain(t *testing.T) {
	_, err := ParseAllCertInfo("")
	if err == nil {
		t.Error("Expected error for empty input, got nil")
	}

	_, err = ParseAllCertInfo("-----BEGIN PRIVATE KEY-----\ndata\n-----END PRIVATE KEY-----")
	if err == nil {
		t.Error("Expected error for private key only input, got nil")
	}
}

