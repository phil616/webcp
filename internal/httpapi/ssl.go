package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"web-panel/internal/domain"
	"web-panel/internal/onepanel/model"
	panelssl "web-panel/internal/onepanel/ssl"
	"web-panel/internal/pki"
	"web-panel/internal/service"
)

func panelDecode(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 10<<20)).Decode(v)
}
func registerCertificates(register func(string, endpoint), m *service.Manager) {
	register("POST /api/mtls/issue", func(w http.ResponseWriter, r *http.Request) error {
		var v pki.IssueRequest
		rev, e := decode(w, r, &v)
		if e != nil {
			return e
		}
		cert, e := m.IssueMTLS(rev, v)
		if e == nil {
			writeJSON(w, 200, cert)
		}
		return e
	})
	register("POST /api/mtls/clients/{id}/revoke", func(w http.ResponseWriter, r *http.Request) error {
		rev, e := revision(r)
		if e != nil {
			return e
		}
		return success(w, m.RevokeMTLS(rev, r.PathValue("id")))
	})
	register("DELETE /api/mtls/authorities/{id}", func(w http.ResponseWriter, r *http.Request) error {
		rev, e := revision(r)
		if e != nil {
			return e
		}
		return success(w, m.DeleteMTLSCA(rev, r.PathValue("id")))
	})
	register("PUT /api/mtls/bindings", func(w http.ResponseWriter, r *http.Request) error {
		var v domain.MTLSBinding
		rev, e := decode(w, r, &v)
		if e != nil {
			return e
		}
		return success(w, m.SaveMTLSBinding(rev, v))
	})
	register("GET /api/mtls/certificates/{id}/download", func(w http.ResponseWriter, r *http.Request) error {
		data, e := m.MTLSBundle(r.PathValue("id"), r.URL.Query().Get("key") == "true", r.URL.Query().Get("crl") == "true")
		if e != nil {
			return e
		}
		w.Header().Set("Content-Disposition", "attachment; filename=mtls.pem")
		w.Header().Set("Content-Type", "application/x-pem-file")
		_, e = w.Write(data)
		return e
	})
	handler := func(w http.ResponseWriter, r *http.Request) error { return panelCertificate(w, r, m) }
	register("GET /api/panel/{path...}", handler)
	register("POST /api/panel/{path...}", handler)
}
func panelCertificate(w http.ResponseWriter, r *http.Request, m *service.Manager) error {
	path := r.PathValue("path")
	mtls := strings.HasPrefix(path, "mtls/ca")
	if mtls {
		path = strings.Replace(path, "mtls/ca", "websites/ca", 1)
	}
	// Preserve the original read/write HTTP contract; GET must never mutate state.
	getAllowed := path == "websites/list" || strings.HasPrefix(path, "websites/") && strings.HasSuffix(path, "/https")
	for _, prefix := range []string{"websites/ssl/", "websites/ca/"} {
		if strings.HasPrefix(path, prefix) && parseUint(strings.TrimPrefix(path, prefix)) > 0 {
			getAllowed = true
		}
	}
	if (r.Method == "GET" && !getAllowed) || (r.Method == "POST" && getAllowed && path != "websites/list" && !strings.HasSuffix(path, "/https")) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return nil
	}
	var data any
	var err error
	// File upload uses the upstream multipart contract, with bounded extraction.
	if path == "websites/ssl/upload/file" || path == "websites/ca/upload" {
		r.Body = http.MaxBytesReader(w, r.Body, 11<<20)
		if err = r.ParseMultipartForm(11 << 20); err != nil {
			return err
		}
		defer r.MultipartForm.RemoveAll()
		if path == "websites/ca/upload" {
			f, header, e := r.FormFile("file")
			if e != nil {
				return e
			}
			defer f.Close()
			if header.Size > 10<<20 || !strings.HasSuffix(strings.ToLower(header.Filename), ".zip") {
				return fmt.Errorf("CA 上传需要 10 MiB 以内的 ZIP 文件")
			}
			blob, e := io.ReadAll(io.LimitReader(f, (10<<20)+1))
			if e != nil {
				return e
			}
			parsed, e := panelssl.ParseCAArchive(blob)
			if e != nil {
				return e
			}
			err = m.ImportCA(mtls, strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename)), parsed.CSR, parsed.PrivateKey)
		} else {
			var cmd service.SSLCommand
			cmd.Type = "upload"
			cmd.SSLID = uint(parseUint(r.FormValue("sslID")))
			cmd.Description = r.FormValue("description")
			for _, item := range []struct {
				names  []string
				target *string
			}{{[]string{"certificate", "certificateFile"}, &cmd.Certificate}, {[]string{"privateKey", "privateKeyFile"}, &cmd.PrivateKey}} {
				for _, name := range item.names {
					f, _, e := r.FormFile(name)
					if e == nil {
						b, e := io.ReadAll(io.LimitReader(f, 1<<20))
						f.Close()
						if e != nil {
							return e
						}
						*item.target = string(b)
						break
					}
				}
			}
			err = m.SSLUpload(cmd)
		}
		if err != nil {
			return err
		}
		writeJSON(w, 200, map[string]bool{"success": true})
		return nil
	}
	if strings.HasPrefix(path, "standalone/files/") {
		return panelFiles(w, r, path)
	}
	if path == "standalone/logs" {
		var v struct {
			ID       uint `json:"id"`
			Page     int  `json:"page"`
			PageSize int  `json:"pageSize"`
		}
		if err = panelDecode(r, &v); err != nil {
			return err
		}
		lines, e := m.SSLLog(v.ID)
		if e != nil {
			return e
		}
		writeJSON(w, 200, map[string]any{"lines": lines, "total": 1, "totalLines": len(lines), "end": true, "path": fmt.Sprintf("ssl/logs/%d.log", v.ID), "scope": "tail"})
		return nil
	}
	if strings.HasPrefix(path, "websites/") && strings.HasSuffix(path, "/https") {
		return panelHTTPS(w, r, m, path)
	}
	if path == "websites/list" {
		s, e := m.SSLList()
		if e != nil {
			return e
		}
		items := []map[string]any{}
		for _, site := range s.Sites {
			bindings := []map[string]any{}
			for _, b := range site.Bindings {
				bindings = append(bindings, map[string]any{"domain": b.Domain, "port": b.Port, "ssl": b.SSL})
			}
			items = append(items, map[string]any{"id": site.PanelID, "primaryDomain": site.Domains[0], "alias": site.Name, "domains": bindings})
		}
		writeJSON(w, 200, items)
		return nil
	}
	var raw json.RawMessage
	if r.Method == "POST" {
		blob, e := io.ReadAll(io.LimitReader(r.Body, 10<<20))
		if e != nil {
			return e
		}
		raw = blob
		if len(raw) == 0 {
			raw = []byte("{}")
		}
	}
	var q struct {
		ID            uint   `json:"id"`
		IDs           []uint `json:"ids"`
		SSLID         uint   `json:"sslID"`
		WebsiteSSLID  uint   `json:"websiteSSLId"`
		Page          int    `json:"page"`
		PageSize      int    `json:"pageSize"`
		Domain        string `json:"domain"`
		AcmeAccountID string `json:"acmeAccountID"`
		OrderBy       string `json:"orderBy"`
		Order         string `json:"order"`
	}
	if len(raw) > 0 {
		if err = json.Unmarshal(raw, &q); err != nil {
			return err
		}
	}
	switch path {
	case "websites/ssl/search", "websites/ssl/list":
		s, e := m.SSLList()
		if e != nil {
			return e
		}
		records := slices.Clone(s.SSL.Records)
		slices.SortFunc(records, func(a, b domain.SSLRecord) int {
			if q.OrderBy == "expire_date" {
				return a.ExpireDate.Compare(b.ExpireDate)
			}
			return a.UpdatedAt.Compare(b.UpdatedAt)
		})
		if q.Order != "ascending" {
			slices.Reverse(records)
		}
		items := []model.WebsiteSSL{}
		for _, r := range records {
			if path == "websites/ssl/list" && q.AcmeAccountID != "" && r.AcmeAccountID != uint(parseUint(q.AcmeAccountID)) {
				continue
			}
			if q.Domain != "" && !strings.Contains(strings.ToLower(r.PrimaryDomain+" "+r.Domains), strings.ToLower(q.Domain)) {
				continue
			}
			v, _ := m.SSLGet(r.ID)
			v.PrivateKey = ""
			if path == "websites/ssl/search" {
				v.Pem = ""
			}
			items = append(items, v)
		}
		if path == "websites/ssl/list" {
			data = items
		} else {
			data = page(items, q.Page, q.PageSize)
		}
	case "websites/ssl":
		var cmd service.SSLCommand
		if err = json.Unmarshal(raw, &cmd); err == nil {
			var id uint
			id, err = m.SSLSave(cmd, true)
			if err == nil && cmd.Provider != "dnsManual" {
				err = m.SSLObtain(id)
			}
			data = map[string]uint{"id": id}
		}
	case "websites/ssl/update":
		var cmd service.SSLCommand
		if err = json.Unmarshal(raw, &cmd); err == nil {
			_, err = m.SSLSave(cmd, false)
		}
	case "websites/ssl/upload":
		var cmd service.SSLCommand
		if err = json.Unmarshal(raw, &cmd); err == nil {
			err = m.SSLUpload(cmd)
		}
	case "websites/ssl/obtain":
		if q.ID == 0 {
			q.ID = q.SSLID
		}
		err = m.SSLObtain(q.ID)
	case "websites/ssl/resolve":
		data, err = m.SSLResolve(r.Context(), q.WebsiteSSLID)
	case "websites/ssl/download", "websites/ca/download":
		var blob []byte
		var e error
		if mtls {
			blob, e = m.CADownload(q.ID, true)
		} else {
			blob, e = m.SSLDownload(q.ID, path == "websites/ca/download")
		}
		if e != nil {
			return e
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", "attachment; filename=certificate.zip")
		_, e = w.Write(blob)
		return e
	case "websites/ssl/del":
		err = m.SSLDelete(r.Context(), q.IDs)
	case "websites/acme/search":
		s, e := m.SSLList()
		if e != nil {
			return e
		}
		items := []model.WebsiteAcmeAccount{}
		for _, a := range s.SSL.Accounts {
			v := a.WebsiteAcmeAccount
			v.PrivateKey = ""
			v.EabHmacKey = ""
			items = append(items, v)
		}
		data = page(items, q.Page, q.PageSize)
	case "websites/acme":
		var account model.WebsiteAcmeAccount
		if err = json.Unmarshal(raw, &account); err == nil {
			err = m.SSLRegister(r.Context(), account)
		}
	case "websites/acme/del":
		err = m.SSLDeleteAccount(q.ID, false)
	case "websites/acme/update":
		var v model.WebsiteAcmeAccount
		if err = json.Unmarshal(raw, &v); err == nil {
			err = m.SSLUpdateAccount(v)
		}
	case "websites/dns/search":
		s, e := m.SSLList()
		if e != nil {
			return e
		}
		items := []map[string]any{}
		for _, dns := range s.SSL.DNSAccounts {
			var auth map[string]string
			json.Unmarshal([]byte(dns.Secret), &auth)
			items = append(items, map[string]any{"id": dns.ID, "name": dns.Name, "type": dns.Type, "authorization": auth, "createdAt": dns.CreatedAt})
		}
		data = page(items, q.Page, q.PageSize)
	case "websites/dns", "websites/dns/update":
		var cmd service.DNSCommand
		if err = json.Unmarshal(raw, &cmd); err == nil {
			err = m.SSLSaveDNS(cmd)
		}
	case "websites/dns/del":
		err = m.SSLDeleteAccount(q.ID, true)
	case "websites/ca/search":
		s, e := m.SSLList()
		if e != nil {
			return e
		}
		if mtls {
			data = page(s.MTLS.CAs, q.Page, q.PageSize)
		} else {
			data = page(s.SSL.CAs, q.Page, q.PageSize)
		}
	case "websites/ca":
		var cmd service.CACreate
		if err = json.Unmarshal(raw, &cmd); err == nil {
			err = m.CreateCA(cmd, mtls)
		}
	case "websites/ca/obtain":
		if mtls {
			return fmt.Errorf("mTLS CA 只签发客户端证书，请使用客户端管理")
		}
		var cmd service.SSLCommand
		if err = json.Unmarshal(raw, &cmd); err == nil {
			var id uint
			id, err = m.SSLSign(cmd)
			data = map[string]uint{"id": id}
		}
	case "websites/ca/renew":
		if mtls {
			return service.ErrNotFound
		}
		_, err = m.SSLSign(service.SSLCommand{SSLID: q.SSLID, Renew: true})
	case "websites/ca/del":
		err = m.DeleteCA(q.ID, mtls)
	case "websites/ca/import":
		var cmd service.SSLCommand
		var named struct {
			Name string `json:"name"`
		}
		json.Unmarshal(raw, &named)
		if err = json.Unmarshal(raw, &cmd); err == nil {
			cert, key := cmd.Certificate, cmd.PrivateKey
			if cmd.Type == "local" {
				cert, err = service.ReadPEMFile(cmd.CertificatePath)
				if err == nil {
					key, err = service.ReadPEMFile(cmd.PrivateKeyPath)
				}
			}
			if err == nil {
				err = m.ImportCA(mtls, named.Name, cert, key)
			}
		}
	default:
		if r.Method == "GET" && strings.HasPrefix(path, "websites/ssl/") {
			id := uint(parseUint(strings.TrimPrefix(path, "websites/ssl/")))
			data, err = m.SSLGet(id)
		} else if r.Method == "GET" && strings.HasPrefix(path, "websites/ca/") {
			id := uint(parseUint(strings.TrimPrefix(path, "websites/ca/")))
			data, err = m.CA(id, mtls)
		} else {
			return service.ErrNotFound
		}
	}
	if err != nil {
		return err
	}
	if data == nil {
		data = map[string]bool{"success": true}
	}
	writeJSON(w, 200, data)
	return nil
}
func parseUint(v string) uint64 { n, _ := strconv.ParseUint(v, 10, 64); return n }
func page[T any](items []T, current, size int) map[string]any {
	if current < 1 {
		current = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 10000 {
		size = 10000
	}
	start := min((current-1)*size, len(items))
	end := min(start+size, len(items))
	return map[string]any{"items": items[start:end], "total": len(items)}
}
