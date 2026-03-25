package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGzipMiddleware_CompressesWhenAccepted(t *testing.T) {
	// Create a handler that returns JSON
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message":"hello world","data":"some test data that should compress well"}`))
	})

	// Wrap with gzip middleware
	gzipped := GzipMiddleware(handler)

	// Create request with Accept-Encoding: gzip
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")

	rec := httptest.NewRecorder()
	gzipped.ServeHTTP(rec, req)

	// Verify response is compressed
	assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))
	assert.Contains(t, rec.Header().Get("Vary"), "Accept-Encoding")

	// Verify we can decompress the response
	reader, err := gzip.NewReader(rec.Body)
	require.NoError(t, err)
	defer reader.Close()

	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Contains(t, string(body), `"message":"hello world"`)
}

func TestGzipMiddleware_NoCompressionWithoutAcceptHeader(t *testing.T) {
	// Create a handler that returns JSON
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message":"hello world"}`))
	})

	// Wrap with gzip middleware
	gzipped := GzipMiddleware(handler)

	// Create request WITHOUT Accept-Encoding header
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)

	rec := httptest.NewRecorder()
	gzipped.ServeHTTP(rec, req)

	// Verify response is NOT compressed
	assert.Empty(t, rec.Header().Get("Content-Encoding"))
	assert.Equal(t, `{"message":"hello world"}`, rec.Body.String())
}

func TestGzipMiddleware_HandlesDeflateOnly(t *testing.T) {
	// Create a handler that returns JSON
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message":"hello world"}`))
	})

	// Wrap with gzip middleware
	gzipped := GzipMiddleware(handler)

	// Create request with only deflate encoding (no gzip)
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Accept-Encoding", "deflate")

	rec := httptest.NewRecorder()
	gzipped.ServeHTTP(rec, req)

	// Verify response is NOT compressed (we only support gzip)
	assert.Empty(t, rec.Header().Get("Content-Encoding"))
	assert.Equal(t, `{"message":"hello world"}`, rec.Body.String())
}

func TestGzipMiddleware_MultipleEncodings(t *testing.T) {
	// Create a handler that returns text
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("hello world"))
	})

	// Wrap with gzip middleware
	gzipped := GzipMiddleware(handler)

	// Create request with multiple encodings including gzip
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Accept-Encoding", "deflate, gzip, br")

	rec := httptest.NewRecorder()
	gzipped.ServeHTTP(rec, req)

	// Verify response is compressed with gzip
	assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))
}

func TestGzipMiddleware_LargeResponse(t *testing.T) {
	// Create a handler that returns a large response
	largeData := strings.Repeat("hello world ", 10000)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(largeData))
	})

	// Wrap with gzip middleware
	gzipped := GzipMiddleware(handler)

	// Create request with Accept-Encoding: gzip
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")

	rec := httptest.NewRecorder()
	gzipped.ServeHTTP(rec, req)

	// Verify response is compressed
	assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))

	// Compressed size should be significantly smaller than original
	assert.Less(t, rec.Body.Len(), len(largeData)/2, "compressed size should be much smaller than original")

	// Verify we can decompress and get the original content
	reader, err := gzip.NewReader(rec.Body)
	require.NoError(t, err)
	defer reader.Close()

	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, largeData, string(body))
}

func TestGzipResponseWriter_Flush(t *testing.T) {
	// Create a handler that flushes
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("chunk 1"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		w.Write([]byte("chunk 2"))
	})

	// Wrap with gzip middleware
	gzipped := GzipMiddleware(handler)

	// Create request with Accept-Encoding: gzip
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")

	rec := httptest.NewRecorder()
	gzipped.ServeHTTP(rec, req)

	// Verify response is compressed
	assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))

	// Verify we can decompress the response
	reader, err := gzip.NewReader(rec.Body)
	require.NoError(t, err)
	defer reader.Close()

	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, "chunk 1chunk 2", string(body))
}
