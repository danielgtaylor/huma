package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/humatest"
	"github.com/stretchr/testify/assert"
)

func TestHandler(t *testing.T) {
	// Set up the test router and register the routes.
	r := humatest.NewRouter(t)
	routes(r)

	// Make a request against the service.
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	// Assert the response is as expected.
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "Hello, test!", w.Body.String())
}
