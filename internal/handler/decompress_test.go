package handler

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func compressBrotli(data []byte) []byte {
	var buf bytes.Buffer
	w := brotli.NewWriter(&buf)
	w.Write(data)
	w.Close()
	return buf.Bytes()
}

func compressDeflate(data []byte) []byte {
	var buf bytes.Buffer
	w, _ := flate.NewWriter(&buf, flate.DefaultCompression)
	w.Write(data)
	w.Close()
	return buf.Bytes()
}

func compressGzip(data []byte) []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	w.Write(data)
	w.Close()
	return buf.Bytes()
}

func TestDecompressBody_Gzip(t *testing.T) {
	original := []byte("event: message_start\ndata: {\"type\":\"message_start\"}\n\n")
	resp := &http.Response{
		Header:        http.Header{"Content-Encoding": []string{"gzip"}},
		Body:          io.NopCloser(bytes.NewReader(compressGzip(original))),
		ContentLength: int64(len(compressGzip(original))),
	}

	decompressBody(resp)

	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, original, got)
	assert.Empty(t, resp.Header.Get("Content-Encoding"))
	assert.Equal(t, int64(-1), resp.ContentLength)
}

func TestDecompressBody_Brotli(t *testing.T) {
	original := []byte("event: message_start\ndata: {\"type\":\"message_start\"}\n\n")
	resp := &http.Response{
		Header:        http.Header{"Content-Encoding": []string{"br"}},
		Body:          io.NopCloser(bytes.NewReader(compressBrotli(original))),
		ContentLength: 100,
	}

	decompressBody(resp)

	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, original, got)
	assert.Empty(t, resp.Header.Get("Content-Encoding"))
	assert.Equal(t, int64(-1), resp.ContentLength)
}

func TestDecompressBody_Deflate(t *testing.T) {
	original := []byte("event: message_start\ndata: {\"type\":\"message_start\"}\n\n")
	resp := &http.Response{
		Header:        http.Header{"Content-Encoding": []string{"deflate"}},
		Body:          io.NopCloser(bytes.NewReader(compressDeflate(original))),
		ContentLength: 100,
	}

	decompressBody(resp)

	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, original, got)
	assert.Empty(t, resp.Header.Get("Content-Encoding"))
	assert.Equal(t, int64(-1), resp.ContentLength)
}

func TestDecompressBody_NoEncoding(t *testing.T) {
	body := []byte("hello")
	resp := &http.Response{
		Header:        http.Header{},
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: 5,
	}

	decompressBody(resp)

	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, body, got)
	assert.Equal(t, int64(5), resp.ContentLength)
}

func TestDecompressBody_UnknownEncoding(t *testing.T) {
	body := []byte("hello")
	resp := &http.Response{
		Header:        http.Header{"Content-Encoding": []string{"zstd"}},
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: 5,
	}

	decompressBody(resp)

	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, body, got)
	assert.Equal(t, "zstd", resp.Header.Get("Content-Encoding"))
	assert.Equal(t, int64(5), resp.ContentLength)
}

type trackingCloser struct {
	closed bool
}

func (t *trackingCloser) Read(p []byte) (int, error) { return 0, io.EOF }
func (t *trackingCloser) Close() error               { t.closed = true; return nil }

func TestDecompressBody_CloseUnderlying(t *testing.T) {
	original := []byte("hello")
	underlying := &trackingCloser{}
	compressed := compressBrotli(original)
	resp := &http.Response{
		Header: http.Header{"Content-Encoding": []string{"br"}},
		Body: struct {
			io.Reader
			io.Closer
		}{bytes.NewReader(compressed), underlying},
		ContentLength: 100,
	}

	decompressBody(resp)
	resp.Body.Close()

	assert.True(t, underlying.closed, "underlying body should be closed")
}

func TestDecompressBody_InvalidGzip(t *testing.T) {
	resp := &http.Response{
		Header:        http.Header{"Content-Encoding": []string{"gzip"}},
		Body:          io.NopCloser(strings.NewReader("not gzip data")),
		ContentLength: 13,
	}

	// Should not panic, resp unchanged
	decompressBody(resp)

	assert.Equal(t, "gzip", resp.Header.Get("Content-Encoding"))
	assert.Equal(t, int64(13), resp.ContentLength)
}
