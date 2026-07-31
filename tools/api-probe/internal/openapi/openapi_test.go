package openapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

var tinySnapshot = []byte(`{
  "openapi": "3.0.4",
  "info": {"title": "test", "version": "1.0"},
  "paths": {
    "/api/v1/projects": {
      "get": {"tags": ["Project"], "responses": {"200": {"description": "ok"}}},
      "post": {"tags": ["Project"], "responses": {"200": {"description": "ok"}}}
    }
  },
  "components": {
    "schemas": {
      "Project": {"type": "object", "properties": {"id": {"type": "string"}, "key": {"type": "string"}}}
    },
    "securitySchemes": {"AccessToken": {"type": "apiKey", "in": "header", "name": "Authorization"}}
  }
}`)

var tinyOverlay = []byte(`{
  "overlay": "1.1.0",
  "info": {"title": "test operation IDs", "version": "1.0.0"},
  "actions": [
    {"target": "$.paths['/api/v1/projects'].get", "update": {"operationId": "listProjects"}},
    {"target": "$.paths['/api/v1/projects'].post", "update": {"operationId": "createProject"}}
  ]
}`)

func TestGenerateIsDeterministicAndInventoriesOperationIDs(t *testing.T) {
	t.Parallel()

	firstSpec, firstInventory, inventory, err := Generate(tinySnapshot, tinyOverlay)
	if err != nil {
		t.Fatal(err)
	}
	secondSpec, secondInventory, _, err := Generate(tinySnapshot, tinyOverlay)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstSpec, secondSpec) || !bytes.Equal(firstInventory, secondInventory) {
		t.Fatal("Generate produced nondeterministic output")
	}
	if inventory.PathCount != 1 || inventory.OperationCount != 2 ||
		inventory.SchemaCount != 1 || inventory.SchemaPropertyCount != 2 ||
		inventory.SchemasWithRequiredCount != 0 || inventory.PropertiesWithEnumCount != 0 ||
		inventory.ResponseStatusCounts["200"] != 2 ||
		inventory.MissingOperationIDCount != 0 {
		t.Fatalf("unexpected inventory: %+v", inventory)
	}
	if !bytes.Contains(firstSpec, []byte(`"operationId": "listProjects"`)) {
		t.Fatalf("overlaid document is missing operationId: %s", firstSpec)
	}
}

func TestGenerateRejectsMissingAndDuplicateOperationIDs(t *testing.T) {
	t.Parallel()

	missingOverlay := []byte(`{
	  "overlay":"1.1.0",
	  "info":{"title":"missing","version":"1.0.0"},
	  "actions":[{"target":"$.paths['/api/v1/projects'].get","update":{"operationId":"listProjects"}}]
	}`)
	if _, _, _, err := Generate(tinySnapshot, missingOverlay); err == nil {
		t.Fatal("Generate accepted an operation without operationId")
	}

	duplicateOverlay := []byte(`{
	  "overlay":"1.1.0",
	  "info":{"title":"duplicate","version":"1.0.0"},
	  "actions":[
	    {"target":"$.paths['/api/v1/projects'].get","update":{"operationId":"projects"}},
	    {"target":"$.paths['/api/v1/projects'].post","update":{"operationId":"projects"}}
	  ]
	}`)
	if _, _, _, err := Generate(tinySnapshot, duplicateOverlay); err == nil {
		t.Fatal("Generate accepted duplicate operationId values")
	}
}

func TestPinVerifiesLockedBytesBeforeWriting(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write(tinySnapshot)
	}))
	defer server.Close()

	lock := Lock{SourceURL: server.URL, SHA256: SHA256(tinySnapshot), Bytes: len(tinySnapshot)}
	client := &http.Client{Timeout: time.Second}
	path := t.TempDir() + "/snapshot.json"
	if err := Pin(context.Background(), client, lock, path); err != nil {
		t.Fatal(err)
	}

	lock.SHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	if err := Pin(context.Background(), client, lock, path); err == nil {
		t.Fatal("Pin accepted bytes that did not match the locked SHA-256")
	}
}
