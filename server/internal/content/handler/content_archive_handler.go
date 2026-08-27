package handler

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/findardi/rakda/server/internal/content/service"
	"github.com/findardi/rakda/server/internal/platform/response"
	"github.com/go-chi/chi/v5"
)

func (h *ContentHandler) CreateArchive(w http.ResponseWriter, r *http.Request) {
	wID := chi.URLParam(r, "workspaceID")

	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	res, err := h.svc.CreateArchive(r.Context(), wID, actor)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrContentForbidden):
			response.Error(w, http.StatusForbidden, err.Error(), nil)
		case errors.Is(err, service.ErrArchiveNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		case errors.Is(err, service.ErrArchiveAlreadyQueued):
			response.Error(w, http.StatusConflict, err.Error(), nil)
		case errors.Is(err, service.ErrArchiveBusy):
			w.Header().Set("Retry-After", "60")
			response.Error(w, http.StatusTooManyRequests, err.Error(), nil)
		default:
			log.Printf("create archive internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusAccepted, "archive queued", res)
}

func (h *ContentHandler) ListArchives(w http.ResponseWriter, r *http.Request) {
	wID := chi.URLParam(r, "workspaceID")

	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	res, err := h.svc.ListArchives(r.Context(), wID, actor)
	if err != nil {
		if errors.Is(err, service.ErrContentForbidden) {
			response.Error(w, http.StatusForbidden, err.Error(), nil)
			return
		}
		log.Printf("list archives internal error: %v", err)
		response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		return
	}

	response.Success(w, http.StatusOK, "list archives success", res)
}

func (h *ContentHandler) DeleteArchive(w http.ResponseWriter, r *http.Request) {
	wID := chi.URLParam(r, "workspaceID")
	aID := chi.URLParam(r, "archiveID")

	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	if err := h.svc.DeleteArchive(r.Context(), wID, aID, actor); err != nil {
		switch {
		case errors.Is(err, service.ErrContentForbidden):
			response.Error(w, http.StatusForbidden, err.Error(), nil)
		case errors.Is(err, service.ErrArchiveNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		default:
			log.Printf("delete archive internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "delete archive success", nil)
}

func (h *ContentHandler) DownloadArchive(w http.ResponseWriter, r *http.Request) {
	wID := chi.URLParam(r, "workspaceID")
	aID := chi.URLParam(r, "archiveID")

	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	obj, err := h.svc.GetArchiveObject(r.Context(), wID, aID, actor)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrContentForbidden):
			response.Error(w, http.StatusForbidden, err.Error(), nil)
		case errors.Is(err, service.ErrArchiveNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		case errors.Is(err, service.ErrArchiveNotReady):
			response.Error(w, http.StatusConflict, err.Error(), nil)
		default:
			log.Printf("download archive internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	offset, length, partial, ok := parseByteRange(r.Header.Get("Range"), obj.Size)
	if !ok {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", obj.Size))
		response.Error(w, http.StatusRequestedRangeNotSatisfiable, "invalid range", nil)
		return
	}

	rc, err := h.svc.OpenArchiveRange(r.Context(), obj.Key, offset, length)
	if err != nil {
		log.Printf("download archive open range: %v", err)
		response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		return
	}
	defer rc.Close()

	// Nama berkas selalu {slug}-arsip-{tanggal}.zip: slug sudah dibatasi
	// [a-z0-9-] di service workspace, jadi tidak ada yang perlu di-escape.
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+obj.FileName+`"`)
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Length", strconv.FormatInt(length, 10))

	if partial {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, offset+length-1, obj.Size))
		w.WriteHeader(http.StatusPartialContent)
	}

	if _, err := io.Copy(w, rc); err != nil {
		log.Printf("download archive aborted mid-stream: %v", err)
	}
}

// parseByteRange menangani satu rentang byte tunggal — bentuk yang dipakai
// peramban saat melanjutkan unduhan. Rentang jamak sengaja tidak didukung:
// jawabannya multipart/byteranges, dan tidak ada klien di sini yang memintanya.
func parseByteRange(header string, size int64) (offset, length int64, partial, ok bool) {
	header = strings.TrimSpace(header)
	if header == "" {
		return 0, size, false, true
	}

	spec, found := strings.CutPrefix(header, "bytes=")
	if !found || strings.Contains(spec, ",") {
		return 0, 0, false, false
	}

	start, end, found := strings.Cut(spec, "-")
	if !found {
		return 0, 0, false, false
	}

	start = strings.TrimSpace(start)
	end = strings.TrimSpace(end)

	if start == "" {
		suffix, err := strconv.ParseInt(end, 10, 64)
		if err != nil || suffix <= 0 {
			return 0, 0, false, false
		}
		if suffix > size {
			suffix = size
		}
		return size - suffix, suffix, true, true
	}

	offset, err := strconv.ParseInt(start, 10, 64)
	if err != nil || offset < 0 || offset >= size {
		return 0, 0, false, false
	}

	last := size - 1
	if end != "" {
		parsed, err := strconv.ParseInt(end, 10, 64)
		if err != nil || parsed < offset {
			return 0, 0, false, false
		}
		if parsed < last {
			last = parsed
		}
	}

	return offset, last - offset + 1, true, true
}
