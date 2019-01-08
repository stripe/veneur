package plugins

import (
	"context"

	"github.com/stripe/veneur/samplers"
)

// Plugin flushes the metrics provided to an arbitrary destination.
// The metrics slice may be shared between plugins, so the plugin may not
// write to it or modify any of its components.
// The name should be a short, lowercase, snake-cased identifier for the plugin.
// When a plugin is registered, the number of metrics flushed successfully and
// the number of errors encountered are automatically reported by veneur, using
// the plugin name.
type Plugin interface {
	Flush(ctx context.Context, metrics []samplers.InterMetric) error
	Name() string
}
