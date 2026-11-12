package openresty

import "testing"

func TestUpstreamParserKeepsLuaTextOutOfGlobalDirectives(t *testing.T) {
	config, err := globalNginxDirectives(`user nobody nogroup;
 load_module /usr/lib/nginx/modules/ngx_stream_module.so;
 events {}
 http { server { listen 80; access_by_lua_block {
  local text = [[
user root;
load_module /tmp/not-a-real-directive.so;
  ]]
  ngx.say(text)
 } } }
 `)
	if err != nil {
		t.Fatal(err)
	}
	var users, modules int
	for _, directive := range config.GetDirectives() {
		switch directive.GetName() {
		case "user":
			users++
			if directive.GetParameters()[0] != "nobody" {
				t.Fatal("Lua text became a global user")
			}
		case "load_module":
			modules++
		}
	}
	if users != 1 || modules != 1 {
		t.Fatalf("unexpected global directive count: users=%d modules=%d", users, modules)
	}
}
