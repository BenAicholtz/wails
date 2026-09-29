//go:build linux && cgo && !android && !server

package application

import (
	"os"
	"path/filepath"
	"strings"
)

// isAMDGPU checks if an AMD GPU (discrete or integrated) is present by reading
// the DRM device vendor ID (0x1002 is AMD/ATI) rather than relying on a
// specific kernel module name, so it catches amdgpu and the legacy radeon
// driver alike.
func isAMDGPU() bool {
	entries, err := filepath.Glob("/sys/class/drm/card*/device/vendor")
	if err != nil {
		return false
	}
	for _, path := range entries {
		if vendor, err := os.ReadFile(path); err == nil && strings.TrimSpace(string(vendor)) == "0x1002" {
			return true
		}
	}
	return false
}

// appendMesaDebugFlag adds flag to the comma-separated Mesa debug env var
// name (e.g. AMD_DEBUG, RADV_DEBUG), preserving any value the user already
// set rather than overwriting it — these vars are flag lists, not booleans.
func appendMesaDebugFlag(name, flag string) {
	existing := os.Getenv(name)
	if existing == "" {
		_ = os.Setenv(name, flag)
		return
	}
	for _, f := range strings.Split(existing, ",") {
		if f == flag {
			return
		}
	}
	_ = os.Setenv(name, existing+","+flag)
}
