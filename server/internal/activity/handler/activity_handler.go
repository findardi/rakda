package handler

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/findardi/Riksa-App/server/internal/activity/dto"
	"github.com/findardi/Riksa-App/server/internal/activity/service"
	"github.com/findardi/Riksa-App/server/internal/platform/middleware"
	"github.com/findardi/Riksa-App/server/internal/platform/response"
	"github.com/go-chi/chi/v5"
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
