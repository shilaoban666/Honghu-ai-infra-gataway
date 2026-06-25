package gatewayhttp

import (
	"context"
	"net/http"
	"strings"
)

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if requestID == "" {
			requestID = randomID("req")
		}
		traceID := strings.TrimSpace(r.Header.Get("X-Trace-ID"))
		if traceID == "" {
			traceID = randomID("trace")
		}

		ctx := context.WithValue(r.Context(), requestIDContextKey{}, requestID)
		ctx = context.WithValue(ctx, traceIDContextKey{}, traceID)
		w.Header().Set("X-Request-ID", requestID)
		w.Header().Set("X-Trace-ID", traceID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func authMiddleware(keys []string, next http.Handler) http.Handler {
	keySet := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if trimmed := strings.TrimSpace(key); trimmed != "" {
			keySet[trimmed] = struct{}{}
		}
	}
	if len(keySet) == 0 {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := keySet[apiKeyFrom(r)]; !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", errUnauthorized.Error())
			return
		}
		next.ServeHTTP(w, r)
	})
}

func apiKeyFrom(r *http.Request) string {
	if key := strings.TrimSpace(r.Header.Get("X-API-Key")); key != "" {
		return key
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[len("Bearer "):])
	}
	return ""
}
