package flow

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMatching(t *testing.T) {
	var tests = []struct {
		RouteMethods []string
		RoutePattern string

		RequestMethod string
		RequestPath   string

		ExpectedStatus      int
		ExpectedParams      map[string]string
		ExpectedAllowHeader string
	}{
		// simple path matching
		{
			[]string{"GET"}, "/one",
			"GET", "/one",
			http.StatusOK, nil, "",
		},
		{
			[]string{"GET"}, "/one",
			"GET", "/two",
			http.StatusNotFound, nil, "",
		},
		// nested
		{
			[]string{"GET"}, "/parent/child/one",
			"GET", "/parent/child/one",
			http.StatusOK, nil, "",
		},
		{
			[]string{"GET"}, "/parent/child/one",
			"GET", "/parent/child/two",
			http.StatusNotFound, nil, "",
		},
		// misc no matches
		{
			[]string{"GET"}, "/not/enough",
			"GET", "/not/enough/items",
			http.StatusNotFound, nil, "",
		},
		{
			[]string{"GET"}, "/not/enough/items",
			"GET", "/not/enough",
			http.StatusNotFound, nil, "",
		},
		// wilcards
		{
			[]string{"GET"}, "/prefix/...",
			"GET", "/prefix/anything/else",
			http.StatusOK, map[string]string{"...": "anything/else"}, "",
		},
		{
			[]string{"GET"}, "/prefix/...",
			"GET", "/prefix/",
			http.StatusOK, map[string]string{"...": ""}, "",
		},
		{
			[]string{"GET"}, "/prefix/...",
			"GET", "/prefix",
			http.StatusNotFound, nil, "",
		},
		{
			[]string{"GET"}, "/prefix",
			"GET", "/prefix/anything/else",
			http.StatusNotFound, nil, "",
		},
		{
			[]string{"GET"}, "/prefix/",
			"GET", "/prefix/anything/else",
			http.StatusNotFound, nil, "",
		},
		{
			[]string{"GET"}, "/prefix...",
			"GET", "/prefix/anything/else",
			http.StatusNotFound, nil, "",
		},
		// path params
		{
			[]string{"GET"}, "/path-params/:era/:group/:member",
			"GET", "/path-params/60/beatles/lennon",
			http.StatusOK, map[string]string{"era": "60", "group": "beatles", "member": "lennon"}, "",
		},
		{
			[]string{"GET"}, "/path-params/:era/:group/:member/foo",
			"GET", "/path-params/60/beatles/lennon/bar",
			http.StatusNotFound, map[string]string{"era": "60", "group": "beatles", "member": "lennon"}, "",
		},
		// regexp
		{
			[]string{"GET"}, "/path-params/:era|^[0-9]{2}$/:group|^[a-z].+$",
			"GET", "/path-params/60/beatles",
			http.StatusOK, map[string]string{"era": "60", "group": "beatles"}, "",
		},
		{
			[]string{"GET"}, "/path-params/:era|^[0-9]{2}$/:group|^[a-z].+$",
			"GET", "/path-params/abc/123",
			http.StatusNotFound, nil, "",
		},
		// kitchen sink
		{
			[]string{"GET"}, "/path-params/:id/:era|^[0-9]{2}$/...",
			"GET", "/path-params/abc/12/foo/bar/baz",
			http.StatusOK, map[string]string{"id": "abc", "era": "12", "...": "foo/bar/baz"}, "",
		},
		{
			[]string{"GET"}, "/path-params/:id/:era|^[0-9]{2}$/...",
			"GET", "/path-params/abc/12",
			http.StatusNotFound, nil, "",
		},
		// leading and trailing slashes
		{
			[]string{"GET"}, "slashes/one",
			"GET", "/slashes/one",
			http.StatusNotFound, nil, "",
		},
		{
			[]string{"GET"}, "/slashes/two",
			"GET", "slashes/two",
			http.StatusNotFound, nil, "",
		},
		{
			[]string{"GET"}, "/slashes/three/",
			"GET", "/slashes/three",
			http.StatusNotFound, nil, "",
		},
		{
			[]string{"GET"}, "/slashes/four",
			"GET", "/slashes/four/",
			http.StatusNotFound, nil, "",
		},
		// empty segments
		{
			[]string{"GET"}, "/baz/:id/:age",
			"GET", "/baz/123/",
			http.StatusNotFound, nil, "",
		},
		{
			[]string{"GET"}, "/baz/:id/:age/",
			"GET", "/baz/123//",
			http.StatusNotFound, nil, "",
		},
		{
			[]string{"GET"}, "/baz/:id/:age",
			"GET", "/baz//21",
			http.StatusNotFound, nil, "",
		},
		{
			[]string{"GET"}, "/baz//:age",
			"GET", "/baz//21",
			http.StatusOK, nil, "",
		},
		{
			// with a regexp to specifically allow empty segments
			[]string{"GET"}, "/baz/:id|^$/:age/",
			"GET", "/baz//21/",
			http.StatusOK, nil, "",
		},
		// methods
		{
			[]string{"POST"}, "/one",
			"POST", "/one",
			http.StatusOK, nil, "",
		},
		{
			[]string{"GET"}, "/one",
			"POST", "/one",
			http.StatusMethodNotAllowed, nil, "",
		},
		// multiple methods
		{
			[]string{"GET", "POST", "PUT"}, "/one",
			"POST", "/one",
			http.StatusOK, nil, "",
		},
		{
			[]string{"GET", "POST", "PUT"}, "/one",
			"PUT", "/one",
			http.StatusOK, nil, "",
		},
		{
			[]string{"GET", "POST", "PUT"}, "/one",
			"DELETE", "/one",
			http.StatusMethodNotAllowed, nil, "",
		},
		// all methods
		{
			[]string{}, "/one",
			"GET", "/one",
			http.StatusOK, nil, "",
		},
		{
			[]string{}, "/one",
			"DELETE", "/one",
			http.StatusOK, nil, "",
		},
		// method casing
		{
			[]string{"gEt"}, "/one",
			"GET", "/one",
			http.StatusOK, nil, "",
		},
		// head requests
		{
			[]string{"GET"}, "/one",
			"HEAD", "/one",
			http.StatusOK, nil, "",
		},
		{
			[]string{"HEAD"}, "/one",
			"HEAD", "/one",
			http.StatusOK, nil, "",
		},
		{
			[]string{"HEAD"}, "/one",
			"GET", "/one",
			http.StatusMethodNotAllowed, nil, "",
		},
		// allow header
		{
			[]string{"GET", "PUT"}, "/one",
			"DELETE", "/one",
			http.StatusMethodNotAllowed, nil, "GET, PUT, HEAD, OPTIONS",
		},
		// options
		{
			[]string{"GET", "PUT"}, "/one",
			"OPTIONS", "/one",
			http.StatusNoContent, nil, "GET, PUT, HEAD, OPTIONS",
		},
	}

	for _, test := range tests {
		m := New()

		var ctx context.Context

		hf := func(w http.ResponseWriter, r *http.Request) {
			ctx = r.Context()
		}

		m.HandleFunc(test.RoutePattern, hf, test.RouteMethods...)

		r, err := http.NewRequest(test.RequestMethod, test.RequestPath, nil)
		if err != nil {
			t.Errorf("NewRequest: %s", err)
		}

		rr := httptest.NewRecorder()
		m.ServeHTTP(rr, r)

		rs := rr.Result()

		if rs.StatusCode != test.ExpectedStatus {
			t.Errorf("%s %s: expected status %d but was %d", test.RequestMethod, test.RequestPath, test.ExpectedStatus, rr.Code)
			continue
		}

		if rs.StatusCode == http.StatusOK && len(test.ExpectedParams) > 0 {
			for expK, expV := range test.ExpectedParams {
				actualValStr := Param(ctx, expK)
				if actualValStr != expV {
					t.Errorf("Param: context value %s expected \"%s\" but was \"%s\"", expK, expV, actualValStr)
				}
			}
		}

		if test.ExpectedAllowHeader != "" {
			actualAllowHeader := rs.Header.Get("Allow")
			if actualAllowHeader != test.ExpectedAllowHeader {
				t.Errorf("%s %s: expected Allow header %q but was %q", test.RequestMethod, test.RequestPath, test.ExpectedAllowHeader, actualAllowHeader)
			}
		}

	}
}

func TestMiddleware(t *testing.T) {
	used := ""

	mw1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			used += "1"
			next.ServeHTTP(w, r)
		})
	}

	mw2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			used += "2"
			next.ServeHTTP(w, r)
		})
	}

	mw3 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			used += "3"
			next.ServeHTTP(w, r)
		})
	}

	mw4 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			used += "4"
			next.ServeHTTP(w, r)
		})
	}

	mw5 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			used += "5"
			next.ServeHTTP(w, r)
		})
	}

	mw6 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			used += "6"
			next.ServeHTTP(w, r)
		})
	}

	hf := func(w http.ResponseWriter, r *http.Request) {}

	m := New()
	m.Use(mw1)
	m.Use(mw2)

	m.HandleFunc("/", hf, "GET")

	m.Group(func(m *Mux) {
		m.Use(mw3, mw4)
		m.HandleFunc("/foo", hf, "GET")

		m.Group(func(m *Mux) {
			m.Use(mw5)
			m.HandleFunc("/nested/foo", hf, "GET")
		})
	})

	m.Group(func(m *Mux) {
		m.Use(mw6)
		m.HandleFunc("/bar", hf, "GET")
	})

	m.HandleFunc("/baz", hf, "GET")

	var tests = []struct {
		RequestMethod  string
		RequestPath    string
		ExpectedUsed   string
		ExpectedStatus int
	}{
		{
			RequestMethod:  "GET",
			RequestPath:    "/",
			ExpectedUsed:   "12",
			ExpectedStatus: http.StatusOK,
		},
		{
			RequestMethod:  "GET",
			RequestPath:    "/foo",
			ExpectedUsed:   "1234",
			ExpectedStatus: http.StatusOK,
		},
		{
			RequestMethod:  "GET",
			RequestPath:    "/nested/foo",
			ExpectedUsed:   "12345",
			ExpectedStatus: http.StatusOK,
		},
		{
			RequestMethod:  "GET",
			RequestPath:    "/bar",
			ExpectedUsed:   "126",
			ExpectedStatus: http.StatusOK,
		},
		{
			RequestMethod:  "GET",
			RequestPath:    "/baz",
			ExpectedUsed:   "12",
			ExpectedStatus: http.StatusOK,
		},
		// Check top-level middleware used on errors and OPTIONS
		{
			RequestMethod:  "GET",
			RequestPath:    "/notfound",
			ExpectedUsed:   "12",
			ExpectedStatus: http.StatusNotFound,
		},
		{
			RequestMethod:  "POST",
			RequestPath:    "/nested/foo",
			ExpectedUsed:   "12",
			ExpectedStatus: http.StatusMethodNotAllowed,
		},
		{
			RequestMethod:  "OPTIONS",
			RequestPath:    "/nested/foo",
			ExpectedUsed:   "12",
			ExpectedStatus: http.StatusNoContent,
		},
	}

	for _, test := range tests {
		used = ""

		r, err := http.NewRequest(test.RequestMethod, test.RequestPath, nil)
		if err != nil {
			t.Errorf("NewRequest: %s", err)
		}

		rr := httptest.NewRecorder()
		m.ServeHTTP(rr, r)

		rs := rr.Result()

		if rs.StatusCode != test.ExpectedStatus {
			t.Errorf("%s %s: expected status %d but was %d", test.RequestMethod, test.RequestPath, test.ExpectedStatus, rs.StatusCode)
		}

		if used != test.ExpectedUsed {
			t.Errorf("%s %s: middleware used: expected %q; got %q", test.RequestMethod, test.RequestPath, test.ExpectedUsed, used)
		}
	}
}

func TestCustomHandlers(t *testing.T) {
	hf := func(w http.ResponseWriter, r *http.Request) {}

	m := New()
	m.NotFound = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("custom not found handler"))
	})
	m.MethodNotAllowed = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("custom method not allowed handler"))
	})
	m.Options = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("custom options handler"))
	})

	m.HandleFunc("/", hf, "GET")

	var tests = []struct {
		RequestMethod string
		RequestPath   string

		ExpectedBody string
	}{
		{
			RequestMethod: "GET",
			RequestPath:   "/notfound",
			ExpectedBody:  "custom not found handler",
		},
		{
			RequestMethod: "POST",
			RequestPath:   "/",
			ExpectedBody:  "custom method not allowed handler",
		},
		{
			RequestMethod: "OPTIONS",
			RequestPath:   "/",
			ExpectedBody:  "custom options handler",
		},
	}

	for _, test := range tests {
		r, err := http.NewRequest(test.RequestMethod, test.RequestPath, nil)
		if err != nil {
			t.Errorf("NewRequest: %s", err)
		}

		rr := httptest.NewRecorder()
		m.ServeHTTP(rr, r)

		rs := rr.Result()

		defer rs.Body.Close()
		body, err := io.ReadAll(rs.Body)
		if err != nil {
			t.Fatal(err)
		}

		if string(body) != test.ExpectedBody {
			t.Errorf("%s %s: expected body %q; got %q", test.RequestMethod, test.RequestPath, test.ExpectedBody, string(body))
		}
	}
}

func TestParams(t *testing.T) {
	var tests = []struct {
		RouteMethods []string
		RoutePattern string

		RequestMethod string
		RequestPath   string

		ParamName  string
		HasParam   bool
		ParamValue string
	}{
		{
			[]string{"GET"}, "/foo/:id",
			"GET", "/foo/123",
			"id", true, "123",
		},
		{
			[]string{"GET"}, "/foo/:id",
			"GET", "/foo/123",
			"missing", false, "",
		},
	}

	for _, test := range tests {
		m := New()

		var ctx context.Context

		hf := func(w http.ResponseWriter, r *http.Request) {
			ctx = r.Context()
		}

		m.HandleFunc(test.RoutePattern, hf, test.RouteMethods...)

		r, err := http.NewRequest(test.RequestMethod, test.RequestPath, nil)
		if err != nil {
			t.Errorf("NewRequest: %s", err)
		}

		rr := httptest.NewRecorder()
		m.ServeHTTP(rr, r)

		actualValStr := Param(ctx, test.ParamName)
		if actualValStr != test.ParamValue {
			t.Errorf("expected \"%s\" but was \"%s\"", test.ParamValue, actualValStr)
		}
	}
}
