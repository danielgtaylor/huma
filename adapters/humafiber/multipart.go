package humafiber

import (
	"errors"
	"math"
	"mime"
	"mime/multipart"
	"net/http"

	"github.com/valyala/fasthttp"
)

// multipartFormWithLimit parses the multipart form of req, enforcing maxBytes.
// Exceeding it returns *http.MaxBytesError, as the net/http adapters do, so
// Huma responds with a 413.
func multipartFormWithLimit(req *fasthttp.Request, srv *fasthttp.Server, maxBytes int64) (*multipart.Form, error) {
	limit := maxBytes
	if !srv.StreamRequestBody && srv.MaxRequestBodySize > 0 {
		// Never raise the app-wide BodyLimit. When streaming, it is only the
		// buffering threshold (often set tiny), so it doesn't cap the body.
		limit = min(limit, int64(srv.MaxRequestBodySize))
	}
	// fasthttp computes limit+1, so keep that from overflowing.
	limit = min(limit, math.MaxInt-1)
	tooLarge := &http.MaxBytesError{Limit: limit}

	// fasthttp may already have parsed a form of known length while reading
	// the request, and MultipartFormWithLimit returns that one unchecked.
	if int64(req.Header.ContentLength()) > limit {
		return nil, tooLarge
	}

	// fasthttp matches the media type and "boundary" case-sensitively, so fold
	// them like Fiber v3's MultipartForm does.
	if mt, params, err := mime.ParseMediaType(string(req.Header.ContentType())); err == nil {
		if ct := mime.FormatMediaType(mt, params); ct != "" {
			req.Header.SetContentType(ct)
		}
	}

	form, err := req.MultipartFormWithLimit(int(limit))
	if errors.Is(err, fasthttp.ErrBodyTooLarge) {
		return nil, tooLarge
	}
	return form, err
}
