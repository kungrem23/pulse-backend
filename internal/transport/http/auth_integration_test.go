package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	appauth "github.com/kungrem23/pulse-backend/internal/app/auth"
	"github.com/kungrem23/pulse-backend/internal/repository/postgres"
	"github.com/kungrem23/pulse-backend/internal/security"
	"github.com/kungrem23/pulse-backend/internal/testsupport/pgtest"
)

type integrationServer struct {
	pool   *pgxpool.Pool
	tokens *security.TokenIssuer
}

func newIntegrationServer(t *testing.T) *integrationServer {
	t.Helper()
	pool := pgtest.NewDatabase(t)
	tokens, err := security.NewTokenIssuer(security.TokenIssuerConfig{
		Secret:          strings.Repeat("s", 32),
		Issuer:          "pulse-test",
		AccessTokenTTL:  time.Minute,
		RefreshTokenTTL: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &integrationServer{pool: pool, tokens: tokens}
}

// do builds a new router per request to simulate a server restart: all state lives in PostgreSQL.
func (s *integrationServer) do(t *testing.T, path, body, accessToken string) *httptest.ResponseRecorder {
	t.Helper()
	passwords, err := security.NewPasswordManager(bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(Dependencies{
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Database: s.pool,
		Tokens:   s.tokens,
		Auth: appauth.NewService(
			postgres.NewUserRepository(s.pool),
			postgres.NewRefreshSessionRepository(s.pool),
			passwords,
			s.tokens,
		),
	})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if accessToken != "" {
		request.Header.Set("Authorization", "Bearer "+accessToken)
	}
	router.ServeHTTP(response, request)
	return response
}

func TestAuthHTTPWithPostgres(t *testing.T) {
	server := newIntegrationServer(t)
	pool, tokens := server.pool, server.tokens
	post := func(path, body string) *httptest.ResponseRecorder {
		return server.do(t, path, body, "")
	}
	userCount := func() int {
		var count int
		if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM users").Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}

	response := post("/api/v1/auth/register", `{"email":"  New.User@Example.COM ","password":"password123","age":18}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("register: expected 201, got %d: %s", response.Code, response.Body)
	}
	if strings.Contains(response.Body.String(), "password") || strings.Contains(response.Body.String(), "$2a$") {
		t.Fatalf("register response leaks password data: %s", response.Body)
	}
	var registered struct {
		User struct {
			ID        string    `json:"id"`
			Email     string    `json:"email"`
			Age       int       `json:"age"`
			IsBlocked bool      `json:"is_blocked"`
			CreatedAt time.Time `json:"created_at"`
		} `json:"user"`
		Session struct {
			AccessToken string `json:"access_token"`
			TokenType   string `json:"token_type"`
		} `json:"session"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &registered); err != nil {
		t.Fatal(err)
	}
	if registered.User.Email != "new.user@example.com" || registered.User.Age != 18 ||
		registered.User.ID == "" || registered.User.CreatedAt.IsZero() || registered.Session.TokenType != "Bearer" {
		t.Fatalf("unexpected registration: %+v", registered)
	}
	claims, err := tokens.Verify(registered.Session.AccessToken, security.AccessToken)
	if err != nil || claims.Subject != registered.User.ID || claims.Principal != security.UserPrincipal {
		t.Fatalf("access token does not identify the new user: %+v, %v", claims, err)
	}

	for _, test := range []struct {
		name   string
		body   string
		status int
		code   string
	}{
		{"duplicate email in another case", `{"email":"NEW.USER@example.com","password":"password123","age":30}`, http.StatusConflict, "conflict"},
		{"age 17", `{"email":"a17@example.com","password":"password123","age":17}`, http.StatusUnprocessableEntity, "validation_failed"},
		{"age 121", `{"email":"a121@example.com","password":"password123","age":121}`, http.StatusUnprocessableEntity, "validation_failed"},
		{"missing age", `{"email":"noage@example.com","password":"password123"}`, http.StatusUnprocessableEntity, "validation_failed"},
		{"negative age", `{"email":"neg@example.com","password":"password123","age":-1}`, http.StatusUnprocessableEntity, "validation_failed"},
		{"fractional age", `{"email":"frac@example.com","password":"password123","age":18.5}`, http.StatusBadRequest, "bad_request"},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := post("/api/v1/auth/register", test.body)
			if response.Code != test.status || errorCode(t, response) != test.code {
				t.Fatalf("expected %d %s, got %d: %s", test.status, test.code, response.Code, response.Body)
			}
		})
	}
	if count := userCount(); count != 1 {
		t.Fatalf("rejected registrations must not create users, found %d", count)
	}

	for _, age := range []string{"18", "120"} {
		response := post("/api/v1/auth/register", `{"email":"edge`+age+`@example.com","password":"password123","age":`+age+`}`)
		if response.Code != http.StatusCreated {
			t.Fatalf("age %s: expected 201, got %d: %s", age, response.Code, response.Body)
		}
	}

	response = post("/api/v1/auth/login", `{"email":"New.User@example.com","password":"password123"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("login: expected 200, got %d: %s", response.Code, response.Body)
	}
	var session struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if claims, err := tokens.Verify(session.AccessToken, security.AccessToken); err != nil || claims.Subject != registered.User.ID {
		t.Fatalf("login token does not identify the user: %+v, %v", claims, err)
	}

	wrongPassword := post("/api/v1/auth/login", `{"email":"new.user@example.com","password":"wrong-password"}`)
	unknownEmail := post("/api/v1/auth/login", `{"email":"nobody@example.com","password":"password123"}`)
	for _, response := range []*httptest.ResponseRecorder{wrongPassword, unknownEmail} {
		if response.Code != http.StatusUnauthorized || errorCode(t, response) != "invalid_credentials" {
			t.Fatalf("expected 401 invalid_credentials, got %d: %s", response.Code, response.Body)
		}
	}
	if wrongPassword.Body.String() != unknownEmail.Body.String() {
		t.Fatalf("login errors must not reveal whether an email exists: %s vs %s", wrongPassword.Body, unknownEmail.Body)
	}

	if _, err := pool.Exec(context.Background(), "UPDATE users SET is_blocked = true WHERE id = $1", registered.User.ID); err != nil {
		t.Fatal(err)
	}
	response = post("/api/v1/auth/login", `{"email":"new.user@example.com","password":"password123"}`)
	if response.Code != http.StatusForbidden {
		t.Fatalf("blocked login: expected 403, got %d: %s", response.Code, response.Body)
	}
}

func errorCode(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body %q: %v", response.Body, err)
	}
	return body.Error.Code
}
