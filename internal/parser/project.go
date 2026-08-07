package parser

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/RaniduNethma/vedoc/internal/models"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

// SourceFile is one JavaScript or TypeScript source file supplied to the
// project-level Express resolver. Path should be project-relative.
type SourceFile struct {
	Path   string
	Source []byte
}

type projectModule struct {
	path       string
	source     []byte
	root       *sitter.Node
	analyzer   *expressReceiverAnalyzer
	kinds      map[string]expressSymbolKind
	aliases    map[string]string
	imports    map[string]importBinding
	exports    map[string]exportBinding
	routes     []projectRoute
	mounts     []projectMount
	synthetics map[string]expressSymbolKind
}

type importBinding struct {
	spec       string
	exportName string
}

type exportBinding struct {
	localName string
	reexport  *importBinding
}

type projectRoute struct {
	receiver     string
	receiverKind expressSymbolKind
	method       string
	path         string
	snippet      string
	position     uint32
}

type mountTarget struct {
	localName string
	importRef *importBinding
}

type projectMount struct {
	parent   string
	prefix   string
	target   mountTarget
	position uint32
}

type projectNode struct {
	module string
	symbol string
}

type projectEdge struct {
	to       projectNode
	prefix   string
	position uint32
}

type projectResolver struct {
	modules map[string]*projectModule
	routes  map[projectNode][]projectRoute
	edges   map[projectNode][]projectEdge
	kinds   map[projectNode]expressSymbolKind
}

var (
	projectDefaultImportPattern   = regexp.MustCompile(`^\s*import\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*(?:,|from)`)
	projectNamespaceImportPattern = regexp.MustCompile(`\*\s+as\s+([A-Za-z_$][A-Za-z0-9_$]*)`)
	projectNamedImportPattern     = regexp.MustCompile(`\{([^}]*)\}`)
	projectExportDefaultPattern   = regexp.MustCompile(`^\s*export\s+default\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*;?\s*$`)
	projectNamedExportPattern     = regexp.MustCompile(`^\s*export\s*\{([^}]*)\}`)
)

// ResolveExpressProject resolves statically provable Express mount chains
// across JavaScript/TypeScript files. It only emits routes reachable from a
// proven Express application through statically resolvable router mounts.
// Dynamic or ambiguous mounts are intentionally left unresolved rather than
// guessed from filenames.
func ResolveExpressProject(files []SourceFile) ([]models.Endpoint, error) {
	resolver := &projectResolver{
		modules: make(map[string]*projectModule),
		routes:  make(map[projectNode][]projectRoute),
		edges:   make(map[projectNode][]projectEdge),
		kinds:   make(map[projectNode]expressSymbolKind),
	}

	sortedFiles := append([]SourceFile(nil), files...)
	sort.Slice(sortedFiles, func(i, j int) bool {
		return normalizeProjectPath(sortedFiles[i].Path) < normalizeProjectPath(sortedFiles[j].Path)
	})

	for _, file := range sortedFiles {
		modulePath := normalizeProjectPath(file.Path)
		if modulePath == "" || modulePath == "." {
			continue
		}
		if _, exists := resolver.modules[modulePath]; exists {
			return nil, fmt.Errorf("duplicate source path %q", modulePath)
		}

		module, err := analyzeProjectModule(modulePath, file.Source)
		if err != nil {
			return nil, err
		}
		resolver.modules[modulePath] = module
	}

	resolver.buildGraph()
	return resolver.resolveEndpoints(), nil
}

func analyzeProjectModule(modulePath string, source []byte) (*projectModule, error) {
	lang := projectLanguage(modulePath)
	parser := sitter.NewParser()
	parser.SetLanguage(lang)
	tree, err := parser.ParseCtx(context.Background(), nil, source)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", modulePath, err)
	}
	root := tree.RootNode()

	module := &projectModule{
		path:       modulePath,
		source:     source,
		root:       root,
		analyzer:   newExpressReceiverAnalyzer(root, source),
		kinds:      make(map[string]expressSymbolKind),
		aliases:    make(map[string]string),
		imports:    make(map[string]importBinding),
		exports:    make(map[string]exportBinding),
		synthetics: make(map[string]expressSymbolKind),
	}

	module.collectImportsAndSymbols(root)
	module.collectExports(root)
	if err := module.collectRoutes(); err != nil {
		return nil, fmt.Errorf("collect routes in %s: %w", modulePath, err)
	}
	if err := module.collectMounts(); err != nil {
		return nil, fmt.Errorf("collect mounts in %s: %w", modulePath, err)
	}
	return module, nil
}

func projectLanguage(filename string) *sitter.Language {
	if strings.HasSuffix(strings.ToLower(filename), ".ts") {
		return typescript.GetLanguage()
	}
	return javascript.GetLanguage()
}

func (m *projectModule) collectImportsAndSymbols(root *sitter.Node) {
	walkProjectNodes(root, func(node *sitter.Node) {
		if node.Type() == "import_statement" {
			m.collectESImport(node)
			return
		}
		if node.Type() != "variable_declarator" || !isProjectModuleLevel(node) {
			return
		}

		name := nonNullNode(node.ChildByFieldName("name"))
		value := nonNullNode(node.ChildByFieldName("value"))
		if name == nil || value == nil {
			return
		}

		if binding, ok := m.importFromExpression(value); ok {
			if name.Type() == "identifier" {
				m.imports[name.Content(m.source)] = binding
			} else if name.Type() == "object_pattern" && binding.exportName == "default" {
				m.collectRequireDestructure(name, binding.spec)
			}
		}

		if name.Type() != "identifier" {
			return
		}

		localName := name.Content(m.source)
		if value.Type() == "identifier" {
			valueName := value.Content(m.source)
			if imported, ok := m.imports[valueName]; ok {
				m.imports[localName] = imported
			}
		}

		kind := m.analyzer.classifyExpression(value, m.analyzer.scopeForNode(value), value.StartByte())
		if kind != expressApp && kind != expressRouter {
			return
		}
		m.kinds[localName] = kind
		m.aliases[localName] = localName
		if value.Type() == "identifier" {
			valueName := value.Content(m.source)
			if _, exists := m.kinds[valueName]; exists {
				m.aliases[localName] = m.canonical(valueName)
			}
		}
	})
}

func (m *projectModule) collectESImport(node *sitter.Node) {
	sourceNode := nonNullNode(node.ChildByFieldName("source"))
	if sourceNode == nil {
		return
	}
	spec := trimStringLiteral(sourceNode.Content(m.source))
	if !isRelativeImport(spec) {
		return
	}

	content := node.Content(m.source)
	if match := projectDefaultImportPattern.FindStringSubmatch(content); len(match) == 2 && match[1] != "type" {
		m.imports[match[1]] = importBinding{spec: spec, exportName: "default"}
	}
	if match := projectNamespaceImportPattern.FindStringSubmatch(content); len(match) == 2 {
		m.imports[match[1]] = importBinding{spec: spec, exportName: "*"}
	}
	if match := projectNamedImportPattern.FindStringSubmatch(content); len(match) == 2 {
		for _, part := range strings.Split(match[1], ",") {
			imported, local, ok := parseImportExportPair(part)
			if !ok {
				continue
			}
			m.imports[local] = importBinding{spec: spec, exportName: imported}
		}
	}
}

func (m *projectModule) collectRequireDestructure(pattern *sitter.Node, spec string) {
	content := strings.TrimSpace(pattern.Content(m.source))
	content = strings.TrimPrefix(content, "{")
	content = strings.TrimSuffix(content, "}")
	for _, part := range strings.Split(content, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		pieces := strings.SplitN(part, ":", 2)
		imported := strings.TrimSpace(pieces[0])
		local := imported
		if len(pieces) == 2 {
			local = strings.TrimSpace(pieces[1])
		}
		if isProjectIdentifier(imported) && isProjectIdentifier(local) {
			m.imports[local] = importBinding{spec: spec, exportName: imported}
		}
	}
}

func (m *projectModule) importFromExpression(node *sitter.Node) (importBinding, bool) {
	if node == nil || node.IsNull() {
		return importBinding{}, false
	}
	if spec, ok := requireSpecifier(node, m.source); ok && isRelativeImport(spec) {
		return importBinding{spec: spec, exportName: "default"}, true
	}
	if node.Type() != "member_expression" {
		return importBinding{}, false
	}
	object := nonNullNode(node.ChildByFieldName("object"))
	property := nonNullNode(node.ChildByFieldName("property"))
	if object == nil || property == nil {
		return importBinding{}, false
	}
	if spec, ok := requireSpecifier(object, m.source); ok && isRelativeImport(spec) {
		return importBinding{spec: spec, exportName: property.Content(m.source)}, true
	}
	return importBinding{}, false
}

func (m *projectModule) collectExports(root *sitter.Node) {
	walkProjectNodes(root, func(node *sitter.Node) {
		switch node.Type() {
		case "export_statement":
			m.collectESExport(node)
		case "assignment_expression":
			if isProjectModuleLevel(node) {
				m.collectCommonJSExport(node)
			}
		}
	})
}

func (m *projectModule) collectESExport(node *sitter.Node) {
	content := strings.TrimSpace(node.Content(m.source))
	sourceNode := nonNullNode(node.ChildByFieldName("source"))
	var sourceSpec string
	if sourceNode != nil {
		sourceSpec = trimStringLiteral(sourceNode.Content(m.source))
	}

	if match := projectExportDefaultPattern.FindStringSubmatch(content); len(match) == 2 {
		m.exports["default"] = exportBinding{localName: match[1]}
		return
	}

	if match := projectNamedExportPattern.FindStringSubmatch(content); len(match) == 2 {
		for _, part := range strings.Split(match[1], ",") {
			local, exported, ok := parseImportExportPair(part)
			if !ok {
				continue
			}
			if sourceSpec != "" && isRelativeImport(sourceSpec) {
				binding := importBinding{spec: sourceSpec, exportName: local}
				m.exports[exported] = exportBinding{reexport: &binding}
			} else {
				m.exports[exported] = exportBinding{localName: local}
			}
		}
		return
	}

	declaration := nonNullNode(node.ChildByFieldName("declaration"))
	if declaration == nil {
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			if child.Type() == "lexical_declaration" || child.Type() == "variable_declaration" {
				declaration = child
				break
			}
		}
	}
	if declaration != nil {
		walkProjectNodes(declaration, func(child *sitter.Node) {
			if child.Type() != "variable_declarator" {
				return
			}
			name := nonNullNode(child.ChildByFieldName("name"))
			if name != nil && name.Type() == "identifier" {
				local := name.Content(m.source)
				m.exports[local] = exportBinding{localName: local}
			}
		})
	}
}

func (m *projectModule) collectCommonJSExport(node *sitter.Node) {
	left := nonNullNode(node.ChildByFieldName("left"))
	right := nonNullNode(node.ChildByFieldName("right"))
	if left == nil || right == nil {
		return
	}

	exportName, ok := commonJSExportName(left.Content(m.source))
	if !ok {
		return
	}
	if right.Type() == "identifier" {
		m.exports[exportName] = exportBinding{localName: right.Content(m.source)}
		return
	}
	if binding, ok := m.importFromExpression(right); ok {
		m.exports[exportName] = exportBinding{reexport: &binding}
	}
}

func (m *projectModule) collectRoutes() error {
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
	q, err := sitter.NewQuery([]byte(queryStr), projectLanguage(m.path))
	if err != nil {
		return err
	}
	qc := sitter.NewQueryCursor()
	qc.Exec(q, m.root)

	validMethods := map[string]bool{"GET": true, "POST": true, "PUT": true, "DELETE": true, "PATCH": true}
	for {
		match, ok := qc.NextMatch()
		if !ok {
			break
		}

		var receiver *sitter.Node
		var method, routePath, snippet string
		var position uint32
		for _, capture := range match.Captures {
			name := q.CaptureNameForId(capture.Index)
			switch name {
			case "receiver":
				receiver = capture.Node
				position = capture.Node.StartByte()
			case "method":
				method = strings.ToUpper(capture.Node.Content(m.source))
			case "path":
				routePath = trimStringLiteral(capture.Node.Content(m.source))
			case "full_route":
				snippet = capture.Node.Content(m.source)
			}
		}
		if !validMethods[method] || !m.analyzer.provesReceiver(receiver) {
			continue
		}

		kind := m.analyzer.classifyExpression(receiver, m.analyzer.scopeForNode(receiver), receiver.StartByte())
		receiverName := m.receiverNodeName(receiver, kind)
		if receiverName == "" {
			continue
		}
		m.routes = append(m.routes, projectRoute{
			receiver:     receiverName,
			receiverKind: kind,
			method:       method,
			path:         normalizeExpressPath(routePath),
			snippet:      snippet,
			position:     position,
		})
	}
	return nil
}

func (m *projectModule) collectMounts() error {
	queryStr := `
	(call_expression
		(member_expression
			object: (_) @receiver
			property: (property_identifier) @method
		)
		(arguments) @arguments
	) @full_call
	`
	q, err := sitter.NewQuery([]byte(queryStr), projectLanguage(m.path))
	if err != nil {
		return err
	}
	qc := sitter.NewQueryCursor()
	qc.Exec(q, m.root)

	for {
		match, ok := qc.NextMatch()
		if !ok {
			break
		}

		var receiver, arguments *sitter.Node
		var method string
		for _, capture := range match.Captures {
			switch q.CaptureNameForId(capture.Index) {
			case "receiver":
				receiver = capture.Node
			case "method":
				method = capture.Node.Content(m.source)
			case "arguments":
				arguments = capture.Node
			}
		}
		if method != "use" || receiver == nil || arguments == nil || !m.analyzer.provesReceiver(receiver) {
			continue
		}

		kind := m.analyzer.classifyExpression(receiver, m.analyzer.scopeForNode(receiver), receiver.StartByte())
		parentName := m.receiverNodeName(receiver, kind)
		if parentName == "" {
			continue
		}

		argumentCount := int(arguments.NamedChildCount())
		if argumentCount == 0 {
			continue
		}

		prefix := ""
		start := 0
		first := arguments.NamedChild(0)
		if first.Type() == "string" {
			prefix = normalizeMountPrefix(trimStringLiteral(first.Content(m.source)))
			start = 1
		} else if argumentCount != 1 {
			// Multiple arguments without a static leading path are ambiguous.
			// Do not invent a pathless router edge.
			continue
		}

		for i := start; i < argumentCount; i++ {
			candidate := arguments.NamedChild(i)
			target, ok := m.mountTargetFromExpression(candidate)
			if !ok {
				continue
			}
			m.mounts = append(m.mounts, projectMount{
				parent:   parentName,
				prefix:   prefix,
				target:   target,
				position: receiver.StartByte(),
			})
		}
	}
	return nil
}

func (m *projectModule) receiverNodeName(node *sitter.Node, kind expressSymbolKind) string {
	if node == nil || node.IsNull() || (kind != expressApp && kind != expressRouter) {
		return ""
	}
	if name := simpleProjectIdentifier(node, m.source); name != "" {
		return m.canonical(name)
	}
	if kind == expressApp {
		name := fmt.Sprintf("@app:%d", node.StartByte())
		m.synthetics[name] = kind
		return name
	}
	return ""
}

func (m *projectModule) mountTargetFromExpression(node *sitter.Node) (mountTarget, bool) {
	if node == nil || node.IsNull() {
		return mountTarget{}, false
	}
	if name := simpleProjectIdentifier(node, m.source); name != "" {
		if _, ok := m.kinds[name]; ok {
			return mountTarget{localName: m.canonical(name)}, true
		}
		if binding, ok := m.imports[name]; ok {
			copy := binding
			return mountTarget{importRef: &copy}, true
		}
		return mountTarget{}, false
	}

	if binding, ok := m.importFromExpression(node); ok {
		copy := binding
		return mountTarget{importRef: &copy}, true
	}

	if node.Type() == "member_expression" {
		object := nonNullNode(node.ChildByFieldName("object"))
		property := nonNullNode(node.ChildByFieldName("property"))
		if object != nil && property != nil && object.Type() == "identifier" {
			if namespace, ok := m.imports[object.Content(m.source)]; ok && namespace.exportName == "*" {
				binding := importBinding{spec: namespace.spec, exportName: property.Content(m.source)}
				return mountTarget{importRef: &binding}, true
			}
		}
	}
	return mountTarget{}, false
}

func (m *projectModule) canonical(name string) string {
	seen := make(map[string]bool)
	for {
		if seen[name] {
			return name
		}
		seen[name] = true
		next, ok := m.aliases[name]
		if !ok || next == name || next == "" {
			return name
		}
		name = next
	}
}

func (r *projectResolver) buildGraph() {
	modulePaths := make([]string, 0, len(r.modules))
	for modulePath := range r.modules {
		modulePaths = append(modulePaths, modulePath)
	}
	sort.Strings(modulePaths)

	for _, modulePath := range modulePaths {
		module := r.modules[modulePath]
		for name, kind := range module.kinds {
			canonical := module.canonical(name)
			node := projectNode{module: modulePath, symbol: canonical}
			if _, exists := r.kinds[node]; !exists {
				r.kinds[node] = kind
			}
		}
		for name, kind := range module.synthetics {
			r.kinds[projectNode{module: modulePath, symbol: name}] = kind
		}
		for _, route := range module.routes {
			node := projectNode{module: modulePath, symbol: route.receiver}
			r.routes[node] = append(r.routes[node], route)
			if _, exists := r.kinds[node]; !exists {
				r.kinds[node] = route.receiverKind
			}
		}
	}

	for _, modulePath := range modulePaths {
		module := r.modules[modulePath]
		for _, mount := range module.mounts {
			from := projectNode{module: modulePath, symbol: mount.parent}
			to, ok := r.resolveMountTarget(module, mount.target)
			if !ok || from == to {
				continue
			}
			r.edges[from] = append(r.edges[from], projectEdge{to: to, prefix: mount.prefix, position: mount.position})
		}
	}

	for node := range r.routes {
		sort.Slice(r.routes[node], func(i, j int) bool {
			return r.routes[node][i].position < r.routes[node][j].position
		})
	}
	for node := range r.edges {
		sort.Slice(r.edges[node], func(i, j int) bool {
			if r.edges[node][i].position != r.edges[node][j].position {
				return r.edges[node][i].position < r.edges[node][j].position
			}
			if r.edges[node][i].prefix != r.edges[node][j].prefix {
				return r.edges[node][i].prefix < r.edges[node][j].prefix
			}
			if r.edges[node][i].to.module != r.edges[node][j].to.module {
				return r.edges[node][i].to.module < r.edges[node][j].to.module
			}
			return r.edges[node][i].to.symbol < r.edges[node][j].to.symbol
		})
	}
}

func (r *projectResolver) resolveMountTarget(module *projectModule, target mountTarget) (projectNode, bool) {
	if target.localName != "" {
		name := module.canonical(target.localName)
		kind := module.kinds[name]
		if kind != expressApp && kind != expressRouter {
			return projectNode{}, false
		}
		return projectNode{module: module.path, symbol: name}, true
	}
	if target.importRef == nil {
		return projectNode{}, false
	}
	return r.resolveImport(module.path, *target.importRef, make(map[string]bool))
}

func (r *projectResolver) resolveImport(importer string, binding importBinding, seen map[string]bool) (projectNode, bool) {
	targetModule, ok := r.resolveModulePath(importer, binding.spec)
	if !ok {
		return projectNode{}, false
	}
	return r.resolveExport(targetModule, binding.exportName, seen)
}

func (r *projectResolver) resolveExport(modulePath, exportName string, seen map[string]bool) (projectNode, bool) {
	key := modulePath + "\x00" + exportName
	if seen[key] {
		return projectNode{}, false
	}
	seen[key] = true
	defer delete(seen, key)

	module := r.modules[modulePath]
	if module == nil {
		return projectNode{}, false
	}
	binding, ok := module.exports[exportName]
	if !ok {
		return projectNode{}, false
	}
	if binding.reexport != nil {
		return r.resolveImport(modulePath, *binding.reexport, seen)
	}
	if binding.localName == "" {
		return projectNode{}, false
	}

	name := module.canonical(binding.localName)
	kind := module.kinds[name]
	if kind != expressApp && kind != expressRouter {
		return projectNode{}, false
	}
	return projectNode{module: modulePath, symbol: name}, true
}

func (r *projectResolver) resolveModulePath(importer, spec string) (string, bool) {
	if !isRelativeImport(spec) {
		return "", false
	}
	base := normalizeProjectPath(path.Join(path.Dir(importer), spec))
	candidates := projectImportCandidates(base)
	matches := make([]string, 0, len(candidates))
	seen := make(map[string]bool)
	for _, candidate := range candidates {
		candidate = normalizeProjectPath(candidate)
		if seen[candidate] {
			continue
		}
		seen[candidate] = true
		if _, ok := r.modules[candidate]; ok {
			matches = append(matches, candidate)
		}
	}
	if len(matches) != 1 {
		return "", false
	}
	return matches[0], true
}

func (r *projectResolver) resolveEndpoints() []models.Endpoint {
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
	sort.Slice(roots, func(i, j int) bool {
		if roots[i].module != roots[j].module {
			return roots[i].module < roots[j].module
		}
		return roots[i].symbol < roots[j].symbol
	})

	var endpoints []models.Endpoint
	seenEndpoints := make(map[string]bool)
	for _, root := range roots {
		r.walkResolved(root, "", make(map[projectNode]bool), &endpoints, seenEndpoints)
	}
	return endpoints
}

func (r *projectResolver) walkResolved(node projectNode, prefix string, stack map[projectNode]bool, endpoints *[]models.Endpoint, seen map[string]bool) {
	if stack[node] {
		return
	}
	stack[node] = true
	defer delete(stack, node)

	for _, route := range r.routes[node] {
		resolvedPath := joinExpressPaths(prefix, route.path)
		key := fmt.Sprintf("%s\x00%d\x00%s\x00%s", node.module, route.position, route.method, resolvedPath)
		if seen[key] {
			continue
		}
		seen[key] = true
		*endpoints = append(*endpoints, models.Endpoint{
			Method:      route.method,
			Path:        resolvedPath,
			CodeSnippet: route.snippet,
		})
	}

	for _, edge := range r.edges[node] {
		r.walkResolved(edge.to, joinExpressPaths(prefix, edge.prefix), stack, endpoints, seen)
	}
}

func walkProjectNodes(node *sitter.Node, visit func(*sitter.Node)) {
	if node == nil || node.IsNull() {
		return
	}
	visit(node)
	for i := 0; i < int(node.NamedChildCount()); i++ {
		walkProjectNodes(node.NamedChild(i), visit)
	}
}

func isProjectModuleLevel(node *sitter.Node) bool {
	for current := node.Parent(); current != nil && !current.IsNull(); current = current.Parent() {
		if current.Type() == "program" {
			return true
		}
		if isFunctionScope(current.Type()) || current.Type() == "statement_block" || current.Type() == "catch_clause" {
			return false
		}
	}
	return false
}

func simpleProjectIdentifier(node *sitter.Node, source []byte) string {
	for node != nil && !node.IsNull() {
		switch node.Type() {
		case "identifier":
			return node.Content(source)
		case "parenthesized_expression", "as_expression", "type_assertion", "non_null_expression", "satisfies_expression":
			if node.NamedChildCount() == 0 {
				return ""
			}
			node = node.NamedChild(0)
		default:
			return ""
		}
	}
	return ""
}

func requireSpecifier(node *sitter.Node, source []byte) (string, bool) {
	if node == nil || node.IsNull() || node.Type() != "call_expression" {
		return "", false
	}
	function := nonNullNode(node.ChildByFieldName("function"))
	arguments := nonNullNode(node.ChildByFieldName("arguments"))
	if function == nil || arguments == nil || function.Type() != "identifier" || function.Content(source) != "require" {
		return "", false
	}
	if arguments.NamedChildCount() != 1 {
		return "", false
	}
	argument := arguments.NamedChild(0)
	if argument.Type() != "string" {
		return "", false
	}
	return trimStringLiteral(argument.Content(source)), true
}

func parseImportExportPair(part string) (string, string, bool) {
	part = strings.TrimSpace(part)
	if part == "" || strings.HasPrefix(part, "type ") {
		return "", "", false
	}
	fields := strings.Fields(part)
	if len(fields) == 1 && isProjectIdentifier(fields[0]) {
		return fields[0], fields[0], true
	}
	if len(fields) == 3 && fields[1] == "as" && isProjectIdentifier(fields[0]) && isProjectIdentifier(fields[2]) {
		return fields[0], fields[2], true
	}
	return "", "", false
}

func commonJSExportName(left string) (string, bool) {
	left = strings.TrimSpace(left)
	if left == "module.exports" {
		return "default", true
	}
	if strings.HasPrefix(left, "module.exports.") {
		name := strings.TrimPrefix(left, "module.exports.")
		return name, isProjectIdentifier(name)
	}
	if strings.HasPrefix(left, "exports.") {
		name := strings.TrimPrefix(left, "exports.")
		return name, isProjectIdentifier(name)
	}
	return "", false
}

func isProjectIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for i, r := range value {
		if i == 0 {
			if !(r == '_' || r == '$' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z') {
				return false
			}
			continue
		}
		if !(r == '_' || r == '$' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func isRelativeImport(spec string) bool {
	return spec == "." || spec == ".." || strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../")
}

func normalizeProjectPath(value string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	value = strings.TrimPrefix(value, "./")
	value = path.Clean(value)
	if value == "." {
		return value
	}
	return strings.TrimPrefix(value, "/")
}

func projectImportCandidates(base string) []string {
	ext := strings.ToLower(path.Ext(base))
	if ext == ".js" || ext == ".ts" {
		stem := strings.TrimSuffix(base, path.Ext(base))
		alternate := ".ts"
		if ext == ".ts" {
			alternate = ".js"
		}
		return []string{base, stem + alternate}
	}
	return []string{
		base,
		base + ".ts",
		base + ".js",
		path.Join(base, "index.ts"),
		path.Join(base, "index.js"),
	}
}

func normalizeMountPrefix(value string) string {
	if value == "" || value == "/" {
		return ""
	}
	return normalizeExpressPath(value)
}

func normalizeExpressPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "/"
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	if len(value) > 1 {
		value = strings.TrimSuffix(value, "/")
	}
	return value
}

func joinExpressPaths(prefix, suffix string) string {
	if prefix == "" || prefix == "/" {
		return normalizeExpressPath(suffix)
	}
	if suffix == "" || suffix == "/" {
		return normalizeExpressPath(prefix)
	}
	return normalizeExpressPath(strings.TrimSuffix(prefix, "/") + "/" + strings.TrimPrefix(suffix, "/"))
}
