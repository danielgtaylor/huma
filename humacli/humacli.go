package humacli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/danielgtaylor/huma/v2/casing"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func deref(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

// CLI is an optional command-line interface for a Huma service. It is provided
// as a convenience for quickly building a service with configuration from
// the environment and/or command-line options, all tied to a simple type-safe
// Go struct.
type CLI interface {
	// Run the CLI. This will parse the command-line arguments and environment
	// variables and then run the appropriate command. If no command is given,
	// the default command will call the `OnStart` function to start a server.
	Run()

	// Root returns the root Cobra command. This can be used to add additional
	// commands or flags. Customize it however you like.
	Root() *cobra.Command
}

// Hooks is an interface for setting up callbacks for the CLI. It is used to
// start and stop the service.
type Hooks interface {
	// OnStart sets a function to call when the service should be started. This
	// is called by the default command if no command is given. The callback
	// should take whatever steps are necessary to start the server, such as
	// `httpServer.ListenAndServer(...)`.
	OnStart(func())

	// OnStop sets a function to call when the service should be stopped. This
	// is called by the default command if no command is given. The callback
	// should take whatever steps are necessary to stop the server, such as
	// `httpServer.Shutdown(...)`.
	OnStop(func())
}

type contextKey string

var optionsKey contextKey = "huma/cli/options"

var durationType = reflect.TypeOf((*time.Duration)(nil)).Elem()

// WithOptions is a helper for custom commands that need to access the options.
//
//	cli.Root().AddCommand(&cobra.Command{
//		Use: "my-custom-command",
//		Run: huma.WithOptions(func(cmd *cobra.Command, args []string, opts *Options) {
//			fmt.Println("Hello " + opts.Name)
//		}),
//	})
func WithOptions[Options any](f func(cmd *cobra.Command, args []string, options *Options)) func(*cobra.Command, []string) {
	return func(cmd *cobra.Command, s []string) {
		var options = cmd.Context().Value(optionsKey).(*Options)
		f(cmd, s, options)
	}
}

type option struct {
	name string
	typ  reflect.Type
	path []int
}

type cli[Options any] struct {
	root     *cobra.Command
	optInfo  []option
	onParsed func(Hooks, *Options)
	start    func()
	stop     func()
}

// getStringValue returns a string value respecting precedence: CLI arg > ENV var > default
func getStringValue(flags *pflag.FlagSet, flagName, envValue string, hasEnv bool) string {
	if flags.Changed(flagName) {
		// CLI arg provided
		value, _ := flags.GetString(flagName)
		return value
	} else if hasEnv {
		// Environment variable provided
		return envValue
	}
	// Default value from flag
	value, _ := flags.GetString(flagName)
	return value
}

// getIntValue returns an int-like value respecting precedence: CLI arg > ENV var > default
// It also handles duration types.
func getIntValue(flags *pflag.FlagSet, flagName, envValue string, hasEnv bool, isDuration bool) any {
	if flags.Changed(flagName) {
		// CLI arg provided
		if isDuration {
			value, _ := flags.GetDuration(flagName)
			return value
		}
		value, _ := flags.GetInt64(flagName)
		return value
	} else if hasEnv {
		// Environment variable provided
		if isDuration {
			value, err := time.ParseDuration(envValue)
			if err == nil {
				return value
			}
			// If parsing fails, fall back to default
		} else {
			value, err := strconv.ParseInt(envValue, 10, 64)
			if err == nil {
				return value
			}
			// If parsing fails, fall back to default
		}
	}

	// Default value from flag
	if isDuration {
		value, _ := flags.GetDuration(flagName)
		return value
	}
	value, _ := flags.GetInt64(flagName)
	return value
}

// getBoolValue returns a boolean value respecting precedence: CLI arg > ENV var > default
func getBoolValue(flags *pflag.FlagSet, flagName, envValue string, hasEnv bool) bool {
	if flags.Changed(flagName) {
		// CLI arg provided
		value, _ := flags.GetBool(flagName)
		return value
	} else if hasEnv {
		// Environment variable provided
		value, err := strconv.ParseBool(envValue)
		if err == nil {
			return value
		}
		// If parsing fails, fall back to default
	}
	// Default value from flag
	value, _ := flags.GetBool(flagName)
	return value
}

// getEnvName converts a flag name to the corresponding environment variable name
func getEnvName(flagName string) string {
	name := strings.ReplaceAll(flagName, "-", "_")
	name = strings.ReplaceAll(name, ".", "_")
	return "SERVICE_" + strings.ToUpper(name)
}

// getValueFromType uses the appropriate getter based on the field type
// and returns the value respecting precedence rules.
func getValueFromType(flags *pflag.FlagSet, flagName string, fieldType reflect.Type) (any, bool) {
	// Check environment variables
	envName := getEnvName(flagName)
	envValue, hasEnv := os.LookupEnv(envName)

	// Determine the appropriate getter based on type
	switch deref(fieldType).Kind() {
	case reflect.String:
		return getStringValue(flags, flagName, envValue, hasEnv), true
	case reflect.Int, reflect.Int64:
		isDuration := fieldType == durationType
		rawValue := getIntValue(flags, flagName, envValue, hasEnv, isDuration)
		return reflect.ValueOf(rawValue).Convert(deref(fieldType)).Interface(), true
	case reflect.Bool:
		return getBoolValue(flags, flagName, envValue, hasEnv), true
	default:
		return nil, false
	}
}

// getValueFromFlagOrEnv retrieves a value from either environment variables or flags
// based on the field type. It handles converting strings to the appropriate types.
// CLI args take precedence over environment variables.
func getValueFromFlagOrEnv(flags *pflag.FlagSet, opt option, fieldType reflect.Type) reflect.Value {
	// Get the value based on type
	value, ok := getValueFromType(flags, opt.name, fieldType)
	if !ok {
		// This shouldn't happen if setupOptions validates types properly
		panic(fmt.Sprintf("unsupported type for option %s: %s", opt.name, fieldType.String()))
	}

	// Create and return proper value
	fv := reflect.ValueOf(value)
	if fieldType.Kind() == reflect.Ptr {
		ptr := reflect.New(fv.Type())
		ptr.Elem().Set(fv)
		fv = ptr
	}

	return fv
}

func (c *cli[Options]) Run() {
	var o Options

	existing := c.root.PersistentPreRun
	c.root.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		// Load config from args/env/files
		v := reflect.ValueOf(&o).Elem()
		flags := c.root.PersistentFlags()
		for _, opt := range c.optInfo {
			f := v
			for _, i := range opt.path {
				// Check if f is a pointer and dereference it before calling Field
				if f.Kind() == reflect.Ptr {
					// Initialize nil pointers
					if f.IsNil() {
						f.Set(reflect.New(f.Type().Elem()))
					}
					f = f.Elem()
				}
				f = f.Field(i)
			}

			// Get field value from flag or environment variable
			fv := getValueFromFlagOrEnv(flags, opt, opt.typ)
			f.Set(fv)
		}

		// Run the parsed callback.
		c.onParsed(c, &o)

		if existing != nil {
			existing(cmd, args)
		}

		// Set options in context, so custom commands can access it.
		cmd.SetContext(context.WithValue(cmd.Context(), optionsKey, &o))
	}

	// Run the command!
	c.root.Execute()
}

func (c *cli[O]) Root() *cobra.Command {
	return c.root
}

func (c *cli[O]) OnStart(fn func()) {
	c.start = fn
}

func (c *cli[O]) OnStop(fn func()) {
	c.stop = fn
}

// registerOption registers an option with the CLI, handling common tasks like
// parsing default values, setting up flags, and storing option metadata.
func (c *cli[O]) registerOption(flags *pflag.FlagSet, field reflect.StructField, currentPath []int, name, defaultValue string) error {
	fieldType := deref(field.Type)

	// Store option metadata regardless of type
	c.optInfo = append(c.optInfo, option{name, field.Type, currentPath})

	// Type-specific flag setup and default parsing
	switch fieldType.Kind() {
	case reflect.String:
		flags.StringP(name, field.Tag.Get("short"), defaultValue, field.Tag.Get("doc"))
	case reflect.Int, reflect.Int64:
		var def int64
		if defaultValue != "" {
			if fieldType == durationType {
				t, err := time.ParseDuration(defaultValue)
				if err != nil {
					return fmt.Errorf("failed to parse duration for field %s: %w", field.Name, err)
				}
				def = int64(t)
			} else {
				var err error
				def, err = strconv.ParseInt(defaultValue, 10, 64)
				if err != nil {
					return fmt.Errorf("failed to parse int for field %s: %w", field.Name, err)
				}
			}
		}
		if fieldType == durationType {
			flags.DurationP(name, field.Tag.Get("short"), time.Duration(def), field.Tag.Get("doc"))
		} else {
			flags.Int64P(name, field.Tag.Get("short"), def, field.Tag.Get("doc"))
		}
	case reflect.Bool:
		var def bool
		if defaultValue != "" {
			var err error
			def, err = strconv.ParseBool(defaultValue)
			if err != nil {
				return fmt.Errorf("failed to parse bool for field %q: %w", field.Name, err)
			}
		}
		flags.BoolP(name, field.Tag.Get("short"), def, field.Tag.Get("doc"))
	default:
		return fmt.Errorf("unsupported option type for field %q: %q", field.Name, field.Type.Kind().String())
	}

	return nil
}

func (c *cli[O]) setupOptions(t reflect.Type, path []int, prefix string) error {
	flags := c.root.PersistentFlags()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		if !field.IsExported() {
			// This isn't a public field, so we cannot use reflect.Value.Set with
			// it. This is usually a struct field with a lowercase name.
			fmt.Fprintln(os.Stderr, "warning: ignoring unexported options field", field.Name)
			continue
		}

		currentPath := append([]int{}, path...)
		currentPath = append(currentPath, i)

		fieldType := deref(field.Type)
		if field.Anonymous {
			// Embedded struct. This enables composition from e.g. company defaults.
			c.setupOptions(fieldType, currentPath, prefix)
			continue
		}

		name := field.Tag.Get("name")
		if name == "" {
			name = casing.Kebab(field.Name)
		}

		// Apply prefix for nested fields
		if prefix != "" {
			name = prefix + "." + name
		}

		// Convert dotted names to snake case with underscores for env vars
		envName := "SERVICE_" + casing.Snake(strings.ReplaceAll(name, ".", "_"), strings.ToUpper)
		defaultValue := field.Tag.Get("default")
		if v, ok := os.LookupEnv(envName); ok {
			// Env vars will override the default value, which is used to document
			// what the value is if no options are passed.
			defaultValue = v
		}

		switch fieldType.Kind() {
		case reflect.String, reflect.Int, reflect.Int64, reflect.Bool:
			if err := c.registerOption(flags, field, currentPath, name, defaultValue); err != nil {
				return fmt.Errorf("failed to register option %q: %w", field.Name, err)
			}
		case reflect.Struct:
			// For nested structs, recurse and pass the current name as a prefix
			if err := c.setupOptions(fieldType, currentPath, name); err != nil {
				return fmt.Errorf("failed to setup options for field %q: %w", field.Name, err)
			}
		case reflect.Ptr:
			// If it's a pointer to a struct, handle it like a struct after dereferencing
			if fieldType.Kind() == reflect.Struct {
				if err := c.setupOptions(fieldType, currentPath, name); err != nil {
					return fmt.Errorf("failed to setup options for field %q: %w", field.Name, err)
				}
			} else {
				return fmt.Errorf("unsupported option type for field %q: pointer to %q", field.Name, fieldType.Kind().String())
			}
		default:
			return fmt.Errorf("unsupported option type for field %q: %q", field.Name, field.Type.Kind().String())
		}
	}

	return nil
}

// New creates a new CLI. The `onParsed` callback is called after the command
// options have been parsed and the options struct has been populated. You
// should set up a `hooks.OnStart` callback to start the server with your
// chosen router.
//
//	// First, define your input options.
//	type Options struct {
//		Debug bool   `doc:"Enable debug logging"`
//		Host  string `doc:"ServerURL to listen on."`
//		Port  int    `doc:"Port to listen on." short:"p" default:"8888"`
//	}
//
//	// Then, create the CLI.
//	cli := humacli.CLI(func(hooks humacli.Hooks, opts *Options) {
//		fmt.Printf("Options are debug:%v host:%v port%v\n",
//			opts.Debug, opts.Host, opts.Port)
//
//		// Set up the router & API
//		router := chi.NewRouter()
//		api := humachi.New(router, huma.DefaultConfig("My API", "1.0.0"))
//		srv := &http.Server{
//			Addr: fmt.Sprintf("%s:%d", opts.Host, opts.Port),
//			Handler: router,
//			// TODO: Set up timeouts!
//		}
//
//		hooks.OnStart(func() {
//			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
//				log.Fatalf("listen: %s\n", err)
//			}
//		})
//
//		hooks.OnStop(func() {
//			srv.Shutdown(context.Background())
//		})
//	})
//
//	// Run the thing!
//	cli.Run()
func New[O any](onParsed func(Hooks, *O)) CLI {
	c := &cli[O]{
		root: &cobra.Command{
			Use: filepath.Base(os.Args[0]),
		},
		onParsed: onParsed,
	}

	var o O
	if err := c.setupOptions(reflect.TypeOf(o), []int{}, ""); err != nil {
		panic(err)
	}

	c.root.Run = func(cmd *cobra.Command, args []string) {
		done := make(chan struct{}, 1)
		if c.start != nil {
			go func() {
				c.start()
				done <- struct{}{}
			}()
		}

		// Handle graceful shutdown.
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

		select {
		case <-done:
			// Server is done, just exit.
		case <-quit:
			if c.stop != nil {
				fmt.Fprintln(os.Stderr, "Gracefully shutting down the server...")
				c.stop()
			}
		}

	}
	return c
}
