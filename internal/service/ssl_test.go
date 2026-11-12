package service

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"slices"
	"strings"
	"testing"
	"time"

	"os"
	"path/filepath"
	"web-panel/internal/backup"
	"web-panel/internal/domain"
	"web-panel/internal/onepanel/model"
	"web-panel/internal/pki"
)

func testCA(name string) CACreate {
	return CACreate{Name: name, CommonName: name, Country: "CN", Organization: "Example", OrganizationUint: "Engineering", KeyType: "EC256"}
}
func TestSSLAndMTLSNamespaces(t *testing.T) {
	m, s, _ := setup(t)
	if e := m.CreateCA(testCA("SSL Root"), false); e != nil {
		t.Fatal(e)
	}
	st, _ := s.Load()
	sslCA := st.SSL.CAs[0]
	sslMaterialID := sslCA.MaterialID
	if _, e := m.IssueMTLS(st.Revision, pki.IssueRequest{Name: "cross-namespace", Kind: "client", CAID: sslMaterialID, Days: 1}); e == nil {
		t.Fatal("SSL CA accepted in mTLS")
	}
	if e := m.CreateCA(testCA("Client Root"), true); e != nil {
		t.Fatal(e)
	}
	st, _ = s.Load()
	mtlsCA := st.MTLS.CAs[0]
	if len(st.SSL.CAs) != 1 || len(st.Certificates) != 1 || mtlsCA.MaterialID == sslMaterialID {
		t.Fatal("namespaces coupled")
	}
	id, e := m.SSLSign(SSLCommand{WebsiteSSL: model.WebsiteSSL{BaseModel: model.BaseModel{ID: sslCA.ID}, Domains: "example.test\nlocalhost\n127.0.0.1", KeyType: "EC384", AutoRenew: true}, Time: 1, Unit: "year"})
	if e != nil {
		t.Fatal(e)
	}
	cert, e := m.SSLGet(id)
	if e != nil {
		t.Fatal(e)
	}
	if cert.Provider != "selfSigned" || cert.Status != "ready" || cert.KeyType != "EC384" || cert.Pem == "" || cert.PrivateKey == "" {
		t.Fatalf("bad SSL result: %+v", cert.Websites)
	}
	pair, e := tls.X509KeyPair([]byte(cert.Pem), []byte(cert.PrivateKey))
	if e != nil {
		t.Fatal(e)
	}
	leaf, _ := x509.ParseCertificate(pair.Certificate[0])
	if e = leaf.VerifyHostname("127.0.0.1"); e != nil {
		t.Fatal(e)
	}
	if !slices.Contains(leaf.ExtKeyUsage, x509.ExtKeyUsageServerAuth) || slices.Contains(leaf.ExtKeyUsage, x509.ExtKeyUsageClientAuth) {
		t.Fatal("wrong SSL EKU")
	}
	st, _ = s.Load()
	client, e := m.IssueMTLS(st.Revision, pki.IssueRequest{Name: "Client", Kind: "client", CAID: mtlsCA.MaterialID, KeyType: "RSA2048", Days: 1})
	if e != nil {
		t.Fatal(e)
	}
	st, _ = s.Load()
	if len(st.SSL.Records) != 1 || len(st.Certificates) != 2 || len(st.MTLS.Clients) != 1 {
		t.Fatal("client leaked into SSL")
	}
	if e = m.SSLDeleteCA(sslCA.ID); e == nil {
		t.Fatal("CA with SSL children deleted")
	}
	if e = m.DeleteMTLSCA(st.Revision, mtlsCA.MaterialID); e == nil {
		t.Fatal("client CA deleted before revocation")
	}
	if e = m.SSLDelete(context.Background(), []uint{id}); e != nil {
		t.Fatal(e)
	}
	if e = m.SSLDeleteCA(sslCA.ID); e != nil {
		t.Fatal(e)
	}
	st, _ = s.Load()
	if len(st.MTLS.Authorities) != 1 || len(st.MTLS.Clients) != 1 {
		t.Fatal("deleting SSL changed mTLS")
	}
	if e = m.RevokeMTLS(st.Revision, client.ID); e != nil {
		t.Fatal(e)
	}
	crl, e := m.MTLSBundle(mtlsCA.MaterialID, false, true)
	if e != nil {
		t.Fatal(e)
	}
	block, _ := pem.Decode(crl)
	revocations, e := x509.ParseRevocationList(block.Bytes)
	if e != nil || len(revocations.RevokedCertificateEntries) != 1 {
		t.Fatal("CRL did not retain serial", e)
	}
	overview, e := m.Overview(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	blob, _ := json.Marshal(overview)
	if strings.Contains(string(blob), "PRIVATE KEY") {
		t.Fatal("overview exposed keys")
	}
}
func TestLegacyMixedCertificateMigration(t *testing.T) {
	ca, e := pki.Issue(pki.IssueRequest{Name: "Legacy Root", Kind: "ca", Days: 30}, nil)
	if e != nil {
		t.Fatal(e)
	}
	client, e := pki.Issue(pki.IssueRequest{Name: "Legacy Client", Kind: "client", CAID: ca.ID, Days: 10}, &ca)
	if e != nil {
		t.Fatal(e)
	}
	server, e := pki.Issue(pki.IssueRequest{Name: "Legacy Server", Kind: "server", CAID: ca.ID, Days: 10, Domains: []string{"legacy.test"}}, &ca)
	if e != nil {
		t.Fatal(e)
	}
	revoked := time.Now().UTC()
	client.RevokedAt = &revoked
	st := domain.Default()
	st.Certificates = []domain.Certificate{ca, server, client}
	siteID := domain.NewID()
	st.Sites = []domain.Site{{ID: siteID, Name: "Legacy", Domains: []string{"legacy.test"}, Kind: "static", TLSID: server.ID, ClientCAID: ca.ID, Enabled: true}}
	// Old archives are normalized before validation, just like state.json.
	_, storage, _ := setup(t)
	record, archiveErr := backup.Create(storage.Root, st)
	if archiveErr != nil {
		t.Fatal(archiveErr)
	}
	archive, archiveErr := os.Open(filepath.Join(storage.Root, "backups", record.ID+".tar.gz"))
	if archiveErr != nil {
		t.Fatal(archiveErr)
	}
	restored, archiveErr := backup.Extract(archive, t.TempDir())
	archive.Close()
	if archiveErr != nil {
		t.Fatal(archiveErr)
	}
	if len(restored.MTLS.Clients) != 1 || restored.MTLS.Clients[0].RevokedAt == nil {
		t.Fatal("legacy archive lost revoked credential")
	}
	domain.NormalizeState(&st)
	if e = domain.Validate(st); e != nil {
		t.Fatal(e)
	}
	domain.NormalizeState(&st)
	if len(st.Certificates) != 2 || len(st.MTLS.Clients) != 1 || len(st.MTLS.Authorities) != 1 || len(st.MTLS.Bindings) != 1 {
		t.Fatal("migration is not idempotent")
	}
	if st.Sites[0].ClientCAID != "" || domain.MTLSAuthority(st, siteID) != ca.ID {
		t.Fatal("legacy binding not separated")
	}
	migrated := st.MTLS.Clients[0]
	if migrated.Serial != client.Serial || migrated.CertPEM != client.CertPEM || migrated.KeyPEM != client.KeyPEM || migrated.RevokedAt == nil || !migrated.RevokedAt.Equal(revoked) {
		t.Fatal("migration changed/re-enabled credential")
	}
}
func TestSSLWorkerDoesNotChangeUnrelatedDraft(t *testing.T) {
	m, s, _ := setup(t)
	before := revisionOf(t, s)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m.StartSSLWorker(ctx)
	if revisionOf(t, s) != before {
		t.Fatal("starting worker dirtied draft")
	}
}

// Fixtures go through the same upstream CA/signing service as the UI.
func issueSSLFixture(t *testing.T, m *Manager, q pki.IssueRequest) (domain.Certificate, error) {
	t.Helper()
	if q.Kind == "ca" {
		if e := m.SSLCreateCA(testCA(q.Name)); e != nil {
			return domain.Certificate{}, e
		}
		s, e := m.SSLList()
		if e != nil {
			return domain.Certificate{}, e
		}
		for _, r := range s.SSL.CAs {
			if r.Name == q.Name {
				return *sslMaterial(&s, r.MaterialID), nil
			}
		}
		return domain.Certificate{}, ErrNotFound
	}
	s, e := m.SSLList()
	if e != nil {
		return domain.Certificate{}, e
	}
	var caID uint
	for _, r := range s.SSL.CAs {
		if r.MaterialID == q.CAID {
			caID = r.ID
		}
	}
	id, e := m.SSLSign(SSLCommand{WebsiteSSL: model.WebsiteSSL{BaseModel: model.BaseModel{ID: caID}, Domains: strings.Join(q.Domains, "\n"), KeyType: "EC256"}, Time: q.Days, Unit: "day"})
	if e != nil {
		return domain.Certificate{}, e
	}
	s, e = m.SSLList()
	if e != nil {
		return domain.Certificate{}, e
	}
	r := sslRecord(&s, id)
	return *sslMaterial(&s, r.MaterialID), nil
}

func TestUploadedReplacementKeepsWebsiteBindingAndReloads(t *testing.T) {
	m, s, runtime := setup(t)
	ca, e := issueSSLFixture(t, m, pki.IssueRequest{Name: "Rotation", Kind: "ca", Days: 30})
	if e != nil {
		t.Fatal(e)
	}
	first, e := issueSSLFixture(t, m, pki.IssueRequest{Name: "Before", Kind: "server", CAID: ca.ID, Domains: []string{"rotate.test"}, Days: 10})
	if e != nil {
		t.Fatal(e)
	}
	site, e := m.SaveSite(revisionOf(t, s), domain.Site{Name: "Rotation", Kind: "static", Domains: []string{"rotate.test"}, Enabled: true, TLSID: first.ID})
	if e != nil {
		t.Fatal(e)
	}
	if e = m.Apply(context.Background(), revisionOf(t, s)); e != nil {
		t.Fatal(e)
	}
	replacement, e := issueSSLFixture(t, m, pki.IssueRequest{Name: "After", Kind: "server", CAID: ca.ID, Domains: site.Domains, Days: 20})
	if e != nil {
		t.Fatal(e)
	}
	st, _ := s.Load()
	var id uint
	for _, r := range st.SSL.Records {
		if r.MaterialID == first.ID {
			id = r.ID
		}
	}
	upload := SSLCommand{SSLID: id, Certificate: replacement.CertPEM, WebsiteSSL: model.WebsiteSSL{Type: "paste", PrivateKey: replacement.KeyPEM}}
	if e = m.SSLUpload(upload); e != nil {
		t.Fatal(e)
	}
	st, _ = s.Load()
	if st.Sites[0].TLSID != first.ID || sslMaterial(&st, first.ID).Serial != replacement.Serial {
		t.Fatal("replacement broke material binding")
	}
	if runtime.applied != st.Revision {
		t.Fatal("uploaded replacement was not deployed")
	}
	runtime.fail = true
	if e = m.SSLUpload(upload); e == nil || !strings.Contains(e.Error(), "证书已保存，但部署失败") {
		t.Fatal("failed deployment reported as success", e)
	}
}
