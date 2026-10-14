package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kungrem23/pulse-backend/internal/domain"
	"github.com/kungrem23/pulse-backend/internal/repository/postgres"
	"github.com/kungrem23/pulse-backend/internal/transport/http/httpx/contracttest"
)

// Real JWT/auth/profile/swipe/match repositories
// and the full router are used. URL signing alone is a test adapter; real S3
// is covered separately by the T08 PostgreSQL/MinIO HTTP test.
func TestMatchingHTTPWithPostgres(t *testing.T) {
	server := newIntegrationServer(t)
	server.storage = &photoFlowStorage{}
	spec := contracttest.Load(t)
	expect := func(method, path string, response *httptest.ResponseRecorder, status int) {
		t.Helper()
		if response.Code != status {
			t.Fatalf("%s %s: expected %d, got %d: %s", method, path, status, response.Code, response.Body)
		}
		spec.CheckResponse(t, method, path, response)
	}
	person := func(name string, sex domain.Sex) (string, string) {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"email": uuid.NewString() + "@example.test", "password": "password123"})
		response := server.do(t, "/api/v1/auth/register", string(body), "")
		expect("POST", "/api/v1/auth/register", response, http.StatusCreated)
		var registered struct {
			User struct {
				ID string `json:"id"`
			} `json:"user"`
			Session struct {
				AccessToken string `json:"access_token"`
			} `json:"session"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &registered); err != nil {
			t.Fatal(err)
		}
		id, token := registered.User.ID, registered.Session.AccessToken
		body, _ = json.Marshal(map[string]any{"name": name, "age": 25, "sex": sex, "city": "Москва"})
		expect("PUT", "/api/v1/profile", server.put(t, "/api/v1/profile", string(body), token), 200)
		expect("PUT", "/api/v1/preferences", server.put(t, "/api/v1/preferences", `{"min_age":18,"max_age":120,"sex":"any"}`, token), 200)
		photoID := uuid.NewString()
		if err := postgres.NewPhotoRepository(server.pool).Create(context.Background(), domain.Photo{ID: photoID, UserID: id, ObjectKey: "private-test-key/" + photoID, Position: 1}); err != nil {
			t.Fatal(err)
		}
		return id, token
	}
	like := func(token, target string, status int, matched bool) string {
		t.Helper()
		response := server.do(t, "/api/v1/swipes", fmt.Sprintf(`{"to_user_id":%q,"is_like":true}`, target), token)
		expect("POST", "/api/v1/swipes", response, status)
		if status != 200 {
			return ""
		}
		var result struct {
			Matched bool   `json:"matched"`
			MatchID string `json:"match_id"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Matched != matched || (result.MatchID != "") != matched {
			t.Fatalf("unexpected swipe: %s", response.Body)
		}
		if strings.Contains(response.Body.String(), "conversation") {
			t.Fatal("MVP swipe created a conversation")
		}
		return result.MatchID
	}
	type item struct {
		ID   string `json:"id"`
		User struct {
			ID       string `json:"id"`
			PhotoURL string `json:"photo_url"`
		} `json:"user"`
	}
	list := func(token, cursor string, limit int) ([]item, string) {
		t.Helper()
		path := fmt.Sprintf("/api/v1/matches?limit=%d", limit)
		if cursor != "" {
			path += "&cursor=" + url.QueryEscape(cursor)
		}
		response := server.get(t, path, token)
		expect("GET", "/api/v1/matches", response, 200)
		for _, secret := range []string{"object_key", "private-test-key", "password", "person1_id", "conversation_id"} {
			if strings.Contains(response.Body.String(), secret) {
				t.Fatalf("match response leaks %s", secret)
			}
		}
		var page struct {
			Items      []item `json:"items"`
			NextCursor string `json:"next_cursor"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if page.Items == nil {
			t.Fatal("items must be an array")
		}
		return page.Items, page.NextCursor
	}

	a, tokenA := person("Анна", domain.SexFemale)
	b, tokenB := person("Борис", domain.SexMale)
	c, tokenC := person("Чужой", domain.SexMale)
	if items, cursor := list(tokenC, "", 1); len(items) != 0 || cursor != "" {
		t.Fatal("new user has matches")
	}
	like(tokenA, b, 200, false)
	firstMatch := like(tokenB, a, 200, true)
	like(tokenA, b, 409, false)
	like(tokenB, a, 409, false)
	for _, side := range []struct{ token, partner string }{{tokenA, b}, {tokenB, a}} {
		items, cursor := list(side.token, "", 1)
		if len(items) != 1 || items[0].ID != firstMatch || items[0].User.ID != side.partner || items[0].User.PhotoURL == "" || cursor != "" {
			t.Fatalf("wrong own match: %+v", items)
		}
	}
	if items, _ := list(tokenC, "", 1); len(items) != 0 {
		t.Fatal("third user sees someone else's match")
	}

	d, tokenD := person("Денис", domain.SexMale)
	like(tokenA, d, 200, false)
	secondMatch := like(tokenD, a, 200, true)
	items, cursor := list(tokenA, "", 1)
	if len(items) != 1 || cursor == "" {
		t.Fatal("missing next page")
	}
	seen := map[string]bool{items[0].ID: true}
	items, next := list(tokenA, cursor, 1)
	if len(items) != 1 || next != "" || seen[items[0].ID] {
		t.Fatal("pagination repeats/skips matches")
	}
	seen[items[0].ID] = true
	if !seen[firstMatch] || !seen[secondMatch] {
		t.Fatal("pagination lost a pair")
	}
	wrongOwner := server.get(t, "/api/v1/matches?cursor="+url.QueryEscape(cursor), tokenC)
	expect("GET", "/api/v1/matches", wrongOwner, 400)

	expect("PATCH", "/api/v1/profile/sleep", server.patch(t, "/api/v1/profile/sleep", `{"is_sleeping":true}`, tokenA), 200)
	like(tokenA, c, 403, false)
	if items, _ := list(tokenA, "", 100); len(items) != 2 {
		t.Fatal("sleeping caller lost existing matches")
	}
	if items, _ := list(tokenB, "", 100); len(items) != 1 || items[0].User.ID != a {
		t.Fatal("sleeping partner disappeared")
	}
	expect("GET", "/api/v1/matches", server.get(t, "/api/v1/matches", ""), 401)
	closePath := "/api/v1/matches/" + firstMatch
	expect("DELETE", "/api/v1/matches/{match_id}", server.request(t, "DELETE", closePath, "", tokenA), 501)
}
