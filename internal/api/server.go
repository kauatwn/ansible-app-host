package api

import "net/http"

func SetupRouter() http.Handler {
	mux := http.NewServeMux()
	hello := NewHelloHandler()

	// O sufixo {$} exige correspondência exata com a raiz "/"
	mux.HandleFunc("GET /{$}", hello.Hello)

	return LoggingMiddleware(mux)
}
