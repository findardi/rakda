package handler

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/findardi/rakda/server/internal/platform/middleware"
	"github.com/findardi/rakda/server/internal/platform/response"
	"github.com/findardi/rakda/server/internal/platform/validation"
	"github.com/findardi/rakda/server/internal/qa/dto"
	"github.com/findardi/rakda/server/internal/qa/service"
	"github.com/go-chi/chi/v5"
)

const (
	MaxBodyBytes    = 1 << 20
	exportBatchSize = 100
)

type QAHandler struct {
	svc *service.QAService
}

func NewQAHandler(svc *service.QAService) *QAHandler {
	return &QAHandler{svc: svc}
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
	return service.Actor{UserID: claims.ID, Name: claims.Username, Email: claims.Email, Role: ms.Role}, true
}

func (h *QAHandler) qaError(w http.ResponseWriter, err error, op string) {
	switch {
	case errors.Is(err, service.ErrQuestionNotFound):
		response.Error(w, http.StatusNotFound, err.Error(), nil)
	case errors.Is(err, service.ErrQAForbidden),
		errors.Is(err, service.ErrQADisabled),
		errors.Is(err, service.ErrOnlyGuestCanAsk),
		errors.Is(err, service.ErrCloseNotAllowed),
		errors.Is(err, service.ErrReopenNotAllowed):
		response.Error(w, http.StatusForbidden, err.Error(), nil)
	case errors.Is(err, service.ErrQuestionLimitReached),
		errors.Is(err, service.ErrQuestionClosed):
		response.Error(w, http.StatusConflict, err.Error(), nil)
	case errors.Is(err, service.ErrQuestionNotClosed),
		errors.Is(err, service.ErrInvalidCursor),
		errors.Is(err, service.ErrInvalidFilter),
		errors.Is(err, service.ErrReferenceNotFound):
		response.Error(w, http.StatusBadRequest, err.Error(), nil)
	default:
		log.Printf("%s internal error: %v", op, err)
		response.Error(w, http.StatusInternalServerError, "internal server error", nil)
	}
}

func (h *QAHandler) ListQuestions(w http.ResponseWriter, r *http.Request) {
	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	q := r.URL.Query()
	limit := 0
	if v := q.Get("limit"); v != "" {
		limit, _ = strconv.Atoi(v)
	}

	req := dto.ListQuestionsRequest{
		WorkspaceID: chi.URLParam(r, "workspaceID"),
		Limit:       limit,
		Cursor:      q.Get("cursor"),
		Status:      q.Get("status"),
		GroupID:     q.Get("group_id"),
	}

	res, err := h.svc.ListQuestions(r.Context(), req, actor)
	if err != nil {
		h.qaError(w, err, "list questions")
		return
	}

	response.Success(w, http.StatusOK, "list questions success", res)
}

func (h *QAHandler) SubmitQuestion(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	req, ok := validation.Bind[dto.SubmitQuestionRequest](w, r)
	if !ok {
		return
	}

	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	req.WorkspaceID = chi.URLParam(r, "workspaceID")

	res, err := h.svc.SubmitQuestion(r.Context(), req, actor)
	if err != nil {
		h.qaError(w, err, "submit question")
		return
	}

	response.Success(w, http.StatusOK, "submit question success", res)
}

func (h *QAHandler) CountWaiting(w http.ResponseWriter, r *http.Request) {
	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	res, err := h.svc.CountWaiting(r.Context(), chi.URLParam(r, "workspaceID"), actor)
	if err != nil {
		h.qaError(w, err, "count waiting questions")
		return
	}

	response.Success(w, http.StatusOK, "count waiting questions success", res)
}

func (h *QAHandler) ExportQuestions(w http.ResponseWriter, r *http.Request) {
	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	q := r.URL.Query()
	if f := q.Get("format"); f != "" && f != "csv" {
		response.Error(w, http.StatusBadRequest, "unsupported format", nil)
		return
	}

	req := dto.ExportQuestionsRequest{
		WorkspaceID: chi.URLParam(r, "workspaceID"),
		Limit:       exportBatchSize,
		Status:      q.Get("status"),
		GroupID:     q.Get("group_id"),
	}

	if _, err := h.svc.ExportQuestions(r.Context(), req, actor); err != nil {
		h.qaError(w, err, "export questions")
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="qa-questions.csv"`)
	if _, err := w.Write([]byte("\xEF\xBB\xBF")); err != nil {
		return
	}

	if err := h.svc.WriteQuestionsCSV(r.Context(), w, req, actor); err != nil {
		log.Printf("export questions aborted mid-stream: %v", err)
	}
}

func (h *QAHandler) GetQuestion(w http.ResponseWriter, r *http.Request) {
	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	res, err := h.svc.GetQuestion(r.Context(), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "questionID"), actor)
	if err != nil {
		h.qaError(w, err, "get question")
		return
	}

	response.Success(w, http.StatusOK, "get question success", res)
}

func (h *QAHandler) ReplyQuestion(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	req, ok := validation.Bind[dto.ReplyQuestionRequest](w, r)
	if !ok {
		return
	}

	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	req.WorkspaceID = chi.URLParam(r, "workspaceID")
	req.QuestionID = chi.URLParam(r, "questionID")

	res, err := h.svc.ReplyQuestion(r.Context(), req, actor)
	if err != nil {
		h.qaError(w, err, "reply question")
		return
	}

	response.Success(w, http.StatusOK, "reply question success", res)
}

func (h *QAHandler) CloseQuestion(w http.ResponseWriter, r *http.Request) {
	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	err := h.svc.CloseQuestion(r.Context(), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "questionID"), actor)
	if err != nil {
		h.qaError(w, err, "close question")
		return
	}

	response.Success(w, http.StatusOK, "close question success", nil)
}

func (h *QAHandler) ReopenQuestion(w http.ResponseWriter, r *http.Request) {
	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	err := h.svc.ReopenQuestion(r.Context(), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "questionID"), actor)
	if err != nil {
		h.qaError(w, err, "reopen question")
		return
	}

	response.Success(w, http.StatusOK, "reopen question success", nil)
}

func (h *QAHandler) ListFaqs(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.ListFaqs(r.Context(), chi.URLParam(r, "workspaceID"))
	if err != nil {
		h.qaError(w, err, "list faqs")
		return
	}

	response.Success(w, http.StatusOK, "list faqs success", res)
}

func (h *QAHandler) CreateFaq(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	req, ok := validation.Bind[dto.CreateFaqRequest](w, r)
	if !ok {
		return
	}

	actor, ok := actorFromRequest(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	req.WorkspaceID = chi.URLParam(r, "workspaceID")

	res, err := h.svc.CreateFaq(r.Context(), req, actor)
	if err != nil {
		h.qaError(w, err, "create faq")
		return
	}

	response.Success(w, http.StatusOK, "create faq success", res)
}
