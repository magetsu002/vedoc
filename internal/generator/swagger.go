package generator

import (
	"encoding/json"
	"os"
	"strings"
	"unicode"

	"github.com/RaniduNethma/vedoc/internal/models"
)

type OpenAPI struct {
	OpenAPI string                          `json:"openapi"`
	Info    OpenAPIInfo                     `json:"info"`
	Paths   map[string]map[string]Operation `json:"paths"`
}

type OpenAPIInfo struct {
	Title   string `json:"title"`
	Version string `json:"version"`
}

type Operation struct {
	Summary     string              `json:"summary,omitempty"`
	Description string              `json:"description,omitempty"`
	Tags        []string            `json:"tags,omitempty"`
	Parameters  []Parameter         `json:"parameters,omitempty"`
	RequestBody *RequestBody        `json:"requestBody,omitempty"`
	Responses   map[string]Response `json:"responses"`
}

type Parameter struct {
	Name     string `json:"name"`
	In       string `json:"in"`
	Required bool   `json:"required"`
	Schema   Schema `json:"schema"`
}

type RequestBody struct {
	Content map[string]MediaType `json:"content"`
}

type MediaType struct {
	Schema  Schema      `json:"schema"`
	Example interface{} `json:"example,omitempty"`
}

type Schema struct {
	Type string `json:"type"`
}

type Response struct {
	Description string `json:"description"`
}

func GenerateSwagger(endpoints []models.Endpoint, filename string) error {
	doc := OpenAPI{
		OpenAPI: "3.0.0",
		Info: OpenAPIInfo{
			Title:   "Vedoc Generated API (Swagger)",
			Version: "1.0.0",
		},
		Paths: make(map[string]map[string]Operation),
	}

	for _, ep := range models.ResolvedEndpoints(endpoints) {
		openAPIPath, parameters := expressPathToOpenAPI(ep.Path)
		method := strings.ToLower(ep.Method)
		op := Operation{
			Summary:     ep.Method + " " + openAPIPath,
			Description: ep.Description,
			Tags:        []string{getFolderName(ep.Path)},
			Parameters:  parameters,
			Responses: map[string]Response{
				"default": {Description: "Response behavior was not statically inferred by Vedoc"},
			},
		}

		if ep.Payload != "" && ep.Payload != "{}" && ep.Payload != `""` {
			var exampleJSON interface{}
			if err := json.Unmarshal([]byte(ep.Payload), &exampleJSON); err != nil {
				exampleJSON = ep.Payload
			}
			op.RequestBody = &RequestBody{
				Content: map[string]MediaType{
					"application/json": {
						Schema:  Schema{Type: "object"},
						Example: exampleJSON,
					},
				},
			}
		}

		if doc.Paths[openAPIPath] == nil {
			doc.Paths[openAPIPath] = make(map[string]Operation)
		}
		doc.Paths[openAPIPath][method] = op
	}

	jsonData, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, jsonData, 0o644)
}

func expressPathToOpenAPI(value string) (string, []Parameter) {
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	segments := strings.Split(value, "/")
	parameters := make([]Parameter, 0)
	seen := make(map[string]bool)
	for i, segment := range segments {
		if !strings.HasPrefix(segment, ":") {
			continue
		}
		name := strings.TrimPrefix(segment, ":")
		if !isOpenAPIParameterName(name) {
			continue
		}
		segments[i] = "{" + name + "}"
		if seen[name] {
			continue
		}
		seen[name] = true
		parameters = append(parameters, Parameter{
			Name:     name,
			In:       "path",
			Required: true,
			Schema:   Schema{Type: "string"},
		})
	}
	return strings.Join(segments, "/"), parameters
}

func isOpenAPIParameterName(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}
