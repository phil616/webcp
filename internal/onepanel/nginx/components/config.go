// SPDX-License-Identifier: GPL-3.0-only
// Derived from 1Panel commit 3adee9445897aa41c0160b4f805a9b1f43f712ac; see web/panel/manifest.json.
package components

type Config struct {
	*Block
	FilePath string
}

var repeatKeys = map[string]struct {
}{
	"limit_conn":       {},
	"limit_conn_zone":  {},
	"set":              {},
	"if":               {},
	"proxy_set_header": {},
	"location":         {},
	"include":          {},
	"sub_filter":       {},
	"add_header":       {},
	"set_real_ip_from": {},
	"error_page":       {},
}

func IsRepeatKey(key string) bool {
	if _, ok := repeatKeys[key]; ok {
		return true
	}
	return false
}
