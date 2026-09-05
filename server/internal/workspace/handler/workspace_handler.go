package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/findardi/rakda/server/internal/platform/middleware"
	"github.com/findardi/rakda/server/internal/platform/permission"
	"github.com/findardi/rakda/server/internal/platform/response"
	"github.com/findardi/rakda/server/internal/platform/validation"
	"github.com/findardi/rakda/server/internal/workspace/dto"
	"github.com/findardi/rakda/server/internal/workspace/service"
	"github.com/go-chi/chi/v5"
)

const (
	MaxBodyBytes = 1 << 20

	// A logo upload may exceed the picture limit by the multipart framing only;
	// the picture limit itself is enforced by the service, which answers 413
	// before this outer bound is reached.
	maxLogoBodyBytes = service.MaxLogoUploadBytes + 64<<10

	logoCacheControl = "private, max-age=86400"
)

// ownerActor is the caller of an owner-only mutation. The owner guard has
// already run, so the role is a fact, not a claim.
func ownerActor(r *http.Request) (service.Actor, bool) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		return service.Actor{}, false
	}
	return service.Actor{UserID: claims.ID, Name: claims.Username, Role: permission.RoleOwner}, true
}

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

	req, ok := validation.Bind[dto.WorkspaceCreateRequest](w, r)
	if !ok {
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
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	id := chi.URLParam(r, "workspaceID")

	res, err := h.svc.GetWorkspace(r.Context(), id, claims.ID)
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

func (h *WorkspaceHandler) GetWorkspaceSummary(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	id := chi.URLParam(r, "workspaceID")

	res, err := h.svc.GetWorkspaceSummary(r.Context(), id, claims.ID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrWorkspaceNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		case errors.Is(err, service.ErrWorkspaceForbidden):
			response.Error(w, http.StatusForbidden, err.Error(), nil)
		default:
			log.Printf("get workspace summary internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "get workspace summary success", res)
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

	actor, ok := ownerActor(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	res, err := h.svc.UpdateStatusWorkspace(r.Context(), req, actor)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidStatus):
			response.Error(w, http.StatusBadRequest, err.Error(), nil)
		case errors.Is(err, service.ErrWorkspaceNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		case errors.Is(err, service.ErrStatusTransition):
			response.Error(w, http.StatusConflict, err.Error(), nil)
		default:
			log.Printf("update status internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "success update status", res)
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

	actor, ok := ownerActor(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	res, err := h.svc.UpdateWorkspace(r.Context(), req, actor)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrWorkspaceNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		case errors.Is(err, service.ErrWorkspaceArchived):
			response.Error(w, http.StatusLocked, err.Error(), nil)
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
		case errors.Is(err, service.ErrWorkspaceArchived):
			response.Error(w, http.StatusLocked, err.Error(), nil)
		default:
			log.Printf("delete workspace internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "success delete workspace", nil)
}

func (h *WorkspaceHandler) GetHeroPresets(w http.ResponseWriter, r *http.Request) {
	response.Success(w, http.StatusOK, "get hero presets success", h.svc.HeroPresets())
}

// SetLogo takes multipart/form-data with the picture in field "file". The part
// is streamed straight into the service, which reads no more than the limit
// and re-encodes what it accepts.
func (h *WorkspaceHandler) SetLogo(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxLogoBodyBytes)

	actor, ok := ownerActor(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	mr, err := r.MultipartReader()
	if err != nil {
		response.Error(w, http.StatusBadRequest, "expected multipart/form-data", nil)
		return
	}
	var file *multipart.Part
	for file == nil {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			response.Error(w, http.StatusBadRequest, "invalid multipart body", nil)
			return
		}
		if part.FormName() == "file" {
			file = part
		}
	}
	if file == nil {
		response.Error(w, http.StatusBadRequest, "field \"file\" is required", nil)
		return
	}
	defer file.Close()

	res, err := h.svc.SetLogo(r.Context(), chi.URLParam(r, "workspaceID"), actor, file)
	if err != nil {
		h.brandingError(w, err, "set logo")
		return
	}

	response.Success(w, http.StatusOK, "set logo success", res)
}

func (h *WorkspaceHandler) RemoveLogo(w http.ResponseWriter, r *http.Request) {
	actor, ok := ownerActor(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	res, err := h.svc.RemoveLogo(r.Context(), chi.URLParam(r, "workspaceID"), actor)
	if err != nil {
		h.brandingError(w, err, "remove logo")
		return
	}

	response.Success(w, http.StatusOK, "remove logo success", res)
}

// GetLogo streams the room's logo to a member. The version token in the ETag
// changes with every upload, so a day of private caching can never go stale.
func (h *WorkspaceHandler) GetLogo(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	known := strings.Trim(r.Header.Get("If-None-Match"), `"`)
	rc, version, err := h.svc.OpenLogo(r.Context(), chi.URLParam(r, "workspaceID"), claims.ID, known)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrWorkspaceNotFound), errors.Is(err, service.ErrLogoNotFound):
			response.Error(w, http.StatusNotFound, "not found", nil)
		default:
			log.Printf("get logo internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	w.Header().Set("ETag", `"`+version+`"`)
	w.Header().Set("Cache-Control", logoCacheControl)
	if rc == nil {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, rc); err != nil {
		log.Printf("get logo: stream: %v", err)
	}
}

func (h *WorkspaceHandler) SetHeroPreset(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	var req dto.WorkspaceHeroRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid body request", nil)
		return
	}
	req.ID = chi.URLParam(r, "workspaceID")

	actor, ok := ownerActor(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	res, err := h.svc.SetHeroPreset(r.Context(), req, actor)
	if err != nil {
		h.brandingError(w, err, "set hero preset")
		return
	}

	response.Success(w, http.StatusOK, "set hero preset success", res)
}

// brandingError maps the errors the three branding mutations share.
func (h *WorkspaceHandler) brandingError(w http.ResponseWriter, err error, op string) {
	switch {
	case errors.Is(err, service.ErrWorkspaceNotFound):
		response.Error(w, http.StatusNotFound, err.Error(), nil)
	case errors.Is(err, service.ErrWorkspaceArchived):
		response.Error(w, http.StatusLocked, err.Error(), nil)
	case errors.Is(err, service.ErrLogoTooLarge):
		response.Error(w, http.StatusRequestEntityTooLarge, err.Error(), nil)
	case errors.Is(err, service.ErrLogoUnsupported):
		response.Error(w, http.StatusUnsupportedMediaType, err.Error(), nil)
	case errors.Is(err, service.ErrLogoInvalid), errors.Is(err, service.ErrHeroPresetInvalid):
		response.Error(w, http.StatusBadRequest, err.Error(), nil)
	default:
		log.Printf("%s internal error: %v", op, err)
		response.Error(w, http.StatusInternalServerError, "internal server error", nil)
	}
}
