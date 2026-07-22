package version

import "runtime"

// Set at link time via -ldflags, e.g.:
//
//	-X github.com/KB-perByte/hiveshare/internal/version.Commit=$(git rev-parse --short HEAD)
//	-X github.com/KB-perByte/hiveshare/internal/version.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)
var (
	Commit    = "unknown"
	BuildTime = "unknown"
)

// Info returns a map suitable for JSON health / status payloads.
func Info() map[string]string {
	return map[string]string{
		"commit":     Commit,
		"build_time": BuildTime,
		"go":         runtime.Version(),
	}
}
