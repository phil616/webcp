// SPDX-License-Identifier: GPL-3.0-only
// Copied from 1Panel agent/app/service/website_ca.go at 3adee9445897aa41c0160b4f805a9b1f43f712ac.
package ssl

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"github.com/go-acme/lego/v5/certcrypto"
)

func leafKeyUsage(keyType string) x509.KeyUsage {
	usage := x509.KeyUsageDigitalSignature
	if KeyType(keyType) != certcrypto.EC256 && KeyType(keyType) != certcrypto.EC384 {
		usage |= x509.KeyUsageKeyEncipherment
	}
	return usage
}

func createPrivateKey(keyType string) (privateKey any, publicKey any, privateKeyBytes []byte, err error) {
	privateKey, err = certcrypto.GeneratePrivateKey(KeyType(keyType))
	if err != nil {
		return
	}
	var (
		caPrivateKeyPEM = new(bytes.Buffer)
	)
	if KeyType(keyType) == certcrypto.EC256 || KeyType(keyType) == certcrypto.EC384 {
		publicKey = &privateKey.(*ecdsa.PrivateKey).PublicKey
		publicKey = publicKey.(*ecdsa.PublicKey)
		block := &pem.Block{
			Type: "EC PRIVATE KEY",
		}
		privateBytes, sErr := x509.MarshalECPrivateKey(privateKey.(*ecdsa.PrivateKey))
		if sErr != nil {
			err = sErr
			return
		}
		block.Bytes = privateBytes
		_ = pem.Encode(caPrivateKeyPEM, block)
	} else {
		publicKey = &privateKey.(*rsa.PrivateKey).PublicKey
		_ = pem.Encode(caPrivateKeyPEM, &pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(privateKey.(*rsa.PrivateKey)),
		})
	}
	privateKeyBytes = caPrivateKeyPEM.Bytes()
	return
}

func CreatePrivateKey(keyType string) (any, any, []byte, error) { return createPrivateKey(keyType) }
func LeafKeyUsage(keyType string) x509.KeyUsage                 { return leafKeyUsage(keyType) }
