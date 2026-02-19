package huma

import (
	"maps"
	"strings"
)

// OperationDocumenter is an interface that can be implemented by an API or
// group to document operations in the OpenAPI document. This bypasses the
// normal `huma.Register` logic and provides complete customization of how
// operations are documented.
type OperationDocumenter interface {
	// DocumentOperation adds an operation to the OpenAPI document. This is
	// called by `huma.Register` when a new operation is registered.
	DocumentOperation(op *Operation)
}

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

var _ API = new(Group)
var _ configProvider[Config] = new(Group)

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

func (g *Group) Config() Config {
	return getConfig[Config](g.API)
}

// DocumentOperation adds an operation to the OpenAPI document. This is called
// by `huma.Register` when a new operation is registered. All modifiers will be
// run on the operation before it is added to the OpenAPI document, so for
// groups with multiple prefixes this will result in multiple operations in the
// OpenAPI document.
func (g *Group) DocumentOperation(op *Operation) {
	g.ModifyOperation(op, func(op *Operation) {
		if documenter, ok := g.API.(OperationDocumenter); ok {
			documenter.DocumentOperation(op)
		} else {
			if op.Hidden {
				return
			}
			g.OpenAPI().AddOperation(op)
		}
	})
}

// UseModifier adds an operation modifier function to the group that will be run
// on all operations in the group. Use this to modify the operation before it is
// registered with the router or OpenAPI document. This behaves similar to
// middleware in that you should invoke `next` to continue the chain. Skip it
// to prevent the operation from being registered and call multiple times for
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
	chain := func(op *Operation) {
		// If this came from unmodified convenience functions, we may need to
		// regenerate the operation ID and summary as they are based on things
		// like the path which may have changed.
		if op.Metadata != nil {
			// Copy so we don't modify the original map.
			meta := make(map[string]any, len(op.Metadata))
			maps.Copy(meta, op.Metadata)
			op.Metadata = meta

			// If the conveniences are set, we need to regenerate the operation ID and
			// summary based on the new path. We also update the metadata to reflect
			// the new generated operation ID and summary so groups of groups can
			// continue to modify them as needed.
			if op.Metadata["_convenience_id"] == op.OperationID {
				op.OperationID = GenerateOperationID(op.Method, op.Path, op.Metadata["_convenience_id_out"])
				op.Metadata["_convenience_id"] = op.OperationID
			}
			if op.Metadata["_convenience_summary"] == op.Summary {
				op.Summary = GenerateSummary(op.Method, op.Path, op.Metadata["_convenience_summary_out"])
				op.Metadata["_convenience_summary"] = op.Summary
			}
		}
		// Call the final handler.
		next(op)
	}
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
// all operations in the group, e.g., authentication/authorization.
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
