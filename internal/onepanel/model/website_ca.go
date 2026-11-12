// SPDX-License-Identifier: GPL-3.0-only
// 1Panel 3adee9445897aa41c0160b4f805a9b1f43f712ac. Persistence provided by the standalone manager.
package model

type WebsiteCA struct {
	BaseModel
	CSR        string `gorm:"not null;" json:"csr"`
	Name       string `gorm:"not null;" json:"name"`
	PrivateKey string `gorm:"not null" json:"privateKey"`
	KeyType    string `gorm:"not null;default:RSA2048" json:"keyType"`
}
