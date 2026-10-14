//go:build integration

package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kungrem23/pulse-backend/internal/transport/http/httpx/contracttest"
)

// Every business operation goes through the router and real PostgreSQL/MinIO.
func TestMVPWithPostgresAndS3(t *testing.T) {
	server := newS3IntegrationServer(t)
	spec := contracttest.Load(t)
	expect := func(method, path string, response *httptest.ResponseRecorder, status int) {
		t.Helper()
		if response.Code != status {
			t.Fatalf("%s %s: expected %d, got %d: %s", method, path, status, response.Code, response.Body)
		}
		spec.CheckResponse(t, method, path, response)
	}
	type person struct {
		id, photoID string
		session     sessionTokens
	}
	create := func(email, sex, city string, complete bool) person {
		t.Helper()
		response := server.do(t, "/api/v1/auth/register", fmt.Sprintf(`{"email":%q,"password":"password123"}`, email), "")
		expect("POST", "/api/v1/auth/register", response, 201)
		var body struct {
			User struct{ ID string } `json:"user"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		p := person{id: body.User.ID, session: decodeSession(t, response)}
		login := server.do(t, "/api/v1/auth/login", fmt.Sprintf(`{"email":%q,"password":"password123"}`, email), "")
		expect("POST", "/api/v1/auth/login", login, 200)
		p.session = decodeSession(t, login)
		if !complete {
			return p
		}
		token := p.session.AccessToken
		expect("PUT", "/api/v1/profile", server.put(t, "/api/v1/profile",
			fmt.Sprintf(`{"name":"Пользователь","age":25,"sex":%q,"city":%q}`, sex, city), token), 200)
		expect("PUT", "/api/v1/preferences", server.put(t, "/api/v1/preferences",
			`{"min_age":18,"max_age":120,"sex":"any"}`, token), 200)
		response = httptest.NewRecorder()
		server.router(t).ServeHTTP(response, photoRequest(t, photoPNG(t), 1, token))
		expect("POST", "/api/v1/photos", response, 201)
		var photo struct{ ID string }
		if err := json.Unmarshal(response.Body.Bytes(), &photo); err != nil {
			t.Fatal(err)
		}
		p.photoID = photo.ID
		return p
	}
	a := create("a@mvp.test", "female", "Москва", true)
	b := create("b@mvp.test", "male", "Казань", true)
	c := create("c@mvp.test", "male", "Москва", false)
	client := &http.Client{Timeout: 5 * time.Second}
	photoAccessible := func(url string) {
		t.Helper()
		response, err := client.Get(url)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatalf("public photo URL returned %d", response.StatusCode)
		}
	}
	feed := func(p person, partnerID string) {
		t.Helper()
		response := server.get(t, "/api/v1/feed", p.session.AccessToken)
		expect("GET", "/api/v1/feed", response, 200)
		var page struct {
			Items []struct {
				UserID string                 `json:"user_id"`
				Photos []struct{ URL string } `json:"photos"`
			} `json:"items"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if partnerID == "" {
			if page.Items == nil || len(page.Items) != 0 {
				t.Fatal("expected exhausted feed")
			}
			return
		}
		if len(page.Items) != 1 || page.Items[0].UserID != partnerID || len(page.Items[0].Photos) != 1 {
			t.Fatalf("unexpected feed: %s", response.Body)
		}
		photoAccessible(page.Items[0].Photos[0].URL)
	}
	feed(a, b.id)
	feed(b, a.id)
	expect("GET", "/api/v1/feed", server.get(t, "/api/v1/feed", c.session.AccessToken), 403)
	expect("DELETE", "/api/v1/photos/{photo_id}", server.request(t, http.MethodDelete,
		"/api/v1/photos/"+a.photoID, "", c.session.AccessToken), 404)
	like := func(from, to person, matched bool) string {
		t.Helper()
		response := server.do(t, "/api/v1/swipes", fmt.Sprintf(`{"to_user_id":%q,"is_like":true}`, to.id), from.session.AccessToken)
		expect("POST", "/api/v1/swipes", response, 200)
		var body struct {
			Matched bool   `json:"matched"`
			MatchID string `json:"match_id"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Matched != matched || (body.MatchID != "") != matched {
			t.Fatal("unexpected mutual match result")
		}
		return body.MatchID
	}
	like(a, b, false)
	matchID := like(b, a, true)
	for _, side := range []struct{ owner, partner person }{{a, b}, {b, a}} {
		feed(side.owner, "")
		response := server.get(t, "/api/v1/matches", side.owner.session.AccessToken)
		expect("GET", "/api/v1/matches", response, 200)
		var page struct {
			Items []struct {
				ID   string
				User struct {
					ID       string
					PhotoURL string `json:"photo_url"`
				}
			} `json:"items"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if len(page.Items) != 1 || page.Items[0].ID != matchID || page.Items[0].User.ID != side.partner.id {
			t.Fatalf("unexpected match list: %s", response.Body)
		}
		photoAccessible(page.Items[0].User.PhotoURL)
	}
	response := server.get(t, "/api/v1/matches", c.session.AccessToken)
	expect("GET", "/api/v1/matches", response, 200)
	var empty struct{ Items []json.RawMessage }
	if err := json.Unmarshal(response.Body.Bytes(), &empty); err != nil || empty.Items == nil || len(empty.Items) != 0 {
		t.Fatal("third user sees a match or an invalid empty list")
	}
	refreshBody := fmt.Sprintf(`{"refresh_token":%q}`, a.session.RefreshToken)
	response = server.do(t, "/api/v1/auth/refresh", refreshBody, "")
	expect("POST", "/api/v1/auth/refresh", response, 200)
	rotated := decodeSession(t, response)
	expect("POST", "/api/v1/auth/refresh", server.do(t, "/api/v1/auth/refresh", refreshBody, ""), 401)
	logoutBody := fmt.Sprintf(`{"refresh_token":%q}`, rotated.RefreshToken)
	expect("POST", "/api/v1/auth/logout", server.do(t, "/api/v1/auth/logout", logoutBody, rotated.AccessToken), 204)
	expect("POST", "/api/v1/auth/refresh", server.do(t, "/api/v1/auth/refresh", logoutBody, ""), 401)
}
