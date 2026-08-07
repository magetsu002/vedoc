package generator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/RaniduNethma/vedoc/internal/models"
)

func TestGenerateSwaggerConvertsExpressPathParameters(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "openapi.json")
	endpoints := []models.Endpoint{
		{Method: "GET", Path: "/users/:id", Resolution: models.ResolutionResolved},
		{Method: "GET", LocalPath: "/orphan", Resolution: models.ResolutionUnresolved},
	}
	if err := GenerateSwagger(endpoints, filename); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	var doc OpenAPI
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	operation, ok := doc.Paths["/users/{id}"]["get"]
	if !ok {
		t.Fatalf("OpenAPI paths = %#v, expected GET /users/{id}", doc.Paths)
	}
	if _, exists := doc.Paths["/users/:id"]; exists {
		t.Fatal("Express colon path leaked into OpenAPI output")
	}
	if len(operation.Parameters) != 1 || operation.Parameters[0].Name != "id" || !operation.Parameters[0].Required {
		t.Fatalf("parameters = %#v, want required path parameter id", operation.Parameters)
	}
	if _, exists := operation.Responses["200"]; exists {
		t.Fatal("generator invented a 200 response")
	}
	if _, exists := operation.Responses["default"]; !exists {
		t.Fatal("generator should declare response behavior as unknown")
	}
}

func TestGeneratePostmanDefinesBaseURLAndSkipsUnresolved(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "postman.json")
	endpoints := []models.Endpoint{
		{Method: "GET", Path: "/users/:id", Resolution: models.ResolutionResolved},
		{Method: "POST", LocalPath: "/orphan", Resolution: models.ResolutionUnresolved},
	}
	if err := GeneratePostmanCollection(endpoints, filename); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	var collection postmanCollection
	if err := json.Unmarshal(data, &collection); err != nil {
		t.Fatal(err)
	}
	if len(collection.Variable) != 1 || collection.Variable[0].Key != "baseUrl" || collection.Variable[0].Value != "" {
		t.Fatalf("variables = %#v, want empty baseUrl placeholder", collection.Variable)
	}
	requestCount := 0
	for _, folder := range collection.Item {
		requestCount += len(folder.Item)
	}
	if requestCount != 1 {
		t.Fatalf("request count = %d, want 1 resolved endpoint only", requestCount)
	}
}
