package parser

import (
	"reflect"
	"testing"

	"github.com/RaniduNethma/vedoc/internal/models"
)

func TestResolveExpressProjectCarriesMountProvenanceAndMiddleware(t *testing.T) {
	files := []SourceFile{
		{Path: "app.ts", Source: []byte(`
import express from "express";
import apiRouter from "./api";
const app = express();
app.use("/api", apiRouter);
`)},
		{Path: "api.ts", Source: []byte(`
import express from "express";
import usersRouter from "./users";
const apiRouter = express.Router();
apiRouter.use("/users", usersRouter);
export default apiRouter;
`)},
		{Path: "users.ts", Source: []byte(`
import express from "express";
const usersRouter = express.Router();
usersRouter.post("/:id/avatar", auth, upload.single("avatar"), handler);
export default usersRouter;
`)},
	}

	endpoints, err := ResolveExpressProject(files)
	if err != nil {
		t.Fatal(err)
	}
	resolved := models.ResolvedEndpoints(endpoints)
	if len(resolved) != 1 {
		t.Fatalf("resolved endpoints = %#v, want one", resolved)
	}
	endpoint := resolved[0]
	if endpoint.Path != "/api/users/:id/avatar" || endpoint.LocalPath != "/:id/avatar" {
		t.Fatalf("paths = %q / %q", endpoint.Path, endpoint.LocalPath)
	}
	filesInChain := make([]string, 0, len(endpoint.Source))
	kinds := make([]string, 0, len(endpoint.Source))
	for _, source := range endpoint.Source {
		filesInChain = append(filesInChain, source.File)
		kinds = append(kinds, source.Kind)
		if source.Line == 0 || source.Column == 0 {
			t.Fatalf("invalid source location: %#v", source)
		}
	}
	if want := []string{"app.ts", "api.ts", "users.ts"}; !reflect.DeepEqual(filesInChain, want) {
		t.Fatalf("source files = %#v, want %#v", filesInChain, want)
	}
	if want := []string{"mount", "mount", "route"}; !reflect.DeepEqual(kinds, want) {
		t.Fatalf("source kinds = %#v, want %#v", kinds, want)
	}
	if want := []string{"auth", `upload.single("avatar")`}; !reflect.DeepEqual(endpoint.Middleware, want) {
		t.Fatalf("middleware = %#v, want %#v", endpoint.Middleware, want)
	}
}

func TestResolveExpressProjectEmitsUnresolvedInsteadOfGuessingDynamicMount(t *testing.T) {
	files := []SourceFile{
		{Path: "app.ts", Source: []byte(`
import express from "express";
import usersRouter from "./users";
const app = express();
const prefix = "/users";
app.use(prefix, usersRouter);
`)},
		{Path: "users.ts", Source: []byte(`
import express from "express";
const router = express.Router();
router.get("/:id", handler);
export default router;
`)},
	}

	endpoints, err := ResolveExpressProject(files)
	if err != nil {
		t.Fatal(err)
	}
	if len(endpoints) != 1 {
		t.Fatalf("endpoints = %#v, want one unresolved route", endpoints)
	}
	endpoint := endpoints[0]
	if endpoint.Resolution != models.ResolutionUnresolved || endpoint.Path != "" || endpoint.LocalPath != "/:id" {
		t.Fatalf("unresolved endpoint = %#v", endpoint)
	}
	if endpoint.UnresolvedReason == "" {
		t.Fatal("unresolved endpoint has no reason")
	}
}
