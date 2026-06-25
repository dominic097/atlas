package routes

import "testing"

// pyFindProducer returns the first producer RawRoute matching method+path, or nil.
func pyFindProducer(raws []RawRoute, method, path string) *RawRoute {
	for i := range raws {
		if raws[i].Role == RoleProducer && raws[i].Method == method && raws[i].Path == path {
			return &raws[i]
		}
	}
	return nil
}

// pyFindConsumer returns the first consumer RawRoute matching method+rawURL, or nil.
func pyFindConsumer(raws []RawRoute, method, rawURL string) *RawRoute {
	for i := range raws {
		if raws[i].Role == RoleConsumer && raws[i].Method == method && raws[i].RawURL == rawURL {
			return &raws[i]
		}
	}
	return nil
}

func TestPythonFastAPIProducerAndRequestsConsumer(t *testing.T) {
	src := `
from fastapi import FastAPI
import requests

app = FastAPI()

@app.get("/api/v1/users/{id}")
async def get_user(id: str):
    upstream = requests.get(f"http://svc/api/v1/users/{id}")
    return upstream.json()
`
	raws := pythonRoutes("svc/main.py", src)

	// PRODUCER: GET /api/v1/users/{id} -> get_user
	p := pyFindProducer(raws, "GET", "/api/v1/users/{id}")
	if p == nil {
		t.Fatalf("expected producer GET /api/v1/users/{id}; got %+v", raws)
	}
	if p.HandlerName != "get_user" {
		t.Fatalf("expected handler get_user, got %q", p.HandlerName)
	}
	if p.File != "svc/main.py" {
		t.Fatalf("expected file svc/main.py, got %q", p.File)
	}

	// CONSUMER: requests.get(f"http://svc/api/v1/users/{id}")
	c := pyFindConsumer(raws, "GET", "http://svc/api/v1/users/{id}")
	if c == nil {
		t.Fatalf("expected consumer GET http://svc/api/v1/users/{id}; got %+v", raws)
	}
	if c.Path != "/api/v1/users/{id}" {
		t.Fatalf("expected consumer path /api/v1/users/{id}, got %q", c.Path)
	}
	if c.CallingSymbol != "get_user" {
		t.Fatalf("expected calling symbol get_user, got %q", c.CallingSymbol)
	}
}

func TestPythonFlaskRouteMethodsList(t *testing.T) {
	src := `
@app.route("/items", methods=["GET", "POST"])
def items():
    pass
`
	raws := pythonRoutes("app.py", src)

	if p := pyFindProducer(raws, "GET", "/items"); p == nil || p.HandlerName != "items" {
		t.Fatalf("expected GET /items -> items; got %+v", raws)
	}
	if p := pyFindProducer(raws, "POST", "/items"); p == nil || p.HandlerName != "items" {
		t.Fatalf("expected POST /items -> items; got %+v", raws)
	}
}

func TestPythonFlaskRouteDefaultsGet(t *testing.T) {
	src := `
@app.route("/ping")
def ping():
    pass
`
	raws := pythonRoutes("app.py", src)
	if p := pyFindProducer(raws, "GET", "/ping"); p == nil {
		t.Fatalf("expected @app.route to default to GET /ping; got %+v", raws)
	}
	// no methods= means exactly one (GET) producer for this route.
	count := 0
	for _, r := range raws {
		if r.Role == RoleProducer && r.Path == "/ping" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 producer for /ping, got %d", count)
	}
}

func TestPythonStackedDecoratorFindsDef(t *testing.T) {
	src := `
@app.post("/login")
@require_auth
def login(req):
    pass
`
	raws := pythonRoutes("auth.py", src)
	p := pyFindProducer(raws, "POST", "/login")
	if p == nil || p.HandlerName != "login" {
		t.Fatalf("expected POST /login -> login through stacked decorator; got %+v", raws)
	}
}

func TestPythonDjangoRoute(t *testing.T) {
	src := `
urlpatterns = [
    path("users/<int:id>/", views.get_user),
    re_path(r"^orders/(?P<oid>[0-9]+)/$", views.get_order),
]
`
	raws := pythonRoutes("urls.py", src)
	if p := pyFindProducer(raws, "ANY", "/users/{id}/"); p == nil || p.HandlerName != "get_user" {
		t.Fatalf("expected ANY /users/{id}/ -> get_user; got %+v", raws)
	}
	if p := pyFindProducer(raws, "ANY", "/orders/{oid}/"); p == nil || p.HandlerName != "get_order" {
		t.Fatalf("expected ANY /orders/{oid}/ -> get_order; got %+v", raws)
	}
}

func TestPythonRequestsRequestMethodArg(t *testing.T) {
	src := `
def call_it():
    requests.request("DELETE", "http://svc/api/v1/users/42")
`
	raws := pythonRoutes("c.py", src)
	c := pyFindConsumer(raws, "DELETE", "http://svc/api/v1/users/42")
	if c == nil || c.Path != "/api/v1/users/42" || c.CallingSymbol != "call_it" {
		t.Fatalf("expected DELETE consumer /api/v1/users/42 in call_it; got %+v", raws)
	}
}

func TestPythonHttpxAndUrlopenConsumers(t *testing.T) {
	src := `
async def fetch_all():
    a = await httpx.AsyncClient().get("http://svc/a/b")
    b = urllib.request.urlopen("https://svc/c/d")
`
	raws := pythonRoutes("h.py", src)
	if c := pyFindConsumer(raws, "GET", "http://svc/a/b"); c == nil || c.Path != "/a/b" {
		t.Fatalf("expected httpx GET /a/b; got %+v", raws)
	}
	// urlopen has unknown method -> empty Method string.
	if c := pyFindConsumer(raws, "", "https://svc/c/d"); c == nil || c.Path != "/c/d" {
		t.Fatalf("expected urlopen consumer /c/d; got %+v", raws)
	}
}
