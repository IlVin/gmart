package humawrapper

import (
	"log/slog"
	"net/http"
)

// 1. Создаем ResponseRecorder, чтобы прочитать записанное тело
type bodyCaptureWriter struct {
	http.ResponseWriter
	body       []byte
	statusCode int
}

func (w *bodyCaptureWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *bodyCaptureWriter) Write(b []byte) (int, error) {
	w.body = append(w.body, b...)
	return w.ResponseWriter.Write(b)
}

// 2. Сам Middleware
func HumaErrorLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Оборачиваем оригинальный Writer
		bcw := &bodyCaptureWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(bcw, r)

		// Логируем, если это ошибка (>= 400)
		if bcw.statusCode >= 400 {
			slog.Error("Huma API Error Response",
				slog.Int("status", bcw.statusCode),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("body", string(bcw.body)), // Тот самый JSON с ошибкой
			)
		}
	})
}
