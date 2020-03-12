package huma

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/gin-gonic/gin"
)

// ErrDependencyInvalid is returned when registering a dependency fails.
var ErrDependencyInvalid = errors.New("dependency invalid")

// ErrDependencyNotFound is returned when the given type isn't registered
// as a dependency.
var ErrDependencyNotFound = errors.New("dependency not found")

// DependencyRegistry let's you register and resolve dependencies based on
// their type.
type DependencyRegistry struct {
	registry map[reflect.Type]interface{}
}

// NewDependencyRegistry creates a new blank dependency registry.
func NewDependencyRegistry() *DependencyRegistry {
	return &DependencyRegistry{
		registry: make(map[reflect.Type]interface{}),
	}
}

// Add a new dependency to the registry.
func (dr *DependencyRegistry) Add(item interface{}) error {
	if dr.registry == nil {
		dr.registry = make(map[reflect.Type]interface{})
	}

	val := reflect.ValueOf(item)
	outType := val.Type()

	valType := val.Type()
	if val.Kind() == reflect.Func {
		for i := 0; i < valType.NumIn(); i++ {
			argType := valType.In(i)
			if argType.String() == "*gin.Context" || argType.String() == "*huma.Operation" {
				// Known hard-coded dependencies. Skip them.
				continue
			}

			if argType.Kind() != reflect.Ptr {
				return fmt.Errorf("should be pointer *%s: %w", argType, ErrDependencyInvalid)
			}

			if _, ok := dr.registry[argType]; !ok {
				return fmt.Errorf("unknown dependency type %s, are dependencies defined in order? %w", argType, ErrDependencyNotFound)
			}
		}

		if val.Type().NumOut() != 2 || val.Type().Out(1).Name() != "error" {
			return fmt.Errorf("function should return (your-type, error): %w", ErrDependencyInvalid)
		}

		outType = val.Type().Out(0)

		if outType.Kind() != reflect.Ptr {
			return fmt.Errorf("should be pointer *%s: %w", outType, ErrDependencyInvalid)
		}

		if _, ok := dr.registry[outType]; ok {
			return fmt.Errorf("duplicate type %s: %w", outType.String(), ErrDependencyInvalid)
		}
	} else {
		if valType.Kind() != reflect.Ptr {
			return fmt.Errorf("should be pointer *%s: %w", valType, ErrDependencyInvalid)
		}

		if _, ok := dr.registry[valType]; ok {
			return fmt.Errorf("duplicate type %s: %w", valType.String(), ErrDependencyInvalid)
		}
	}

	// To prevent mistakes we limit dependencies to non-scalar types, since
	// scalars like strings/numbers are typically used for params like headers.
	switch outType.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64, reflect.String:
		return fmt.Errorf("dependeny cannot be scalar type %s: %w", outType.Kind(), ErrDependencyInvalid)
	}

	dr.registry[outType] = item

	return nil
}

// Get a resolved dependency from the registry.
func (dr *DependencyRegistry) Get(op *Operation, c *gin.Context, t reflect.Type) (interface{}, error) {
	if t.String() == "*gin.Context" {
		// Special case: current gin context.
		return c, nil
	}

	if t.String() == "*huma.Operation" {
		// Special case: current operation.
		return op, nil
	}

	if f, ok := dr.registry[t]; ok {
		// This argument matches a known registered dependency. If it's a
		// function, then call it, otherwise just return the value.
		vf := reflect.ValueOf(f)
		if vf.Kind() == reflect.Func {
			// Build the input argument list, which can consist of other dependencies.
			args := make([]reflect.Value, vf.Type().NumIn())

			for i := 0; i < vf.Type().NumIn(); i++ {
				v, err := dr.Get(op, c, vf.Type().In(i))
				if err != nil {
					return nil, err
				}
				args[i] = reflect.ValueOf(v)
			}

			out := vf.Call(args)

			if !out[1].IsNil() {
				return nil, out[1].Interface().(error)
			}
			return out[0].Interface(), nil
		}

		// Not a function, just return the value.
		return f, nil
	}

	return nil, fmt.Errorf("%s: %w", t, ErrDependencyNotFound)
}
