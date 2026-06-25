package routes

import (
	"strings"
	"testing"
)

// jsFindProducer returns the first producer raw whose path contains
// pathContains, or nil. Path is the RAW (pre-normalize) form. (Distinct name
// from the python test's helpers to avoid a same-package redeclaration.)
func jsFindProducer(raws []RawRoute, pathContains string) *RawRoute {
	for i := range raws {
		if raws[i].Role == RoleProducer && strings.Contains(raws[i].Path, pathContains) {
			return &raws[i]
		}
	}
	return nil
}

// jsFindConsumer returns the first consumer raw whose RawURL contains urlContains.
func jsFindConsumer(raws []RawRoute, urlContains string) *RawRoute {
	for i := range raws {
		if raws[i].Role == RoleConsumer && strings.Contains(raws[i].RawURL, urlContains) {
			return &raws[i]
		}
	}
	return nil
}

func TestJSProducerExpressNamedHandler(t *testing.T) {
	src := `
const express = require('express');
const app = express();
app.get("/api/v1/users/:id", getUser);
app.post('/api/v1/users', createUser);
`
	raws := jsRoutes("server.js", src)

	p := jsFindProducer(raws, "/api/v1/users/:id")
	if p == nil {
		t.Fatalf("expected a producer raw for /api/v1/users/:id, got %+v", raws)
	}
	if p.Method != "GET" {
		t.Errorf("method = %q, want GET", p.Method)
	}
	if p.HandlerName != "getUser" {
		t.Errorf("handler = %q, want getUser", p.HandlerName)
	}
	if p.File != "server.js" {
		t.Errorf("file = %q, want server.js", p.File)
	}

	var post *RawRoute
	for i := range raws {
		if raws[i].Role == RoleProducer && raws[i].Method == "POST" && raws[i].Path == "/api/v1/users" {
			post = &raws[i]
		}
	}
	if post == nil || post.HandlerName != "createUser" {
		t.Errorf("expected POST /api/v1/users -> createUser, got %+v", post)
	}
}

func TestJSProducerRouterAndAnonymousHandler(t *testing.T) {
	src := `
router.put("/items/:itemId", (req, res) => { res.send('ok'); });
fastify.get("/health", healthCheck);
`
	raws := jsRoutes("routes.ts", src)

	put := jsFindProducer(raws, "/items/:itemId")
	if put == nil {
		t.Fatalf("expected producer for /items/:itemId")
	}
	if put.Method != "PUT" {
		t.Errorf("method = %q, want PUT", put.Method)
	}
	if put.HandlerName != "" {
		t.Errorf("anonymous handler should yield empty HandlerName, got %q", put.HandlerName)
	}

	h := jsFindProducer(raws, "/health")
	if h == nil || h.HandlerName != "healthCheck" {
		t.Fatalf("expected fastify producer /health -> healthCheck, got %+v", h)
	}
}

func TestJSProducerNestDecorators(t *testing.T) {
	src := `
@Controller("/users")
export class UsersController {
  @Get("/:id")
  async findOne(@Param('id') id: string) {
    return this.svc.find(id);
  }

  @Post()
  create(@Body() dto: CreateUserDto) {
    return this.svc.create(dto);
  }
}
`
	raws := jsRoutes("users.controller.ts", src)

	get := jsFindProducer(raws, "/users/:id")
	if get == nil {
		t.Fatalf("expected Nest producer /users/:id, got %+v", raws)
	}
	if get.Method != "GET" {
		t.Errorf("method = %q, want GET", get.Method)
	}
	if get.HandlerName != "findOne" {
		t.Errorf("handler = %q, want findOne", get.HandlerName)
	}

	// @Post() with no arg should still produce the controller prefix path.
	post := jsFindProducer(raws, "/users")
	if post == nil {
		t.Fatalf("expected Nest producer for @Post() under /users prefix")
	}
}

func TestJSConsumerAxiosTemplateLiteral(t *testing.T) {
	src := "" +
		"async function loadUser(id) {\n" +
		"  const res = await axios.get(`http://svc/api/v1/users/${id}`);\n" +
		"  return res.data;\n" +
		"}\n"
	raws := jsRoutes("client.ts", src)

	c := jsFindConsumer(raws, "/api/v1/users/")
	if c == nil {
		t.Fatalf("expected a consumer raw for the axios.get call, got %+v", raws)
	}
	if c.Method != "GET" {
		t.Errorf("method = %q, want GET", c.Method)
	}
	if !strings.Contains(c.RawURL, "/api/v1/users/${id}") {
		t.Errorf("RawURL = %q, want it to contain /api/v1/users/${id}", c.RawURL)
	}
	// Per the consumer contract Path is left empty; the resolver derives the path
	// from RawURL. extractPath(RawURL) must surface the served path.
	if got := extractPath(c.RawURL); !strings.Contains(got, "/api/v1/users/") {
		t.Errorf("extractPath(RawURL) = %q, want it to contain /api/v1/users/", got)
	}
	if c.CallingSymbol != "loadUser" {
		t.Errorf("calling symbol = %q, want loadUser", c.CallingSymbol)
	}
}

func TestJSConsumerFetchAndPostMethod(t *testing.T) {
	src := "" +
		"export const createThing = async (body) => {\n" +
		"  return fetch('https://api.example.com/api/v1/things', { method: 'POST', body });\n" +
		"};\n"
	raws := jsRoutes("api.ts", src)

	c := jsFindConsumer(raws, "/api/v1/things")
	if c == nil {
		t.Fatalf("expected a consumer raw for fetch, got %+v", raws)
	}
	if c.Method != "POST" {
		t.Errorf("method = %q, want POST (from options)", c.Method)
	}
	if c.CallingSymbol != "createThing" {
		t.Errorf("calling symbol = %q, want createThing", c.CallingSymbol)
	}
}

func TestJSConsumerVariousClients(t *testing.T) {
	src := `
got('http://svc/api/v1/got-path');
superagent.get('http://svc/api/v1/sa-path');
axios({ method: 'DELETE', url: 'http://svc/api/v1/axios-bare' });
`
	raws := jsRoutes("misc.js", src)

	if c := jsFindConsumer(raws, "/api/v1/got-path"); c == nil || c.Method != "GET" {
		t.Errorf("got() consumer missing or wrong method: %+v", c)
	}
	if c := jsFindConsumer(raws, "/api/v1/sa-path"); c == nil || c.Method != "GET" {
		t.Errorf("superagent.get consumer missing or wrong method: %+v", c)
	}
	if c := jsFindConsumer(raws, "/api/v1/axios-bare"); c == nil || c.Method != "DELETE" {
		t.Errorf("axios({method,url}) consumer missing or wrong method: %+v", c)
	}
}

// End-to-end through Resolve: a producer path normalizes :id -> {id} so a
// consumer call resolves to the same path and the matcher can line them up.
func TestJSResolveNormalizesPaths(t *testing.T) {
	src := `app.get("/api/v1/users/:id", getUser);`
	raws := jsRoutes("routes.js", src)
	routesOut := Resolve("acme/api", raws, nil)

	var found bool
	for _, r := range routesOut {
		if r.Role == RoleProducer && r.PathPattern == "/api/v1/users/{id}" && r.Method == "GET" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected resolved producer GET /api/v1/users/{id}, got %+v", routesOut)
	}
}
