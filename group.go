package huma

import (
	"strings"
)

// PrefixModifier provides a fan-out to register one or more operations with
// the given prefix for every one operation added to a group.
func PrefixModifier(prefixes []string) func(o *Operation, next func(*Operation)) {
	return func(o *Operation, next func(*Operation)) {
		for _, prefix := range prefixes {
			modified := *o
			if len(prefixes) > 1 && prefix != "" {
				// If there are multiple prefixes, update the ID and tags so you can
				// differentiate between them in clients & the UI.
				friendlyPrefix := strings.ReplaceAll(strings.Trim(prefix, "/"), "/", "-")
				modified.OperationID = friendlyPrefix + "-" + modified.OperationID
				tags := append([]string{}, modified.Tags...)
				modified.Tags = append(tags, friendlyPrefix)
			}
			modified.Path = prefix + modified.Path
			next(&modified)
		}
	}
}

// groupAdapter is an Adapter wrapper that registers multiple operation handlers
// with the underlying adapter based on the group's prefixes.
type groupAdapter struct {
	Adapter
	group *Group
}

func (a *groupAdapter) Handle(op *Operation, handler func(Context)) {
	a.group.ModifyOperation(op, func(op *Operation) {
		a.Adapter.Handle(op, handler)
	})
}

// Group is a collection of routes that share a common prefix and set of
// operation modifiers, middlewares, and transformers.
//
// This is useful for grouping related routes together and applying common
// settings to them. For example, you might create a group for all routes that
// require authentication.
type Group struct {
	API
	prefixes     []string
	adapter      Adapter
	modifiers    []func(o *Operation, next func(*Operation))
	middlewares  Middlewares
	transformers []Transformer
}

// NewGroup creates a new group of routes with the given prefixes, if any. A
// group enables a collection of operations to have the same prefix and share
// operation modifiers, middlewares, and transformers.
//
//	grp := huma.NewGroup(api, "/v1")
//	grp.UseMiddleware(authMiddleware)
//
//	huma.Get(grp, "/users", func(ctx huma.Context, input *MyInput) (*MyOutput, error) {
//		// Your code here...
//	})
func NewGroup(api API, prefixes ...string) *Group {
	group := &Group{API: api, prefixes: prefixes}
	group.adapter = &groupAdapter{Adapter: api.Adapter(), group: group}
	if len(prefixes) > 0 {
		group.UseModifier(PrefixModifier(prefixes))
	}
	return group
}

func (g *Group) Adapter() Adapter {
	return g.adapter
}

// OpenAPI returns the group's OpenAPI, which acts as a passthrough to the
// underlying API's OpenAPI. You should not modify this directly as changes
// may not be propagated to the underlying API. Instead, only modify the
// original API's OpenAPI directly.
func (g *Group) OpenAPI() *OpenAPI {
	// Provide a callback that `huma.Register` will call, and take the operation
	// that is added and modify it as needed, registering it with the real
	// underlying OpenAPI document. This ensure the documentation matches the
	// server behavior! The one caveat is that this *only* works for operations
	// which invoke the `OpenAPI.AddOperation(...)` call.
	openapi := *g.API.OpenAPI()
	openapi.Paths = nil
	openapi.OnAddOperation = []AddOpFunc{
		func(oapi *OpenAPI, op *Operation) {
			oapi.Paths = nil // discourage manual edits
			g.ModifyOperation(op, func(op *Operation) {
				g.API.OpenAPI().AddOperation(op)
			})
		},
	}
	return &openapi
}

// UseModifier adds an operation modifier function to the group that will be run
// on all operations in the group. Use this to modify the operation before it is
// registered with the router or OpenAPI document. This behaves similar to
// middleware in that you should invoke `next` to continue the chain. Skip it
// to prevent the operation from being registered, and call multiple times for
// a fan-out effect.
func (g *Group) UseModifier(modifier func(o *Operation, next func(*Operation))) {
	g.modifiers = append(g.modifiers, modifier)
}

// UseSimpleModifier adds an operation modifier function to the group that
// will be run on all operations in the group. Use this to modify the operation
// before it is registered with the router or OpenAPI document.
func (g *Group) UseSimpleModifier(modifier func(o *Operation)) {
	g.modifiers = append(g.modifiers, func(o *Operation, next func(*Operation)) {
		modifier(o)
		next(o)
	})
}

// ModifyOperation runs all operation modifiers in the group on the given
// operation, in the order they were added. This is useful for modifying an
// operation before it is registered with the router or OpenAPI document.
func (g *Group) ModifyOperation(op *Operation, next func(*Operation)) {
	chain := next
	for i := len(g.modifiers) - 1; i >= 0; i-- {
		// Use an inline func to provide a closure around the index & chain.
		func(i int, n func(*Operation)) {
			chain = func(op *Operation) { g.modifiers[i](op, n) }
		}(i, chain)
	}
	chain(op)
}

// UseMiddleware adds one or more middleware functions to the group that will be
// run on all operations in the group. Use this to add common functionality to
// all operations in the group, e.g. authentication/authorization.
func (g *Group) UseMiddleware(middlewares ...func(ctx Context, next func(Context))) {
	g.middlewares = append(g.middlewares, middlewares...)
}

func (g *Group) Middlewares() Middlewares {
	m := append(Middlewares{}, g.API.Middlewares()...)
	return append(m, g.middlewares...)
}

// UseTransformer adds one or more transformer functions to the group that will
// be run on all responses in the group.
func (g *Group) UseTransformer(transformers ...Transformer) {
	g.transformers = append(g.transformers, transformers...)
}

func (g *Group) Transform(ctx Context, status string, v any) (any, error) {
	v, err := g.API.Transform(ctx, status, v)
	if err != nil {
		return v, err
	}
	for _, transformer := range g.transformers {
		v, err = transformer(ctx, status, v)
		if err != nil {
			return v, err
		}
	}
	return v, nil
}
