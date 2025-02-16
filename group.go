package huma

import "strings"

// modifyOperation modifies an operation to include the group's prefix and
// operation ID as well as runs any supplied operation modifier functions.
func modifyOperation(g *Group, prefix string, op *Operation) {
	if prefix != "" {
		friendlyPrefix := strings.ReplaceAll(strings.Trim(prefix, "/"), "/", "-")
		op.OperationID = friendlyPrefix + "-" + op.OperationID
		tags := append([]string{}, op.Tags...)
		op.Tags = append(tags, friendlyPrefix)
		op.Path = prefix + op.Path
	}
	for _, modifier := range g.opModifiers {
		modifier(op)
	}
}

// groupAdapter is an Adapter wrapper that registers multiple operation handlers
// with the underlying adapter based on the group's prefixes.
type groupAdapter struct {
	Adapter
	group *Group
}

func (a *groupAdapter) Handle(op *Operation, handler func(Context)) {
	// Make sure the router handles each full route (prefix + operation path).
	for _, prefix := range a.group.prefixes {
		modified := *op
		modifyOperation(a.group, prefix, &modified)
		a.Adapter.Handle(&modified, handler)
	}
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
	opModifiers  []func(o *Operation)
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
	if len(prefixes) == 0 {
		prefixes = append(prefixes, "")
	}
	group := &Group{API: api, prefixes: prefixes}
	group.adapter = &groupAdapter{Adapter: api.Adapter(), group: group}
	return group
}

func (g *Group) Adapter() Adapter {
	return g.adapter
}

func (g *Group) OpenAPI() *OpenAPI {
	// Provide a callback that `huma.Register` will call, and take the operation
	// that is added and modify it as needed for each prefix, registering it with
	// the real underlying OpenAPI document. This ensure the documentation
	// matches the server behavior!
	// The one caveat is that this *only* works for operations which invoke
	// the `OpenAPI.AddOperation(...)` call.
	openapi := *g.API.OpenAPI()
	openapi.OnAddOperation = append(openapi.OnAddOperation, func(oapi *OpenAPI, op *Operation) {
		for _, prefix := range g.prefixes {
			modified := *op
			modifyOperation(g, prefix, &modified)
			g.API.OpenAPI().AddOperation(&modified)
		}
	})
	return &openapi
}

// UseOperationModifier adds an operation modifier function to the group that
// will be run on all operations in the group. Use this to modify the operation
// before it is registered with the router or OpenAPI document.
func (g *Group) UseOperationModifier(handler func(o *Operation)) {
	g.opModifiers = append(g.opModifiers, handler)
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
