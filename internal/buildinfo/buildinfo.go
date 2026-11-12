// Package buildinfo exposes non-secret build identity for support and release verification.
package buildinfo

import "runtime"

// Release builds inject these values through scripts/build.py.
var (
	Version   = "development"
	Commit    = "unknown"
	BuildTime = "unknown"
)

type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"buildTime"`
	GoVersion string `json:"goVersion"`
	Platform  string `json:"platform"`
}

func Current() Info {
	return Info{Version: Version, Commit: Commit, BuildTime: BuildTime, GoVersion: runtime.Version(), Platform: runtime.GOOS + "/" + runtime.GOARCH}
}
