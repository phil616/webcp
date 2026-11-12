package domain

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// OpenRestyConfig stores only supported HTTP directives, not nginx fragments.
// String values preserve nginx size suffixes and compound time expressions.
type OpenRestyConfig struct {
	HTTP map[string]string `json:"http"`
}

func DefaultOpenResty() OpenRestyConfig {
	return OpenRestyConfig{HTTP: map[string]string{
		"server_names_hash_bucket_size": "512", "client_header_buffer_size": "1k", "client_max_body_size": "1m",
		"keepalive_timeout": "75s", "gzip": "off", "gzip_min_length": "20", "gzip_comp_level": "1",
		"proxy_connect_timeout": "60s", "proxy_send_timeout": "60s", "proxy_read_timeout": "60s",
	}}
}
func EffectiveOpenResty(config *OpenRestyConfig) OpenRestyConfig {
	value := DefaultOpenResty()
	if config != nil {
		maps.Copy(value.HTTP, config.HTTP)
	}
	return value
}
func OpenRestyDirectives(config *OpenRestyConfig) []string {
	return slices.Sorted(maps.Keys(EffectiveOpenResty(config).HTTP))
}

var nginxSize = regexp.MustCompile(`^([0-9]{1,9})([kKmMgG]?)$`)
var nginxTimeToken = regexp.MustCompile(`([0-9]+)(ms|[yMwdhms])?`)

func nginxTime(value string, wholeSeconds bool) (uint64, bool) {
	if value == "" {
		return 0, false
	}
	units := map[string]struct {
		rank  int
		scale uint64
	}{"y": {0, 31536000000}, "M": {1, 2592000000}, "w": {2, 604800000}, "d": {3, 86400000}, "h": {4, 3600000}, "m": {5, 60000}, "s": {6, 1000}, "ms": {7, 1}, "": {8, 1000}}
	tokens := nginxTimeToken.FindAllStringSubmatch(value, -1)
	remaining := value
	previous := -1
	var total uint64
	limit := uint64(2147483647)
	if wholeSeconds {
		limit *= 1000
	}
	for _, token := range tokens {
		if !strings.HasPrefix(remaining, token[0]) {
			return 0, false
		}
		remaining = strings.TrimPrefix(remaining, token[0])
		u := units[token[2]]
		if u.rank <= previous || (wholeSeconds && token[2] == "ms") || (!wholeSeconds && u.rank < 2) {
			return 0, false
		}
		previous = u.rank
		number, err := strconv.ParseUint(token[1], 10, 64)
		if err != nil || number > limit/u.scale || total > limit-number*u.scale {
			return 0, false
		}
		total += number * u.scale
	}
	return total, remaining == "" && len(tokens) > 0
}
func ValidateOpenResty(config *OpenRestyConfig) error {
	defaults := DefaultOpenResty().HTTP
	for key, value := range EffectiveOpenResty(config).HTTP {
		if _, ok := defaults[key]; !ok {
			return fmt.Errorf("不支持的 OpenResty 参数: %s", key)
		}
		valid := false
		switch key {
		case "gzip":
			valid = value == "on" || value == "off"
		case "gzip_comp_level", "server_names_hash_bucket_size":
			n, e := strconv.Atoi(value)
			max := 9999
			if key == "gzip_comp_level" {
				max = 9
			}
			valid = e == nil && n >= 1 && n <= max && strconv.Itoa(n) == value
		case "client_header_buffer_size", "client_max_body_size", "gzip_min_length":
			match := nginxSize.FindStringSubmatch(value)
			if match != nil {
				n, _ := strconv.ParseUint(match[1], 10, 64)
				valid = key != "client_header_buffer_size" || n > 0
			}
		default:
			parts := strings.Fields(value)
			if len(parts) >= 1 && len(parts) <= 2 && strings.Join(parts, " ") == value {
				n, ok := nginxTime(parts[0], false)
				valid = ok
				if key != "keepalive_timeout" {
					valid = valid && n > 0 && len(parts) == 1
				} else if len(parts) == 2 {
					_, ok = nginxTime(parts[1], true)
					valid = valid && ok
				}
			}
		}
		if !valid {
			return fmt.Errorf("OpenResty 参数 %s 的值无效", key)
		}
	}
	return nil
}
