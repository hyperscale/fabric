package otel

import (
	"github.com/euskadi31/wire"
	"go.opentelemetry.io/otel/propagation"
)

var OTelPropagatorSet = wire.NewSet(PropagatorFactory)

func PropagatorFactory() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}
