package fabric

import "go.opentelemetry.io/otel"

// Meter can be a global/package variable.
var meter = otel.GetMeterProvider().Meter("fabric")
