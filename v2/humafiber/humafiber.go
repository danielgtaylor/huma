package humafiber

import (
	"context"
	"io"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v2"
)

type API = huma.API[*fiber.Ctx, *fiber.Ctx]

type fiberAdapter struct {
	router fiber.Router
}

func (a *fiberAdapter) Handle(method, path string, handler func(w *fiber.Ctx, r *fiber.Ctx)) {
	// Convert {param} to :param
	path = strings.ReplaceAll(path, "{", ":")
	path = strings.ReplaceAll(path, "}", "")
	a.router.Add(method, path, func(c *fiber.Ctx) error {
		handler(c, c)
		return nil
	})
}

func (a *fiberAdapter) GetMatched(r *fiber.Ctx) string {
	return r.Route().Path
}

func (a *fiberAdapter) GetContext(r *fiber.Ctx) context.Context {
	return r.Context()
}

func (a *fiberAdapter) GetParam(r *fiber.Ctx, name string) string {
	return r.Params(name)
}

func (a *fiberAdapter) GetQuery(r *fiber.Ctx, name string) string {
	return r.Query(name)
}

func (a *fiberAdapter) GetHeader(r *fiber.Ctx, name string) string {
	return r.Get(name)
}

func (a *fiberAdapter) GetBody(r *fiber.Ctx) ([]byte, error) {
	return r.Body(), nil
}

func (a *fiberAdapter) WriteStatus(w *fiber.Ctx, code int) {
	w.Status(code)
}

func (a *fiberAdapter) AppendHeader(w *fiber.Ctx, name string, value string) {
	w.Append(name, value)
}

func (a *fiberAdapter) WriteHeader(w *fiber.Ctx, name string, value string) {
	w.Set(name, value)
}

func (a *fiberAdapter) BodyWriter(w *fiber.Ctx) io.Writer {
	return w
}

func New(r fiber.Router, config huma.Config) API {
	return huma.NewAPI[*fiber.Ctx, *fiber.Ctx](config, &fiberAdapter{router: r})
}
