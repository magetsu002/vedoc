package parser

import (
	"reflect"
	"testing"

	"github.com/RaniduNethma/vedoc/internal/models"
)

func TestParseExpressCodeOnlyAcceptsProvenExpressReceivers(t *testing.T) {
	source := []byte(`
const express = require("express");
const app = express();
const router = express.Router();

app.get("/app", handler);
router.post("/router", handler);
axios.get("/axios");
cache.get("/cache");
client.delete("/client");
`)

	got := endpointSignatures(ParseExpressCode(source, "routes.js", ""))
	want := []string{"GET /app", "POST /router"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseExpressCode() = %#v, want %#v", got, want)
	}
}

func TestParseExpressCodeTracksFactoriesAndAliases(t *testing.T) {
	source := []byte(`
import express, { Router as makeRouter } from "express";

const app = express();
const appAlias = app;
const router = makeRouter();
const routerAlias = router;
const factoryAlias = express.Router;
const factoryRouter = factoryAlias();

appAlias.patch("/app-alias", handler);
routerAlias.put("/router-alias", handler);
factoryRouter.get("/factory-alias", handler);
express().get("/direct-app", handler);
express.Router().delete("/direct-router", handler);
`)

	got := endpointSignatures(ParseExpressCode(source, "routes.ts", ""))
	want := []string{
		"PATCH /app-alias",
		"PUT /router-alias",
		"GET /factory-alias",
		"GET /direct-app",
		"DELETE /direct-router",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseExpressCode() = %#v, want %#v", got, want)
	}
}

func TestParseExpressCodeSupportsCommonJSRouterDestructuring(t *testing.T) {
	source := []byte(`
const { Router } = require("express");
const { Router: makeRouter } = require("express");
const directRouter = require("express").Router();
const router = Router();
const aliasRouter = makeRouter();

router.get("/router", handler);
aliasRouter.post("/alias", handler);
directRouter.patch("/direct", handler);
`)

	got := endpointSignatures(ParseExpressCode(source, "routes.js", ""))
	want := []string{"GET /router", "POST /alias", "PATCH /direct"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseExpressCode() = %#v, want %#v", got, want)
	}
}

func TestParseExpressCodeInvalidatesAmbiguousReassignments(t *testing.T) {
	source := []byte(`
const express = require("express");
let router = express.Router();
router.get("/before", handler);
router = client;
router.get("/after", handler);

const maybeRouter = condition ? express.Router() : client;
maybeRouter.post("/conditional", handler);
`)

	got := endpointSignatures(ParseExpressCode(source, "routes.js", ""))
	want := []string{"GET /before"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseExpressCode() = %#v, want %#v", got, want)
	}
}

func TestParseExpressCodeRespectsLexicalShadowing(t *testing.T) {
	source := []byte(`
const express = require("express");
const router = express.Router();

function configure(router) {
  router.get("/shadowed-parameter", handler);
}

function localRoutes() {
  const localRouter = express.Router();
  localRouter.get("/local", handler);
}

router.get("/outer", handler);
localRouter.get("/leaked", handler);
`)

	got := endpointSignatures(ParseExpressCode(source, "routes.js", ""))
	want := []string{"GET /local", "GET /outer"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseExpressCode() = %#v, want %#v", got, want)
	}
}

func endpointSignatures(endpoints []models.Endpoint) []string {
	signatures := make([]string, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if !endpoint.IsResolved() || endpoint.Path == "" {
			continue
		}
		signatures = append(signatures, endpoint.Method+" "+endpoint.Path)
	}
	return signatures
}
