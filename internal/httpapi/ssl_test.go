package httpapi

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"web-panel/internal/domain"
	"web-panel/internal/service"
	"web-panel/internal/store"
)

// Exercise the original Vue request contracts, including their ID capitalization.
func TestOfficialSSLAndIndependentMTLSContracts(t *testing.T) {
	s, e := store.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	m, e := service.New(s, runtimeStub{})
	if e != nil {
		t.Fatal(e)
	}
	h := newAPI(m, fstest.MapFS{"index.html": {Data: []byte("ui")}})
	send := func(method, path string, v any, code int) []byte {
		t.Helper()
		var body bytes.Buffer
		if v != nil {
			json.NewEncoder(&body).Encode(v)
		}
		req := httptest.NewRequest(method, "http://localhost/api"+path, &body)
		st, _ := s.Load()
		req.Header.Set("If-Match", fmt.Sprint(st.Revision))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != code {
			t.Fatalf("%s %s: %d: %s", method, path, w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}
	ca := map[string]any{"name": "Server CA", "commonName": "Server CA", "country": "CN", "organization": "Test", "organizationUint": "Engineering", "keyType": "EC256"}
	send("POST", "/panel/websites/ca", ca, 200)
	ca["name"] = "Client CA"
	ca["commonName"] = "Client CA"
	send("POST", "/panel/mtls/ca", ca, 200)
	st, _ := s.Load()
	sslCA := st.SSL.CAs[0].ID
	mtlsCA := st.MTLS.CAs[0]
	send("POST", "/panel/websites/ca/obtain", map[string]any{"id": sslCA, "domains": "example.test\nlocalhost", "keyType": "RSA2048", "time": 1, "unit": "year", "autoRenew": true}, 200)
	st, _ = s.Load()
	ssl := st.SSL.Records[0]
	old := st.Certificates[len(st.Certificates)-1].Serial
	send("POST", "/panel/websites/ca/renew", map[string]any{"SSLID": ssl.ID}, 200)
	st, _ = s.Load()
	if st.Certificates[len(st.Certificates)-1].Serial == old {
		t.Fatal("renew did not replace material")
	}
	listed := send("POST", "/panel/websites/ssl/search", map[string]any{"page": 1, "pageSize": 20}, 200)
	if strings.Contains(string(listed), "PRIVATE KEY") {
		t.Fatal("list exposed key")
	}
	detail := send("GET", fmt.Sprintf("/panel/websites/ssl/%d", ssl.ID), nil, 200)
	if !strings.Contains(string(detail), "PRIVATE KEY") {
		t.Fatal("explicit detail missing key")
	}
	download := send("POST", "/panel/websites/ssl/download", map[string]any{"id": ssl.ID}, 200)
	z, e := zip.NewReader(bytes.NewReader(download), int64(len(download)))
	if e != nil || len(z.File) != 2 {
		t.Fatal("invalid SSL download", e)
	}
	send("POST", "/mtls/issue", map[string]any{"name": "Invalid namespace", "kind": "client", "caId": ssl.MaterialID, "days": 30}, 400)
	send("POST", "/mtls/issue", map[string]any{"name": "Browser client", "kind": "client", "caId": mtlsCA.MaterialID, "days": 30, "keyType": "EC384"}, 200)
	st, _ = s.Load()
	if len(st.SSL.Records) != 1 || len(st.MTLS.Clients) != 1 {
		t.Fatal("namespace leak")
	}
	send("GET", fmt.Sprintf("/panel/mtls/ca/%d", mtlsCA.ID), nil, 200)
	send("POST", "/panel/mtls/ca/del", map[string]any{"id": mtlsCA.ID}, 400)
	send("POST", "/panel/websites/dns", map[string]any{"name": "Cloudflare", "type": "CloudFlare", "authorization": map[string]string{"apiKey": "fixture-only-token"}}, 200)
	listed = send("GET", "/overview", nil, 200)
	if strings.Contains(string(listed), "fixture-only-token") || strings.Contains(string(listed), "PRIVATE KEY") {
		t.Fatal("overview leaked credentials")
	}
	// The removed mixed namespace is no longer a public API.
	send("POST", "/certificates/issue", map[string]any{"kind": "client"}, 404)
	site := domain.Site{Name: "Web", Kind: "static", Domains: []string{"example.test"}, Enabled: true}
	send("POST", "/sites", site, 200)
	st, _ = s.Load()
	web := st.Sites[0]
	send("POST", fmt.Sprintf("/panel/websites/%d/https", web.PanelID), map[string]any{"websiteId": web.PanelID, "enable": true, "type": "existed", "websiteSSLId": ssl.ID, "httpConfig": "HTTPToHTTPS", "SSLProtocol": []string{"TLSv1.2", "TLSv1.3"}, "algorithm": "HIGH:!aNULL"}, 200)
	send("PUT", "/mtls/bindings", domain.MTLSBinding{SiteID: web.ID, CAID: mtlsCA.MaterialID, Enabled: true}, 200)
	send("POST", fmt.Sprintf("/panel/websites/%d/https", web.PanelID), map[string]any{"enable": false}, 400)
	send("DELETE", "/sites/"+web.ID, nil, 200)
	st, _ = s.Load()
	if len(st.MTLS.Bindings) != 0 {
		t.Fatal("deleted website left mTLS binding")
	}
}

func TestOriginalCAZipUploadContract(t *testing.T) {
	s, e := store.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	m, e := service.New(s, runtimeStub{})
	if e != nil {
		t.Fatal(e)
	}
	if e = m.SSLCreateCA(service.CACreate{Name: "Exported", CommonName: "Exported", Country: "CN", Organization: "Test", KeyType: "EC256"}); e != nil {
		t.Fatal(e)
	}
	st, _ := s.Load()
	blob, e := m.CADownload(st.SSL.CAs[0].ID, false)
	if e != nil {
		t.Fatal(e)
	}
	zipped, e := zip.NewReader(bytes.NewReader(blob), int64(len(blob)))
	if e != nil {
		t.Fatal(e)
	}
	names := []string{}
	for _, f := range zipped.File {
		names = append(names, f.Name)
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"ca.crt", "ca.key"}) {
		t.Fatalf("upstream CA ZIP filenames changed: %v", names)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, e := writer.CreateFormFile("file", "Imported.zip")
	if e != nil {
		t.Fatal(e)
	}
	part.Write(blob)
	writer.Close()
	req := httptest.NewRequest("POST", "http://localhost/api/panel/websites/ca/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	h := newAPI(m, fstest.MapFS{"index.html": {Data: []byte("ui")}})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	st, _ = s.Load()
	if len(st.SSL.CAs) != 2 || st.SSL.CAs[1].Name != "Imported" || len(st.MTLS.Authorities) != 0 {
		t.Fatal("upstream filename import or namespace isolation failed")
	}
}
