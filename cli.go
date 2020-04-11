package huma

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

	"github.com/gin-gonic/autotls"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap/zapcore"
)

// GlobalFlag adds a new global flag on the root command.
func GlobalFlag(name, short, description string, defaultValue interface{}) RouterOption {
	return &routerOption{func(r *Router) {
		viper.SetDefault(name, defaultValue)

		flags := r.root.PersistentFlags()
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
	}}
}

// Root returns the router's root command.
func (r *Router) Root() *cobra.Command {
	return r.root
}

// setupCLI sets up the CLI commands.
func (r *Router) setupCLI() {
	viper.SetEnvPrefix("SERVICE")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	r.root = &cobra.Command{
		Use:     filepath.Base(os.Args[0]),
		Version: r.api.Version,
		Run: func(cmd *cobra.Command, args []string) {
			// Call any pre-start functions.
			for _, f := range r.prestart {
				f()
			}

			if viper.GetBool("debug") {
				if logLevel != nil {
					logLevel.SetLevel(zapcore.DebugLevel)
				}
			}

			// Start the server.
			go func() {
				// Start either an HTTP or HTTPS server based on whether TLS cert/key
				// paths were given or Let's Encrypt is used.
				autoTLS := viper.GetString("autotls")
				if autoTLS != "" {
					domains := strings.Split(autoTLS, ",")
					if err := autotls.Run(r, domains...); err != nil && err != http.ErrServerClosed {
						panic(err)
					}
					return
				}

				cert := viper.GetString("cert")
				key := viper.GetString("key")
				if cert == "" && key == "" {
					if err := r.Listen(fmt.Sprintf("%s:%v", viper.Get("host"), viper.Get("port"))); err != nil && err != http.ErrServerClosed {
						panic(err)
					}
					return
				}

				if cert != "" && key != "" {
					if err := r.ListenTLS(fmt.Sprintf("%s:%v", viper.Get("host"), viper.Get("port")), cert, key); err != nil && err != http.ErrServerClosed {
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
			r.Shutdown(ctx)
		},
	}

	r.root.AddCommand(&cobra.Command{
		Use:   "openapi FILENAME.json",
		Short: "Get OpenAPI spec",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			// Get the OpenAPI route from the server.
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/openapi.json", nil)
			r.ServeHTTP(w, req)

			if w.Result().StatusCode != 200 {
				panic(w.Body.String())
			}

			// Dump the response to a file.
			ioutil.WriteFile(args[0], append(w.Body.Bytes(), byte('\n')), 0644)

			fmt.Printf("Successfully wrote OpenAPI JSON to %s\n", args[0])
		},
	})

	flags := []RouterOption{
		GlobalFlag("host", "", "Hostname", "0.0.0.0"),
		GlobalFlag("port", "p", "Port", 8888),
		GlobalFlag("cert", "", "SSL certificate file path", ""),
		GlobalFlag("key", "", "SSL key file path", ""),
		GlobalFlag("autotls", "", "Let's Encrypt automatic TLS domains (ignores port)", ""),
		GlobalFlag("debug", "d", "Enable debug logs", false),
		GlobalFlag("grace-period", "", "Graceful shutdown wait duration in seconds", 20),
	}

	for _, flag := range flags {
		flag.ApplyRouter(r)
	}
}
