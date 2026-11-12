package pki

import (
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"web-panel/internal/domain"
)

func TestCertificatePurposesAndRevocation(t *testing.T) {
	ca, err := Issue(IssueRequest{Name: "Root", Kind: "ca", Days: 30}, nil)
	if err != nil {
		t.Fatal(err)
	}
	client, err := Issue(IssueRequest{Name: "Client", Kind: "client", CAID: ca.ID, Days: 90}, &ca)
	if err != nil {
		t.Fatal(err)
	}
	leaf, _, err := parse(client)
	if err != nil {
		t.Fatal(err)
	}
	root, _, _ := parse(ca)
	pool := x509.NewCertPool()
	pool.AddCert(root)
	if _, err = leaf.Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err != nil {
		t.Fatal(err)
	}
	if _, err = leaf.Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); err == nil {
		t.Fatal("client cert accepted for server auth")
	}
	if client.NotAfter.After(ca.NotAfter) {
		t.Fatal("child outlived CA")
	}
	now := time.Now()
	client.RevokedAt = &now
	encoded, err := CRL(ca, []domain.Certificate{client})
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(encoded)
	crl, err := x509.ParseRevocationList(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if err = crl.CheckSignatureFrom(root); err != nil {
		t.Fatal(err)
	}
	if len(crl.RevokedCertificateEntries) != 1 || crl.RevokedCertificateEntries[0].SerialNumber.String() != client.Serial {
		t.Fatal("missing revoked serial")
	}
}
