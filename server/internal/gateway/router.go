package gateway

import "net/http"

func NewRouter(h *Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/proxy", h.Proxy)
	mux.HandleFunc("POST /v1/embeddings", h.Embeddings)
	mux.HandleFunc("POST /v1/generate", h.Generate)
	mux.HandleFunc("GET /v1/models", h.Models)
	mux.HandleFunc("GET /v1/usage", h.Usage)
	return mux
}
