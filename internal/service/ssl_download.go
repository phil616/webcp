package service

import (
	"archive/zip"
	"bytes"
)

func (m *Manager) SSLDownload(id uint, ca bool) ([]byte, error) {
	s, e := m.SSLList()
	if e != nil {
		return nil, e
	}
	var cert, key string
	if ca {
		for _, r := range s.SSL.CAs {
			if r.ID == id {
				if c := sslMaterial(&s, r.MaterialID); c != nil {
					cert, key = c.CertPEM, c.KeyPEM
				}
			}
		}
	} else {
		r := sslRecord(&s, id)
		if r != nil {
			v := sslModel(&s, *r, true)
			cert, key = v.Pem, v.PrivateKey
		}
	}
	if cert == "" {
		return nil, ErrNotFound
	}
	return certificateArchive(cert, key, ca)
}
func certificateArchive(cert, key string, ca bool) ([]byte, error) {
	certName, keyName := "fullchain.pem", "privkey.pem"
	if ca {
		certName, keyName = "ca.crt", "ca.key"
	}
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for file, value := range map[string]string{certName: cert, keyName: key} {
		header := &zip.FileHeader{Name: file, Method: zip.Deflate}
		header.SetMode(0600)
		w, e := z.CreateHeader(header)
		if e != nil {
			return nil, e
		}
		if _, e = w.Write([]byte(value)); e != nil {
			return nil, e
		}
	}
	if e := z.Close(); e != nil {
		return nil, e
	}
	return b.Bytes(), nil
}
func (m *Manager) CADownload(id uint, mtls bool) ([]byte, error) {
	s, e := m.SSLList()
	if e != nil {
		return nil, e
	}
	records, materials := caRepository(&s, mtls)
	for _, r := range *records {
		if r.ID == id {
			if c := caMaterial(*materials, r.MaterialID); c != nil {
				return certificateArchive(c.CertPEM, c.KeyPEM, true)
			}
		}
	}
	return nil, ErrNotFound
}
