package handler

import (
	"compress/flate"
	"compress/gzip"
	"io"
	"net/http"

	"github.com/andybalholm/brotli"
)

// decompressBody wraps resp.Body with a decompressing reader based on
// Content-Encoding. Supported encodings: gzip, br (brotli), deflate.
// After wrapping, the Content-Encoding header is removed and ContentLength
// is set to -1 since the decompressed size is unknown.
// If the encoding is empty, unknown, or decompression fails, resp is unchanged.
func decompressBody(resp *http.Response) {
	ce := resp.Header.Get("Content-Encoding")
	if ce == "" {
		return
	}

	var r io.Reader
	switch ce {
	case "gzip":
		gr, err := gzip.NewReader(resp.Body)
		if err != nil {
			return
		}
		r = gr
	case "br":
		r = brotli.NewReader(resp.Body)
	case "deflate":
		r = flate.NewReader(resp.Body)
	default:
		return
	}

	resp.Body = &readerWithClose{reader: r, underlying: resp.Body}
	resp.Header.Del("Content-Encoding")
	resp.ContentLength = -1
}

// readerWithClose wraps an io.Reader with the underlying io.ReadCloser's Close.
type readerWithClose struct {
	reader     io.Reader
	underlying io.ReadCloser
}

func (r *readerWithClose) Read(p []byte) (int, error) { return r.reader.Read(p) }
func (r *readerWithClose) Close() error               { return r.underlying.Close() }
