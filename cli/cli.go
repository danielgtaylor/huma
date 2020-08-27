package cli

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/danielgtaylor/huma"
	"github.com/danielgtaylor/huma/middleware"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// CLI provides a command line interface to a Huma router.
type CLI struct {
	*huma.Router

	// Root entrypoint command
	root *cobra.Command

	// Functions to run before the server starts up.
	prestart []func()
}

// NewRouter creates a new router, new CLI, sets the default middlware, and
// returns the CLI/router as a convenience function.
func NewRouter(docs, version string) *CLI {
	// Create the router and CLI
	r := huma.New(docs, version)
	app := New(r)

	// Set up the default middleware
	middleware.Defaults(app)

	return app
}

// New creates a new CLI instance from an existing router.
func New(router *huma.Router) *CLI {
	viper.SetEnvPrefix("SERVICE")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	app := &CLI{
		Router: router,
	}

	app.root = &cobra.Command{
		Use:     filepath.Base(os.Args[0]),
		Version: app.GetVersion(),
		Run: func(cmd *cobra.Command, args []string) {
			// Call any pre-start functions.
			for _, f := range app.prestart {
				f()
			}

			// Start the server.
			go func() {
				// Start either an HTTP or HTTPS server based on whether TLS cert/key
				// paths were given or Let's Encrypt is used.
				cert := viper.GetString("cert")
				key := viper.GetString("key")
				if cert == "" && key == "" {
					if err := app.Listen(fmt.Sprintf("%s:%v", viper.Get("host"), viper.Get("port"))); err != nil && err != http.ErrServerClosed {
						panic(err)
					}
					return
				}

				if cert != "" && key != "" {
					if err := app.ListenTLS(fmt.Sprintf("%s:%v", viper.Get("host"), viper.Get("port")), cert, key); err != nil && err != http.ErrServerClosed {
						panic(err)
					}
					return
				}

				panic("must pass key and cert for TLS")
			}()

			// Handle graceful shutdown.
			quit := make(chan os.Signal)
			signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
			<-quit

			fmt.Println("Gracefully shutting down the server...")

			ctx, cancel := context.WithTimeout(context.Background(), viper.GetDuration("grace-period")*time.Second)
			defer cancel()
			app.Shutdown(ctx)
		},
	}

	app.root.AddCommand(&cobra.Command{
		Use:   "openapi FILENAME.json",
		Short: "Get OpenAPI spec",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			// Get the OpenAPI route from the server.
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/openapi.json", nil)
			app.ServeHTTP(w, req)

			if w.Result().StatusCode != 200 {
				panic(w.Body.String())
			}

			// Dump the response to a file.
			ioutil.WriteFile(args[0], append(w.Body.Bytes(), byte('\n')), 0644)

			fmt.Printf("Successfully wrote OpenAPI JSON to %s\n", args[0])
		},
	})

	app.Flag("host", "", "Hostname", "0.0.0.0")
	app.Flag("port", "p", "Port", 8888)
	app.Flag("cert", "", "SSL certificate file path", "")
	app.Flag("key", "", "SSL key file path", "")
	app.Flag("grace-period", "", "Graceful shutdown wait duration in seconds", 20)

	return app
}

// Root returns the CLI's root command. Use this to add flags and custom
// commands to the CLI.
func (c *CLI) Root() *cobra.Command {
	return c.root
}

// Flag adds a new global flag on the root command of this router.
func (c *CLI) Flag(name, short, description string, defaultValue interface{}) {
	viper.SetDefault(name, defaultValue)

	flags := c.root.PersistentFlags()
	switch v := defaultValue.(type) {
	case bool:
		flags.BoolP(name, short, viper.GetBool(name), description)
	case int, int16, int32, int64, uint16, uint32, uint64:
		flags.IntP(name, short, viper.GetInt(name), description)
	case float32, float64:
		flags.Float64P(name, short, viper.GetFloat64(name), description)
	default:
		flags.StringP(name, short, fmt.Sprintf("%v", v), description)
	}
	viper.BindPFlag(name, flags.Lookup(name))
}

// PreStart registers a function to run before the server starts but after
// command line arguments have been parsed.
func (c *CLI) PreStart(f func()) {
	c.prestart = append(c.prestart, f)
}

// Run runs the CLI.
func (c *CLI) Run() {
	if err := c.root.Execute(); err != nil {
		panic(err)
	}
}
