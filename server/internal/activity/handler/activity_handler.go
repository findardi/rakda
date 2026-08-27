package handler

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/findardi/rakda/server/internal/activity/dto"
	"github.com/findardi/rakda/server/internal/activity/service"
	"github.com/findardi/rakda/server/internal/platform/middleware"
	"github.com/findardi/rakda/server/internal/platform/response"
	"github.com/findardi/rakda/server/internal/platform/validation"
	"github.com/go-chi/chi/v5"
)

const (
	MaxBodyBytes    = 1 << 20
	exportBatchSize = 100
)

type ActivityHandler struct {
	svc *service.ActivityService
}

func NewActivityHandler(svc *service.ActivityService) *ActivityHandler {
	return &ActivityHandler{
		svc: svc,
	}
}

func (h *ActivityHandler) ListActivity(w http.ResponseWriter, r *http.Request) {
	wID := chi.URLParam(r, "workspaceID")

	ms, ok := middleware.MembershipFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		return
	}

	q := r.URL.Query()

	limit := 0
	if v := q.Get("limit"); v != "" {
		limit, _ = strconv.Atoi(v)
	}

	req := dto.ListActivityRequest{
		WorkspaceID: wID,
		Limit:       limit,
		Cursor:      q.Get("cursor"),
		From:        q.Get("from"),
		To:          q.Get("to"),
		ActorID:     q.Get("actor_id"),
		Action:      q.Get("action"),
	}

	res, err := h.svc.ListActivity(r.Context(), req, ms.Role)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrActivityForbidden):
			response.Error(w, http.StatusForbidden, err.Error(), nil)
		case errors.Is(err, service.ErrInvalidCursor), errors.Is(err, service.ErrInvalidFilter):
			response.Error(w, http.StatusBadRequest, err.Error(), nil)
		default:
			log.Printf("list activity internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "list activity success", res)
}

func (h *ActivityHandler) RecordDurations(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	ms, ok := middleware.MembershipFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		return
	}

	var req dto.RecordDurationsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid body request", nil)
		return
	}

	if errs := validation.Validate(&req); errs != nil {
		response.Error(w, http.StatusBadRequest, "validation failed", errs)
		return
	}

	req.WorkspaceID = chi.URLParam(r, "workspaceID")
	req.DocumentID = chi.URLParam(r, "documentID")

	if err := h.svc.RecordPageDurations(r.Context(), req, claims.ID, claims.Email, ms.Role); err != nil {
		switch {
		case errors.Is(err, service.ErrDocumentNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		case errors.Is(err, service.ErrInvalidFilter):
			response.Error(w, http.StatusBadRequest, err.Error(), nil)
		default:
			log.Printf("record durations internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "record durations success", nil)
}

func (h *ActivityHandler) GetDocumentReaders(w http.ResponseWriter, r *http.Request) {
	wID := chi.URLParam(r, "workspaceID")
	dID := chi.URLParam(r, "documentID")

	ms, ok := middleware.MembershipFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		return
	}

	res, err := h.svc.GetDocumentReaders(r.Context(), wID, dID, ms.Role)
	if err != nil {
		h.engagementError(w, err, "get document readers")
		return
	}

	response.Success(w, http.StatusOK, "get document readers success", res)
}

func (h *ActivityHandler) GetReaderPages(w http.ResponseWriter, r *http.Request) {
	wID := chi.URLParam(r, "workspaceID")
	dID := chi.URLParam(r, "documentID")
	aID := chi.URLParam(r, "actorID")

	ms, ok := middleware.MembershipFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		return
	}

	res, err := h.svc.GetReaderPages(r.Context(), wID, dID, aID, ms.Role)
	if err != nil {
		h.engagementError(w, err, "get reader pages")
		return
	}

	response.Success(w, http.StatusOK, "get reader pages success", res)
}

func (h *ActivityHandler) engagementError(w http.ResponseWriter, err error, op string) {
	switch {
	case errors.Is(err, service.ErrActivityForbidden):
		response.Error(w, http.StatusForbidden, err.Error(), nil)
	case errors.Is(err, service.ErrDocumentNotFound):
		response.Error(w, http.StatusNotFound, err.Error(), nil)
	case errors.Is(err, service.ErrInvalidFilter):
		response.Error(w, http.StatusBadRequest, err.Error(), nil)
	default:
		log.Printf("%s internal error: %v", op, err)
		response.Error(w, http.StatusInternalServerError, "internal server error", nil)
	}
}

func (h *ActivityHandler) ExportActivity(w http.ResponseWriter, r *http.Request) {
	wID := chi.URLParam(r, "workspaceID")

	ms, ok := middleware.MembershipFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		return
	}

	q := r.URL.Query()
	if f := q.Get("format"); f != "" && f != "csv" {
		response.Error(w, http.StatusBadRequest, "unsupported format", nil)
		return
	}

	req := dto.ListActivityRequest{
		WorkspaceID: wID,
		Limit:       exportBatchSize,
		From:        q.Get("from"),
		To:          q.Get("to"),
		ActorID:     q.Get("actor_id"),
		Action:      q.Get("action"),
	}

	if _, err := h.svc.ListActivity(r.Context(), req, ms.Role); err != nil {
		switch {
		case errors.Is(err, service.ErrActivityForbidden):
			response.Error(w, http.StatusForbidden, err.Error(), nil)
		case errors.Is(err, service.ErrInvalidFilter):
			response.Error(w, http.StatusBadRequest, err.Error(), nil)
		default:
			log.Printf("export activity internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="activity-log.csv"`)
	if _, err := w.Write([]byte("\xEF\xBB\xBF")); err != nil {
		return
	}

	if err := h.svc.WriteActivityCSV(r.Context(), w, req, ms.Role); err != nil {
		log.Printf("export activity aborted mid-stream: %v", err)
	}
}

func (h *ActivityHandler) ExportDocumentEngagement(w http.ResponseWriter, r *http.Request) {
	wID := chi.URLParam(r, "workspaceID")
	dID := chi.URLParam(r, "documentID")

	ms, ok := middleware.MembershipFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		return
	}

	if f := r.URL.Query().Get("format"); f != "" && f != "csv" {
		response.Error(w, http.StatusBadRequest, "unsupported format", nil)
		return
	}

	data, err := h.svc.GetEngagementBreakdown(r.Context(), wID, dID, ms.Role)
	if err != nil {
		h.engagementError(w, err, "export engagement")
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+exportFilename("engagement", data.DocumentName)+`"`)
	if _, err := w.Write([]byte("\xEF\xBB\xBF")); err != nil {
		return
	}

	cw := csv.NewWriter(w)
	cw.Write([]string{"actor_id", "actor_name", "actor_email", "page_no", "opens", "read_ms"})

	for _, row := range data.Rows {
		cw.Write([]string{
			row.ActorID,
			row.ActorName,
			row.ActorEmail,
			strconv.FormatInt(int64(row.PageNo), 10),
			strconv.FormatInt(row.Opens, 10),
			strconv.FormatInt(row.ReadMs, 10),
		})
	}

	cw.Flush()
	if err := cw.Error(); err != nil {
		log.Printf("export engagement write aborted: %v", err)
	}
}

func exportFilename(prefix, name string) string {
	var b strings.Builder
	for _, c := range name {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_', c == '.':
			b.WriteRune(c)
		case c == ' ':
			b.WriteRune('-')
		}
	}

	if b.Len() == 0 {
		return prefix + ".csv"
	}
	return prefix + "-" + b.String() + ".csv"
}
