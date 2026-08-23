package handler

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/findardi/rakda/server/internal/platform/middleware"
	"github.com/findardi/rakda/server/internal/platform/response"
	"github.com/findardi/rakda/server/internal/platform/validation"
	"github.com/findardi/rakda/server/internal/workspace/dto"
	"github.com/findardi/rakda/server/internal/workspace/service"
	"github.com/go-chi/chi/v5"
)

const (
	MaxBodyBytes = 1 << 20
)

type WorkspaceHandler struct {
	svc *service.WorkspaceService
}

func NewWorkspaceHandler(svc *service.WorkspaceService) *WorkspaceHandler {
	return &WorkspaceHandler{
		svc: svc,
	}
}

func (h *WorkspaceHandler) Create(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	var req dto.WorkspaceCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid body request", nil)
		return
	}

	if errs := validation.Validate(&req); errs != nil {
		response.Error(w, http.StatusBadRequest, "validation failed", errs)
		return
	}

	req.OwnerID = claims.ID

	res, err := h.svc.CreateWorkspace(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrWorkspaceNameTaken), errors.Is(err, service.ErrWorkspaceNameInvalid), errors.Is(err, service.ErrWorkspaceExceedLimits):
			response.Error(w, http.StatusBadRequest, err.Error(), nil)
		default:
			log.Printf("create workspace internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusCreated, "create workspace success", res)
}

func (h *WorkspaceHandler) GetWorkspaces(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	res, err := h.svc.GetWorkspaces(r.Context(), claims.ID)
	if err != nil {
		log.Printf("get workspaces internal error: %v", err)
		response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		return
	}

	response.Success(w, http.StatusOK, "get workspaces success", res)
}

func (h *WorkspaceHandler) GetWorkspace(w http.ResponseWriter, r *http.Request) {
	// ownership already enforced by RequireOwner middleware
	id := chi.URLParam(r, "workspaceID")

	res, err := h.svc.GetWorkspace(r.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrWorkspaceNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		default:
			log.Printf("get workspace internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "get workspace success", res)
}

func (h *WorkspaceHandler) UpdateStatusWorkspace(w http.ResponseWriter, r *http.Request) {
	var req dto.WorkspaceUpdateStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid body request", nil)
		return
	}

	req.ID = chi.URLParam(r, "workspaceID")

	if errs := validation.Validate(&req); errs != nil {
		response.Error(w, http.StatusBadRequest, "validation failed", errs)
		return
	}

	if err := h.svc.UpdateStatusWorkspace(r.Context(), req); err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidStatus):
			response.Error(w, http.StatusBadRequest, err.Error(), nil)
		case errors.Is(err, service.ErrWorkspaceNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		default:
			log.Printf("update status internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "success update status", nil)
}

func (h *WorkspaceHandler) UpdateWorkspace(w http.ResponseWriter, r *http.Request) {
	var req dto.WorkspaceUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid body request", nil)
		return
	}

	req.ID = chi.URLParam(r, "workspaceID")

	if errs := validation.Validate(&req); errs != nil {
		response.Error(w, http.StatusBadRequest, "validation failed", errs)
		return
	}

	res, err := h.svc.UpdateWorkspace(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrWorkspaceNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		case errors.Is(err, service.ErrWorkspaceNameTaken), errors.Is(err, service.ErrWorkspaceNameInvalid):
			response.Error(w, http.StatusBadRequest, err.Error(), nil)
		default:
			log.Printf("update workspace internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "success update workspace", res)
}

func (h *WorkspaceHandler) DeleteWorkspace(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "workspaceID")

	if err := h.svc.DeleteWorkspace(r.Context(), id); err != nil {
		switch {
		case errors.Is(err, service.ErrWorkspaceNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		default:
			log.Printf("delete workspace internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "success delete workspace", nil)
}
