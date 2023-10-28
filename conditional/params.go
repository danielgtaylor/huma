// Package conditional provides utilities for working with HTTP conditional
// requests using the `If-Match`, `If-None-Match`, `If-Modified-Since`, and
// `If-Unmodified-Since` headers along with ETags and last modified times.
//
// In general, conditional requests with tight integration into your data
// store will be preferred as they are more efficient. However, this package
// provides a simple way to get started with conditional requests and once
// the functionality is in place the performance can be improved later. You
// still get the benefits of not sending extra data over the wire and
// distributed write protections that prevent different users from
// overwriting each other's changes.
package conditional

import (
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

// trimETag removes the quotes and `W/` prefix for incoming ETag values to
// make comparisons easier.
func trimETag(value string) string {
	if strings.HasPrefix(value, "W/") && len(value) > 2 {
		value = value[2:]
	}
	return strings.Trim(value, "\"")
}

// Params allow clients to send ETags or times to make a read or
// write conditional based on the state of the resource on the server, e.g.
// when it was last modified. This is useful for determining when a cache
// should be updated or to prevent multiple writers from overwriting each
// other's changes.
type Params struct {
	IfMatch           []string  `header:"If-Match" doc:"Succeeds if the server's resource matches one of the passed values."`
	IfNoneMatch       []string  `header:"If-None-Match" doc:"Succeeds if the server's resource matches none of the passed values. On writes, the special value * may be used to match any existing value."`
	IfModifiedSince   time.Time `header:"If-Modified-Since" doc:"Succeeds if the server's resource date is more recent than the passed date."`
	IfUnmodifiedSince time.Time `header:"If-Unmodified-Since" doc:"Succeeds if the server's resource date is older or the same as the passed date."`

	// isWrite tracks whether we should emit errors vs. a 304 Not Modified from
	// the `PreconditionFailed` method.
	isWrite bool
}

func (p *Params) Resolve(ctx huma.Context) []error {
	switch ctx.Method() {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		p.isWrite = true
	}
	return nil
}

// HasConditionalParams returns true if any conditional request headers have
// been set on the incoming request.
func (p *Params) HasConditionalParams() bool {
	return len(p.IfMatch) > 0 || len(p.IfNoneMatch) > 0 || !p.IfModifiedSince.IsZero() || !p.IfUnmodifiedSince.IsZero()
}

// PreconditionFailed returns false if no conditional headers are present, or if
// the values passed fail based on the conditional read/write rules. See also:
// https://developer.mozilla.org/en-US/docs/Web/HTTP/Conditional_requests.
// This method assumes there is some fast/efficient way to get a resource's
// current ETag and/or last-modified time before it is run.
func (p *Params) PreconditionFailed(etag string, modified time.Time) huma.StatusError {
	failed := false
	errors := []error{}

	foundMsg := "found no existing resource"
	if etag != "" {
		foundMsg = "found resource with ETag " + etag
	}

	// If-None-Match fails on the first match. The `*` is a special case meaning
	// to match any existing value.
	for _, match := range p.IfNoneMatch {
		trimmed := trimETag(match)
		if trimmed == etag || (trimmed == "*" && etag != "") {
			// We matched an existing resource, abort!
			if p.isWrite {
				errors = append(errors, &huma.ErrorDetail{
					Message:  "If-None-Match: " + match + " precondition failed, " + foundMsg,
					Location: "request.headers.If-None-Match",
					Value:    match,
				})
			}
			failed = true
		}
	}

	// If-Match fails if none of the passed ETags matches the current resource.
	if len(p.IfMatch) > 0 {
		found := false
		for _, match := range p.IfMatch {
			if trimETag(match) == etag {
				found = true
				break
			}
		}

		if !found {
			// We did not match the expected resource, abort!
			if p.isWrite {
				errors = append(errors, &huma.ErrorDetail{
					Message:  "If-Match precondition failed, " + foundMsg,
					Location: "request.headers.If-Match",
					Value:    p.IfMatch,
				})
			}
			failed = true
		}
	}

	if !p.IfModifiedSince.IsZero() && !modified.After(p.IfModifiedSince) {
		// Resource was modified *before* the date that was passed, abort!
		if p.isWrite {
			errors = append(errors, &huma.ErrorDetail{
				Message:  "If-Modified-Since: " + p.IfModifiedSince.Format(http.TimeFormat) + " precondition failed, resource was modified at " + modified.Format(http.TimeFormat),
				Location: "request.headers.If-Modified-Since",
				Value:    p.IfModifiedSince.Format(http.TimeFormat),
			})
		}
		failed = true
	}

	if !p.IfUnmodifiedSince.IsZero() && modified.After(p.IfUnmodifiedSince) {
		// Resource was modified *after* the date that was passed, abort!
		if p.isWrite {
			errors = append(errors, &huma.ErrorDetail{
				Message:  "If-Unmodified-Since: " + p.IfUnmodifiedSince.Format(http.TimeFormat) + " precondition failed, resource was modified at " + modified.Format(http.TimeFormat),
				Location: "request.headers.If-Unmodified-Since",
				Value:    p.IfUnmodifiedSince.Format(http.TimeFormat),
			})
		}
		failed = true
	}

	if failed {
		if p.isWrite {
			return huma.NewError(
				http.StatusPreconditionFailed,
				http.StatusText(http.StatusPreconditionFailed),
				errors...,
			)
		}

		return huma.Status304NotModified()
	}

	return nil
}
