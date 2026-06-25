package routes

import "testing"

// findProducer returns the first producer raw matching method+path (path matched
// after Resolve-style :id->{id} is NOT applied here; raws keep the as-written
// path, but the test passes already-{}-form paths).
func findJavaProducer(raws []RawRoute, method, path string) (RawRoute, bool) {
	for _, r := range raws {
		if r.Role == RoleProducer && r.Method == method && r.Path == path {
			return r, true
		}
	}
	return RawRoute{}, false
}

func findJavaConsumer(raws []RawRoute, method, rawURL string) (RawRoute, bool) {
	for _, r := range raws {
		if r.Role == RoleConsumer && r.Method == method && r.RawURL == rawURL {
			return r, true
		}
	}
	return RawRoute{}, false
}

// TestJavaSpringProducer verifies a class-level @RequestMapping base is combined
// with a method-level @GetMapping path and the handler method name is captured.
func TestJavaSpringProducer(t *testing.T) {
	src := `package com.example.api;

import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping("/api/v1")
public class UserController {

    @GetMapping("/users/{id}")
    public User getUser(@PathVariable String id) {
        return service.find(id);
    }

    @PostMapping("/users")
    public User createUser(@RequestBody User u) {
        return service.create(u);
    }
}
`
	raws := javaRoutes("UserController.java", src)

	got, ok := findJavaProducer(raws, "GET", "/api/v1/users/{id}")
	if !ok {
		t.Fatalf("expected producer GET /api/v1/users/{id}; got raws=%+v", raws)
	}
	if got.HandlerName != "getUser" {
		t.Errorf("HandlerName = %q, want getUser", got.HandlerName)
	}
	if got.File != "UserController.java" {
		t.Errorf("File = %q, want UserController.java", got.File)
	}

	if got, ok := findJavaProducer(raws, "POST", "/api/v1/users"); !ok {
		t.Errorf("expected producer POST /api/v1/users; got raws=%+v", raws)
	} else if got.HandlerName != "createUser" {
		t.Errorf("POST HandlerName = %q, want createUser", got.HandlerName)
	}
}

// TestJavaRequestMappingMethod verifies the generic @RequestMapping(value=..,
// method=RequestMethod.GET) form (method from the method= arg).
func TestJavaRequestMappingMethod(t *testing.T) {
	src := `@RequestMapping("/api")
public class OrderController {

    @RequestMapping(value = "/orders/{id}", method = RequestMethod.DELETE)
    public void deleteOrder(String id) {}
}
`
	raws := javaRoutes("OrderController.java", src)
	got, ok := findJavaProducer(raws, "DELETE", "/api/orders/{id}")
	if !ok {
		t.Fatalf("expected producer DELETE /api/orders/{id}; got raws=%+v", raws)
	}
	if got.HandlerName != "deleteOrder" {
		t.Errorf("HandlerName = %q, want deleteOrder", got.HandlerName)
	}
}

// TestJavaJaxrsProducer verifies JAX-RS class @Path + method @Path + @GET.
func TestJavaJaxrsProducer(t *testing.T) {
	src := `@Path("/api/v2")
public class ItemResource {

    @GET
    @Path("/items/{id}")
    @Produces("application/json")
    public Item getItem(@PathParam("id") String id) {
        return repo.get(id);
    }
}
`
	raws := javaRoutes("ItemResource.java", src)
	got, ok := findJavaProducer(raws, "GET", "/api/v2/items/{id}")
	if !ok {
		t.Fatalf("expected producer GET /api/v2/items/{id}; got raws=%+v", raws)
	}
	if got.HandlerName != "getItem" {
		t.Errorf("HandlerName = %q, want getItem", got.HandlerName)
	}
}

// TestJavaRestTemplateConsumer verifies a RestTemplate.getForObject call with a
// concatenated URL yields a consumer raw whose path is the literal prefix and
// whose calling symbol is the enclosing method.
func TestJavaRestTemplateConsumer(t *testing.T) {
	src := `public class UserGateway {

    public User fetchUser(String id) {
        return restTemplate.getForObject("http://svc/api/v1/users/" + id, User.class);
    }
}
`
	raws := javaRoutes("UserGateway.java", src)
	got, ok := findJavaConsumer(raws, "GET", "http://svc/api/v1/users/")
	if !ok {
		t.Fatalf("expected consumer GET http://svc/api/v1/users/ ; got raws=%+v", raws)
	}
	if got.Path != "/api/v1/users/" {
		t.Errorf("Path = %q, want /api/v1/users/", got.Path)
	}
	if got.CallingSymbol != "fetchUser" {
		t.Errorf("CallingSymbol = %q, want fetchUser", got.CallingSymbol)
	}
	if got.File != "UserGateway.java" {
		t.Errorf("File = %q, want UserGateway.java", got.File)
	}
}

// TestJavaConsumerClients exercises the other Java HTTP clients.
func TestJavaConsumerClients(t *testing.T) {
	src := `public class Clients {

    public void exchangeCall() {
        restTemplate.exchange("http://svc/api/v1/orders", HttpMethod.PUT, entity, String.class);
    }

    public void webClientCall() {
        webClient.post().uri("http://svc/api/v1/items").retrieve();
    }

    public void okHttpCall() {
        Request req = new Request.Builder().url("http://svc/api/v1/things").build();
    }

    public void netHttpCall() {
        HttpRequest r = HttpRequest.newBuilder().uri(URI.create("http://svc/api/v1/data")).GET().build();
    }
}
`
	raws := javaRoutes("Clients.java", src)

	if got, ok := findJavaConsumer(raws, "PUT", "http://svc/api/v1/orders"); !ok {
		t.Errorf("expected exchange consumer PUT /api/v1/orders; got raws=%+v", raws)
	} else if got.CallingSymbol != "exchangeCall" {
		t.Errorf("exchange CallingSymbol = %q, want exchangeCall", got.CallingSymbol)
	}

	if _, ok := findJavaConsumer(raws, "POST", "http://svc/api/v1/items"); !ok {
		t.Errorf("expected webClient consumer POST /api/v1/items; got raws=%+v", raws)
	}

	// OkHttp .url() with no inline verb -> method unknown ("").
	if got, ok := findJavaConsumer(raws, "", "http://svc/api/v1/things"); !ok {
		t.Errorf("expected okHttp consumer /api/v1/things; got raws=%+v", raws)
	} else if got.CallingSymbol != "okHttpCall" {
		t.Errorf("okHttp CallingSymbol = %q, want okHttpCall", got.CallingSymbol)
	}

	// java.net.http .uri(URI.create(...)).GET() -> GET.
	if _, ok := findJavaConsumer(raws, "GET", "http://svc/api/v1/data"); !ok {
		t.Errorf("expected netHttp consumer GET /api/v1/data; got raws=%+v", raws)
	}
}
