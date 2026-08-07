package parser

import (
	"reflect"
	"testing"
)

func TestResolveExpressProjectNestedRoutersESM(t *testing.T) {
	files := []SourceFile{
		{
			Path: "src/app.ts",
			Source: []byte(`
import express from "express";
import apiRouter from "./api";

const app = express();
app.use("/api", apiRouter);
`),
		},
		{
			Path: "src/api.ts",
			Source: []byte(`
import express from "express";
import { usersRouter } from "./users";

const apiRouter = express.Router();
apiRouter.use("/users", usersRouter);
export default apiRouter;
`),
		},
		{
			Path: "src/users.ts",
			Source: []byte(`
import express from "express";

export const usersRouter = express.Router();
usersRouter.get("/:id", handler);
`),
		},
	}

	got, err := ResolveExpressProject(files)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"GET /api/users/:id"}
	if signatures := endpointSignatures(got); !reflect.DeepEqual(signatures, want) {
		t.Fatalf("ResolveExpressProject() = %#v, want %#v", signatures, want)
	}
}

func TestResolveExpressProjectCommonJSAndBasenameCollisions(t *testing.T) {
	files := []SourceFile{
		{
			Path: "app.js",
			Source: []byte(`
const express = require("express");
const adminUsers = require("./admin/users");
const customerUsers = require("./customer/users");
const app = express();
app.use("/admin", adminUsers);
app.use("/customer", customerUsers);
`),
		},
		{
			Path: "admin/users.js",
			Source: []byte(`
const express = require("express");
const router = express.Router();
router.get("/:id", handler);
module.exports = router;
`),
		},
		{
			Path: "customer/users.js",
			Source: []byte(`
const { Router } = require("express");
const router = Router();
router.get("/:id", handler);
module.exports = router;
`),
		},
	}

	got, err := ResolveExpressProject(files)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"GET /admin/:id", "GET /customer/:id"}
	if signatures := endpointSignatures(got); !reflect.DeepEqual(signatures, want) {
		t.Fatalf("ResolveExpressProject() = %#v, want %#v", signatures, want)
	}
}

func TestResolveExpressProjectMultipleMountsProduceMultipleResolvedPaths(t *testing.T) {
	files := []SourceFile{
		{
			Path: "app.js",
			Source: []byte(`
const express = require("express");
const users = require("./users");
const app = express();
app.use("/v1/users", users);
app.use("/v2/users", users);
`),
		},
		{
			Path: "users.js",
			Source: []byte(`
const express = require("express");
const router = express.Router();
router.get("/:id", handler);
module.exports = router;
`),
		},
	}

	got, err := ResolveExpressProject(files)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"GET /v1/users/:id", "GET /v2/users/:id"}
	if signatures := endpointSignatures(got); !reflect.DeepEqual(signatures, want) {
		t.Fatalf("ResolveExpressProject() = %#v, want %#v", signatures, want)
	}
}

func TestResolveExpressProjectDoesNotGuessDynamicOrOrphanMounts(t *testing.T) {
	files := []SourceFile{
		{
			Path: "app.ts",
			Source: []byte(`
import express from "express";
import usersRouter from "./routes/users";
const app = express();
const prefix = "/users";
app.use(prefix, usersRouter);
app.get("/health", handler);
`),
		},
		{
			Path: "routes/users.ts",
			Source: []byte(`
import express from "express";
const router = express.Router();
router.get("/", handler);
export default router;
`),
		},
	}

	got, err := ResolveExpressProject(files)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"GET /health"}
	if signatures := endpointSignatures(got); !reflect.DeepEqual(signatures, want) {
		t.Fatalf("ResolveExpressProject() = %#v, want %#v", signatures, want)
	}
}

func TestResolveExpressProjectDoesNotDerivePathFromFilename(t *testing.T) {
	files := []SourceFile{
		{
			Path: "users.ts",
			Source: []byte(`
import express from "express";
const app = express();
app.get("/", handler);
`),
		},
	}

	got, err := ResolveExpressProject(files)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"GET /"}
	if signatures := endpointSignatures(got); !reflect.DeepEqual(signatures, want) {
		t.Fatalf("ResolveExpressProject() = %#v, want %#v", signatures, want)
	}
}

func TestResolveExpressProjectLocalNestedRouter(t *testing.T) {
	files := []SourceFile{
		{
			Path: "app.js",
			Source: []byte(`
const express = require("express");
const app = express();
const api = express.Router();
const users = express.Router();
app.use("/api", api);
api.use("/users", users);
users.post("/", handler);
`),
		},
	}

	got, err := ResolveExpressProject(files)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"POST /api/users"}
	if signatures := endpointSignatures(got); !reflect.DeepEqual(signatures, want) {
		t.Fatalf("ResolveExpressProject() = %#v, want %#v", signatures, want)
	}
}

func TestResolveExpressProjectResolvesBarrelReExport(t *testing.T) {
	files := []SourceFile{
		{
			Path: "src/app.ts",
			Source: []byte(`
import express from "express";
import usersRouter from "./routes";
const app = express();
app.use("/users", usersRouter);
`),
		},
		{
			Path:   "src/routes/index.ts",
			Source: []byte(`export { default } from "./users";`),
		},
		{
			Path: "src/routes/users.ts",
			Source: []byte(`
import express from "express";
const router = express.Router();
router.patch("/:id", handler);
export default router;
`),
		},
	}

	got, err := ResolveExpressProject(files)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"PATCH /users/:id"}
	if signatures := endpointSignatures(got); !reflect.DeepEqual(signatures, want) {
		t.Fatalf("ResolveExpressProject() = %#v, want %#v", signatures, want)
	}
}

func TestResolveExpressProjectRejectsAmbiguousExtensionResolution(t *testing.T) {
	files := []SourceFile{
		{
			Path: "app.ts",
			Source: []byte(`
import express from "express";
import router from "./users";
const app = express();
app.use("/users", router);
`),
		},
		{
			Path: "users.ts",
			Source: []byte(`
import express from "express";
const router = express.Router();
router.get("/ts", handler);
export default router;
`),
		},
		{
			Path: "users.js",
			Source: []byte(`
const express = require("express");
const router = express.Router();
router.get("/js", handler);
module.exports = router;
`),
		},
	}

	got, err := ResolveExpressProject(files)
	if err != nil {
		t.Fatal(err)
	}
	if signatures := endpointSignatures(got); len(signatures) != 0 {
		t.Fatalf("ResolveExpressProject() emitted ambiguous resolved routes: %#v", signatures)
	}
}
