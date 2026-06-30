package middleware

import (
	"mime"
	"net/http"
)

// maxBodyBytes is the upper limit for request body size on mutation requests.
const maxBodyBytes = 1 << 20 // 1 MiB

// Validation checks basic request structure before it reaches a handler.
// Enforces Content-Type and body size on mutation requests.
func Validation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
			hasBody := r.Body != nil && (r.ContentLength > 0 || len(r.TransferEncoding) > 0)
			if hasBody {
				if r.ContentLength > maxBodyBytes {
					http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
					return
				}
				r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

				mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
				if err != nil || mediaType != "application/json" {
					http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
					return
				}
			}
		}

		next.ServeHTTP(w, r)
	})
}
