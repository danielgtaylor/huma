package huma

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// addGlobalFlag will make a new global flag on the root command.
func addGlobalFlag(root *cobra.Command, name, short, description string, defaultValue interface{}) {
	viper.SetDefault(name, defaultValue)

	flags := root.PersistentFlags()
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

// Run the router's main command.
func (r *Router) Run() {
	viper.SetEnvPrefix("SERVICE")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	root := &cobra.Command{
		Use:     filepath.Base(os.Args[0]),
		Version: r.api.Version,
		Run: func(cmd *cobra.Command, args []string) {
			if err := r.Listen(fmt.Sprintf("%s:%v", viper.Get("host"), viper.Get("port"))); err != nil {
				panic(err)
			}
		},
	}

	root.AddCommand(&cobra.Command{
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

	addGlobalFlag(root, "host", "", "Hostname", "0.0.0.0")
	addGlobalFlag(root, "port", "p", "Port", 8888)
	addGlobalFlag(root, "debug", "d", "Enable debug logs", false)

	if err := root.Execute(); err != nil {
		panic(err)
	}
}
