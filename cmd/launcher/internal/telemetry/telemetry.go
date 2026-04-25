// Package telemetry contains the internal shared logic for initializing telemetry in launchers.
package telemetry

import (
	"context"

	"github.com/nickqiaoo/tadk/cmd/launcher"
	"github.com/nickqiaoo/tadk/telemetry"
)

// InitAndSetGlobalOtelProviders initializes telemetry and sets the global OTel providers.
func InitAndSetGlobalOtelProviders(ctx context.Context, config *launcher.Config, otelToCloud bool) (*telemetry.Providers, error) {
	opts := append(config.TelemetryOptions, telemetry.WithOtelToCloud(otelToCloud))
	telemetryProviders, err := telemetry.New(ctx, opts...)
	if err != nil {
		return nil, err
	}
	telemetryProviders.SetGlobalOtelProviders()
	return telemetryProviders, nil
}
