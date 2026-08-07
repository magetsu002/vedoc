package scanner

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var defaultIgnoredDirectories = map[string]struct{}{
	"node_modules": {},
	"dist":         {},
	"build":        {},
	"coverage":     {},
	"out":          {},
	"vendor":       {},
}

type ignoreMatcher struct {
	rules []ignoreRule
}

type ignoreRule struct {
	negated  bool
	dirOnly  bool
	anchored bool
	hasSlash bool
	pattern  *regexp.Regexp
}

type discoveredFile struct {
	path string
	key  string
}

// Discover returns JavaScript and TypeScript source files below root in a
// deterministic order. Generated/vendor directories, hidden directories,
// symlinks, and paths ignored by root .gitignore/.vedocignore files are skipped.
func Discover(root string) ([]string, error) {
	root = filepath.Clean(root)
	matcher, err := loadIgnoreMatcher(root)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	files := make([]discoveredFile, 0)

	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(filepath.Clean(rel))

		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if entry.IsDir() {
			if shouldIgnoreDirectory(entry.Name()) || matcher.ignored(rel, true) {
				return filepath.SkipDir
			}
			return nil
		}

		if matcher.ignored(rel, false) || !isSourceFile(entry.Name()) {
			return nil
		}

		key := rel
		if _, exists := seen[key]; exists {
			return nil
		}
		seen[key] = struct{}{}
		files = append(files, discoveredFile{path: path, key: key})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].key < files[j].key
	})

	paths := make([]string, len(files))
	for i, file := range files {
		paths[i] = file.path
	}
	return paths, nil
}

func shouldIgnoreDirectory(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	_, ignored := defaultIgnoredDirectories[name]
	return ignored
}

func isSourceFile(name string) bool {
	return strings.HasSuffix(name, ".js") || strings.HasSuffix(name, ".ts")
}

func loadIgnoreMatcher(root string) (ignoreMatcher, error) {
	matcher := ignoreMatcher{}
	for _, name := range []string{".gitignore", ".vedocignore"} {
		file, err := os.Open(filepath.Join(root, name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return ignoreMatcher{}, err
		}

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			if rule, ok := parseIgnoreRule(scanner.Text()); ok {
				matcher.rules = append(matcher.rules, rule)
			}
		}
		scanErr := scanner.Err()
		closeErr := file.Close()
		if scanErr != nil {
			return ignoreMatcher{}, scanErr
		}
		if closeErr != nil {
			return ignoreMatcher{}, closeErr
		}
	}
	return matcher, nil
}

func parseIgnoreRule(line string) (ignoreRule, bool) {
	line = strings.TrimSuffix(line, "\r")
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return ignoreRule{}, false
	}

	if strings.HasPrefix(line, `\#`) || strings.HasPrefix(line, `\!`) {
		line = line[1:]
	}

	negated := strings.HasPrefix(line, "!")
	if negated {
		line = strings.TrimSpace(strings.TrimPrefix(line, "!"))
	}
	if line == "" {
		return ignoreRule{}, false
	}

	line = filepath.ToSlash(line)
	dirOnly := strings.HasSuffix(line, "/")
	line = strings.TrimSuffix(line, "/")
	anchored := strings.HasPrefix(line, "/")
	line = strings.TrimPrefix(line, "/")
	if line == "" {
		return ignoreRule{}, false
	}

	hasSlash := strings.Contains(line, "/")
	return ignoreRule{
		negated:  negated,
		dirOnly:  dirOnly,
		anchored: anchored,
		hasSlash: hasSlash,
		pattern:  regexp.MustCompile("^" + globRegexp(line) + "$"),
	}, true
}

func (m ignoreMatcher) ignored(rel string, isDir bool) bool {
	rel = strings.TrimPrefix(filepath.ToSlash(filepath.Clean(rel)), "./")
	ignored := false
	for _, rule := range m.rules {
		if rule.matches(rel, isDir) {
			ignored = !rule.negated
		}
	}
	return ignored
}

func (r ignoreRule) matches(rel string, isDir bool) bool {
	if r.dirOnly && !isDir {
		return false
	}
	if r.anchored || r.hasSlash {
		return r.pattern.MatchString(rel)
	}
	for _, segment := range strings.Split(rel, "/") {
		if r.pattern.MatchString(segment) {
			return true
		}
	}
	return false
}

func globRegexp(pattern string) string {
	var b strings.Builder
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				i++
				if i+1 < len(pattern) && pattern[i+1] == '/' {
					i++
					b.WriteString("(?:.*/)?")
				} else {
					b.WriteString(".*")
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		case '[':
			end := strings.IndexByte(pattern[i+1:], ']')
			if end < 0 {
				b.WriteString(`\[`)
				continue
			}
			end += i + 1
			class := pattern[i+1 : end]
			b.WriteByte('[')
			if strings.HasPrefix(class, "!") {
				b.WriteByte('^')
				class = class[1:]
			}
			for j := 0; j < len(class); j++ {
				if class[j] == '\\' || class[j] == ']' {
					b.WriteByte('\\')
				}
				b.WriteByte(class[j])
			}
			b.WriteByte(']')
			i = end
		default:
			b.WriteString(regexp.QuoteMeta(string(pattern[i])))
		}
	}
	return b.String()
}
