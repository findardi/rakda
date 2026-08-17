package handler

import (
	"errors"
	"log"
	"net/http"

	"github.com/findardi/Riksa-App/server/internal/invitation/service"
	"github.com/findardi/Riksa-App/server/internal/platform/middleware"
	"github.com/findardi/Riksa-App/server/internal/platform/response"
	"github.com/go-chi/chi/v5"
)

const (
	MaxBodyBytes = 1 << 20
)

type InvitationHandler struct {
	svc *service.InvitationService
}

func NewInvitationHandler(svc *service.InvitationService) *InvitationHandler {
	return &InvitationHandler{
		svc: svc,
	}
}

func (h *InvitationHandler) GetListInvitations(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	res, err := h.svc.GetListInvitations(r.Context(), claims.ID)
	if err != nil {
		log.Printf("register internal error: %v", err)
		response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		return
	}

	response.Success(w, http.StatusOK, "get list invitation success", res)
}

func (h *InvitationHandler) AcceptInvitation(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	invID := chi.URLParam(r, "invitationID")

	if err := h.svc.AcceptInvitation(r.Context(), invID, service.Actor{UserID: claims.ID, Name: claims.Username, Email: claims.Email}); err != nil {
		switch {
		case errors.Is(err, service.ErrInvitationNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		case errors.Is(err, service.ErrInvitationExpired), errors.Is(err, service.ErrInvitationNotPending):
			response.Error(w, http.StatusBadRequest, err.Error(), nil)
		case errors.Is(err, service.ErrInvitationForbidden):
			response.Error(w, http.StatusForbidden, err.Error(), nil)
		default:
			log.Printf("register internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "accept invitation success", nil)
}

func (h *InvitationHandler) RejectInvitation(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	invID := chi.URLParam(r, "invitationID")

	if err := h.svc.RejectInvitation(r.Context(), invID, service.Actor{UserID: claims.ID, Name: claims.Username, Email: claims.Email}); err != nil {
		switch {
		case errors.Is(err, service.ErrInvitationNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		case errors.Is(err, service.ErrInvitationNotPending):
			response.Error(w, http.StatusBadRequest, err.Error(), nil)
		case errors.Is(err, service.ErrInvitationForbidden):
			response.Error(w, http.StatusForbidden, err.Error(), nil)
		default:
			log.Printf("register internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "reject invitation success", nil)
}
