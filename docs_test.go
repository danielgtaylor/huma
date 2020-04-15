package huma

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

var handlers = []struct {
	name    string
	handler Handler
}{
	{"RapiDoc", RapiDocHandler("Test API")},
	{"ReDoc", ReDocHandler("Test API")},
	{"SwaggerUI", SwaggerUIHandler("Test API")},
}

func TestDocHandlers(outer *testing.T) {
	for _, tt := range handlers {
		local := tt
		outer.Run(fmt.Sprintf("%v", tt.name), func(t *testing.T) {
			r := NewTestRouter(t, DocsHandler(local.handler))

			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/docs", nil)
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
		})
	}
}
