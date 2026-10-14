package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
)

func TestProfileWithPostgres(t *testing.T) {
	server := newIntegrationServer(t)
	ctx := context.Background()
	expect := func(code, status int, step, body string) {
		t.Helper()
		if code != status {
			t.Fatalf("%s: expected %d, got %d: %s", step, status, code, body)
		}
	}
	register := func(email string) (string, string) {
		response := server.do(t, "/api/v1/auth/register", `{"email":"`+email+`","password":"password123"}`, "")
		expect(response.Code, http.StatusCreated, "register "+email, response.Body.String())
		var body struct {
			User    struct{ ID string }
			Session struct {
				AccessToken string `json:"access_token"`
			}
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.User.ID, body.Session.AccessToken
	}
	storedAge := func(userID string) *int {
		var age *int
		if err := server.pool.QueryRow(ctx, "SELECT age FROM users WHERE id = $1", userID).Scan(&age); err != nil {
			t.Fatal(err)
		}
		return age
	}

	userA, tokenA := register("a@example.com")
	if age := storedAge(userA); age != nil {
		t.Fatalf("age must be NULL before creating a profile, got %v", age)
	}
	response := server.get(t, "/api/v1/profile", tokenA)
	expect(response.Code, http.StatusNotFound, "profile right after registration", response.Body.String())

	for name, body := range map[string]string{
		"missing age": `{"name":"Анна","sex":"female","city":"Москва"}`,
		"null age":    `{"name":"Анна","age":null,"sex":"female","city":"Москва"}`,
		"age 121":     `{"name":"Анна","age":121,"sex":"female","city":"Москва"}`,
		"age 17":      `{"name":"Анна","age":17,"sex":"female","city":"Москва"}`,
		"sex other":   `{"name":"Анна","age":30,"sex":"other","city":"Москва"}`,
		"no city":     `{"name":"Анна","age":30,"sex":"female"}`,
		"blank name":  `{"name":"  ","age":30,"sex":"female","city":"Москва"}`,
		"long bio":    `{"name":"Анна","age":30,"sex":"female","city":"Москва","bio":"` + strings.Repeat("ж", 501) + `"}`,
	} {
		response := server.put(t, "/api/v1/profile", body, tokenA)
		expect(response.Code, http.StatusUnprocessableEntity, "invalid create: "+name, response.Body.String())
	}
	expect(server.get(t, "/api/v1/profile", tokenA).Code, http.StatusNotFound, "rejected creates leave no profile", "")
	expect(server.put(t, "/api/v1/profile", `{"name":"Анна","age":18.5,"sex":"female","city":"Москва"}`, tokenA).Code,
		http.StatusBadRequest, "fractional age", "")
	if age := storedAge(userA); age != nil {
		t.Fatalf("rejected creates must not change users.age, got %v", age)
	}

	response = server.put(t, "/api/v1/profile",
		`{"name":" Анна ","age":30,"sex":"female","city":"Москва","bio":"`+strings.Repeat("ж", 500)+`"}`, tokenA)
	expect(response.Code, http.StatusOK, "create profile", response.Body.String())
	var saved struct {
		UserID     string `json:"user_id"`
		Name       string `json:"name"`
		Age        int    `json:"age"`
		IsSleeping bool   `json:"is_sleeping"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.UserID != userA || saved.Name != "Анна" || saved.Age != 30 || saved.IsSleeping {
		t.Fatalf("unexpected saved profile: %+v", saved)
	}
	if age := storedAge(userA); age == nil || *age != 30 {
		t.Fatalf("users.age must be updated with the profile, got %v", age)
	}

	response = server.get(t, "/api/v1/profile", tokenA)
	expect(response.Code, http.StatusOK, "read profile", response.Body.String())
	for _, part := range []string{`"name":"Анна"`, `"age":30`, `"city":"Москва"`, `"preferences":null`, `"photos":[]`} {
		if !strings.Contains(response.Body.String(), part) {
			t.Fatalf("profile view must contain %s: %s", part, response.Body)
		}
	}

	if _, err := server.pool.Exec(ctx, "UPDATE profiles SET is_sleeping = true WHERE user_id = $1", userA); err != nil {
		t.Fatal(err)
	}
	response = server.put(t, "/api/v1/profile", `{"name":"Аня","age":31,"sex":"female","city":"Казань"}`, tokenA)
	expect(response.Code, http.StatusOK, "update profile", response.Body.String())
	if !strings.Contains(response.Body.String(), `"is_sleeping":true`) || !strings.Contains(response.Body.String(), `"bio":""`) {
		t.Fatalf("update must keep sleep mode and replace omitted bio: %s", response.Body)
	}
	before := server.get(t, "/api/v1/profile", tokenA).Body.String()
	expect(server.put(t, "/api/v1/profile", `{"name":"Аня","age":121,"sex":"female","city":"Казань"}`, tokenA).Code,
		http.StatusUnprocessableEntity, "invalid update", "")
	if after := server.get(t, "/api/v1/profile", tokenA).Body.String(); after != before || storedAge(userA) == nil || *storedAge(userA) != 31 {
		t.Fatalf("invalid update must not change anything:\nbefore %s\nafter  %s", before, after)
	}

	expect(server.put(t, "/api/v1/preferences", `{"min_age":25,"max_age":35,"sex":"any"}`, tokenA).Code,
		http.StatusOK, "set preferences", "")
	if body := server.get(t, "/api/v1/profile", tokenA).Body.String(); !strings.Contains(body, `"min_age":25,"max_age":35,"sex":"any"`) {
		t.Fatalf("stored preferences must be returned: %s", body)
	}

	userB, tokenB := register("b@example.com")
	expect(server.get(t, "/api/v1/profile", tokenB).Code, http.StatusNotFound, "another user does not see A's profile", "")

	const attempts = 8
	var wg sync.WaitGroup
	codes := make(chan int, attempts)
	for range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- server.put(t, "/api/v1/profile", `{"name":"Борис","age":40,"sex":"male","city":"Омск"}`, tokenB).Code
		}()
	}
	wg.Wait()
	close(codes)
	for code := range codes {
		expect(code, http.StatusOK, "concurrent first save", "")
	}
	var profiles int
	if err := server.pool.QueryRow(ctx, "SELECT count(*) FROM profiles WHERE user_id = $1", userB).Scan(&profiles); err != nil {
		t.Fatal(err)
	}
	if profiles != 1 {
		t.Fatalf("expected one profile for B, got %d", profiles)
	}
	for _, age := range []string{"18", "120"} {
		id, token := register("edge" + age + "@example.com")
		response := server.put(t, "/api/v1/profile", `{"name":"Анна","age":`+age+`,"sex":"female","city":"Москва"}`, token)
		expect(response.Code, http.StatusOK, "create profile at age boundary "+age, response.Body.String())
		if !strings.Contains(response.Body.String(), `"age":`+age) || storedAge(id) == nil {
			t.Fatalf("age %s was not saved with the first profile: %s", age, response.Body)
		}
	}
}
