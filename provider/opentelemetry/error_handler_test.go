package otel

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorHandlerHandle(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		expected map[string]any
	}{
		{
			name: "logs export failure at error level",
			err:  errors.New("failed to upload metrics: context deadline exceeded"),
			expected: map[string]any{
				"level": "ERROR",
				"msg":   "opentelemetry error",
				"error": "failed to upload metrics: context deadline exceeded",
			},
		},
		{
			name:     "ignores nil error",
			err:      nil,
			expected: nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer

			logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
				ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
					if a.Key == slog.TimeKey {
						return slog.Attr{}
					}

					return a
				},
			}))

			newErrorHandler(func() *slog.Logger { return logger }).Handle(c.err)

			if c.expected == nil {
				assert.Empty(t, buf.String())

				return
			}

			var got map[string]any
			require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
			assert.Equal(t, c.expected, got)
		})
	}
}

func TestErrorHandlerFollowsDefaultLogger(t *testing.T) {
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })

	h := newErrorHandler(slog.Default)

	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))

	h.Handle(errors.New("traces export: boom"))

	assert.Contains(t, buf.String(), `"level":"ERROR"`)
	assert.Contains(t, buf.String(), `"error":"traces export: boom"`)
}
