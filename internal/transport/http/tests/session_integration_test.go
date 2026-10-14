package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

type sessionTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func decodeSession(t *testing.T, response *httptest.ResponseRecorder) sessionTokens {
	t.Helper()
	var body struct {
		sessionTokens
		Session *sessionTokens `json:"session"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode session %q: %v", response.Body, err)
	}
	if body.Session != nil {
		return *body.Session
	}
	return body.sessionTokens
}

func TestSessionLifecycleWithPostgres(t *testing.T) {
	server := newIntegrationServer(t)
	expect := func(response *httptest.ResponseRecorder, status int, step string) {
		t.Helper()
		if response.Code != status {
			t.Fatalf("%s: expected %d, got %d: %s", step, status, response.Code, response.Body)
		}
	}
	refresh := func(token string) *httptest.ResponseRecorder {
		return server.do(t, "/api/v1/auth/refresh", `{"refresh_token":"`+token+`"}`, "")
	}

	registered := server.do(t, "/api/v1/auth/register", `{"email":"a@example.com","password":"password123"}`, "")
	expect(registered, http.StatusCreated, "register A")
	first := decodeSession(t, registered)
	other := server.do(t, "/api/v1/auth/register", `{"email":"b@example.com","password":"password123"}`, "")
	expect(other, http.StatusCreated, "register B")
	foreign := decodeSession(t, other)

	var stored int
	if err := server.pool.QueryRow(context.Background(),
		"SELECT count(*) FROM refresh_sessions WHERE token_hash = $1", first.RefreshToken).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != 0 {
		t.Fatal("the raw refresh token must not be stored")
	}

	rotated := refresh(first.RefreshToken)
	expect(rotated, http.StatusOK, "refresh")
	second := decodeSession(t, rotated)
	expect(refresh(first.RefreshToken), http.StatusUnauthorized, "reuse of a rotated refresh token")
	expect(refresh(first.AccessToken), http.StatusUnauthorized, "access token used as refresh")

	const attempts = 8
	var wg sync.WaitGroup
	statuses := make(chan int, attempts)
	for range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			statuses <- refresh(second.RefreshToken).Code
		}()
	}
	wg.Wait()
	close(statuses)
	succeeded := 0
	for status := range statuses {
		switch status {
		case http.StatusOK:
			succeeded++
		case http.StatusUnauthorized:
		default:
			t.Fatalf("unexpected concurrent refresh status %d", status)
		}
	}
	if succeeded != 1 {
		t.Fatalf("exactly one concurrent refresh must succeed, got %d", succeeded)
	}

	var current sessionTokens
	login := server.do(t, "/api/v1/auth/login", `{"email":"a@example.com","password":"password123"}`, "")
	expect(login, http.StatusOK, "login")
	current = decodeSession(t, login)

	expect(server.do(t, "/api/v1/auth/logout", `{"refresh_token":"`+foreign.RefreshToken+`"}`, current.AccessToken),
		http.StatusUnauthorized, "logout with another user's refresh token")
	expect(refresh(foreign.RefreshToken), http.StatusOK, "foreign session survives a rejected logout")

	expect(server.do(t, "/api/v1/auth/logout", `{"refresh_token":"`+current.RefreshToken+`"}`, ""),
		http.StatusUnauthorized, "logout without access token")
	expect(server.do(t, "/api/v1/auth/logout", `{"refresh_token":"`+current.RefreshToken+`"}`, current.AccessToken),
		http.StatusNoContent, "logout")
	expect(server.do(t, "/api/v1/auth/logout", `{"refresh_token":"`+current.RefreshToken+`"}`, current.AccessToken),
		http.StatusNoContent, "repeated logout")
	expect(refresh(current.RefreshToken), http.StatusUnauthorized, "refresh after logout")

	if _, err := server.pool.Exec(context.Background(), "UPDATE users SET is_blocked = true WHERE email = 'b@example.com'"); err != nil {
		t.Fatal(err)
	}
	login = server.do(t, "/api/v1/auth/login", `{"email":"b@example.com","password":"password123"}`, "")
	expect(login, http.StatusForbidden, "blocked login")
	expect(server.get(t, "/api/v1/profile", foreign.AccessToken), http.StatusForbidden, "blocked user with a still valid access token")
	expect(server.get(t, "/api/v1/profile", current.AccessToken), http.StatusNotFound, "active user without a profile passes authentication")
	expect(server.get(t, "/api/v1/profile", current.RefreshToken), http.StatusUnauthorized, "refresh token used as access")

	if _, err := server.pool.Exec(context.Background(), "DELETE FROM users WHERE email = 'a@example.com'"); err != nil {
		t.Fatal(err)
	}
	expect(server.get(t, "/api/v1/profile", current.AccessToken), http.StatusUnauthorized, "deleted user with a still valid access token")
}
