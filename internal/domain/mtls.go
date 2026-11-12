package domain

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"slices"
)

func ValidateMTLS(s State) error {
	if len(s.MTLS.Authorities) > 1000 || len(s.MTLS.Clients) > 10000 || len(s.MTLS.Bindings) > 10000 {
		return fmt.Errorf("mTLS 数量超出限制")
	}
	cas := map[string]*x509.Certificate{}
	ids := map[string]bool{}
	for _, ca := range s.MTLS.Authorities {
		if ca.Kind != "ca" {
			return fmt.Errorf("mTLS CA 存储包含非 CA 证书")
		}
	}
	for _, client := range s.MTLS.Clients {
		if client.Kind != "client" {
			return fmt.Errorf("mTLS 客户端存储包含非客户端证书")
		}
	}
	for _, c := range append(slices.Clone(s.MTLS.Authorities), s.MTLS.Clients...) {
		if !ValidID(c.ID) || ids[c.ID] {
			return fmt.Errorf("mTLS 证书 ID 无效或重复")
		}
		ids[c.ID] = true
		pair, err := tls.X509KeyPair([]byte(c.CertPEM), []byte(c.KeyPEM))
		if err != nil {
			return fmt.Errorf("mTLS 证书: %w", err)
		}
		parsed, err := x509.ParseCertificate(pair.Certificate[0])
		if err != nil {
			return err
		}
		if c.Serial != parsed.SerialNumber.String() || !c.NotAfter.Equal(parsed.NotAfter) || !c.NotBefore.Equal(parsed.NotBefore) {
			return fmt.Errorf("mTLS 元数据不匹配")
		}
		if c.Kind == "ca" && parsed.IsCA {
			if parsed.KeyUsage&x509.KeyUsageCRLSign == 0 {
				return fmt.Errorf("mTLS CA 必须允许签署 CRL")
			}
			cas[c.ID] = parsed
		} else if c.Kind != "client" || parsed.IsCA || !slices.Contains(parsed.ExtKeyUsage, x509.ExtKeyUsageClientAuth) {
			return fmt.Errorf("mTLS 仅允许 CA 和 clientAuth 证书")
		}
	}
	for _, c := range s.MTLS.Clients {
		ca := cas[c.CAID]
		if ca == nil {
			return fmt.Errorf("mTLS 签发 CA 不存在")
		}
		pair, _ := tls.X509KeyPair([]byte(c.CertPEM), []byte(c.KeyPEM))
		leaf, _ := x509.ParseCertificate(pair.Certificate[0])
		if err := leaf.CheckSignatureFrom(ca); err != nil {
			return err
		}
	}
	bound := map[string]bool{}
	for _, b := range s.MTLS.Bindings {
		if bound[b.SiteID] {
			return fmt.Errorf("mTLS 网站绑定重复")
		}
		bound[b.SiteID] = true
		siteIndex := slices.IndexFunc(s.Sites, func(site Site) bool { return site.ID == b.SiteID })
		if siteIndex < 0 {
			return fmt.Errorf("mTLS 绑定网站不存在")
		}
		if cas[b.CAID] == nil {
			return fmt.Errorf("mTLS 信任 CA 不存在")
		}
		if !b.Enabled {
			continue
		}
		site := SiteDefaults(s.Sites[siteIndex])
		if site.TLSID == "" || site.HTTPS.HTTPConfig == "HTTPAlso" {
			return fmt.Errorf("请先启用网站 HTTPS，并关闭明文 HTTP 访问，再启用 mTLS")
		}
	}
	return nil
}
