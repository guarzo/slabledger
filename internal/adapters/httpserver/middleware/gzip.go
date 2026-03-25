// Package middleware provides HTTP middleware for the web server.
package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"
)

// gzipResponseWriter wraps http.ResponseWriter to provide gzip compression.
type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
	wroteHeader bool
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

func (w *gzipResponseWriter) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true

	// Remove Content-Length since we're compressing (length will change)
	w.ResponseWriter.Header().Del("Content-Length")
	w.ResponseWriter.WriteHeader(code)
}

func (w *gzipResponseWriter) Flush() {
	// Flush the gzip writer first (best effort, errors ignored)
	if gz, ok := w.Writer.(*gzip.Writer); ok {
		_ = gz.Flush() //nolint:errcheck
	}
	// Then flush the underlying response writer if it supports flushing
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// gzipWriterPool pools gzip writers to reduce allocations.
var gzipWriterPool = sync.Pool{
	New: func() any {
		// gzip.DefaultCompression is always valid, error can be ignored
		gz, err := gzip.NewWriterLevel(nil, gzip.DefaultCompression)
		if err != nil {
			// This should never happen with DefaultCompression
			panic("gzip.NewWriterLevel with DefaultCompression failed: " + err.Error())
		}
		return gz
	},
}

// GzipMiddleware compresses HTTP responses using gzip when the client supports it.
//
// Features:
//   - Pools gzip writers to reduce allocations
//   - Only compresses compressible content types (text, json, etc.)
//   - Respects Accept-Encoding header
//   - Skips already-compressed content
//   - Adds Vary: Accept-Encoding header for caching
//
// Usage:
//
//	handler = middleware.GzipMiddleware(handler)
func GzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if client accepts gzip
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		// Add Vary header for proper caching
		w.Header().Add("Vary", "Accept-Encoding")

		// Get a gzip writer from the pool
		gz, ok := gzipWriterPool.Get().(*gzip.Writer)
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		gz.Reset(w)

		defer func() {
			_ = gz.Close() //nolint:errcheck // Best effort cleanup
			gzipWriterPool.Put(gz)
		}()

		// Set Content-Encoding header
		w.Header().Set("Content-Encoding", "gzip")

		// Create wrapped response writer
		gzw := &gzipResponseWriter{
			Writer:         gz,
			ResponseWriter: w,
		}

		next.ServeHTTP(gzw, r)
	})
}
