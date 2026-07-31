package probe

import (
	"bytes"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
)

const (
	redactedValue  = "<REDACTED>"
	redactedToken  = "<TOKEN>"
	redactedEmail  = "<MEMBER_EMAIL>"
	redactedTenant = "<TENANT>"
)

var (
	emailPattern    = regexp.MustCompile(`(?i)\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b`)
	apiTokenPattern = regexp.MustCompile(`\bapi-[A-Za-z0-9_-]{8,}\b`)
	jwtPattern      = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b`)
)

// RedactHeaders returns a copy suitable for diagnostic output.
func RedactHeaders(headers http.Header) http.Header {
	out := make(http.Header, len(headers))
	for name, values := range headers {
		if sensitiveHeader(name) {
			out[name] = []string{redactedValue}
			continue
		}
		out[name] = append([]string(nil), values...)
	}
	return out
}

// RedactJSON preserves the response shape while removing secrets, tenant
// context, and member email addresses. Invalid JSON is treated as plain text.
func RedactJSON(input []byte) []byte {
	var value interface{}
	dec := json.NewDecoder(bytes.NewReader(input))
	dec.UseNumber()
	if err := dec.Decode(&value); err != nil {
		return []byte(RedactText(string(input)))
	}
	value = redactValue(value, "")
	output, err := json.Marshal(value)
	if err != nil {
		return []byte(redactedValue)
	}
	return output
}

func RedactText(input string) string {
	input = apiTokenPattern.ReplaceAllString(input, redactedToken)
	input = jwtPattern.ReplaceAllString(input, redactedToken)
	input = emailPattern.ReplaceAllString(input, redactedEmail)
	return input
}

func redactValue(value interface{}, parentKey string) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		secretObject := normalizeKey(parentKey) == "secrets" || looksLikeSecretObject(typed)
		for key, child := range typed {
			normalized := normalizeKey(key)
			switch {
			case sensitiveJSONKey(normalized):
				out[key] = redactedValue
			case tenantJSONKey(normalized):
				out[key] = redactedTenant
			case normalized == "email":
				out[key] = redactedEmail
			case normalized == "value" && secretObject:
				out[key] = redactedValue
			default:
				out[key] = redactValue(child, key)
			}
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(typed))
		for i, child := range typed {
			out[i] = redactValue(child, parentKey)
		}
		return out
	case string:
		return RedactText(typed)
	default:
		return value
	}
}

func sensitiveHeader(name string) bool {
	switch strings.ToLower(name) {
	case "authorization", "cookie", "set-cookie", "proxy-authorization", "x-api-key":
		return true
	default:
		return false
	}
}

func sensitiveJSONKey(normalized string) bool {
	if strings.Contains(normalized, "password") ||
		strings.HasSuffix(normalized, "token") ||
		normalized == "clientkey" ||
		normalized == "serverkey" {
		return true
	}
	switch normalized {
	case "authorization", "token", "accesstoken", "refreshtoken", "password",
		"cookie", "clientsecret", "serversecret", "apikey":
		return true
	default:
		return false
	}
}

func tenantJSONKey(normalized string) bool {
	switch normalized {
	case "tenantid", "organizationid", "workspaceid":
		return true
	default:
		return false
	}
}

func looksLikeSecretObject(value map[string]interface{}) bool {
	_, hasType := value["type"]
	name, hasName := value["name"].(string)
	if !hasType || !hasName {
		return false
	}
	name = strings.ToLower(name)
	return strings.Contains(name, "key") || strings.Contains(name, "secret")
}

func normalizeKey(value string) string {
	replacer := strings.NewReplacer("_", "", "-", "", ".", "")
	return strings.ToLower(replacer.Replace(value))
}
