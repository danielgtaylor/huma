package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"io/ioutil"
	"net/http"
	"path"
	"strconv"
	"sync"

	"github.com/andybalholm/brotli"
	"github.com/istreamlabs/huma/negotiation"
)

const gzipEncoding = "gzip"
const brotliEncoding = "br"

var supportedEncodings []string = []string{brotliEncoding, gzipEncoding}
var compressDenyList []string = []string{".gif", ".png", ".jpg", ".jpeg", ".zip", ".gz", ".bz2"}

type contentEncodingWriter struct {
	http.ResponseWriter
	status      int
	encoding    string
	buf         *bytes.Buffer
	writer      io.Writer
	minSize     int
	gzPool      *sync.Pool
	brPool      *sync.Pool
	wroteHeader bool
}

func (w *contentEncodingWriter) Write(data []byte) (int, error) {
	if w.writer != nil {
		// We are writing compressed data.
		return w.writer.Write(data)
	}

	// Buffer the data until we can decide whether to compress it or not.
	w.buf.Write(data)

	cl, _ := strconv.Atoi(w.Header().Get("Content-Length"))
	if cl >= w.minSize || w.buf.Len() >= w.minSize {
		// We reached our minimum compression size. Set the writer, write the buffer
		// and make sure to set the correct headers.
		switch w.encoding {
		case gzipEncoding:
			gz := w.gzPool.Get().(*gzip.Writer)
			gz.Reset(w.ResponseWriter)
			w.writer = gz
		case brotliEncoding:
			br := w.brPool.Get().(*brotli.Writer)
			br.Reset(w.ResponseWriter)
			w.writer = br
		}
		w.Header().Set("Content-Encoding", w.encoding)
		w.Header().Set("Vary", "Accept-Encoding")
		w.ResponseWriter.WriteHeader(w.status)
		w.wroteHeader = true
		bufData := w.buf.Bytes()
		w.buf.Reset()
		return w.writer.Write(bufData)
	}

	// Not sure yet whether this should be compressed.
	return len(data), nil
}

func (w *contentEncodingWriter) WriteHeader(code int) {
	w.Header().Del("Content-Length")
	w.status = code
}

func (w *contentEncodingWriter) Close() {
	if !w.wroteHeader {
		w.ResponseWriter.WriteHeader(w.status)
	}

	if w.buf.Len() > 0 {
		w.ResponseWriter.Write(w.buf.Bytes())
	}

	if w.writer != nil {
		if wc, ok := w.writer.(io.WriteCloser); ok {
			wc.Close()
		}

		// Return the writer to the pool so it can be reused.
		switch w.encoding {
		case gzipEncoding:
			w.gzPool.Put(w.writer)
		case brotliEncoding:
			w.brPool.Put(w.writer)
		}
	}
}

// ContentEncoding uses content negotiation with the client to pick
// an appropriate encoding (compression) method and transparently encodes
// the response. Supports GZip and Brotli.
func ContentEncoding(next http.Handler) http.Handler {
	// Use pools to reduce allocations. We use a byte buffer to temporarily store
	// some of each response in order to determine whether compression should
	// be applied. The others are just re-using the GZip and Brotli compressors.
	bufPool := sync.Pool{
		New: func() interface{} {
			return new(bytes.Buffer)
		},
	}

	gzPool := sync.Pool{
		New: func() interface{} {
			return gzip.NewWriter(ioutil.Discard)
		},
	}

	brPool := sync.Pool{
		New: func() interface{} {
			return brotli.NewWriter(ioutil.Discard)
		},
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ext := path.Ext(r.URL.Path); ext != "" {
			for _, deny := range compressDenyList {
				if ext == deny {
					// This is a file type we should not try to compress.
					next.ServeHTTP(w, r)
					return
				}
			}
		}

		if ac := r.Header.Get("Accept-Encoding"); ac != "" {
			best := negotiation.SelectQValue(ac, supportedEncodings)

			if best != "" {
				buf := bufPool.Get().(*bytes.Buffer)
				buf.Reset()
				defer bufPool.Put(buf)

				cew := &contentEncodingWriter{
					ResponseWriter: w,
					status:         http.StatusOK,
					encoding:       best,
					buf:            buf,
					gzPool:         &gzPool,
					brPool:         &brPool,

					// minSize of the body at which compression is enabled. Internet MTU
					// size is 1500 bytes, so anything smaller will still require sending
					// at least that size (including headers). Google's research at
					// http://dev.chromium.org/spdy/spdy-whitepaper suggests headers
					// are at least 200 bytes and average 700-800 bytes. If we assume
					// an average 30% compression ratio and 500 bytes of headers, then
					// (1400 * 0.7) + 500 = 1480 bytes, just about the minimum MTU.
					minSize: 1400,
				}

				// Since we aren't sure if we will be compressing the response (due
				// to size), here we trigger a call to close the writer after all
				// writes have completed. This will send the status/headers and flush
				// any buffers as needed.
				defer cew.Close()
				w = cew
			}
		}

		next.ServeHTTP(w, r)
	})
}
