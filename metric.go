package fabric

import "go.opentelemetry.io/otel/metric/global"

// Meter can be a global/package variable.
var meter = global.MeterProvider().Meter("fabric")
