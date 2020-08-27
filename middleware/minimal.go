package middleware

import (
	"net/http"
)

type minimalWriter struct {
	http.ResponseWriter
	status int
}

func (w *minimalWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}

	if w.status == http.StatusNoContent {
		return 0, nil
	}

	return w.ResponseWriter.Write(data)
}

func (w *minimalWriter) WriteHeader(statusCode int) {
	if statusCode >= 200 && statusCode < 300 {
		statusCode = http.StatusNoContent
	}

	w.status = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

// PreferMinimal will remove the response body and return 204 No
// Content for any 2xx response where the request had the Prefer: return=minimal
// set on the request.
func PreferMinimal(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Wrap the response writer
		if r.Header.Get("Prefer") == "return=minimal" {
			w = &minimalWriter{ResponseWriter: w}
		}

		next.ServeHTTP(w, r)
	})
}
