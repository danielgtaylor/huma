package huma

import (
	"log"

	"github.com/danielgtaylor/huma/v2/humacli"
	"github.com/spf13/cobra"
)

// CLI is an optional command-line interface for a Huma service. It is provided
// as a convenience for quickly building a service with configuration from
// the environment and/or command-line options, all tied to a simple type-safe
// Go struct.
//
// Deprecated: use `humacli.CLI` instead.
type CLI = humacli.CLI

// Hooks is an interface for setting up callbacks for the CLI. It is used to
// start and stop the service.
//
// Deprecated: use `humacli.Hooks` instead.
type Hooks = humacli.Hooks

// WithOptions is a helper for custom commands that need to access the options.
//
//	cli.Root().AddCommand(&cobra.Command{
//		Use: "my-custom-command",
//		Run: huma.WithOptions(func(cmd *cobra.Command, args []string, opts *Options) {
//			fmt.Println("Hello " + opts.Name)
//		}),
//	})
//
// Deprecated: use `humacli.WithOptions` instead.
func WithOptions[Options any](f func(cmd *cobra.Command, args []string, options *Options)) func(*cobra.Command, []string) {
	log.Println("huma.WithOptions is deprecated, use humacli.WithOptions instead")
	return humacli.WithOptions(f)
}

// NewCLI creates a new CLI. The `onParsed` callback is called after the command
// options have been parsed and the options struct has been populated. You
// should set up a `hooks.OnStart` callback to start the server with your
// chosen router.
//
//	// First, define your input options.
//	type Options struct {
//		Debug bool   `doc:"Enable debug logging"`
//		Host  string `doc:"Hostname to listen on."`
//		Port  int    `doc:"Port to listen on." short:"p" default:"8888"`
//	}
//
//	// Then, create the CLI.
//	cli := huma.NewCLI(func(hooks huma.Hooks, opts *Options) {
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
//
// Deprecated: use `humacli.New` instead.
func NewCLI[O any](onParsed func(Hooks, *O)) CLI {
	log.Println("huma.NewCLI is deprecated, use humacli.New instead")
	return humacli.New(onParsed)
}
