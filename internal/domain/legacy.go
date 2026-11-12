package domain

import (
	"encoding/json"
	"fmt"
)

// DecodeExport accepts the previous portable schema and separates HTML assets
// before constructing current controller state. HTML has no field in State or Site.
func DecodeExport(data []byte) (State, map[string]map[string]string, error) {
	var state State
	pages := map[string]map[string]string{}
	if len(data) > MaxStateSize {
		return state, pages, fmt.Errorf("配置快照超过 16 MiB 限制")
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, pages, err
	}
	if state.Schema == 1 {
		var legacy struct {
			Pages map[string]string `json:"pages"`
			Sites []struct {
				ID    string            `json:"id"`
				Pages map[string]string `json:"pages"`
			} `json:"sites"`
		}
		if err := json.Unmarshal(data, &legacy); err != nil {
			return state, pages, err
		}
		if legacy.Pages != nil {
			pages[""] = legacy.Pages
		}
		for _, site := range legacy.Sites {
			if len(site.Pages) > 0 {
				pages[site.ID] = site.Pages
			}
		}
		for id, html := range pages {
			if id != "" && !ValidID(id) {
				return state, pages, fmt.Errorf("无效页面所属网站")
			}
			if err := ValidatePages(html); err != nil {
				return state, pages, err
			}
		}
		state.Schema = SchemaVersion
	}
	NormalizeState(&state)
	return state, pages, Validate(state)
}
