---
description: Add a CLI to your service for easy configuration and custom commands.
---

# Service CLI

## Service CLI { .hidden }

Huma ships with a built-in lightweight utility to wrap your service with a CLI, enabling you to run it with different arguments and easily write custom commands to do things like print out the OpenAPI or run on-demand database migrations.

The CLI options use a similar strategy to input & output structs, enabling you to use the same pattern for validation and documentation of command line arguments. It uses [Cobra](https://cobra.dev/) & [Viper](https://github.com/spf13/viper) under the hood, enabling automatic environment variable binding and more.

```go title="main.go"
// First, define your input options.
type Options struct {
	Debug bool   `doc:"Enable debug logging"`
	Host  string `doc:"Hostname to listen on."`
	Port  int    `doc:"Port to listen on." short:"p" default:"8888"`
}

func main() {
	// Then, create the CLI.
	cli := huma.NewCLI(func(hooks huma.Hooks, opts *Options) {
		fmt.Printf("I was run with debug:%v host:%v port%v\n",
			opts.Debug, opts.Host, opts.Port)
	})

	// Run the thing!
	cli.Run()
}
```

You can then run the CLI with and see the results:

```sh title="Terminal"
$ go run main.go
I was run with debug:false host: port:8888
```

To do useful work, you will want to register a handler for the default start command and optionally a way to gracefully shutdown the server:

```go title="main.go"
cli := huma.NewCLI(func(hooks huma.Hooks, opts *Options) {
	// Set up the router and API
	// ...

	// Create the HTTP server.
	server := http.Server{
		Addr:    fmt.Sprintf(":%d", options.Port),
		Handler: router,
	}

	hooks.OnStart(func() {
		// Start your server here
		server.ListenAndServe()
	})

	hooks.OnStop(func() {
		// Gracefully shutdown your server here
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	})
})
```

!!! info "Naming"

    Option fields are automatically converted to `--kebab-casing` for use on the command line. If you want to use a different name, use the `name` struct tag to override the default behavior!

## Custom Options

Custom options are defined by adding to your options struct. The following types are supported:

| Type            | Example Inputs                    |
| --------------- | --------------------------------- |
| `bool`          | `true`, `false`                   |
| `int` / `int64` | `1234`, `5`, `-1`                 |
| `string`        | `prod`, `http://api.example.tld/` |

The following struct tags are available:

| Tag       | Description                             | Example                 |
| --------- | --------------------------------------- | ----------------------- |
| `default` | Default value (parsed automatically)    | `default:"123"`         |
| `doc`     | Describe the option                     | `doc:"Who to greet"`    |
| `name`    | Override the name of the option         | `name:"my-option-name"` |
| `short`   | Single letter short name for the option | `short:"p"` for `-p`    |

Here is an example of how to use them:

```go title="main.go"
type Options struct {
	Debug bool   `doc:"Enable debug logging"`
	Host  string `doc:"Hostname to listen on."`
	Port  int    `doc:"Port to listen on." short:"p" default:"8888"`
}
```

## Custom Commands

You can access the root [`cobra.Command`](https://pkg.go.dev/github.com/spf13/cobra#Command) via `cli.Root()` and add new custom commands via `cli.Root().AddCommand(...)`. For example, to have a command print out the generated OpenAPI:

```go title="main.go"
var api huma.API

// ... set up the CLI, create the API wrapping the router ...

cli.Root().AddCommand(&cobra.Command{
	Use:   "openapi",
	Short: "Print the OpenAPI spec",
	Run: func(cmd *cobra.Command, args []string) {
		b, _ := yaml.Marshal(api.OpenAPI())
		fmt.Println(string(b))
	},
})
```

Now you can run your service and use the new command: `go run . openapi`.

If you want to access your custom options struct with custom commands, use the [`huma.WithOptions(func(cmd *cobra.Command, args []string, options *YourOptions)) func(cmd *cobra.Command, args []string)`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#WithOptions) utitity function. It ensures the options are parsed and available before running your command.

!!! info "More Customization"

    You can also overwite `cli.Root().Run` to completely customize how you run the server. Or just ditch the `cli` package altogether!

## Dive Deeper

-   Tutorial
    -   [Service Configuration Tutorial](../tutorial/service-configuration.md) includes a working CLI example
-   How-To
    -   [Graceful Shutdown](../how-to/graceful-shutdown.md) on service stop
-   Reference
    -   [`huma.CLI`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#CLI) the CLI instance
    -   [`huma.NewCLI`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#NewCLI) creates a new CLI instance
    -   [`huma.Hooks`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Hooks) for startup / shutdown
    -   [`huma.WithOptions`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#WithOptions) wraps a command with options parsing
    -   [`huma.API`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#API) the API instance
-   External Links
    -   [Cobra](https://cobra.dev/) CLI library
    -   [Viper](https://github.com/spf13/viper) Configuration library
