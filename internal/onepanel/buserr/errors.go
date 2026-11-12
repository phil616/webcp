package buserr

import "errors"

// Localized strings from 1Panel agent/i18n/lang/zh.yaml at the pinned commit.
func New(message string) error {
	switch message {
	case "ErrCAUpload":
		message = "请上传包含有效自签根证书 ca.crt 及匹配私钥 ca.key 的 ZIP 文件（最大 10 MB），密钥算法须为支持的 RSA 或 EC。"
	case "ErrCAImport":
		message = "请提供有效的自签根证书及匹配私钥（每个最大 1 MB），密钥算法须为支持的 RSA 或 EC。"
	}
	return errors.New(message)
}
