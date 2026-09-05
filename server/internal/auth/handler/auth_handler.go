package handler

import (
	"errors"
	"log"
	"net/http"

	"github.com/findardi/rakda/server/internal/auth/dto"
	"github.com/findardi/rakda/server/internal/auth/service"
	"github.com/findardi/rakda/server/internal/platform/middleware"
	"github.com/findardi/rakda/server/internal/platform/oauth"
	"github.com/findardi/rakda/server/internal/platform/response"
	"github.com/findardi/rakda/server/internal/platform/validation"
	"github.com/go-chi/chi/v5"
)

const (
	MaxBodyBytes = 1 << 20
)

type AuthHandler struct {
	svc       *service.AuthService
	providers map[string]oauth.Provider
}

func NewAuthHandler(svc *service.AuthService, providers map[string]oauth.Provider) *AuthHandler {
	return &AuthHandler{
		svc:       svc,
		providers: providers,
	}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	req, ok := validation.Bind[dto.RegisterRequest](w, r)
	if !ok {
		return
	}

	res, err := h.svc.RegisterUser(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrEmailUnique), errors.Is(err, service.ErrUsernameUnique):
			response.Error(w, http.StatusConflict, err.Error(), nil)
		default:
			log.Printf("register internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusCreated, "success registered account", res)
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	req, ok := validation.Bind[dto.LoginRequest](w, r)
	if !ok {
		return
	}

	res, err := h.svc.LoginUser(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidCredentials):
			response.Error(w, http.StatusUnauthorized, err.Error(), nil)
		default:
			log.Printf("login internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "login success", res)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	req, ok := validation.Bind[dto.LogoutRequest](w, r)
	if !ok {
		return
	}

	req.UserID = claims.ID

	if err := h.svc.LogoutUser(r.Context(), req); err != nil {
		log.Printf("logout user internal error: %v", err)
		response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		return
	}

	response.Success(w, http.StatusOK, "logout success", nil)
}

func (h *AuthHandler) ResendOTP(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	if err := h.svc.ResendOTP(r.Context(), claims.Email); err != nil {
		log.Printf("resend otp internal error: %v", err)
		response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		return
	}

	response.Success(w, http.StatusOK, "resend success", nil)
}

func (h *AuthHandler) VerifyAccount(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	req, ok := validation.Bind[dto.VerifyOtpRequest](w, r)
	if !ok {
		return
	}

	req.Email = claims.Email

	if err := h.svc.VerifyEmail(r.Context(), req); err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidCodeOTP), errors.Is(err, service.ErrEmailAlreadyVerified):
			response.Error(w, http.StatusBadRequest, err.Error(), nil)
		default:
			log.Printf("verify internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "verify success", nil)
}

func (h *AuthHandler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	req, ok := validation.Bind[dto.RefreshTokenRequest](w, r)
	if !ok {
		return
	}

	res, err := h.svc.RefreshToken(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidRefreshToken):
			response.Error(w, http.StatusUnauthorized, "invalid or expired refresh token", nil)
		case errors.Is(err, service.ErrRefreshReuseDetected):
			log.Printf("SECURITY: refresh token reuse detected for revoked session")
			response.Error(w, http.StatusUnauthorized, "invalid or expired refresh token", nil)
		default:
			log.Printf("refresh token internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "refresh token success", res)
}

func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	req, ok := validation.Bind[dto.ForgotPasswordRequest](w, r)
	if !ok {
		return
	}

	if err := h.svc.ForgotPassword(r.Context(), req); err != nil {
		log.Printf("forgot password internal error: %v", err)
		response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		return
	}

	response.Success(w, http.StatusOK, "if the email is registered, a reset code has been sent", nil)
}

func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	req, ok := validation.Bind[dto.ResetPasswordRequest](w, r)
	if !ok {
		return
	}

	if err := h.svc.ResetPassword(r.Context(), req); err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidCodeOTP):
			response.Error(w, http.StatusBadRequest, err.Error(), nil)
		default:
			log.Printf("reset password internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "password reset success", nil)
}

func (h *AuthHandler) CheckOTP(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	req, ok := validation.Bind[dto.ValidateOtpRequest](w, r)
	if !ok {
		return
	}

	if err := h.svc.ValidateOTP(r.Context(), req); err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidCodeOTP):
			response.Error(w, http.StatusBadRequest, err.Error(), nil)
		default:
			log.Printf("validate otp internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "success validation otp", nil)
}

func (h *AuthHandler) CheckEmail(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	req, ok := validation.Bind[dto.ForgotPasswordRequest](w, r)
	if !ok {
		return
	}

	if err := h.svc.CheckEmail(r.Context(), req); err != nil {
		switch {
		case errors.Is(err, service.ErrEmailUnique):
			response.Error(w, http.StatusBadRequest, err.Error(), nil)
		default:
			log.Printf("validate otp internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "success check email", nil)
}

func (h *AuthHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	res, err := h.svc.GetMe(r.Context(), claims.ID)
	if err != nil {
		log.Printf("get me internal error: %v", err)
		response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		return
	}

	response.Success(w, http.StatusOK, "success", res)
}

func (h *AuthHandler) SSOAuthUrl(w http.ResponseWriter, r *http.Request) {
	p, ok := h.providers[chi.URLParam(r, "provider")]
	if !ok {
		response.Error(w, http.StatusNotFound, "unknown provider", nil)
		return
	}

	state := r.URL.Query().Get("state")
	if state == "" {
		response.Error(w, http.StatusBadRequest, "missing state", nil)
		return
	}

	response.Success(w, http.StatusOK, "success", map[string]string{
		"url": p.AuthCodeURL(state),
	})
}

func (h *AuthHandler) SSOExchange(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	p, ok := h.providers[provider]
	if !ok {
		response.Error(w, http.StatusNotFound, "unknown provider", nil)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	req, ok := validation.Bind[dto.SSOExchangeRequest](w, r)
	if !ok {
		return
	}

	identity, err := p.Identity(r.Context(), req.Code)
	if err != nil {
		log.Printf("sso identity error: %v", err)
		response.Error(w, http.StatusBadGateway, "failed to fetch identity from provider", nil)
		return
	}

	res, err := h.svc.SSOLogin(r.Context(), provider, service.SSOIdentity{
		ProviderUID:   identity.ProviderUID,
		Email:         identity.Email,
		EmailVerified: identity.EmailVerified,
		Username:      identity.Username,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrEmailAlreadyRegistered):
			response.Error(w, http.StatusConflict, err.Error(), nil)
		case errors.Is(err, service.ErrSSOEmailMissing):
			response.Error(w, http.StatusBadRequest, err.Error(), nil)
		default:
			log.Printf("sso login internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "login success", res)
}

func (h *AuthHandler) PreviewInvitation(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")

	res, err := h.svc.PreviewInvitationSignup(r.Context(), token)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvitationInvalid):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		default:
			log.Printf("preview invitation internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusOK, "invitation valid", res)
}

func (h *AuthHandler) AcceptInvitation(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	token := chi.URLParam(r, "token")

	req, ok := validation.Bind[dto.AcceptInvitationRequest](w, r)
	if !ok {
		return
	}
	req.Token = token

	res, err := h.svc.AcceptInvitationSignup(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvitationInvalid):
			response.Error(w, http.StatusNotFound, err.Error(), nil)
		case errors.Is(err, service.ErrInviteEmailRegistered), errors.Is(err, service.ErrUsernameUnique):
			response.Error(w, http.StatusConflict, err.Error(), nil)
		default:
			log.Printf("accept invitation internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
		}
		return
	}

	response.Success(w, http.StatusCreated, "invitation accepted", res)
}
