package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/findardi/rakda/server/internal/content/dto"
	"github.com/findardi/rakda/server/internal/content/service"
	"github.com/findardi/rakda/server/internal/platform/middleware"
	"github.com/findardi/rakda/server/internal/platform/response"
	"github.com/findardi/rakda/server/internal/platform/validation"
	"github.com/findardi/rakda/server/internal/platform/watermark"
	"github.com/go-chi/chi/v5"
)

const (
	MaxBodyBytes = 1 << 20
)

type ContentHandler struct {
	svc *service.ContentService
}

func NewContentHandler(svc *service.ContentService) *ContentHandler {
	return &ContentHandler{
		svc: svc,
	}
}

func (h *ContentHandler) CreateFolder(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	var req dto.CreateFolderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid body request", nil)
		return
	}

	if errs := validation.Validate(&req); errs != nil {
		response.Error(w, http.StatusBadRequest, "validation failed", errs)
		return
	}

	req.WorkspaceID = chi.URLParam(r, "workspaceID")
	req.CreatedBy = actor.UserID

	res, err := h.svc.CreateFolder(r.Context(), req, actor)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrParentNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		case errors.Is(err, service.ErrParentCrossWorkspace):
			response.Error(w, http.StatusBadRequest, err.Error(), nil)
		case errors.Is(err, service.ErrFolderNameTaken):
			response.Error(w, http.StatusConflict, err.Error(), nil)
		default:
			log.Printf("create folder internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusCreated, "create folder success", res)
}

func (h *ContentHandler) MoveFolder(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	var req dto.MoveFolderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid body request", nil)
		return
	}

	req.WorkspaceID = chi.URLParam(r, "workspaceID")
	req.FolderID = chi.URLParam(r, "folderID")

	if err := h.svc.MoveFolder(r.Context(), req, actor); err != nil {
		switch {
		case errors.Is(err, service.ErrFolderNotFound), errors.Is(err, service.ErrParentNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		case errors.Is(err, service.ErrMoveDefault):
			response.Error(w, http.StatusForbidden, err.Error(), nil)
		case errors.Is(err, service.ErrParentCrossWorkspace), errors.Is(err, service.ErrCycle), errors.Is(err, service.ErrFolderTreeTooDeep):
			response.Error(w, http.StatusBadRequest, err.Error(), nil)
		case errors.Is(err, service.ErrFolderNameTaken):
			response.Error(w, http.StatusConflict, err.Error(), nil)
		default:
			log.Printf("move folder internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "move folder success", nil)
}

func actorFromRequest(r *http.Request) (service.Actor, bool) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		return service.Actor{}, false
	}

	ms, ok := middleware.MembershipFromContext(r.Context())
	if !ok {
		return service.Actor{}, false
	}

	return service.Actor{
		UserID:     claims.ID,
		Role:       ms.Role,
		Name:       claims.Username,
		Email:      claims.Email,
		RoomStatus: ms.WorkspaceStatus,
	}, true
}

func (h *ContentHandler) GetFoldersTree(w http.ResponseWriter, r *http.Request) {
	wID := chi.URLParam(r, "workspaceID")

	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	res, err := h.svc.GetFoldersTree(r.Context(), wID, actor)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrContentForbidden):
			response.Error(w, http.StatusForbidden, err.Error(), nil)
		default:
			log.Printf("get folders tree internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "get folders tree success", res)
}

func (h *ContentHandler) SearchContent(w http.ResponseWriter, r *http.Request) {
	wID := chi.URLParam(r, "workspaceID")

	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	res, err := h.svc.SearchContent(r.Context(), wID, r.URL.Query().Get("q"), actor)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrContentForbidden):
			response.Error(w, http.StatusForbidden, err.Error(), nil)
		default:
			log.Printf("search content internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "search success", res)
}

func (h *ContentHandler) SearchContentPages(w http.ResponseWriter, r *http.Request) {
	wID := chi.URLParam(r, "workspaceID")

	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	res, err := h.svc.SearchContentPages(r.Context(), wID, r.URL.Query().Get("documentId"), r.URL.Query().Get("q"), actor)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrContentForbidden):
			response.Error(w, http.StatusForbidden, err.Error(), nil)
		default:
			log.Printf("search content pages internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "search content pages success", res)
}

func (h *ContentHandler) LogSearch(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	var req dto.SearchLogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid body request", nil)
		return
	}

	h.svc.LogSearch(r.Context(), chi.URLParam(r, "workspaceID"), req.Query, req.DocumentID, actor)

	response.Success(w, http.StatusOK, "search logged", nil)
}

func (h *ContentHandler) SearchBoxes(w http.ResponseWriter, r *http.Request) {
	wID := chi.URLParam(r, "workspaceID")
	dID := chi.URLParam(r, "documentID")

	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	res, err := h.svc.SearchBoxes(r.Context(), wID, dID, r.URL.Query().Get("q"), actor)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrContentForbidden):
			response.Error(w, http.StatusForbidden, err.Error(), nil)
		case errors.Is(err, service.ErrDocumentNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		default:
			log.Printf("search boxes internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "search boxes success", res)
}

func (h *ContentHandler) RenameFolder(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	var req dto.RenameFolderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid body request", nil)
		return
	}

	if errs := validation.Validate(&req); errs != nil {
		response.Error(w, http.StatusBadRequest, "validation failed", errs)
		return
	}

	req.WorkspaceID = chi.URLParam(r, "workspaceID")
	req.FolderID = chi.URLParam(r, "folderID")

	res, err := h.svc.RenameFolder(r.Context(), req, actor)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrFolderNameTaken):
			response.Error(w, http.StatusConflict, err.Error(), nil)
		case errors.Is(err, service.ErrFolderNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		default:
			log.Printf("rename folder internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "rename folder success", res)
}

func (h *ContentHandler) DeleteFolder(w http.ResponseWriter, r *http.Request) {
	wID := chi.URLParam(r, "workspaceID")
	fID := chi.URLParam(r, "folderID")
	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	if err := h.svc.DeleteFolder(r.Context(), fID, wID, actor); err != nil {
		switch {
		case errors.Is(err, service.ErrFolderNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		case errors.Is(err, service.ErrDeleteDefault):
			response.Error(w, http.StatusForbidden, err.Error(), nil)
		default:
			log.Printf("delete folder internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "delete folder success", nil)
}

func (h *ContentHandler) BulkDeleteFolders(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	var req dto.BulkDeleteFoldersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid body request", nil)
		return
	}

	if errs := validation.Validate(&req); errs != nil {
		response.Error(w, http.StatusBadRequest, "validation failed", errs)
		return
	}

	req.WorkspaceID = chi.URLParam(r, "workspaceID")

	if err := h.svc.BulkDeleteFolders(r.Context(), req, actor); err != nil {
		switch {
		case errors.Is(err, service.ErrFolderNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		case errors.Is(err, service.ErrDeleteDefault):
			response.Error(w, http.StatusForbidden, err.Error(), nil)
		default:
			log.Printf("bulk delete folders internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "bulk delete folders success", nil)
}

func (h *ContentHandler) BulkDeleteDocuments(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	var req dto.BulkDeleteDocumentsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid body request", nil)
		return
	}

	if errs := validation.Validate(&req); errs != nil {
		response.Error(w, http.StatusBadRequest, "validation failed", errs)
		return
	}

	req.WorkspaceID = chi.URLParam(r, "workspaceID")

	if err := h.svc.BulkDeleteDocuments(r.Context(), req, actor); err != nil {
		switch {
		case errors.Is(err, service.ErrDocumentNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		default:
			log.Printf("bulk delete documents internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "bulk delete documents success", nil)
}

func (h *ContentHandler) RequestUploadURL(w http.ResponseWriter, r *http.Request) {
	wID := chi.URLParam(r, "workspaceID")
	fID := chi.URLParam(r, "folderID")

	var req dto.UploadURLRequest
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxBodyBytes)).Decode(&req)

	res, err := h.svc.RequestUploadURL(r.Context(), wID, fID, req.StorageKey)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrFolderNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		case errors.Is(err, service.ErrInvalidStorageKey):
			response.Error(w, http.StatusBadRequest, err.Error(), nil)
		default:
			log.Printf("request upload url internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "request upload url success", res)
}

func (h *ContentHandler) CompletedUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	var req dto.CompleteUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid body request", nil)
		return
	}

	if errs := validation.Validate(&req); errs != nil {
		response.Error(w, http.StatusBadRequest, "validation failed", errs)
		return
	}

	req.WorkspaceID = chi.URLParam(r, "workspaceID")
	req.FolderID = chi.URLParam(r, "folderID")
	req.UploadedBy = actor.UserID

	res, err := h.svc.CompletedUpload(r.Context(), req, actor)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrFolderNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		case errors.Is(err, service.ErrUploadNotFound),
			errors.Is(err, service.ErrInvalidStorageKey):
			response.Error(w, http.StatusBadRequest, err.Error(), nil)
		case errors.Is(err, service.ErrNotUploadable):
			response.Error(w, http.StatusUnsupportedMediaType, err.Error(), nil)
		case errors.Is(err, service.ErrUploadTooLarge):
			response.Error(w, http.StatusRequestEntityTooLarge, err.Error(), nil)
		default:
			log.Printf("complete upload internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusCreated, "upload document success", res)
}

func (h *ContentHandler) ListDocuments(w http.ResponseWriter, r *http.Request) {
	wID := chi.URLParam(r, "workspaceID")
	fID := chi.URLParam(r, "folderID")

	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	res, err := h.svc.ListDocuments(r.Context(), wID, fID, actor)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrFolderNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		case errors.Is(err, service.ErrContentForbidden):
			response.Error(w, http.StatusForbidden, err.Error(), nil)
		default:
			log.Printf("list documents internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "list documents success", res)
}

func (h *ContentHandler) ListVersions(w http.ResponseWriter, r *http.Request) {
	wID := chi.URLParam(r, "workspaceID")
	dID := chi.URLParam(r, "documentID")

	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	res, err := h.svc.ListVersions(r.Context(), wID, dID, actor)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrContentForbidden):
			response.Error(w, http.StatusForbidden, err.Error(), nil)
		case errors.Is(err, service.ErrDocumentNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		default:
			log.Printf("list versions internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "list versions success", res)
}

func (h *ContentHandler) RequestUploadVersion(w http.ResponseWriter, r *http.Request) {
	wID := chi.URLParam(r, "workspaceID")
	dID := chi.URLParam(r, "documentID")

	res, err := h.svc.RequestVersionUpload(r.Context(), wID, dID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrDocumentNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		default:
			log.Printf("request version upload internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "request version upload url success", res)
}

func (h *ContentHandler) CompletedVersionUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	var req dto.CompleteVersionRequest
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
	req.UploadedBy = actor.UserID

	res, err := h.svc.CompletedVersion(r.Context(), req, actor)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrDocumentNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		case errors.Is(err, service.ErrUploadNotFound):
			response.Error(w, http.StatusBadRequest, err.Error(), nil)
		case errors.Is(err, service.ErrVersionTypeMismatch):
			response.Error(w, http.StatusUnsupportedMediaType, err.Error(), nil)
		case errors.Is(err, service.ErrUploadTooLarge):
			response.Error(w, http.StatusRequestEntityTooLarge, err.Error(), nil)
		default:
			log.Printf("complete version internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusCreated, "upload version success", res)
}

func (h *ContentHandler) GetDownloadURL(w http.ResponseWriter, r *http.Request) {
	wID := chi.URLParam(r, "workspaceID")
	dID := chi.URLParam(r, "documentID")
	versionID := r.URL.Query().Get("version")

	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	mark := watermark.Mark{
		Primary:   claims.Email,
		Secondary: time.Now().UTC().Format("2006-01-02 15:04 MST") + " · " + middleware.ClientIP(r),
	}

	body, name, err := h.svc.DownloadDocument(r.Context(), wID, dID, versionID, actor, mark)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrContentForbidden):
			response.Error(w, http.StatusForbidden, err.Error(), nil)
		case errors.Is(err, service.ErrDocumentNotFound), errors.Is(err, service.ErrVersionNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		case errors.Is(err, service.ErrDownloadBusy):
			w.Header().Set("Retry-After", "10")
			response.Error(w, http.StatusTooManyRequests, err.Error(), nil)
		case errors.Is(err, service.ErrWatermarkDownloadTooLarge):
			response.Error(w, http.StatusRequestEntityTooLarge, err.Error(), nil)
		case errors.Is(err, service.ErrNotViewable), errors.Is(err, service.ErrStampFailed),
			errors.Is(err, service.ErrRenditionFailed), errors.Is(err, service.ErrTooManyPages):
			response.Error(w, http.StatusUnprocessableEntity, err.Error(), nil)
		default:
			log.Printf("get download url internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}
	defer body.Close()

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)

	if _, err := io.Copy(w, body); err != nil {
		log.Printf("stream download body: %v", err)
	}
}

func (h *ContentHandler) RetryRendition(w http.ResponseWriter, r *http.Request) {
	wID := chi.URLParam(r, "workspaceID")
	dID := chi.URLParam(r, "documentID")
	vID := chi.URLParam(r, "versionID")

	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	if err := h.svc.RetryRendition(r.Context(), wID, dID, vID, actor); err != nil {
		switch {
		case errors.Is(err, service.ErrContentForbidden):
			response.Error(w, http.StatusForbidden, err.Error(), nil)
		case errors.Is(err, service.ErrDocumentNotFound), errors.Is(err, service.ErrVersionNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		default:
			log.Printf("retry rendition internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "retry rendition success", nil)
}

func (h *ContentHandler) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	wID := chi.URLParam(r, "workspaceID")
	dID := chi.URLParam(r, "documentID")

	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	if err := h.svc.DeleteDocument(r.Context(), wID, dID, actor); err != nil {
		switch {
		case errors.Is(err, service.ErrDocumentNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		default:
			log.Printf("delete document internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "delete document success", nil)
}

func (h *ContentHandler) ListTrash(w http.ResponseWriter, r *http.Request) {
	wID := chi.URLParam(r, "workspaceID")

	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	res, err := h.svc.ListTrash(r.Context(), wID, actor)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrContentForbidden):
			response.Error(w, http.StatusForbidden, err.Error(), nil)
		default:
			log.Printf("list trash internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "list trash success", res)
}

func (h *ContentHandler) RestoreTrashDocument(w http.ResponseWriter, r *http.Request) {
	wID := chi.URLParam(r, "workspaceID")
	dID := chi.URLParam(r, "documentID")

	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	res, err := h.svc.RestoreDocument(r.Context(), wID, dID, actor)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrContentForbidden):
			response.Error(w, http.StatusForbidden, err.Error(), nil)
		case errors.Is(err, service.ErrNotInTrash):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		case errors.Is(err, service.ErrDocumentNameTaken):
			response.Error(w, http.StatusConflict, err.Error(), nil)
		default:
			log.Printf("restore document internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "restore document success", res)
}

func (h *ContentHandler) RestoreTrashFolder(w http.ResponseWriter, r *http.Request) {
	wID := chi.URLParam(r, "workspaceID")
	fID := chi.URLParam(r, "folderID")

	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	res, err := h.svc.RestoreFolders(r.Context(), wID, fID, actor)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrContentForbidden):
			response.Error(w, http.StatusForbidden, err.Error(), nil)
		case errors.Is(err, service.ErrNotInTrash):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		case errors.Is(err, service.ErrFolderNameTaken):
			response.Error(w, http.StatusConflict, err.Error(), nil)
		default:
			log.Printf("restore folder internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "restore folder success", res)
}

func (h *ContentHandler) MoveDocument(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	var req dto.MoveDocumentRequest
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

	if err := h.svc.MoveDocument(r.Context(), req, actor); err != nil {
		switch {
		case errors.Is(err, service.ErrDocumentNotFound), errors.Is(err, service.ErrFolderNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		case errors.Is(err, service.ErrParentCrossWorkspace):
			response.Error(w, http.StatusBadRequest, err.Error(), nil)
		default:
			log.Printf("move document internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "move document success", nil)
}

func (h *ContentHandler) SetFolderAccess(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	var req dto.SetFolderAccessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid body request", nil)
		return
	}

	if errs := validation.Validate(&req); errs != nil {
		response.Error(w, http.StatusBadRequest, "validation failed", errs)
		return
	}

	req.WorkspaceID = chi.URLParam(r, "workspaceID")
	req.FolderID = chi.URLParam(r, "folderID")

	if err := h.svc.SetFolderAccess(r.Context(), req, actor); err != nil {
		switch {
		case errors.Is(err, service.ErrFolderNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		case errors.Is(err, service.ErrAccessTargetInvalid),
			errors.Is(err, service.ErrAccessFlagsConflict):
			response.Error(w, http.StatusBadRequest, err.Error(), nil)
		default:
			log.Printf("set folder access internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "success set folder access", nil)
}

func (h *ContentHandler) RemoveFolderAccess(w http.ResponseWriter, r *http.Request) {
	WorkspaceID := chi.URLParam(r, "workspaceID")
	FolderID := chi.URLParam(r, "folderID")
	groupID := chi.URLParam(r, "groupID")

	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	if err := h.svc.RemoveFolderAccess(r.Context(), WorkspaceID, groupID, FolderID, actor); err != nil {
		switch {
		case errors.Is(err, service.ErrFolderNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		case errors.Is(err, service.ErrAccessTargetInvalid):
			response.Error(w, http.StatusBadRequest, err.Error(), nil)
		default:
			log.Printf("remove folder access internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "success remove folder access", nil)
}

func (h *ContentHandler) GetViewMeta(w http.ResponseWriter, r *http.Request) {
	wID := chi.URLParam(r, "workspaceID")
	dID := chi.URLParam(r, "documentID")
	versionID := r.URL.Query().Get("version")

	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	res, err := h.svc.GetViewMeta(r.Context(), wID, dID, versionID, actor)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrContentForbidden):
			response.Error(w, http.StatusForbidden, err.Error(), nil)
		case errors.Is(err, service.ErrDocumentNotFound), errors.Is(err, service.ErrVersionNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		case errors.Is(err, service.ErrNotViewable), errors.Is(err, service.ErrRenditionFailed),
			errors.Is(err, service.ErrTooManyPages):
			response.Error(w, http.StatusUnprocessableEntity, err.Error(), nil)
		default:
			log.Printf("get view meta internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "get view meta success", res)
}

func (h *ContentHandler) GetViewPage(w http.ResponseWriter, r *http.Request) {
	wID := chi.URLParam(r, "workspaceID")
	dID := chi.URLParam(r, "documentID")
	versionID := r.URL.Query().Get("version")

	page, err := strconv.Atoi(chi.URLParam(r, "page"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid page number", nil)
		return
	}

	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	secondary := time.Now().UTC().Format("2006-01-02 15:04 MST") + " · " + middleware.ClientIP(r)

	img, err := h.svc.GetPageImage(r.Context(), dto.ViewPageRequest{
		WorkspaceID:   wID,
		DocumentID:    dID,
		Page:          page,
		VersionID:     versionID,
		MarkPrimary:   claims.Email,
		MarkSecondary: secondary,
	}, actor)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrContentForbidden):
			response.Error(w, http.StatusForbidden, err.Error(), nil)
		case errors.Is(err, service.ErrDocumentNotFound), errors.Is(err, service.ErrPageOutOfRange), errors.Is(err, service.ErrVersionNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		case errors.Is(err, service.ErrNotViewable), errors.Is(err, service.ErrRenditionFailed),
			errors.Is(err, service.ErrTooManyPages):
			response.Error(w, http.StatusUnprocessableEntity, err.Error(), nil)
		default:
			log.Printf("get view page internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(img)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(img)
}

func (h *ContentHandler) GetDownloadLimits(w http.ResponseWriter, _ *http.Request) {
	response.Success(w, http.StatusOK, "success get download limits", h.svc.DownloadLimits())
}

func (h *ContentHandler) ListFolderAccess(w http.ResponseWriter, r *http.Request) {
	WorkspaceID := chi.URLParam(r, "workspaceID")
	FolderID := chi.URLParam(r, "folderID")

	res, err := h.svc.ListFolderAccess(r.Context(), WorkspaceID, FolderID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrFolderNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		default:
			log.Printf("list folder access internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "success get list folder access", res)
}

func (h *ContentHandler) BulkCreateFolders(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	var req dto.BulkCreateFolderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid body request", nil)
		return
	}

	if errs := validation.Validate(&req); errs != nil {
		response.Error(w, http.StatusBadRequest, "validation failed", errs)
		return
	}

	req.WorkspaceID = chi.URLParam(r, "workspaceID")
	req.CreatedBy = actor.UserID

	res, err := h.svc.BulkCreateFolders(r.Context(), req, actor)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrParentNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		case errors.Is(err, service.ErrParentCrossWorkspace),
			errors.Is(err, service.ErrBulkTooManyFolders),
			errors.Is(err, service.ErrBulkTooDeep),
			errors.Is(err, service.ErrFolderNameInvalid):
			response.Error(w, http.StatusBadRequest, err.Error(), nil)
		case errors.Is(err, service.ErrFolderNameTaken):
			response.Error(w, http.StatusConflict, err.Error(), nil)
		default:
			log.Printf("bulk create folders internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusCreated, "bulk create folders success", res)
}

func (h *ContentHandler) ListFolderTemplates(w http.ResponseWriter, r *http.Request) {
	response.Success(w, http.StatusOK, "list folder templates success", h.svc.ListFolderTemplates())
}

func (h *ContentHandler) ApplyFolderTemplate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	var req dto.ApplyTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid body request", nil)
		return
	}

	if errs := validation.Validate(&req); errs != nil {
		response.Error(w, http.StatusBadRequest, "validation failed", errs)
		return
	}

	req.WorkspaceID = chi.URLParam(r, "workspaceID")
	req.TemplateKey = chi.URLParam(r, "templateKey")

	res, err := h.svc.ApplyFolderTemplate(r.Context(), req, actor)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrTemplateNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		case errors.Is(err, service.ErrBulkTooManyFolders),
			errors.Is(err, service.ErrBulkTooDeep),
			errors.Is(err, service.ErrFolderNameInvalid):
			response.Error(w, http.StatusBadRequest, err.Error(), nil)
		default:
			log.Printf("apply folder template internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusCreated, "apply folder template success", res)
}

func (h *ContentHandler) InitMultipart(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	var req dto.InitMultipartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid body request", nil)
		return
	}

	if errs := validation.Validate(&req); errs != nil {
		response.Error(w, http.StatusBadRequest, "validation failed", errs)
		return
	}

	req.WorkspaceID = chi.URLParam(r, "workspaceID")
	req.FolderID = chi.URLParam(r, "folderID")

	res, err := h.svc.InitMultipart(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrFolderNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		case errors.Is(err, service.ErrUploadTooLarge):
			response.Error(w, http.StatusRequestEntityTooLarge, err.Error(), nil)
		case errors.Is(err, service.ErrDocumentNameTaken):
			response.Error(w, http.StatusConflict, err.Error(), nil)
		case errors.Is(err, service.ErrNotUploadable):
			response.Error(w, http.StatusUnsupportedMediaType, err.Error(), nil)
		default:
			log.Printf("init multipart internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "init multipart success", res)
}

func (h *ContentHandler) MultipartPartURLs(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	var req dto.MultipartPartURLsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid body request", nil)
		return
	}

	if errs := validation.Validate(&req); errs != nil {
		response.Error(w, http.StatusBadRequest, "validation failed", errs)
		return
	}

	req.WorkspaceID = chi.URLParam(r, "workspaceID")
	req.FolderID = chi.URLParam(r, "folderID")

	res, err := h.svc.MultipartPartURLs(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidStorageKey),
			errors.Is(err, service.ErrInvalidPartNumber),
			errors.Is(err, service.ErrTooManyParts):
			response.Error(w, http.StatusBadRequest, err.Error(), nil)
		default:
			log.Printf("multipart part urls internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "get part urls success", res)
}

func (h *ContentHandler) MultipartParts(w http.ResponseWriter, r *http.Request) {
	req := dto.ListPartsRequest{
		WorkspaceID: chi.URLParam(r, "workspaceID"),
		FolderID:    chi.URLParam(r, "folderID"),
		UploadID:    r.URL.Query().Get("upload_id"),
		StorageKey:  r.URL.Query().Get("storage_key"),
	}

	if req.UploadID == "" || req.StorageKey == "" {
		response.Error(w, http.StatusBadRequest, "upload_id and storage_key are required", nil)
		return
	}

	res, err := h.svc.MultipartParts(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidStorageKey):
			response.Error(w, http.StatusBadRequest, err.Error(), nil)
		default:
			log.Printf("multipart parts internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "get uploaded parts success", res)
}

func (h *ContentHandler) CompleteMultipart(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	var req dto.CompleteMultipartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid body request", nil)
		return
	}

	if errs := validation.Validate(&req); errs != nil {
		response.Error(w, http.StatusBadRequest, "validation failed", errs)
		return
	}

	req.WorkspaceID = chi.URLParam(r, "workspaceID")
	req.FolderID = chi.URLParam(r, "folderID")
	req.UploadedBy = actor.UserID

	res, err := h.svc.CompleteMultipart(r.Context(), req, actor)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrFolderNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		case errors.Is(err, service.ErrInvalidStorageKey),
			errors.Is(err, service.ErrUploadNotFound):
			response.Error(w, http.StatusBadRequest, err.Error(), nil)
		case errors.Is(err, service.ErrNotUploadable):
			response.Error(w, http.StatusUnsupportedMediaType, err.Error(), nil)
		case errors.Is(err, service.ErrUploadTooLarge):
			response.Error(w, http.StatusRequestEntityTooLarge, err.Error(), nil)
		default:
			log.Printf("complete multipart internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusCreated, "upload document success", res)
}

func (h *ContentHandler) AbortMultipart(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	var req dto.AbortMultipartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid body request", nil)
		return
	}

	if errs := validation.Validate(&req); errs != nil {
		response.Error(w, http.StatusBadRequest, "validation failed", errs)
		return
	}

	req.WorkspaceID = chi.URLParam(r, "workspaceID")
	req.FolderID = chi.URLParam(r, "folderID")

	if err := h.svc.AbortMultipart(r.Context(), req); err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidStorageKey):
			response.Error(w, http.StatusBadRequest, err.Error(), nil)
		default:
			log.Printf("abort multipart internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "abort multipart success", nil)
}

func (h *ContentHandler) RestoreVersion(w http.ResponseWriter, r *http.Request) {
	wID := chi.URLParam(r, "workspaceID")
	dID := chi.URLParam(r, "documentID")
	vID := chi.URLParam(r, "versionID")

	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	res, err := h.svc.RestoreVersion(r.Context(), wID, dID, vID, actor)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrDocumentNotFound), errors.Is(err, service.ErrVersionNotFound):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		case errors.Is(err, service.ErrAlreadyCurrent):
			response.Error(w, http.StatusConflict, err.Error(), nil)
		default:
			log.Printf("restore version internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "restore version success", res)
}
