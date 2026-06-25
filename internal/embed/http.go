package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// httpTimeout bounds a single embeddings request.
const httpTimeout = 30 * time.Second

// httpProvider POSTs to an OpenAI/Ollama-compatible embeddings endpoint. It is
// selected when ATLAS_EMBED_URL is set, so a real embedding model can back
// semantic search instead of the offline Hashing default.
//
// Request shape (best-effort compatibility):
//   - OpenAI: {"model": MODEL, "input": [TEXT, ...]}
//   - Ollama: {"model": MODEL, "prompt": TEXT} for a single text — we send the
//     {"input": [...]} batch form and ALSO populate "prompt" with the first text
//     so a single-text call works against either server.
//
// Response shape (any of):
//   - {"data": [{"embedding": [...]}, ...]}     (OpenAI)
//   - {"embeddings": [[...], ...]}              (Ollama batch)
//   - {"embedding": [...]}                      (Ollama single)
//
// Dim() is learned lazily from the first successful response; it is 0 until then.
type httpProvider struct {
	url    string
	model  string
	client *http.Client
	dim    int
}

// NewHTTP builds the HTTP embeddings provider for url (and optional model).
func NewHTTP(url, model string) Provider {
	return &httpProvider{
		url:    url,
		model:  model,
		client: &http.Client{Timeout: httpTimeout},
	}
}

func (h *httpProvider) Dim() int     { return h.dim }
func (h *httpProvider) Name() string { return "http" }

// embedRequest is the union request body covering OpenAI ("input") and Ollama
// ("prompt") servers. The model is omitted when empty so a server with a fixed
// model still accepts the call.
type embedRequest struct {
	Model  string   `json:"model,omitempty"`
	Input  []string `json:"input,omitempty"`
	Prompt string   `json:"prompt,omitempty"`
}

// embedResponse decodes any of the three accepted response shapes.
type embedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Embeddings [][]float32 `json:"embeddings"`
	Embedding  []float32   `json:"embedding"`
}

// Embed sends one request for all texts and returns one vector per text in input
// order. It errors when the endpoint is unreachable, returns non-2xx, or the
// response shape yields a different count than was requested.
func (h *httpProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	reqBody := embedRequest{Model: h.model, Input: texts}
	// Populate the single-text Ollama field too so a one-text call works on a
	// server that only reads "prompt".
	if len(texts) == 1 {
		reqBody.Prompt = texts[0]
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("embed: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embed: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed: POST %s: %w", h.url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("embed: %s returned status %d", h.url, resp.StatusCode)
	}

	var decoded embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("embed: decode response: %w", err)
	}

	vecs := decoded.vectors()
	if len(vecs) != len(texts) {
		return nil, fmt.Errorf("embed: server returned %d vectors for %d texts", len(vecs), len(texts))
	}

	// L2-normalize so cosine == dot at query time (mirroring the Hashing provider),
	// and learn the dimension from the first vector.
	for i := range vecs {
		l2Normalize(vecs[i])
	}
	if len(vecs[0]) > 0 {
		h.dim = len(vecs[0])
	}
	return vecs, nil
}

// NOTE on URLs: the env value (ATLAS_EMBED_URL) is the EXACT POST target — we do
// not append /v1/embeddings. Point it at the full endpoint (e.g.
// http://localhost:11434/api/embeddings for Ollama,
// https://api.openai.com/v1/embeddings for OpenAI).

// vectors flattens whichever response shape was populated into an ordered slice.
func (r embedResponse) vectors() [][]float32 {
	switch {
	case len(r.Data) > 0:
		out := make([][]float32, len(r.Data))
		for i := range r.Data {
			out[i] = r.Data[i].Embedding
		}
		return out
	case len(r.Embeddings) > 0:
		return r.Embeddings
	case len(r.Embedding) > 0:
		return [][]float32{r.Embedding}
	default:
		return nil
	}
}
