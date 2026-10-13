// Package contracttest checks real HTTP responses against api/openapi.yaml.
// It validates the declared status, the JSON content type, the body schema and
// declared response headers that a test asks for. It supports the JSON Schema
// subset used by the contract: $ref, type (including "null"), properties,
// required, additionalProperties, items, enum, anyOf/oneOf/allOf, string
// length/pattern/format (uuid, date-time, uri) and numeric bounds.
package contracttest

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

type Spec struct {
	root map[string]any
}

// Load reads api/openapi.yaml from the module root.
func Load(t testing.TB) *Spec {
	t.Helper()
	path, err := specPath()
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := yaml.Unmarshal(content, &root); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return &Spec{root: root}
}

func specPath() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "api", "openapi.yaml"), nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above the test directory")
		}
		dir = parent
	}
}

// CheckResponse fails the test if the response does not match the operation
// for method and pathTemplate, for example ("GET", "/api/v1/matches").
func (s *Spec) CheckResponse(t testing.TB, method, pathTemplate string, response *httptest.ResponseRecorder) {
	t.Helper()
	if err := s.Validate(method, pathTemplate, response); err != nil {
		t.Fatalf("%s %s -> %d does not match OpenAPI: %v\nbody: %s", method, pathTemplate, response.Code, err, response.Body)
	}
}

// Validate is CheckResponse returning the problem instead of failing.
func (s *Spec) Validate(method, pathTemplate string, response *httptest.ResponseRecorder) error {
	operation, err := s.operation(method, pathTemplate)
	if err != nil {
		return err
	}
	responses, _ := operation["responses"].(map[string]any)
	declared, ok := responses[fmt.Sprint(response.Code)]
	if !ok {
		declared, ok = responses["default"]
	}
	if !ok {
		return fmt.Errorf("status %d is not declared; declared: %s", response.Code, strings.Join(keys(responses), ", "))
	}
	resolved, err := s.resolve(declared)
	if err != nil {
		return err
	}
	if err := s.checkHeaders(resolved, response.Header()); err != nil {
		return err
	}

	content, _ := resolved["content"].(map[string]any)
	if len(content) == 0 {
		if response.Body.Len() != 0 {
			return fmt.Errorf("no body is declared, got %d bytes", response.Body.Len())
		}
		return nil
	}
	media, ok := content["application/json"].(map[string]any)
	if !ok {
		return fmt.Errorf("only application/json bodies are supported, declared: %s", strings.Join(keys(content), ", "))
	}
	if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		return fmt.Errorf("Content-Type must be application/json, got %q", contentType)
	}
	decoder := json.NewDecoder(strings.NewReader(response.Body.String()))
	decoder.UseNumber()
	var body any
	if err := decoder.Decode(&body); err != nil {
		return fmt.Errorf("body is not JSON: %w", err)
	}
	return s.validate(media["schema"], body, "body")
}

// checkHeaders requires every declared response header to be present: in this
// contract they are declared only where the server must send them.
func (s *Spec) checkHeaders(response map[string]any, header http.Header) error {
	headers, _ := response["headers"].(map[string]any)
	for name := range headers {
		if header.Get(name) == "" {
			return fmt.Errorf("declared header %s is missing", name)
		}
	}
	return nil
}

func (s *Spec) operation(method, pathTemplate string) (map[string]any, error) {
	paths, _ := s.root["paths"].(map[string]any)
	item, ok := paths[pathTemplate].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("path %s is not in OpenAPI", pathTemplate)
	}
	operation, ok := item[strings.ToLower(method)].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s %s is not in OpenAPI", method, pathTemplate)
	}
	return operation, nil
}

// Operations lists every "METHOD /path" with its operationId.
func (s *Spec) Operations() map[string]string {
	result := map[string]string{}
	paths, _ := s.root["paths"].(map[string]any)
	for path, raw := range paths {
		item, _ := raw.(map[string]any)
		for method, rawOperation := range item {
			operation, ok := rawOperation.(map[string]any)
			if !ok {
				continue
			}
			id, _ := operation["operationId"].(string)
			result[strings.ToUpper(method)+" "+path] = id
		}
	}
	return result
}

// BrokenRefs returns every local $ref that does not resolve.
func (s *Spec) BrokenRefs() []string {
	var broken []string
	var walk func(node any)
	walk = func(node any) {
		switch value := node.(type) {
		case map[string]any:
			if ref, ok := value["$ref"].(string); ok {
				if _, err := s.lookup(ref); err != nil {
					broken = append(broken, ref)
				}
			}
			for _, child := range value {
				walk(child)
			}
		case []any:
			for _, child := range value {
				walk(child)
			}
		}
	}
	walk(s.root)
	sort.Strings(broken)
	return broken
}

func (s *Spec) lookup(ref string) (map[string]any, error) {
	if !strings.HasPrefix(ref, "#/") {
		return nil, fmt.Errorf("only local $ref is supported: %s", ref)
	}
	var node any = s.root
	for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		object, ok := node.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("unresolved $ref %s", ref)
		}
		if node, ok = object[part]; !ok {
			return nil, fmt.Errorf("unresolved $ref %s", ref)
		}
	}
	object, ok := node.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("$ref %s is not an object", ref)
	}
	return object, nil
}

// resolve follows $ref chains.
func (s *Spec) resolve(node any) (map[string]any, error) {
	object, ok := node.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected an object, got %T", node)
	}
	for depth := 0; depth < 32; depth++ {
		ref, ok := object["$ref"].(string)
		if !ok {
			return object, nil
		}
		next, err := s.lookup(ref)
		if err != nil {
			return nil, err
		}
		object = next
	}
	return nil, fmt.Errorf("$ref chain is too deep")
}

func (s *Spec) validate(rawSchema, value any, path string) error {
	if rawSchema == nil {
		return nil
	}
	schema, err := s.resolve(rawSchema)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	if all, ok := schema["allOf"].([]any); ok {
		for _, part := range all {
			if err := s.validate(part, value, path); err != nil {
				return err
			}
		}
	}
	for _, keyword := range []string{"anyOf", "oneOf"} {
		options, ok := schema[keyword].([]any)
		if !ok {
			continue
		}
		matched := 0
		var last error
		for _, option := range options {
			if err := s.validate(option, value, path); err != nil {
				last = err
			} else {
				matched++
			}
		}
		if matched == 0 || (keyword == "oneOf" && matched > 1) {
			return fmt.Errorf("%s: %s matched %d schemas (last error: %v)", path, keyword, matched, last)
		}
	}
	if enum, ok := schema["enum"].([]any); ok && !inEnum(enum, value) {
		return fmt.Errorf("%s: %v is not one of %v", path, value, enum)
	}
	if constant, ok := schema["const"]; ok && fmt.Sprint(constant) != fmt.Sprint(value) {
		return fmt.Errorf("%s: must be %v", path, constant)
	}

	types := schemaTypes(schema["type"])
	if len(types) == 0 {
		return nil
	}
	actual := jsonType(value)
	if !types[actual] && !(actual == "integer" && types["number"]) {
		return fmt.Errorf("%s: expected type %v, got %s (%v)", path, keys(types), actual, value)
	}
	switch actual {
	case "object":
		return s.validateObject(schema, value.(map[string]any), path)
	case "array":
		items := value.([]any)
		for i, item := range items {
			if err := s.validate(schema["items"], item, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
		if minItems, ok := number(schema["minItems"]); ok && float64(len(items)) < minItems {
			return fmt.Errorf("%s: fewer than %v items", path, minItems)
		}
		if maxItems, ok := number(schema["maxItems"]); ok && float64(len(items)) > maxItems {
			return fmt.Errorf("%s: more than %v items", path, maxItems)
		}
	case "string":
		return validateString(schema, value.(string), path)
	case "integer", "number":
		n, _ := value.(json.Number).Float64()
		if minimum, ok := number(schema["minimum"]); ok && n < minimum {
			return fmt.Errorf("%s: %v is below %v", path, n, minimum)
		}
		if maximum, ok := number(schema["maximum"]); ok && n > maximum {
			return fmt.Errorf("%s: %v is above %v", path, n, maximum)
		}
	}
	return nil
}

func (s *Spec) validateObject(schema, object map[string]any, path string) error {
	properties, _ := schema["properties"].(map[string]any)
	required, _ := schema["required"].([]any)
	for _, name := range required {
		if _, ok := object[name.(string)]; !ok {
			return fmt.Errorf("%s: required property %q is missing", path, name)
		}
	}
	for _, name := range sortedKeys(object) {
		child := path + "." + name
		if propertySchema, ok := properties[name]; ok {
			if err := s.validate(propertySchema, object[name], child); err != nil {
				return err
			}
			continue
		}
		switch additional := schema["additionalProperties"].(type) {
		case bool:
			if !additional {
				return fmt.Errorf("%s: property %q is not allowed", path, name)
			}
		case map[string]any:
			if err := s.validate(additional, object[name], child); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateString(schema map[string]any, value, path string) error {
	length := float64(utf8.RuneCountInString(value))
	if minLength, ok := number(schema["minLength"]); ok && length < minLength {
		return fmt.Errorf("%s: shorter than %v characters", path, minLength)
	}
	if maxLength, ok := number(schema["maxLength"]); ok && length > maxLength {
		return fmt.Errorf("%s: longer than %v characters", path, maxLength)
	}
	if pattern, ok := schema["pattern"].(string); ok {
		matched, err := regexp.MatchString(pattern, value)
		if err != nil || !matched {
			return fmt.Errorf("%s: %q does not match %s", path, value, pattern)
		}
	}
	switch schema["format"] {
	case "uuid":
		if parsed, err := uuid.Parse(value); err != nil || parsed.String() != strings.ToLower(value) {
			return fmt.Errorf("%s: %q is not a UUID", path, value)
		}
	case "date-time":
		if _, err := time.Parse(time.RFC3339Nano, value); err != nil {
			return fmt.Errorf("%s: %q is not an RFC 3339 date-time", path, value)
		}
	case "uri":
		if parsed, err := url.Parse(value); err != nil || parsed.Scheme == "" {
			return fmt.Errorf("%s: %q is not an absolute URI", path, value)
		}
	}
	return nil
}

func schemaTypes(raw any) map[string]bool {
	types := map[string]bool{}
	switch value := raw.(type) {
	case string:
		types[value] = true
	case []any:
		for _, item := range value {
			types[fmt.Sprint(item)] = true
		}
	}
	return types
}

func jsonType(value any) string {
	switch typed := value.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	case json.Number:
		if f, err := typed.Float64(); err == nil && f == math.Trunc(f) && !strings.ContainsAny(typed.String(), ".eE") {
			return "integer"
		}
		return "number"
	}
	return fmt.Sprintf("%T", value)
}

func inEnum(enum []any, value any) bool {
	for _, item := range enum {
		if fmt.Sprint(item) == fmt.Sprint(value) {
			return true
		}
	}
	return false
}

func number(raw any) (float64, bool) {
	switch value := raw.(type) {
	case int:
		return float64(value), true
	case float64:
		return value, true
	}
	return 0, false
}

func keys[V any](m map[string]V) []string {
	result := make([]string, 0, len(m))
	for key := range m {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func sortedKeys(m map[string]any) []string {
	return keys(m)
}
