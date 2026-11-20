//go:build !windows

package tray

import "github.com/babo96/tradepost/companion/internal/app"

// Start is a no-op outside Windows; Linux uses the console/systemd service.
func Start(*app.App, string, func()) {}

// Stop is a no-op outside Windows.
func Stop() {}
