package convert

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/findardi/rakda/server/internal/platform/config"
)

type GotenbergConverter struct {
	baseURL string
	client  *http.Client
}

func NewGotenberg(cfg config.ViewerConfig) *GotenbergConverter {
	return &GotenbergConverter{
		baseURL: strings.TrimSuffix(cfg.GotenbergURL, "/"),
		client: &http.Client{
			Timeout: cfg.ConvertTimeout,
		},
	}
}

func (g *GotenbergConverter) ToPDF(ctx context.Context, src io.Reader, filename string) (io.ReadCloser, error) {
	name := filepath.Base(filename)
	if !Supported(name) {
		return nil, ErrUnsupportedFile
	}

	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)

	go func() {
		part, err := mw.CreateFormFile("files", name)
		if err != nil {
			pw.CloseWithError(err)
			return
		}

		if _, err := io.Copy(part, src); err != nil {
			pw.CloseWithError(err)
			return
		}

		if err := mw.Close(); err != nil {
			pw.CloseWithError(err)
			return
		}

		pw.Close()
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/forms/libreoffice/convert", pr)
	if err != nil {
		pr.CloseWithError(err)
		return nil, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gotenberg call: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("%w: status %d: %s", ErrConversionFailed, resp.StatusCode, bytes.TrimSpace(msg))
	}

	return resp.Body, nil
}
