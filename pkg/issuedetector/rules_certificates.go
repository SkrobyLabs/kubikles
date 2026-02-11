package issuedetector

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"math"
	"time"

	v1 "k8s.io/api/core/v1"
)

func certRules() []Rule {
	return []Rule{
		&ruleCERT001{baseRule: baseRule{id: "CERT001", name: "Certificate Expiring Soon", description: "TLS certificate in Secret expires within 30 days", severity: SeverityWarning, category: CategorySecurity, requires: []string{"secrets"}}},
		&ruleCERT002{baseRule: baseRule{id: "CERT002", name: "Certificate Expired", description: "TLS certificate in Secret has already expired", severity: SeverityCritical, category: CategorySecurity, requires: []string{"secrets"}}},
		&ruleCERT003{baseRule: baseRule{id: "CERT003", name: "Incomplete CA Chain", description: "TLS certificate chain appears incomplete (missing intermediate CA)", severity: SeverityWarning, category: CategorySecurity, requires: []string{"secrets"}}},
		&ruleCERT004{baseRule: baseRule{id: "CERT004", name: "Ingress TLS Secret Expiring", description: "Ingress references a TLS Secret with a certificate expiring within 30 days", severity: SeverityWarning, category: CategorySecurity, requires: []string{"ingresses", "secrets"}}},
	}
}

// parseTLSCerts parses PEM-encoded certificate data and returns the parsed certificates.
func parseTLSCerts(pemData []byte) ([]*x509.Certificate, error) {
	var certs []*x509.Certificate
	rest := pemData
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return certs, err
		}
		certs = append(certs, cert)
	}
	return certs, nil
}

// certSubjectName returns a human-readable subject name from a certificate.
func certSubjectName(cert *x509.Certificate) string {
	if cert.Subject.CommonName != "" {
		return cert.Subject.CommonName
	}
	if len(cert.DNSNames) > 0 {
		return cert.DNSNames[0]
	}
	return cert.Subject.String()
}

// CERT001: Certificate expiring within 30 days
type ruleCERT001 struct{ baseRule }

func (r *ruleCERT001) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	secrets := cache.Secrets()
	now := time.Now()
	threshold := now.Add(30 * 24 * time.Hour)
	var findings []Finding

	for _, sec := range secrets {
		if sec.Type != v1.SecretTypeTLS {
			continue
		}
		certPEM, ok := sec.Data["tls.crt"]
		if !ok || len(certPEM) == 0 {
			continue
		}

		certs, err := parseTLSCerts(certPEM)
		if err != nil || len(certs) == 0 {
			continue
		}

		leaf := certs[0]
		if leaf.NotAfter.Before(now) {
			continue // CERT002 handles expired certs
		}
		if leaf.NotAfter.Before(threshold) {
			daysRemaining := int(math.Ceil(time.Until(leaf.NotAfter).Hours() / 24))
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "Secret", Name: sec.Name, Namespace: sec.Namespace},
				fmt.Sprintf("Secret '%s' TLS certificate for '%s' expires in %d days", sec.Name, certSubjectName(leaf), daysRemaining),
				"Renew the certificate before it expires",
				map[string]string{
					"secretName":    sec.Name,
					"namespace":     sec.Namespace,
					"expiresAt":     leaf.NotAfter.UTC().Format(time.RFC3339),
					"daysRemaining": fmt.Sprintf("%d", daysRemaining),
					"subject":       certSubjectName(leaf),
				},
			))
		}
	}
	return findings, nil
}

// CERT002: Certificate already expired
type ruleCERT002 struct{ baseRule }

func (r *ruleCERT002) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	secrets := cache.Secrets()
	now := time.Now()
	var findings []Finding

	for _, sec := range secrets {
		if sec.Type != v1.SecretTypeTLS {
			continue
		}
		certPEM, ok := sec.Data["tls.crt"]
		if !ok || len(certPEM) == 0 {
			continue
		}

		certs, err := parseTLSCerts(certPEM)
		if err != nil || len(certs) == 0 {
			continue
		}

		leaf := certs[0]
		if leaf.NotAfter.Before(now) {
			daysSinceExpiry := int(math.Ceil(time.Since(leaf.NotAfter).Hours() / 24))
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "Secret", Name: sec.Name, Namespace: sec.Namespace},
				fmt.Sprintf("Secret '%s' TLS certificate for '%s' expired %d days ago", sec.Name, certSubjectName(leaf), daysSinceExpiry),
				"Replace the expired certificate immediately",
				map[string]string{
					"secretName":      sec.Name,
					"namespace":       sec.Namespace,
					"expiresAt":       leaf.NotAfter.UTC().Format(time.RFC3339),
					"daysSinceExpiry": fmt.Sprintf("%d", daysSinceExpiry),
					"subject":         certSubjectName(leaf),
				},
			))
		}
	}
	return findings, nil
}

// CERT003: Incomplete CA chain
type ruleCERT003 struct{ baseRule }

func (r *ruleCERT003) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	secrets := cache.Secrets()
	var findings []Finding

	for _, sec := range secrets {
		if sec.Type != v1.SecretTypeTLS {
			continue
		}
		certPEM, ok := sec.Data["tls.crt"]
		if !ok || len(certPEM) == 0 {
			continue
		}

		certs, err := parseTLSCerts(certPEM)
		if err != nil || len(certs) == 0 {
			continue
		}

		// Only flag if chain has exactly 1 cert and it is not self-signed
		if len(certs) == 1 {
			leaf := certs[0]
			if leaf.Issuer.String() != leaf.Subject.String() {
				findings = append(findings, makeFinding(r,
					ResourceRef{Kind: "Secret", Name: sec.Name, Namespace: sec.Namespace},
					fmt.Sprintf("Secret '%s' has a single certificate for '%s' without the issuing CA — chain may be incomplete", sec.Name, certSubjectName(leaf)),
					"Include intermediate CA certificates in the tls.crt bundle",
					map[string]string{
						"secretName": sec.Name,
						"namespace":  sec.Namespace,
						"subject":    certSubjectName(leaf),
						"issuer":     leaf.Issuer.CommonName,
					},
				))
			}
		}
	}
	return findings, nil
}

// CERT004: Ingress TLS Secret expiring
type ruleCERT004 struct{ baseRule }

func (r *ruleCERT004) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	ingresses := cache.Ingresses()
	secrets := cache.Secrets()
	now := time.Now()
	threshold := now.Add(30 * 24 * time.Hour)

	// Build index of TLS secrets by ns/name
	secretIndex := make(map[string]v1.Secret)
	for _, sec := range secrets {
		if sec.Type == v1.SecretTypeTLS {
			secretIndex[sec.Namespace+"/"+sec.Name] = sec
		}
	}

	var findings []Finding
	for _, ing := range ingresses {
		for _, tls := range ing.Spec.TLS {
			if tls.SecretName == "" {
				continue
			}
			key := ing.Namespace + "/" + tls.SecretName
			sec, ok := secretIndex[key]
			if !ok {
				continue
			}

			certPEM, ok := sec.Data["tls.crt"]
			if !ok || len(certPEM) == 0 {
				continue
			}

			certs, err := parseTLSCerts(certPEM)
			if err != nil || len(certs) == 0 {
				continue
			}

			leaf := certs[0]
			if leaf.NotAfter.Before(now) || leaf.NotAfter.After(threshold) {
				continue // expired handled by CERT002, far-future is fine
			}

			daysRemaining := int(math.Ceil(time.Until(leaf.NotAfter).Hours() / 24))
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "Ingress", Name: ing.Name, Namespace: ing.Namespace},
				fmt.Sprintf("Ingress '%s' TLS secret '%s' certificate for '%s' expires in %d days",
					ing.Name, tls.SecretName, certSubjectName(leaf), daysRemaining),
				"Renew the TLS certificate referenced by this Ingress",
				map[string]string{
					"secretName":    tls.SecretName,
					"namespace":     ing.Namespace,
					"expiresAt":     leaf.NotAfter.UTC().Format(time.RFC3339),
					"daysRemaining": fmt.Sprintf("%d", daysRemaining),
					"subject":       certSubjectName(leaf),
				},
			))
		}
	}
	return findings, nil
}
