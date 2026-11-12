// Package pki implements a small ECDSA certificate authority for website mTLS.
package pki

import (
	"crypto"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"strings"
	"time"
	"unicode/utf8"

	"web-panel/internal/domain"
	panelssl "web-panel/internal/onepanel/ssl"
)

type IssueRequest struct {
	KeyType string   `json:"keyType"`
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	CAID    string   `json:"caId"`
	Domains []string `json:"domains"`
	Days    int      `json:"days"`
}

func parse(c domain.Certificate) (*x509.Certificate, crypto.Signer, error) {
	pair, err := tls.X509KeyPair([]byte(c.CertPEM), []byte(c.KeyPEM))
	if err != nil {
		return nil, nil, err
	}
	cert, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return nil, nil, err
	}
	key, ok := pair.PrivateKey.(crypto.Signer)
	if !ok {
		return nil, nil, fmt.Errorf("unsupported signing key")
	}
	return cert, key, nil
}

func Issue(req IssueRequest, ca *domain.Certificate) (domain.Certificate, error) {
	var result domain.Certificate
	if strings.TrimSpace(req.Name) == "" || utf8.RuneCountInString(req.Name) > 128 {
		return result, fmt.Errorf("证书名称必须为 1–128 字符")
	}
	if req.Kind != "ca" && req.Kind != "server" && req.Kind != "client" {
		return result, fmt.Errorf("证书类型无效")
	}
	if req.Days < 1 || req.Days > 3650 {
		return result, fmt.Errorf("有效期必须为 1–3650 天")
	}
	if req.Kind == "server" {
		if err := domain.ValidateDomains(req.Domains); err != nil {
			return result, err
		}
	}
	if req.Kind == "ca" && req.CAID != "" {
		return result, fmt.Errorf("目前只支持根 CA")
	}
	if req.KeyType == "" {
		req.KeyType = "EC256"
	}
	key, public, keyPEM, err := panelssl.CreatePrivateKey(req.KeyType)
	if err != nil {
		return result, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return result, err
	}
	if serial.Sign() == 0 {
		serial.SetInt64(1)
	}
	now := time.Now().UTC().Truncate(time.Second)
	tmpl := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: req.Name}, NotBefore: now.Add(-5 * time.Minute), NotAfter: now.Add(time.Duration(req.Days) * 24 * time.Hour), BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature}
	var signer crypto.Signer = key.(crypto.Signer)
	parent := tmpl
	if req.Kind == "ca" {
		tmpl.IsCA = true
		tmpl.MaxPathLenZero = true
		tmpl.KeyUsage |= x509.KeyUsageCertSign | x509.KeyUsageCRLSign
	} else {
		if ca == nil || ca.Kind != "ca" || ca.RevokedAt != nil {
			return result, fmt.Errorf("签发服务器或客户端证书需要 CA")
		}
		parent, signer, err = parse(*ca)
		if err != nil {
			return result, err
		}
		if now.After(parent.NotAfter) || now.Before(parent.NotBefore) {
			return result, fmt.Errorf("CA 不在有效期内")
		}
		if tmpl.NotAfter.After(parent.NotAfter) {
			tmpl.NotAfter = parent.NotAfter
		}
		if req.Kind == "server" {
			tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
			for _, name := range req.Domains {
				if ip := net.ParseIP(name); ip != nil {
					tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
				} else {
					tmpl.DNSNames = append(tmpl.DNSNames, name)
				}
			}
		} else {
			tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, public, signer)
	if err != nil {
		return result, err
	}
	result = domain.Certificate{ID: domain.NewID(), Name: req.Name, Kind: req.Kind, CAID: req.CAID, Serial: serial.String(), Domains: req.Domains, CertPEM: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), KeyPEM: string(keyPEM), NotBefore: tmpl.NotBefore, NotAfter: tmpl.NotAfter}
	return result, nil
}

// ImportServer accepts a PEM leaf/chain and its matching private key.
func ImportServer(name, certPEM, keyPEM string) (domain.Certificate, error) {
	var result domain.Certificate
	if strings.TrimSpace(name) == "" || utf8.RuneCountInString(name) > 128 {
		return result, fmt.Errorf("需要证书名称")
	}
	pair, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		return result, err
	}
	c, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return result, err
	}
	if c.IsCA {
		return result, fmt.Errorf("请导入服务器叶证书")
	}
	allowed := len(c.ExtKeyUsage) == 0
	for _, usage := range c.ExtKeyUsage {
		if usage == x509.ExtKeyUsageServerAuth || usage == x509.ExtKeyUsageAny {
			allowed = true
		}
	}
	if !allowed {
		return result, fmt.Errorf("证书用途不包含 serverAuth")
	}
	if time.Now().After(c.NotAfter) || time.Now().Before(c.NotBefore) {
		return result, fmt.Errorf("证书不在有效期内")
	}
	domains := append([]string{}, c.DNSNames...)
	for _, ip := range c.IPAddresses {
		domains = append(domains, ip.String())
	}
	result = domain.Certificate{ID: domain.NewID(), Name: name, Kind: "server", Serial: c.SerialNumber.String(), Domains: domains, CertPEM: certPEM, KeyPEM: keyPEM, NotBefore: c.NotBefore, NotAfter: c.NotAfter}
	return result, nil
}

// CRL includes every revoked child. A generation is valid no longer than the CA.
func CRL(ca domain.Certificate, certs []domain.Certificate) ([]byte, error) {
	issuer, key, err := parse(ca)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	// Revocation changes are explicitly deployed. Keeping NextUpdate aligned with
	// CA expiry avoids silently disabling mTLS after an unattended seven days.
	next := issuer.NotAfter
	entries := []x509.RevocationListEntry{}
	for _, c := range certs {
		if c.CAID == ca.ID && c.RevokedAt != nil {
			serial, ok := new(big.Int).SetString(c.Serial, 10)
			if !ok {
				return nil, fmt.Errorf("invalid serial")
			}
			entries = append(entries, x509.RevocationListEntry{SerialNumber: serial, RevocationTime: *c.RevokedAt, ReasonCode: 1})
		}
	}
	list := &x509.RevocationList{Number: big.NewInt(now.UnixNano()), ThisUpdate: now.Add(-time.Minute), NextUpdate: next, RevokedCertificateEntries: entries}
	der, err := x509.CreateRevocationList(rand.Reader, list, issuer, key)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: der}), nil
}
