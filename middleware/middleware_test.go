package middleware

import (
	"net/http"
	"testing"
)

type fakeApp struct{}

func (a *fakeApp) Middleware(middlewares ...func(next http.Handler) http.Handler) {}

func (a *fakeApp) Flag(name string, short string, description string, defaultValue interface{}) {}

func (a *fakeApp) PreStart(f func()) {
	f()
}

func TestDefaults(t *testing.T) {
	app := &fakeApp{}
	Defaults(app)
}
