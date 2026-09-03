package render

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPopplerRunReportsContextError(t *testing.T) {
	tests := []struct {
		name    string
		ctx     func() (context.Context, context.CancelFunc)
		timeout time.Duration
		want    error
	}{
		{
			name:    "own deadline",
			ctx:     func() (context.Context, context.CancelFunc) { return context.WithCancel(context.Background()) },
			timeout: 10 * time.Millisecond,
			want:    context.DeadlineExceeded,
		},
		{
			name: "parent cancelled",
			ctx: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx, cancel
			},
			timeout: time.Second,
			want:    context.Canceled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := tt.ctx()
			defer cancel()

			p := &PopplerRenderer{sem: make(chan struct{}, 1), timeout: tt.timeout}
			_, err := p.run(ctx, "sleep", "5")

			require.ErrorIs(t, err, tt.want)
			assert.NotErrorIs(t, err, ErrRenderFailed)
		})
	}
}

func TestPopplerRunKeepsRenderFailedForProcessErrors(t *testing.T) {
	p := &PopplerRenderer{sem: make(chan struct{}, 1), timeout: time.Second}
	_, err := p.run(context.Background(), "sh", "-c", "echo boom >&2; exit 1")

	require.ErrorIs(t, err, ErrRenderFailed)
	assert.Contains(t, err.Error(), "boom")
}
