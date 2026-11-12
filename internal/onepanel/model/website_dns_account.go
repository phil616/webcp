// SPDX-License-Identifier: GPL-3.0-only
// 1Panel 3adee9445897aa41c0160b4f805a9b1f43f712ac. Persistence provided by the standalone manager.
package model

type WebsiteDnsAccount struct {
	BaseModel
	Name          string `gorm:"not null" json:"name"`
	Type          string `gorm:"not null" json:"type"`
	Authorization string `gorm:"not null" json:"-"`
}

func (w WebsiteDnsAccount) TableName() string {
	return "website_dns_accounts"
}
