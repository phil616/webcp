package domain

import "testing"

func TestOpenRestyPerformanceValidation(t *testing.T) {
	for _, test := range []struct {
		key, value string
		valid      bool
	}{
		{"keepalive_timeout", "1m30s 90", true}, {"proxy_read_timeout", "250ms", true}, {"keepalive_timeout", "0", true}, {"client_max_body_size", "0", true}, {"client_max_body_size", "32M", true},
		{"gzip_comp_level", "9", true}, {"gzip", "on", true}, {"proxy_read_timeout", "1s1m", false}, {"keepalive_timeout", "1ms 1ms", false}, {"proxy_connect_timeout", "0", false},
		{"proxy_read_timeout", "999999999999999999999d", false}, {"gzip", "on; include /tmp/a", false}, {"client_header_buffer_size", "0", false}, {"gzip_comp_level", "10", false}, {"gzip_types", "text/html", false},
	} {
		t.Run(test.key+test.value, func(t *testing.T) {
			err := ValidateOpenResty(&OpenRestyConfig{HTTP: map[string]string{test.key: test.value}})
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v: %v", test.valid, err)
			}
		})
	}
}
func TestOpenRestyDefaultsAndPageCodes(t *testing.T) {
	state := Default()
	if err := Validate(state); err != nil {
		t.Fatal(err)
	}
	if EffectiveOpenResty(state.OpenResty).HTTP["gzip"] != "off" {
		t.Fatal("legacy gzip default changed")
	}
	config := EffectiveOpenResty(&OpenRestyConfig{HTTP: map[string]string{"gzip": "on"}})
	if config.HTTP["client_max_body_size"] != "1m" {
		t.Fatal("partial configuration lost defaults")
	}
	if err := ValidatePages(map[string]string{"495": "invalid certificate", "496": "missing certificate"}); err != nil {
		t.Fatal(err)
	}
}
