package humamux

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/gorilla/mux"
)

type Option func(*options)

func WithRouteCustomizer(f func(op *huma.Operation, r *mux.Route)) Option {
	return func(o *options) {
		o.routeCustomizer = f
	}
}

func parseOptions(optionList []Option) options {
	var optns options
	for _, opt := range optionList {
		opt(&optns)
	}
	return optns
}

type options struct {
	routeCustomizer func(op *huma.Operation, r *mux.Route)
}
