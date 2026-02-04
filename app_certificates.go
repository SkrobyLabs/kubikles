package main

import (
	"fmt"

	"kubikles/pkg/certviewer"
)

// =============================================================================
// Certificates
// =============================================================================

// CertSubjectInfo contains parsed subject/issuer fields
type CertSubjectInfo struct {
	CommonName         string `json:"commonName"`
	Organization       string `json:"organization"`
	OrganizationalUnit string `json:"organizationalUnit"`
	Country            string `json:"country"`
	Province           string `json:"province"`
	Locality           string `json:"locality"`
}

// CertKeyInfo contains public key information
type CertKeyInfo struct {
	Algorithm string `json:"algorithm"`
	Size      int    `json:"size"`
}

// CertificateInfo contains parsed certificate information for the frontend
type CertificateInfo struct {
	IsCertificate bool `json:"isCertificate"`

	// Subject and Issuer
	Subject    CertSubjectInfo `json:"subject"`
	SubjectRaw string          `json:"subjectRaw"`
	Issuer     CertSubjectInfo `json:"issuer"`
	IssuerRaw  string          `json:"issuerRaw"`

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
	PublicKey          CertKeyInfo `json:"publicKey"`
	SignatureAlgorithm string      `json:"signatureAlgorithm"`
	KeyUsage           []string    `json:"keyUsage"`
	ExtKeyUsage        []string    `json:"extKeyUsage"`

	// Identifiers
	SerialNumber string `json:"serialNumber"`
	Version      int    `json:"version"`

	// Fingerprints
	FingerprintSHA256 string `json:"fingerprintSHA256"`
	FingerprintSHA1   string `json:"fingerprintSHA1"`
}

// GetCertificateInfo parses PEM certificate data and returns info for display
func (a *App) GetCertificateInfo(pemData string) (*CertificateInfo, error) {
	if !certviewer.IsPEMCertificate(pemData) {
		return &CertificateInfo{IsCertificate: false}, nil
	}

	info, err := certviewer.ParseCertInfo(pemData)
	if err != nil {
		return nil, err
	}

	return &CertificateInfo{
		IsCertificate: true,
		Subject: CertSubjectInfo{
			CommonName:         info.Subject.CommonName,
			Organization:       info.Subject.Organization,
			OrganizationalUnit: info.Subject.OrganizationalUnit,
			Country:            info.Subject.Country,
			Province:           info.Subject.Province,
			Locality:           info.Subject.Locality,
		},
		SubjectRaw: info.SubjectRaw,
		Issuer: CertSubjectInfo{
			CommonName:         info.Issuer.CommonName,
			Organization:       info.Issuer.Organization,
			OrganizationalUnit: info.Issuer.OrganizationalUnit,
			Country:            info.Issuer.Country,
			Province:           info.Issuer.Province,
			Locality:           info.Issuer.Locality,
		},
		IssuerRaw:          info.IssuerRaw,
		NotBefore:          info.NotBefore,
		NotAfter:           info.NotAfter,
		IsExpired:          info.IsExpired,
		IsNotYetValid:      info.IsNotYetValid,
		DaysUntilExpiry:    info.DaysUntilExpiry,
		ValidityPercentage: info.ValidityPercentage,
		DNSNames:           info.DNSNames,
		IPAddresses:        info.IPAddresses,
		EmailAddresses:     info.EmailAddresses,
		PublicKey: CertKeyInfo{
			Algorithm: info.PublicKey.Algorithm,
			Size:      info.PublicKey.Size,
		},
		SignatureAlgorithm: info.SignatureAlgorithm,
		KeyUsage:           info.KeyUsage,
		ExtKeyUsage:        info.ExtKeyUsage,
		SerialNumber:       info.SerialNumber,
		Version:            info.Version,
		FingerprintSHA256:  info.FingerprintSHA256,
		FingerprintSHA1:    info.FingerprintSHA1,
	}, nil
}

// GetAllCertificateInfo parses PEM data and returns info for all certificates (for chains)
func (a *App) GetAllCertificateInfo(pemData string) ([]*CertificateInfo, error) {
	if !certviewer.IsPEMCertificate(pemData) {
		return nil, fmt.Errorf("data does not contain valid PEM certificates")
	}

	infos, err := certviewer.ParseAllCertInfo(pemData)
	if err != nil {
		return nil, err
	}

	var result []*CertificateInfo
	for _, info := range infos {
		result = append(result, &CertificateInfo{
			IsCertificate: true,
			Subject: CertSubjectInfo{
				CommonName:         info.Subject.CommonName,
				Organization:       info.Subject.Organization,
				OrganizationalUnit: info.Subject.OrganizationalUnit,
				Country:            info.Subject.Country,
				Province:           info.Subject.Province,
				Locality:           info.Subject.Locality,
			},
			SubjectRaw: info.SubjectRaw,
			Issuer: CertSubjectInfo{
				CommonName:         info.Issuer.CommonName,
				Organization:       info.Issuer.Organization,
				OrganizationalUnit: info.Issuer.OrganizationalUnit,
				Country:            info.Issuer.Country,
				Province:           info.Issuer.Province,
				Locality:           info.Issuer.Locality,
			},
			IssuerRaw:          info.IssuerRaw,
			NotBefore:          info.NotBefore,
			NotAfter:           info.NotAfter,
			IsExpired:          info.IsExpired,
			IsNotYetValid:      info.IsNotYetValid,
			DaysUntilExpiry:    info.DaysUntilExpiry,
			ValidityPercentage: info.ValidityPercentage,
			DNSNames:           info.DNSNames,
			IPAddresses:        info.IPAddresses,
			EmailAddresses:     info.EmailAddresses,
			PublicKey: CertKeyInfo{
				Algorithm: info.PublicKey.Algorithm,
				Size:      info.PublicKey.Size,
			},
			SignatureAlgorithm: info.SignatureAlgorithm,
			KeyUsage:           info.KeyUsage,
			ExtKeyUsage:        info.ExtKeyUsage,
			SerialNumber:       info.SerialNumber,
			Version:            info.Version,
			FingerprintSHA256:  info.FingerprintSHA256,
			FingerprintSHA1:    info.FingerprintSHA1,
		})
	}

	return result, nil
}
