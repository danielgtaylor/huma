package huma

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

var handlers = []struct {
	name    string
	handler http.Handler
}{
	{"RapiDoc", RapiDocHandler(New("Test API", "1.0.0"))},
	{"ReDoc", ReDocHandler(New("Test API", "1.0.0"))},
	{"SwaggerUI", SwaggerUIHandler(New("Test API", "1.0.0"))},
}

func TestDocHandlers(outer *testing.T) {
	for _, tt := range handlers {
		local := tt
		outer.Run(local.name, func(t *testing.T) {
			app := newTestRouter()
			app.DocsHandler(local.handler)

			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/docs", nil)
			app.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
		})
	}
}

func TestSplitDocs(t *testing.T) {
	title, desc := splitDocs("One two\nthree\nfour five")
	assert.Equal(t, "One two", title)
	assert.Equal(t, "three\nfour five", desc)
}
