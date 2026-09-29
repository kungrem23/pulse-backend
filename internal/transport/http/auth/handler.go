package auth

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/example/pulse-backend/internal/app/adminauth"
	appauth "github.com/example/pulse-backend/internal/app/auth"
	"github.com/example/pulse-backend/internal/transport/http/httpx"
)

type Handler struct {
	service      appauth.Service
	adminService adminauth.Service
}

func NewHandler(service appauth.Service, adminService adminauth.Service) *Handler {
	return &Handler{
		service:      service,
		adminService: adminService,
	}
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var request registerRequest
	if err := decodeJSON(r, &request); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if h.service == nil {
		httpx.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}

	registration, err := h.service.Register(r.Context(), appauth.RegisterCommand{
		Email:    request.Email,
		Password: request.Password,
	})
	if err != nil {
		switch {
		case errors.Is(err, appauth.ErrInvalidEmail):
			httpx.Error(w, http.StatusUnprocessableEntity, "invalid email")
		case errors.Is(err, appauth.ErrInvalidPassword):
			httpx.Error(w, http.StatusUnprocessableEntity, "invalid password")
		case errors.Is(err, appauth.ErrEmailAlreadyExists):
			httpx.Error(w, http.StatusConflict, "email already exists")
		default:
			httpx.Error(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, registerResponse{
		User:    registration.User,
		Session: userSessionResponse(registration.Session),
	})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var request userLoginRequest
	if err := decodeJSON(r, &request); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if h.service == nil {
		httpx.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}

	session, err := h.service.Login(r.Context(), appauth.LoginCommand{
		Email:    request.Email,
		Password: request.Password,
	})
	if err != nil {
		switch {
		case errors.Is(err, appauth.ErrInvalidCredentials):
			httpx.Error(w, http.StatusUnauthorized, "invalid credentials")
		case errors.Is(err, appauth.ErrUserBlocked):
			httpx.Error(w, http.StatusForbidden, "user is blocked")
		default:
			httpx.Error(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	httpx.WriteJSON(w, http.StatusOK, userSessionResponse(session))
}

func (h *Handler) LoginAdmin(w http.ResponseWriter, r *http.Request) {
	var request adminLoginRequest
	if err := decodeJSON(r, &request); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if h.adminService == nil {
		httpx.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}

	session, err := h.adminService.Login(r.Context(), adminauth.LoginCommand{
		Username: request.Username,
		Password: request.Password,
	})
	if err != nil {
		if errors.Is(err, adminauth.ErrInvalidCredentials) {
			httpx.Error(w, http.StatusUnauthorized, "invalid credentials")
		} else {
			httpx.Error(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	httpx.WriteJSON(w, http.StatusOK, adminSessionResponse(session))
}

func decodeJSON(r *http.Request, destination any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain a single JSON value")
	}
	return nil
}

func userSessionResponse(session appauth.Session) sessionResponse {
	return sessionResponse{
		AccessToken:      session.AccessToken,
		RefreshToken:     session.RefreshToken,
		TokenType:        "Bearer",
		AccessExpiresAt:  session.AccessExpiresAt,
		RefreshExpiresAt: session.RefreshExpiresAt,
	}
}

func adminSessionResponse(session adminauth.Session) sessionResponse {
	return sessionResponse{
		AccessToken:      session.AccessToken,
		RefreshToken:     session.RefreshToken,
		TokenType:        "Bearer",
		AccessExpiresAt:  session.AccessExpiresAt,
		RefreshExpiresAt: session.RefreshExpiresAt,
	}
}
