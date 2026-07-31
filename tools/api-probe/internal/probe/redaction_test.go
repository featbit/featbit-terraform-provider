package probe

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
)

func TestRedactHeaders(t *testing.T) {
	t.Parallel()

	headers := http.Header{
		"Authorization": []string{"api-synthetic-secret-token"},
		"Cookie":        []string{"session=synthetic-cookie"},
		"Content-Type":  []string{"application/json"},
	}
	redacted := RedactHeaders(headers)
	rendered := redacted.Get("Authorization") + redacted.Get("Cookie")
	if strings.Contains(rendered, "synthetic") {
		t.Fatalf("sensitive header leaked: %q", rendered)
	}
	if got := redacted.Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := headers.Get("Authorization"); got == redactedValue {
		t.Fatal("RedactHeaders mutated its input")
	}
}

func TestRedactJSON(t *testing.T) {
	t.Parallel()

	input := []byte(`{
		"token":"api-synthetic-secret-token",
		"initialPassword":"synthetic-initial-password",
		"serverKey":"synthetic-server-key",
		"organizationId":"private-org-id",
		"email":"real.person@example.test",
		"secrets":[{"id":"safe-test-id","name":"Server Key","type":"Server","value":"environment-secret-value"}],
		"data":{"description":"contact other.person@example.test","id":"safe-resource-id"}
	}`)
	output := RedactJSON(input)
	for _, forbidden := range [][]byte{
		[]byte("api-synthetic-secret-token"),
		[]byte("synthetic-initial-password"),
		[]byte("synthetic-server-key"),
		[]byte("private-org-id"),
		[]byte("real.person@example.test"),
		[]byte("other.person@example.test"),
		[]byte("environment-secret-value"),
	} {
		if bytes.Contains(output, forbidden) {
			t.Fatalf("redacted JSON leaked %q: %s", forbidden, output)
		}
	}
	for _, expected := range [][]byte{
		[]byte("REDACTED"),
		[]byte("TENANT"),
		[]byte("MEMBER_EMAIL"),
		[]byte("safe-test-id"),
		[]byte("safe-resource-id"),
	} {
		if !bytes.Contains(output, expected) {
			t.Fatalf("redacted JSON omitted %q: %s", expected, output)
		}
	}
}

func TestRedactTextTokenJWTAndEmail(t *testing.T) {
	t.Parallel()

	input := "api-abcdefghijk eyJheader.payload.signature person@example.test"
	output := RedactText(input)
	if strings.Contains(output, "api-abcdefghijk") ||
		strings.Contains(output, "eyJheader.payload.signature") ||
		strings.Contains(output, "person@example.test") {
		t.Fatalf("RedactText leaked sensitive input: %q", output)
	}
}
