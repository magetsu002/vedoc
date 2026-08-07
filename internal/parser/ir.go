package parser

import (
	"fmt"
	"strings"

	"github.com/RaniduNethma/vedoc/internal/models"
	sitter "github.com/smacker/go-tree-sitter"
)

func (r *projectResolver) resolveIR() []models.Endpoint {
	incoming := make(map[projectNode]int)
	for _, edges := range r.edges {
		for _, edge := range edges {
			incoming[edge.to]++
		}
	}

	roots := make([]projectNode, 0)
	for node, kind := range r.kinds {
		if kind == expressApp && incoming[node] == 0 {
			roots = append(roots, node)
		}
	}
	sortProjectNodes(roots)

	var endpoints []models.Endpoint
	resolvedRoutes := make(map[string]bool)
	seenResolved := make(map[string]bool)
	for _, root := range roots {
		r.walkIR(root, "", nil, make(map[projectNode]bool), &endpoints, resolvedRoutes, seenResolved)
	}

	for node, routes := range r.routes {
		for _, route := range routes {
			key := routeIdentity(node, route)
			if resolvedRoutes[key] {
				continue
			}
			endpoints = append(endpoints, models.Endpoint{
				Method:           route.method,
				LocalPath:        route.path,
				Resolution:       models.ResolutionUnresolved,
				UnresolvedReason: "route is not reachable from a statically resolved Express app mount",
				Source:           []models.SourceLocation{r.routeLocation(node, route)},
				Middleware:       r.routeMiddleware(node, route),
				CodeSnippet:      route.snippet,
			})
		}
	}

	models.SortEndpoints(endpoints)
	return endpoints
}

func (r *projectResolver) walkIR(
	node projectNode,
	prefix string,
	provenance []models.SourceLocation,
	stack map[projectNode]bool,
	endpoints *[]models.Endpoint,
	resolvedRoutes map[string]bool,
	seenResolved map[string]bool,
) {
	if stack[node] {
		return
	}
	stack[node] = true
	defer delete(stack, node)

	for _, route := range r.routes[node] {
		resolvedPath := joinExpressPaths(prefix, route.path)
		identity := routeIdentity(node, route)
		resolvedRoutes[identity] = true

		endpointKey := fmt.Sprintf("%s\x00%s\x00%s\x00%d", route.method, resolvedPath, node.module, route.position)
		if seenResolved[endpointKey] {
			continue
		}
		seenResolved[endpointKey] = true

		source := append([]models.SourceLocation(nil), provenance...)
		source = append(source, r.routeLocation(node, route))
		*endpoints = append(*endpoints, models.Endpoint{
			Method:      route.method,
			Path:        resolvedPath,
			LocalPath:   route.path,
			Resolution:  models.ResolutionResolved,
			Source:      source,
			Middleware:  r.routeMiddleware(node, route),
			CodeSnippet: route.snippet,
		})
	}

	for _, edge := range r.edges[node] {
		chain := append([]models.SourceLocation(nil), provenance...)
		chain = append(chain, r.mountLocation(node, edge))
		r.walkIR(edge.to, joinExpressPaths(prefix, edge.prefix), chain, stack, endpoints, resolvedRoutes, seenResolved)
	}
}

func sortProjectNodes(nodes []projectNode) {
	for i := 1; i < len(nodes); i++ {
		for j := i; j > 0; j-- {
			left := nodes[j-1]
			right := nodes[j]
			if left.module < right.module || left.module == right.module && left.symbol <= right.symbol {
				break
			}
			nodes[j-1], nodes[j] = nodes[j], nodes[j-1]
		}
	}
}

func routeIdentity(node projectNode, route projectRoute) string {
	return fmt.Sprintf("%s\x00%s\x00%d\x00%s\x00%s", node.module, node.symbol, route.position, route.method, route.path)
}

func (r *projectResolver) routeLocation(node projectNode, route projectRoute) models.SourceLocation {
	module := r.modules[node.module]
	return sourceLocation(module, route.position, "route", route.snippet)
}

func (r *projectResolver) mountLocation(node projectNode, edge projectEdge) models.SourceLocation {
	module := r.modules[node.module]
	return sourceLocation(module, edge.position, "mount", callSnippetAt(module, edge.position))
}

func sourceLocation(module *projectModule, position uint32, kind, snippet string) models.SourceLocation {
	if module == nil {
		return models.SourceLocation{Kind: kind, Snippet: snippet}
	}
	line, column := byteLineColumn(module.source, position)
	return models.SourceLocation{
		File:    module.path,
		Line:    line,
		Column:  column,
		Kind:    kind,
		Snippet: strings.TrimSpace(snippet),
	}
}

func byteLineColumn(source []byte, position uint32) (uint32, uint32) {
	if int(position) > len(source) {
		position = uint32(len(source))
	}
	line := uint32(1)
	column := uint32(1)
	for i := uint32(0); i < position; i++ {
		if source[i] == '\n' {
			line++
			column = 1
		} else {
			column++
		}
	}
	return line, column
}

func callSnippetAt(module *projectModule, position uint32) string {
	call := callExpressionAt(module, position)
	if call == nil {
		return ""
	}
	return call.Content(module.source)
}

func (r *projectResolver) routeMiddleware(node projectNode, route projectRoute) []string {
	module := r.modules[node.module]
	call := callExpressionAt(module, route.position)
	if call == nil {
		return nil
	}
	arguments := nonNullNode(call.ChildByFieldName("arguments"))
	if arguments == nil || arguments.NamedChildCount() <= 2 {
		return nil
	}

	middleware := make([]string, 0, int(arguments.NamedChildCount())-2)
	for i := 1; i < int(arguments.NamedChildCount())-1; i++ {
		value := strings.TrimSpace(arguments.NamedChild(i).Content(module.source))
		if value != "" {
			middleware = append(middleware, value)
		}
	}
	return middleware
}

func callExpressionAt(module *projectModule, position uint32) *sitter.Node {
	if module == nil || module.root == nil {
		return nil
	}
	return findCallExpression(module.root, position)
}

func findCallExpression(node *sitter.Node, position uint32) *sitter.Node {
	if node == nil || node.IsNull() || position < node.StartByte() || position >= node.EndByte() {
		return nil
	}
	if node.Type() == "call_expression" && node.StartByte() == position {
		return node
	}

	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if found := findCallExpression(child, position); found != nil {
			return found
		}
	}
	if node.Type() == "call_expression" {
		return node
	}
	return nil
}
