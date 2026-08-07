package generator

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/RaniduNethma/vedoc/internal/models"
)

type postmanCollection struct {
	Info     Info       `json:"info"`
	Item     []Item     `json:"item"`
	Variable []Variable `json:"variable,omitempty"`
}

type Info struct {
	Name   string `json:"name"`
	Schema string `json:"schema"`
}

type Variable struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Type  string `json:"type,omitempty"`
}

type Item struct {
	Name    string   `json:"name"`
	Item    []Item   `json:"item,omitempty"`
	Request *Request `json:"request,omitempty"`
}

type Request struct {
	Method      string   `json:"method"`
	Description string   `json:"description,omitempty"`
	Header      []Header `json:"header,omitempty"`
	Body        *Body    `json:"body,omitempty"`
	Url         Url      `json:"url"`
}

type Header struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Body struct {
	Mode    string `json:"mode"`
	Raw     string `json:"raw"`
	Options struct {
		Raw struct {
			Language string `json:"language"`
		} `json:"raw"`
	} `json:"options"`
}

type Url struct {
	Raw  string   `json:"raw"`
	Host []string `json:"host"`
	Path []string `json:"path"`
}

func getFolderName(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return "General"
	}

	folder := parts[0]
	if strings.ToLower(parts[0]) == "api" && len(parts) > 1 {
		folder = parts[1]
	}
	if len(folder) > 0 {
		folder = strings.ToUpper(string(folder[0])) + folder[1:]
	}
	return folder
}

func GeneratePostmanCollection(endpoints []models.Endpoint, filename string) error {
	collection := postmanCollection{
		Info: Info{
			Name:   "Vedoc Generated API",
			Schema: "https://schema.getpostman.com/json/collection/v2.1.0/collection.json",
		},
		Variable: []Variable{{Key: "baseUrl", Value: "", Type: "string"}},
	}

	folders := make(map[string]*Item)
	for _, ep := range models.ResolvedEndpoints(endpoints) {
		pathParts := strings.Split(strings.TrimPrefix(ep.Path, "/"), "/")
		req := &Request{
			Method:      ep.Method,
			Description: ep.Description,
			Url: Url{
				Raw:  "{{baseUrl}}" + ep.Path,
				Host: []string{"{{baseUrl}}"},
				Path: pathParts,
			},
		}

		if ep.Payload != "" && ep.Payload != "{}" && ep.Payload != `""` {
			req.Header = append(req.Header, Header{Key: "Content-Type", Value: "application/json"})
			req.Body = &Body{Mode: "raw", Raw: ep.Payload}
			req.Body.Options.Raw.Language = "json"
		}

		folderName := getFolderName(ep.Path)
		if _, exists := folders[folderName]; !exists {
			folders[folderName] = &Item{Name: folderName, Item: []Item{}}
		}
		folders[folderName].Item = append(folders[folderName].Item, Item{
			Name:    fmt.Sprintf("%s %s", ep.Method, ep.Path),
			Request: req,
		})
	}

	folderNames := make([]string, 0, len(folders))
	for name := range folders {
		folderNames = append(folderNames, name)
	}
	sort.Strings(folderNames)
	for _, name := range folderNames {
		collection.Item = append(collection.Item, *folders[name])
	}

	jsonData, err := json.MarshalIndent(collection, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, jsonData, 0o644)
}
