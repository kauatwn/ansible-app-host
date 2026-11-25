package api

import (
	"log/slog"
	"net/http"
	"time"
)

// statusResponseWriter encapsula o http.ResponseWriter padrão para capturar
// o primeiro código de status HTTP enviado pelo handler.
type statusResponseWriter struct {
	http.ResponseWriter
	statusCode  int
	wroteHeader bool
}

// WriteHeader intercepta o envio explícito do status code.
// A trava wroteHeader garante que apenas o primeiro status enviado seja registrado,
// refletindo o comportamento real do protocolo HTTP no net/http.
func (w *statusResponseWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.statusCode = code
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(code)
}

// Write intercepta a escrita do corpo da resposta.
// Caso o handler escreva dados sem chamar WriteHeader antes, o net/http assume
// automaticamente o status 200 OK e trava o cabeçalho.
func (w *statusResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.wroteHeader = true
	}
	return w.ResponseWriter.Write(b)
}

// Unwrap expõe o http.ResponseWriter original para o http.ResponseController,
// preservando recursos opcionais da conexão (como http.Flusher ou http.Hijacker).
func (w *statusResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		sw := &statusResponseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		defer func() {
			slog.Info("HTTP request received",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", sw.statusCode),
				slog.Float64("duration_seconds", time.Since(start).Seconds()),
				slog.String("remote_addr", r.RemoteAddr),
			)
		}()

		next.ServeHTTP(sw, r)
	})
}
