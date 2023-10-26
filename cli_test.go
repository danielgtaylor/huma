package huma_test

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"syscall"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func ExampleCLI() {
	// First, define your input options.
	type Options struct {
		Debug bool   `doc:"Enable debug logging"`
		Host  string `doc:"Hostname to listen on."`
		Port  int    `doc:"Port to listen on." short:"p" default:"8888"`
	}

	// Then, create the CLI.
	cli := huma.NewCLI(func(hooks huma.Hooks, opts *Options) {
		fmt.Printf("Options are debug:%v host:%v port%v\n",
			opts.Debug, opts.Host, opts.Port)

		// Set up the router & API
		router := chi.NewRouter()
		api := humachi.New(router, huma.DefaultConfig("My API", "1.0.0"))

		huma.Register(api, huma.Operation{
			OperationID: "hello",
			Method:      http.MethodGet,
			Path:        "/hello",
		}, func(ctx context.Context, input *struct{}) (*struct{}, error) {
			// TODO: implement handler
			return nil, nil
		})

		srv := &http.Server{
			Addr:    fmt.Sprintf("%s:%d", opts.Host, opts.Port),
			Handler: router,
			// TODO: Set up timeouts!
		}

		hooks.OnStart(func() {
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("listen: %s\n", err)
			}
		})

		hooks.OnStop(func() {
			srv.Shutdown(context.Background())
		})
	})

	// Run the thing!
	cli.Run()
}

func TestCLIPlain(t *testing.T) {
	type Options struct {
		Debug bool
		Host  string
		Port  int

		// ignore private fields, should not crash.
		ingore bool
	}

	cli := huma.NewCLI(func(hooks huma.Hooks, options *Options) {
		assert.Equal(t, true, options.Debug)
		assert.Equal(t, "localhost", options.Host)
		assert.Equal(t, 8001, options.Port)
		assert.Equal(t, false, options.ingore)
		hooks.OnStart(func() {
			// Do nothing
		})
	})

	cli.Root().SetArgs([]string{"--debug", "--host", "localhost", "--port", "8001"})
	cli.Run()
}

func TestCLIAdvanced(t *testing.T) {
	type DebugOption struct {
		Debug bool `doc:"Enable debug mode." default:"false"`
	}

	type Options struct {
		// Example of option composition via embedded type.
		DebugOption
		Host string `doc:"Hostname to listen on."`
		Port int    `doc:"Port to listen on." short:"p" default:"8000"`
	}

	cli := huma.NewCLI(func(hooks huma.Hooks, options *Options) {
		assert.Equal(t, true, options.Debug)
		assert.Equal(t, "localhost", options.Host)
		assert.Equal(t, 8001, options.Port)
		hooks.OnStart(func() {
			// Do nothing
		})
	})

	// A custom pre-run isn't overwritten and should still work!
	customPreRun := false
	cli.Root().PersistentPreRun = func(cmd *cobra.Command, args []string) {
		customPreRun = true
	}

	cli.Root().SetArgs([]string{"--debug", "--host", "localhost", "--port", "8001"})
	cli.Run()
	assert.True(t, customPreRun)
}

func TestCLIHelp(t *testing.T) {
	type Options struct {
		Debug bool
		Host  string
		Port  int
	}

	cli := huma.NewCLI(func(hooks huma.Hooks, options *Options) {
		// Do nothing
	})

	cli.Root().SetArgs([]string{"--help"})
	buf := bytes.NewBuffer(nil)
	cli.Root().SetOut(buf)
	cli.Root().SetErr(buf)
	cli.Run()

	assert.Equal(t, "Usage:\n  myapp [flags]\n\nFlags:\n      --debug         \n  -h, --help          help for myapp\n      --host string   \n      --port int\n", buf.String())
}

func TestCLICommandWithOptions(t *testing.T) {
	type Options struct {
		Debug bool
	}

	cli := huma.NewCLI(func(hooks huma.Hooks, options *Options) {
		// Do nothing
	})

	wasSet := false
	cli.Root().AddCommand(&cobra.Command{
		Use: "custom",
		Run: huma.WithOptions(func(cmd *cobra.Command, args []string, options *Options) {
			if options.Debug {
				wasSet = true
			}
		}),
	})

	cli.Root().SetArgs([]string{"custom", "--debug"})
	cli.Run()

	assert.True(t, wasSet)
}

func TestCLIShutdown(t *testing.T) {
	type Options struct{}

	started := false
	stopping := make(chan bool, 1)
	cli := huma.NewCLI(func(hooks huma.Hooks, options *Options) {
		hooks.OnStart(func() {
			started = true
			<-stopping
		})
		hooks.OnStop(func() {
			stopping <- true
		})
	})

	go func() {
		time.Sleep(10 * time.Millisecond)
		syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	}()

	cli.Root().SetArgs([]string{})
	cli.Run()
	assert.True(t, started)
}

func TestCLIBadType(t *testing.T) {
	type Options struct {
		Debug []struct{}
	}

	assert.Panics(t, func() {
		huma.NewCLI(func(hooks huma.Hooks, options *Options) {})
	})
}

func TestCLIBadDefaults(t *testing.T) {
	type OptionsBool struct {
		Debug bool `default:"notabool"`
	}

	type OptionsInt struct {
		Debug int `default:"notanint"`
	}

	assert.Panics(t, func() {
		huma.NewCLI(func(hooks huma.Hooks, options *OptionsBool) {})
	})

	assert.Panics(t, func() {
		huma.NewCLI(func(hooks huma.Hooks, options *OptionsInt) {})
	})
}
