package otel

import "github.com/euskadi31/wire"

var OTelSet = wire.NewSet(OTelConfigSet, OTelPropagatorSet, OTelResourceSet, OTelTraceSet, OTelMetricSet, OTelLogSet)
