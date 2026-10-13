package httpx

import (
	"net/http/httptest"
	"testing"

	"github.com/kungrem23/pulse-backend/internal/domain"
)

func TestParsePageRequest(t *testing.T) {
	for _, test := range []struct {
		query string
		want  domain.PageRequest
	}{
		{"", domain.PageRequest{}},
		{"?limit=1", domain.PageRequest{Limit: 1}},
		{"?limit=100", domain.PageRequest{Limit: 100}},
		{"?limit=101", domain.PageRequest{Limit: 101}}, // range is checked by app/pagination
		{"?cursor=abc&limit=20", domain.PageRequest{Cursor: "abc", Limit: 20}},
	} {
		got, err := ParsePageRequest(httptest.NewRequest("GET", "/list"+test.query, nil))
		if err != nil || got != test.want {
			t.Fatalf("%q: expected %+v, got %+v, %v", test.query, test.want, got, err)
		}
	}
}

func TestParsePageRequestRejectsExplicitInvalidValues(t *testing.T) {
	for _, query := range []string{
		"?other=1", "?limit=1;cursor=a",
		"?cursor=", "?cursor=a&cursor=b",
		"?limit=", "?limit=0", "?limit=000", "?limit=-1", "?limit=+5", "?limit=1.5", "?limit=1e2",
		"?limit=%201", "?limit=abc", "?limit=1&limit=2", "?limit=99999999999999999999",
	} {
		if _, err := ParsePageRequest(httptest.NewRequest("GET", "/list"+query, nil)); err == nil {
			t.Fatalf("%q must be rejected", query)
		}
	}
}

func TestParsePageRequestRejectsMalformedEncoding(t *testing.T) {
	request := httptest.NewRequest("GET", "/list", nil)
	request.URL.RawQuery = "limit=%zz"
	if _, err := ParsePageRequest(request); err == nil {
		t.Fatal("malformed query must not silently become the first page")
	}
}
