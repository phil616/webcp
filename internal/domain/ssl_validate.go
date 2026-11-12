package domain

import "fmt"

// Validate references within SSL metadata separately from mTLS trust material.
func ValidateSSL(s State) error {
	if len(s.SSL.Records) > 10000 || len(s.SSL.CAs) > 1000 || len(s.SSL.Accounts) > 1000 || len(s.SSL.DNSAccounts) > 1000 {
		return fmt.Errorf("SSL 记录过多")
	}
	materials := map[string]Certificate{}
	for _, c := range s.Certificates {
		materials[c.ID] = c
	}
	cas := map[uint]bool{}
	for _, r := range s.SSL.CAs {
		if r.ID == 0 || cas[r.ID] || materials[r.MaterialID].Kind != "ca" {
			return fmt.Errorf("SSL CA 元数据引用无效")
		}
		cas[r.ID] = true
	}
	accounts := map[uint]bool{}
	for _, a := range s.SSL.Accounts {
		if a.ID == 0 || accounts[a.ID] || a.Secret == "" {
			return fmt.Errorf("ACME 账户元数据无效")
		}
		accounts[a.ID] = true
	}
	dns := map[uint]bool{}
	for _, a := range s.SSL.DNSAccounts {
		if a.ID == 0 || dns[a.ID] || a.Secret == "" {
			return fmt.Errorf("DNS 账户元数据无效")
		}
		dns[a.ID] = true
	}
	records := map[uint]bool{}
	for _, r := range s.SSL.Records {
		if r.ID == 0 || records[r.ID] {
			return fmt.Errorf("SSL 记录 ID 无效")
		}
		records[r.ID] = true
		if r.MaterialID != "" && materials[r.MaterialID].Kind != "server" {
			return fmt.Errorf("SSL 服务器证书引用无效")
		}
		switch r.Provider {
		case "manual":
		case "selfSigned":
			if !cas[r.CaID] {
				return fmt.Errorf("SSL 签发 CA 不存在")
			}
		case "http", "dnsManual", "dnsAccount":
			if !accounts[r.AcmeAccountID] {
				return fmt.Errorf("SSL ACME 账户不存在")
			}
			if r.Provider == "dnsAccount" && !dns[r.DnsAccountID] {
				return fmt.Errorf("SSL DNS 账户不存在")
			}
		default:
			return fmt.Errorf("SSL 申请方式无效")
		}
	}
	return nil
}
