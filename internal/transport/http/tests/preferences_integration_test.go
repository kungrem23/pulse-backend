package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
)

func TestPreferencesAndSleepWithPostgres(t *testing.T) {
	server := newIntegrationServer(t)
	expect := func(code, status int, step, body string) {
		t.Helper()
		if code != status {
			t.Fatalf("%s: expected %d, got %d: %s", step, status, code, body)
		}
	}
	register := func(email string) string {
		response := server.do(t, "/api/v1/auth/register", `{"email":"`+email+`","password":"password123"}`, "")
		expect(response.Code, http.StatusCreated, "register", response.Body.String())
		var body struct {
			Session struct {
				AccessToken string `json:"access_token"`
			}
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.Session.AccessToken
	}
	profileBody := func(token string) string {
		response := server.get(t, "/api/v1/profile", token)
		expect(response.Code, http.StatusOK, "read profile", response.Body.String())
		return response.Body.String()
	}

	token := register("a@example.com")
	for path, request := range map[string]func() int{
		"preferences": func() int {
			return server.put(t, "/api/v1/preferences", `{"min_age":20,"max_age":30,"sex":"any"}`, token).Code
		},
		"sleep": func() int { return server.patch(t, "/api/v1/profile/sleep", `{"is_sleeping":true}`, token).Code },
	} {
		expect(request(), http.StatusConflict, path+" before a profile exists", "")
	}

	expect(server.put(t, "/api/v1/profile", `{"name":"Анна","age":25,"sex":"female","city":"Москва"}`, token).Code,
		http.StatusOK, "create profile", "")
	for name, body := range map[string]string{
		"min above max": `{"min_age":31,"max_age":30,"sex":"any"}`,
		"min 17":        `{"min_age":17,"max_age":30,"sex":"any"}`,
		"max 121":       `{"min_age":18,"max_age":121,"sex":"any"}`,
		"sex other":     `{"min_age":18,"max_age":30,"sex":"other"}`,
	} {
		response := server.put(t, "/api/v1/preferences", body, token)
		expect(response.Code, http.StatusUnprocessableEntity, "invalid preferences: "+name, response.Body.String())
	}
	if !strings.Contains(profileBody(token), `"preferences":null`) {
		t.Fatal("rejected preferences must not be stored")
	}

	response := server.put(t, "/api/v1/preferences", `{"min_age":18,"max_age":120,"sex":"any"}`, token)
	expect(response.Code, http.StatusOK, "set boundary preferences", response.Body.String())
	if !strings.Contains(response.Body.String(), `"min_age":18,"max_age":120,"sex":"any"`) {
		t.Fatalf("unexpected preferences response: %s", response.Body)
	}
	expect(server.put(t, "/api/v1/preferences", `{"min_age":25,"max_age":25,"sex":"male"}`, token).Code,
		http.StatusOK, "replace preferences", "")
	if body := profileBody(token); !strings.Contains(body, `"min_age":25,"max_age":25,"sex":"male"`) {
		t.Fatalf("replaced preferences must be returned: %s", body)
	}

	response = server.patch(t, "/api/v1/profile/sleep", `{"is_sleeping":true}`, token)
	expect(response.Code, http.StatusOK, "fall asleep", response.Body.String())
	if strings.TrimSpace(response.Body.String()) != `{"is_sleeping":true}` {
		t.Fatalf("unexpected sleep response: %s", response.Body)
	}
	expect(server.patch(t, "/api/v1/profile/sleep", `{}`, token).Code, http.StatusBadRequest, "sleep without a value", "")
	if !strings.Contains(profileBody(token), `"is_sleeping":true`) {
		t.Fatal("sleep mode must survive a restart and a rejected request")
	}
	expect(server.put(t, "/api/v1/profile", `{"name":"Анна","age":26,"sex":"female","city":"Москва"}`, token).Code,
		http.StatusOK, "edit profile while asleep", "")
	if !strings.Contains(profileBody(token), `"is_sleeping":true`) {
		t.Fatal("editing the profile must not wake it up")
	}
	expect(server.patch(t, "/api/v1/profile/sleep", `{"is_sleeping":false}`, token).Code, http.StatusOK, "wake up", "")
	if !strings.Contains(profileBody(token), `"is_sleeping":false`) {
		t.Fatal("waking up must be stored")
	}

	other := register("b@example.com")
	expect(server.put(t, "/api/v1/profile", `{"name":"Борис","age":30,"sex":"male","city":"Омск"}`, other).Code,
		http.StatusOK, "create second profile", "")
	const attempts = 8
	var wg sync.WaitGroup
	codes := make(chan int, attempts)
	for range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- server.put(t, "/api/v1/preferences", `{"min_age":20,"max_age":40,"sex":"female"}`, other).Code
		}()
	}
	wg.Wait()
	close(codes)
	for code := range codes {
		expect(code, http.StatusOK, "concurrent first preferences save", "")
	}
	if body := profileBody(token); !strings.Contains(body, `"sex":"male"`) || strings.Contains(body, `"max_age":40`) {
		t.Fatalf("another user's preferences must not affect A: %s", body)
	}
}
