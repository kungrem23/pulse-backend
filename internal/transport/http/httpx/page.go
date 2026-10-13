package httpx

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/kungrem23/pulse-backend/internal/domain"
)

var errInvalidPageQuery = errors.New("invalid pagination parameters")

// ParsePageRequest reads the optional cursor and limit query parameters.
// Range checks and cursor decoding belong to app/pagination; here an omitted
// parameter is the zero value and every explicitly given one must be usable:
// an empty or repeated cursor, and a limit that is repeated, not a plain
// decimal number or 0, are rejected.
func ParsePageRequest(r *http.Request) (domain.PageRequest, error) {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return domain.PageRequest{}, errInvalidPageQuery
	}
	for key := range query {
		if key != "cursor" && key != "limit" {
			return domain.PageRequest{}, errInvalidPageQuery
		}
	}
	var request domain.PageRequest

	if values, ok := query["cursor"]; ok {
		if len(values) != 1 || values[0] == "" || len(values[0]) > 512 {
			return domain.PageRequest{}, errInvalidPageQuery
		}
		request.Cursor = values[0]
	}
	if values, ok := query["limit"]; ok {
		if len(values) != 1 || !plainNumber(values[0]) {
			return domain.PageRequest{}, errInvalidPageQuery
		}
		limit, err := strconv.Atoi(values[0])
		if err != nil || limit == 0 {
			return domain.PageRequest{}, errInvalidPageQuery
		}
		request.Limit = limit
	}
	return request, nil
}

// plainNumber accepts 1-3 ASCII digits: no sign, spaces or exponent. Longer
// values are out of range anyway and would only risk overflow.
func plainNumber(value string) bool {
	if value == "" || len(value) > 3 {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}
