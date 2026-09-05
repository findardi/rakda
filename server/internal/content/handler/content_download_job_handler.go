package handler

import (
	"errors"
	"fmt"
	"github.com/findardi/rakda/server/internal/platform/middleware"
	"io"
	"log"
	"net/http"
	"strconv"

	"github.com/findardi/rakda/server/internal/content/service"
	"github.com/findardi/rakda/server/internal/platform/response"
	"github.com/go-chi/chi/v5"
)

func (h *ContentHandler) ListDownloadJobs(w http.ResponseWriter, r *http.Request) {
	wID := chi.URLParam(r, "workspaceID")

	actor, ok := middleware.ActorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	jobs, err := h.svc.ListDownloadJobs(r.Context(), wID, actor)
	if err != nil {
		log.Printf("list download jobs internal error: %v", err)
		response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		return
	}

	response.Success(w, http.StatusOK, "list download jobs success", jobs)
}

func (h *ContentHandler) GetDownloadJob(w http.ResponseWriter, r *http.Request) {
	wID := chi.URLParam(r, "workspaceID")
	jID := chi.URLParam(r, "jobID")

	actor, ok := middleware.ActorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	job, err := h.svc.GetDownloadJob(r.Context(), wID, jID, actor)
	if err != nil {
		writeDownloadJobError(w, "get download job", err)
		return
	}

	response.Success(w, http.StatusOK, "get download job success", job)
}

func (h *ContentHandler) DownloadJobArtifact(w http.ResponseWriter, r *http.Request) {
	wID := chi.URLParam(r, "workspaceID")
	jID := chi.URLParam(r, "jobID")

	actor, ok := middleware.ActorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	obj, err := h.svc.GetDownloadJobObject(r.Context(), wID, jID, actor)
	if err != nil {
		writeDownloadJobError(w, "download job artifact", err)
		return
	}

	offset, length, partial, ok := parseByteRange(r.Header.Get("Range"), obj.Size)
	if !ok {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", obj.Size))
		response.Error(w, http.StatusRequestedRangeNotSatisfiable, "invalid range", nil)
		return
	}

	rc, err := h.svc.OpenDownloadJobRange(r.Context(), obj, offset, length)
	if err != nil {
		log.Printf("download job open range: %v", err)
		response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		return
	}
	defer rc.Close()

	if offset == 0 {
		h.svc.RecordDownloadJobDelivery(r.Context(), wID, jID, actor)
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, obj.FileName))
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
	w.Header().Set("Cache-Control", "no-store")

	if partial {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, offset+length-1, obj.Size))
		w.WriteHeader(http.StatusPartialContent)
	}

	if _, err := io.Copy(w, rc); err != nil {
		log.Printf("download job aborted mid-stream: %v", err)
	}
}

func writeDownloadJobError(w http.ResponseWriter, label string, err error) {
	switch {
	case errors.Is(err, service.ErrContentForbidden):
		response.Error(w, http.StatusForbidden, err.Error(), nil)
	case errors.Is(err, service.ErrDownloadJobNotFound), errors.Is(err, service.ErrDocumentNotFound):
		response.Error(w, http.StatusNotFound, err.Error(), nil)
	case errors.Is(err, service.ErrDownloadJobNotReady):
		response.Error(w, http.StatusConflict, err.Error(), nil)
	case errors.Is(err, service.ErrDownloadJobLost):
		response.Error(w, http.StatusGone, err.Error(), nil)
	default:
		log.Printf("%s internal error: %v", label, err)
		response.Error(w, http.StatusInternalServerError, "internal server error", nil)
	}
}
