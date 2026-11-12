// SPDX-License-Identifier: GPL-3.0-only
// 1Panel 3adee9445897aa41c0160b4f805a9b1f43f712ac. Persistence provided by the standalone manager.
package model

import "time"

type WebsiteSSL struct {
	BaseModel
	PrimaryDomain  string    `json:"primaryDomain"`
	PrivateKey     string    `json:"privateKey"`
	Pem            string    `json:"pem"`
	Domains        string    `json:"domains"`
	CertURL        string    `json:"certURL"`
	Type           string    `json:"type"`
	Provider       string    `json:"provider"`
	Organization   string    `json:"organization"`
	DnsAccountID   uint      `json:"dnsAccountId"`
	AcmeAccountID  uint      `gorm:"column:acme_account_id" json:"acmeAccountId" `
	CaID           uint      `json:"caId"`
	AutoRenew      bool      `json:"autoRenew"`
	ExpireDate     time.Time `json:"expireDate"`
	StartDate      time.Time `json:"startDate"`
	Status         string    `json:"status"`
	Message        string    `json:"message"`
	KeyType        string    `json:"keyType"`
	PushDir        bool      `json:"pushDir"`
	Dir            string    `json:"dir"`
	Description    string    `json:"description"`
	SkipDNS        bool      `json:"skipDNS"`
	Nameserver1    string    `json:"nameserver1"`
	Nameserver2    string    `json:"nameserver2"`
	DisableCNAME   bool      `json:"disableCNAME"`
	ExecShell      bool      `json:"execShell"`
	Shell          string    `json:"shell"`
	MasterSSLID    uint      `json:"masterSslId"`
	Nodes          string    `json:"nodes"`
	PushNode       bool      `json:"pushNode"`
	PrivateKeyPath string    `json:"privateKeyPath"`
	CertPath       string    `json:"certPath"`
	IsIp           bool      `json:"isIP"`

	AcmeAccount WebsiteAcmeAccount `json:"acmeAccount" gorm:"-:migration"`
	DnsAccount  WebsiteDnsAccount  `json:"dnsAccount" gorm:"-:migration"`
	Websites    []Website          `json:"websites" gorm:"-:migration"`
}

func (w WebsiteSSL) TableName() string {
	return "website_ssls"
}
