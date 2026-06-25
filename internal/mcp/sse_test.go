package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// readSSEEvent scans an open SSE stream for the next event with a matching name
// and returns its concatenated `data:` payload. It returns ("", false) if the
// reader hits EOF / error before such an event arrives.
func readSSEEvent(t *testing.T, sc *bufio.Scanner, want string) (string, bool) {
	t.Helper()
	var (
		event string
		data  []string
	)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			// Blank line terminates an event.
			if event == want && len(data) > 0 {
				return strings.Join(data, "\n"), true
			}
			event, data = "", nil
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		case strings.HasPrefix(line, ":"):
			// comment (ping) — ignore
		}
	}
	return "", false
}

// TestSSEHandler_EndpointAndMessage exercises the full legacy SSE round-trip:
// open GET /sse, read the `endpoint` event to learn the POST-back URL, POST a
// tools/list to /message, and assert the JSON-RPC tools list arrives as a
// `message` SSE frame on the open stream.
func TestSSEHandler_EndpointAndMessage(t *testing.T) {
	handler := NewServer(stubEngine{}).SSEHandler()
	mux := http.NewServeMux()
	mux.Handle("/sse", handler)
	mux.Handle("/message", handler)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Open the event-stream.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/sse", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open sse: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type = %q, want text/event-stream", ct)
	}

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	// First event must be `endpoint` carrying /message?sessionId=<id>.
	endpoint, ok := readSSEEvent(t, sc, "endpoint")
	if !ok {
		t.Fatal("did not receive endpoint event")
	}
	if !strings.HasPrefix(endpoint, "/message?sessionId=") {
		t.Fatalf("endpoint = %q, want prefix /message?sessionId=", endpoint)
	}

	// POST a tools/list to the advertised endpoint.
	postURL := ts.URL + endpoint
	postReq, err := http.NewRequestWithContext(ctx, http.MethodPost, postURL,
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	if err != nil {
		t.Fatalf("new post request: %v", err)
	}
	postReq.Header.Set("Content-Type", "application/json")
	postResp, err := http.DefaultClient.Do(postReq)
	if err != nil {
		t.Fatalf("post message: %v", err)
	}
	postResp.Body.Close()
	if postResp.StatusCode != http.StatusAccepted {
		t.Fatalf("POST /message status = %d, want 202", postResp.StatusCode)
	}

	// The JSON-RPC response must arrive on the stream as a `message` event.
	msg, ok := readSSEEvent(t, sc, "message")
	if !ok {
		t.Fatal("did not receive message event with the tools/list response")
	}

	var rpc struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  struct {
			Tools []Tool `json:"tools"`
		} `json:"result"`
		Error *rpcError `json:"error"`
	}
	if err := json.Unmarshal([]byte(msg), &rpc); err != nil {
		t.Fatalf("unmarshal message: %v; data=%s", err, msg)
	}
	if rpc.JSONRPC != "2.0" {
		t.Errorf("jsonrpc = %q, want 2.0", rpc.JSONRPC)
	}
	if string(rpc.ID) != "1" {
		t.Errorf("id = %s, want 1", rpc.ID)
	}
	if rpc.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", rpc.Error)
	}
	if len(rpc.Result.Tools) == 0 {
		t.Fatal("tools list is empty over SSE")
	}
	var found bool
	for _, tool := range rpc.Result.Tools {
		if tool.Name == "status" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("SSE tools/list missing status tool; got %d tools", len(rpc.Result.Tools))
	}
}

// TestSSEHandler_UnknownSession asserts a POST to an unregistered sessionId is
// rejected (the stream registry is the source of truth).
func TestSSEHandler_UnknownSession(t *testing.T) {
	handler := NewServer(stubEngine{}).SSEHandler()
	mux := http.NewServeMux()
	mux.Handle("/sse", handler)
	mux.Handle("/message", handler)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/message?sessionId=does-not-exist", "application/json",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for unknown session", resp.StatusCode)
	}
}
