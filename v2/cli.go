package huma

import (
	"fmt"
	"os"
	"os/signal"
	"reflect"
	"strconv"
	"strings"
	"syscall"

	"github.com/danielgtaylor/casing"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

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

type option struct {
	name string
	typ  reflect.Type
	path []int
}

type cli[Options any] struct {
	root     *cobra.Command
	optInfo  []option
	cfg      *viper.Viper
	onParsed func(CLI, *Options)
	start    func()
	stop     func()
}

func (c *cli[Options]) Run() {
	var o Options

	existing := c.root.PersistentPreRun
	c.root.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		// Load config from args/env/files
		v := reflect.ValueOf(&o).Elem()
		for _, opt := range c.optInfo {
			f := v
			for _, i := range opt.path {
				f = f.Field(i)
			}
			switch opt.typ.Kind() {
			case reflect.String:
				f.Set(reflect.ValueOf(c.cfg.GetString(opt.name)))
			case reflect.Int, reflect.Int64:
				f.Set(reflect.ValueOf(c.cfg.GetInt64(opt.name)).Convert(opt.typ))
			case reflect.Bool:
				f.Set(reflect.ValueOf(c.cfg.GetBool(opt.name)))
			}
		}

		// Run the parsed callback.
		c.onParsed(c, &o)

		if existing != nil {
			existing(cmd, args)
		}
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

func (c *cli[O]) setupOptions(flags *pflag.FlagSet, t reflect.Type, path []int) {
	var err error
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		currentPath := append([]int{}, path...)
		currentPath = append(currentPath, i)

		if field.Anonymous {
			// Embedded struct. This enables composition from e.g. company defaults.
			c.setupOptions(flags, deref(field.Type), currentPath)
			continue
		}

		name := field.Tag.Get("name")
		if name == "" {
			name = casing.Kebab(field.Name)
		}

		c.optInfo = append(c.optInfo, option{name, field.Type, currentPath})
		switch field.Type.Kind() {
		case reflect.String:
			c.cfg.SetDefault(name, field.Tag.Get("default"))
			flags.StringP(name, field.Tag.Get("short"), field.Tag.Get("default"), field.Tag.Get("doc"))
		case reflect.Int, reflect.Int64:
			var def int64
			if d := field.Tag.Get("default"); d != "" {
				def, err = strconv.ParseInt(d, 10, 64)
				if err != nil {
					panic(err)
				}
			}
			c.cfg.SetDefault(name, def)
			flags.Int64P(name, field.Tag.Get("short"), def, field.Tag.Get("doc"))
		case reflect.Bool:
			var def bool
			if d := field.Tag.Get("default"); d != "" {
				def, err = strconv.ParseBool(d)
				if err != nil {
					panic(err)
				}
			}
			c.cfg.SetDefault(name, def)
			flags.BoolP(name, field.Tag.Get("short"), def, field.Tag.Get("doc"))
		default:
			panic("Unsupported option type: " + field.Type.Kind().String())
		}
		c.cfg.BindPFlag(name, flags.Lookup(name))
	}
}

// NewCLI creates a new CLI. The `onParsed` callback is called after the command
// options have been parsed and the options struct has been populated. You
// should set up a `cli.OnStart` callback to start the server with your chosen
// router.
func NewCLI[O any](onParsed func(CLI, *O)) CLI {
	c := &cli[O]{
		root: &cobra.Command{
			Use: "myapp",
		},
		onParsed: onParsed,
		cfg:      viper.New(),
	}

	cfg := c.cfg
	cfg.SetEnvPrefix("SERVICE")
	cfg.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	cfg.AutomaticEnv()

	var o O
	c.setupOptions(c.root.PersistentFlags(), reflect.TypeOf(o), []int{})

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
				fmt.Println("Gracefully shutting down the server...")
				c.stop()
			}
		}

	}
	return c
}
