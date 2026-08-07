package scanner

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestDiscoverAppliesDefaultsAndIgnoreFiles(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, ".gitignore", "ignored-by-git/\n*.generated.ts\n")
	writeFile(t, root, ".vedocignore", "ignored-by-vedoc/\n")
	writeFile(t, root, "src/z.ts", "")
	writeFile(t, root, "src/a.js", "")
	writeFile(t, root, "src/readme.md", "")
	writeFile(t, root, "src/value.generated.ts", "")
	writeFile(t, root, "dist/copied.ts", "")
	writeFile(t, root, "node_modules/pkg/index.js", "")
	writeFile(t, root, ".cache/hidden.ts", "")
	writeFile(t, root, "ignored-by-git/route.ts", "")
	writeFile(t, root, "ignored-by-vedoc/route.js", "")

	files, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	want := []string{
		filepath.Join(root, "src", "a.js"),
		filepath.Join(root, "src", "z.ts"),
	}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("Discover() = %#v, want %#v", files, want)
	}
}

func TestDiscoverPreservesBasenameCollisions(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "src/admin/users.ts", "")
	writeFile(t, root, "src/public/users.ts", "")

	files, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	want := []string{
		filepath.Join(root, "src", "admin", "users.ts"),
		filepath.Join(root, "src", "public", "users.ts"),
	}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("Discover() = %#v, want %#v", files, want)
	}
}

func TestDiscoverSupportsNegationAndGlobstar(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".vedocignore", "fixtures/**/*.ts\n!fixtures/keep.ts\n")
	writeFile(t, root, "fixtures/drop.ts", "")
	writeFile(t, root, "fixtures/nested/drop.ts", "")
	writeFile(t, root, "fixtures/keep.ts", "")

	files, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	want := []string{filepath.Join(root, "fixtures", "keep.ts")}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("Discover() = %#v, want %#v", files, want)
	}
}

func TestDiscoverSkipsSymlinkedSources(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is not reliably available on Windows test runners")
	}

	root := t.TempDir()
	writeFile(t, root, "outside.ts", "")
	if err := os.Symlink(filepath.Join(root, "outside.ts"), filepath.Join(root, "linked.ts")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	files, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	want := []string{filepath.Join(root, "outside.ts")}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("Discover() = %#v, want %#v", files, want)
	}
}

func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}
