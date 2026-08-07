package parser

import (
	"context"
	"regexp"
	"strings"

	"github.com/RaniduNethma/vedoc/internal/models"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

func ExtractBaseRoutes(code string) map[string]string {
	routes := make(map[string]string)

	useRegex := regexp.MustCompile(`use\s*\(\s*['"]([^'"]+)['"]\s*,\s*([a-zA-Z0-9_]+)\s*\)`)
	importRegex := regexp.MustCompile(`import\s+(?:\{\s*)?([a-zA-Z0-9_]+)(?:\s*\})?\s+from\s+['"]([^'"]+)['"]`)
	requireRegex := regexp.MustCompile(`(?:const|let|var)\s+([a-zA-Z0-9_]+)\s*=\s*require\s*\(\s*['"]([^'"]+)['"]\s*\)`)
	inlineRegex := regexp.MustCompile(`use\s*\(\s*['"]([^'"]+)['"]\s*,\s*require\s*\(\s*['"]([^'"]+)['"]\s*\)\s*\)`)

	varMap := make(map[string]string)

	for _, m := range importRegex.FindAllStringSubmatch(code, -1) {
		varMap[m[1]] = m[2]
	}
	for _, m := range requireRegex.FindAllStringSubmatch(code, -1) {
		varMap[m[1]] = m[2]
	}

	for _, m := range inlineRegex.FindAllStringSubmatch(code, -1) {
		basePath, importPath := m[1], m[2]
		parts := strings.Split(importPath, "/")
		routes[parts[len(parts)-1]] = basePath
	}

	for _, m := range useRegex.FindAllStringSubmatch(code, -1) {
		basePath, routerVar := m[1], m[2]
		if importPath, exists := varMap[routerVar]; exists {
			parts := strings.Split(importPath, "/")
			routes[parts[len(parts)-1]] = basePath
		}
	}

	return routes
}

func ParseExpressCode(sourceCode []byte, filename string, exactBasePath string) []models.Endpoint {
	var endpoints []models.Endpoint
	var lang *sitter.Language

	if strings.HasSuffix(filename, ".ts") {
		lang = typescript.GetLanguage()
	} else {
		lang = javascript.GetLanguage()
	}

	parser := sitter.NewParser()
	parser.SetLanguage(lang)

	tree, _ := parser.ParseCtx(context.Background(), nil, sourceCode)
	rootNode := tree.RootNode()
	receiverAnalyzer := newExpressReceiverAnalyzer(rootNode, sourceCode)

	queryStr := `
	(call_expression
		(member_expression
			object: (_) @receiver
			property: (property_identifier) @method
		)
		(arguments
			(string) @path
		)
	) @full_route
	`

	q, err := sitter.NewQuery([]byte(queryStr), lang)
	if err != nil {
		return endpoints
	}

	qc := sitter.NewQueryCursor()
	qc.Exec(q, rootNode)

	for {
		m, ok := qc.NextMatch()
		if !ok {
			break
		}

		var endpoint models.Endpoint
		var receiverNode *sitter.Node

		for _, c := range m.Captures {
			nodeName := q.CaptureNameForId(c.Index)
			nodeContent := c.Node.Content(sourceCode)

			if nodeName == "receiver" {
				receiverNode = c.Node
			} else if nodeName == "method" {
				endpoint.Method = strings.ToUpper(nodeContent)
			} else if nodeName == "path" {
				rawPath := strings.Trim(nodeContent, `'"`)

				if exactBasePath != "" {
					exactBasePath = strings.TrimSuffix(exactBasePath, "/")
					rawPath = strings.TrimPrefix(rawPath, "/")

					if rawPath == "" {
						rawPath = exactBasePath
					} else {
						rawPath = exactBasePath + "/" + rawPath
					}
				}

				endpoint.Path = rawPath
			} else if nodeName == "full_route" {
				endpoint.CodeSnippet = nodeContent
			}
		}

		validMethods := map[string]bool{"GET": true, "POST": true, "PUT": true, "DELETE": true, "PATCH": true}

		if validMethods[endpoint.Method] && receiverAnalyzer.provesReceiver(receiverNode) {
			endpoints = append(endpoints, endpoint)
		}
	}

	return endpoints
}
