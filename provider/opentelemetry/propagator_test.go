package otel

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPropagatorFactory(t *testing.T) {
	propagator := PropagatorFactory()
	assert.NotNil(t, propagator)

	// Verify the propagator returns expected fields
	fields := propagator.Fields()
	assert.NotEmpty(t, fields)

	// Check that traceparent is in the fields (from TraceContext)
	hasTraceparent := false
	for _, field := range fields {
		if field == "traceparent" {
			hasTraceparent = true
			break
		}
	}
	assert.True(t, hasTraceparent, "propagator should contain traceparent field")
}
