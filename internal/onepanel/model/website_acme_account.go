// SPDX-License-Identifier: GPL-3.0-only
// 1Panel 3adee9445897aa41c0160b4f805a9b1f43f712ac. Persistence provided by the standalone manager.
package model

type WebsiteAcmeAccount struct {
	BaseModel
	Email      string `gorm:"not null" json:"email"`
	URL        string `gorm:"not null" json:"url"`
	PrivateKey string `gorm:"not null" json:"-"`
	Type       string `gorm:"not null;default:letsencrypt" json:"type"`
	EabKid     string `json:"eabKid"`
	EabHmacKey string `json:"eabHmacKey"`
	KeyType    string `gorm:"not null;default:RSA2048" json:"keyType"`
	UseProxy   bool   `gorm:"default:false" json:"useProxy"`
	CaDirURL   string `json:"caDirURL"`
	UseEAB     bool   `json:"useEAB"`
}

func (w WebsiteAcmeAccount) TableName() string {
	return "website_acme_accounts"
}
