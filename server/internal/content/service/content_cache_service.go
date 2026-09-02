package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"

	"github.com/findardi/rakda/server/internal/platform/diskcache"
)

func (s *ContentService) renditionGet(ctx context.Context, key string) (io.ReadCloser, error) {
	if r, ok := s.renditions.Open(key); ok {
		return r, nil
	}

	src, err := s.store.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	w, err := s.renditions.Create(key)
	if err != nil {
		if !errors.Is(err, diskcache.ErrDisabled) {
			log.Printf("diskcache: rendition %s not cached: %v", key, err)
		}

		return src, nil
	}

	return &readThrough{key: key, src: src, w: w}, nil
}

type readThrough struct {
	key string
	src io.ReadCloser
	w   *diskcache.Writer
	eof bool
}

func (r *readThrough) Read(p []byte) (int, error) {
	n, err := r.src.Read(p)

	if n > 0 && r.w != nil {
		if _, werr := r.w.Write(p[:n]); werr != nil {
			log.Printf("diskcache: rendition %s read-through aborted: %v", r.key, werr)
			r.w.Abort()
			r.w = nil
		}
	}

	if errors.Is(err, io.EOF) {
		r.eof = true
	}

	return n, err
}

func (r *readThrough) Close() error {
	err := r.src.Close()

	if r.w == nil {
		return err
	}

	if !r.eof {
		r.w.Abort()
		return err
	}

	if cerr := r.w.Close(); cerr != nil {
		log.Printf("diskcache: rendition %s not committed: %v", r.key, cerr)
	}

	return err
}

func (s *ContentService) renditionPut(key string, r io.Reader) {
	if err := s.renditions.Put(key, r); err != nil && !errors.Is(err, diskcache.ErrDisabled) {
		log.Printf("diskcache: rendition %s not cached: %v", key, err)
	}
}

func (s *ContentService) cachedPage(ctx context.Context, key string) ([]byte, bool) {
	var rc io.ReadCloser

	if s.pages != nil {
		r, ok := s.pages.Open(key)
		if !ok {
			return nil, false
		}
		rc = r
	} else {
		r, err := s.store.Get(ctx, key)
		if err != nil {
			return nil, false
		}
		rc = r
	}
	defer rc.Close()

	b, err := io.ReadAll(rc)
	if err != nil || len(b) == 0 {
		return nil, false
	}

	return b, true
}

func (s *ContentService) storePage(ctx context.Context, key string, img []byte) error {
	if s.pages == nil {
		return s.store.Put(ctx, key, bytes.NewReader(img), int64(len(img)), "image/png")
	}

	if err := s.pages.Put(key, bytes.NewReader(img)); err != nil {
		log.Printf("diskcache: page %s not cached: %v", key, err)
	}

	return nil
}
