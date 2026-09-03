package convert

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/findardi/rakda/server/internal/platform/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGotenbergToPDFStatusError(t *testing.T) {
	tests := []struct {
		name string
		code int
	}{
		{"bad document", http.StatusBadRequest},
		{"busy", http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.code)
				_, _ = w.Write([]byte("  nope \n"))
			}))
			defer srv.Close()

			g := NewGotenberg(config.ViewerConfig{GotenbergURL: srv.URL, ConvertTimeout: time.Second})
			_, err := g.ToPDF(context.Background(), strings.NewReader("x"), "a.docx")
			require.Error(t, err)

			var se *StatusError
			require.ErrorAs(t, err, &se)
			assert.Equal(t, tt.code, se.Code)
			assert.Equal(t, "nope", se.Body)
			assert.ErrorIs(t, err, ErrConversionFailed)
		})
	}
}

func TestGotenbergToPDFTransportErrorIsNotStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()

	g := NewGotenberg(config.ViewerConfig{GotenbergURL: srv.URL, ConvertTimeout: 20 * time.Millisecond})
	_, err := g.ToPDF(context.Background(), strings.NewReader("x"), "a.docx")
	require.Error(t, err)

	var se *StatusError
	assert.False(t, errors.As(err, &se))
	assert.NotErrorIs(t, err, ErrConversionFailed)
}
