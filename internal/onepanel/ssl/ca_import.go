// SPDX-License-Identifier: GPL-3.0-only
// Source: 1Panel agent/app/service/website_ca.go, commit 3adee9445897aa41c0160b4f805a9b1f43f712ac.
package ssl

import (
	"archive/zip"
	"bytes"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"path"
	"time"
	"web-panel/internal/onepanel/buserr"
	"web-panel/internal/onepanel/model"
)

func parseCAArchive(archive []byte) (*model.WebsiteCA, error) {
	invalid := func() (*model.WebsiteCA, error) { return nil, buserr.New("ErrCAUpload") }
	if len(archive) > 10<<20 {
		return invalid()
	}
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil || len(reader.File) > 16 {
		return invalid()
	}
	contents := make(map[string][]byte)
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		name := path.Base(file.Name)
		if (name != "ca.crt" && name != "ca.key") || contents[name] != nil || file.UncompressedSize64 > 1<<20 {
			return invalid()
		}
		stream, err := file.Open()
		if err != nil {
			return invalid()
		}
		data, err := io.ReadAll(io.LimitReader(stream, (1<<20)+1))
		stream.Close()
		if err != nil || len(data) > 1<<20 {
			return invalid()
		}
		contents[name] = data
	}
	return parseAndValidateRootCA(contents["ca.crt"], contents["ca.key"])
}

func parseAndValidateRootCA(certificate, privateKey []byte) (*model.WebsiteCA, error) {
	invalid := func() (*model.WebsiteCA, error) { return nil, buserr.New("ErrCAImport") }
	if len(certificate) > 1<<20 || len(privateKey) > 1<<20 {
		return invalid()
	}
	pair, err := tls.X509KeyPair(certificate, privateKey)
	if err != nil || len(pair.Certificate) != 1 {
		return invalid()
	}
	cert := pair.Leaf
	now := time.Now()
	if cert == nil || !cert.IsCA || !cert.BasicConstraintsValid || cert.KeyUsage&x509.KeyUsageCertSign == 0 ||
		!bytes.Equal(cert.RawIssuer, cert.RawSubject) || cert.CheckSignatureFrom(cert) != nil ||
		now.Before(cert.NotBefore) || !now.Before(cert.NotAfter) {
		return invalid()
	}
	ca := &model.WebsiteCA{CSR: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}))}
	// Normalize keys to the formats used by ObtainSSL, including PKCS#8 uploads.
	switch key := pair.PrivateKey.(type) {
	case *ecdsa.PrivateKey:
		bits := key.Curve.Params().BitSize
		if bits != 256 && bits != 384 {
			return invalid()
		}
		der, err := x509.MarshalECPrivateKey(key)
		if err != nil {
			return invalid()
		}
		ca.KeyType = fmt.Sprintf("EC%d", bits)
		ca.PrivateKey = string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}))
	case *rsa.PrivateKey:
		bits := key.N.BitLen()
		if bits != 2048 && bits != 3072 && bits != 4096 && bits != 8192 {
			return invalid()
		}
		ca.KeyType = fmt.Sprintf("RSA%d", bits)
		ca.PrivateKey = string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	default:
		return invalid()
	}
	return ca, nil
}

func ParseCAArchive(archive []byte) (*model.WebsiteCA, error) { return parseCAArchive(archive) }
func ParseRootCA(certificate, privateKey []byte) (*model.WebsiteCA, error) {
	return parseAndValidateRootCA(certificate, privateKey)
}
