---
description: Add a CLI to your service for easy configuration and custom commands.
---

# Service CLI

## Service CLI { .hidden }

Huma ships with a built-in lightweight utility to wrap your service with a CLI, enabling you to run it with different arguments and easily write custom commands to do things like print out the OpenAPI or run on-demand database migrations.

The CLI options use a similar strategy to input & output structs, enabling you to use the same pattern for validation and documentation of command line arguments. It uses [Cobra](https://cobra.dev/) under the hood, enabling custom commands and including automatic environment variable binding and more.

```go title="main.go"
// First, define your input options.
type Options struct {
	Debug bool   `doc:"Enable debug logging"`
	Host  string `doc:"Hostname to listen on."`
	Port  int    `doc:"Port to listen on." short:"p" default:"8888"`
}

func main() {
	// Then, create the CLI.
	cli := humacli.New(func(hooks humacli.Hooks, opts *Options) {
		fmt.Printf("I was run with debug:%v host:%v port%v\n",
			opts.Debug, opts.Host, opts.Port)
	})

	// Run the thing!
	cli.Run()
}
```

You can then run the CLI and see the results:

```sh title="Terminal"
// Run with defaults
$ go run main.go
I was run with debug:false host: port:8888

// Run with options
$ go run main.go --debug=true --host=localhost --port=8000
I was run with debug:true host:localhost port:8000
```

To do useful work, you will want to register a handler for the default start command and optionally a way to gracefully shutdown the server:

```go title="main.go"
cli := humacli.New(func(hooks humacli.Hooks, opts *Options) {
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

## Passing Options

Options can be passed explicitly as command-line arguments to the service or they can be provided by environment variables prefixed with `SERVICE_`. For example, to run the service on port 8000:

```bash
# Example passing command-line args
$ go run main.go --port=8000

# Short arguments are also supported
$ go run main.go -p 8000

# Example passing by environment variables
$ SERVICE_PORT=8000 go run main.go
```

!!! warning "Precedence"

    If both environment variable and command-line arguments are present, then command-line arguments take priority.

## Custom Options

Custom options are defined by adding to your options struct. The following types are supported:

| Type            | Example Inputs                    |
| --------------- | --------------------------------- |
| `bool`          | `true`, `false`                   |
| `int` / `int64` | `1234`, `5`, `-1`                 |
| `string`        | `prod`, `http://api.example.tld/` |
| `time.Duration` | `500ms`, `3s`, `1h30m`            |

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
		b, err := api.OpenAPI().YAML()
		if err != nil {
			panic(err)
		}
		fmt.Println(string(b))
	},
})
```

!!! info "Note"

    You can use `api.OpenAPI().DowngradeYAML()` to output OpenAPI 3.0 instead of 3.1 for tools that don't support 3.1 yet.

Now you can run your service and use the new command: `go run . openapi`. Notice that it never starts the server; it just runs your command handler code. Some ideas for custom commands:

-   Print the OpenAPI spec
-   Print JSON Schemas
-   Run database migrations
-   Run customer scenario tests
-   Bundle common actions into a single utility command, like adding a new user

### Custom Commands with Options

If you want to access your custom options struct with custom commands, use the [`huma.WithOptions(func(cmd *cobra.Command, args []string, options *YourOptions)) func(cmd *cobra.Command, args []string)`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#WithOptions) utility function. It ensures the options are parsed and available before running your command.

!!! info "More Customization"

    You can also overwrite `cli.Root().Run` to completely customize how you run the server. Or just ditch the `cli` package altogether!

## App Name & Version

You can set the app name and version to be used in the help output and version command. By default, the app name is the name of the binary and the version is unset. You can set them using the root [`cobra.Command`](https://pkg.go.dev/github.com/spf13/cobra#Command)'s `Use` and `Version` fields:

```go title="main.go"
// cli := humacli.New(...)

cmd := cli.Root()
cmd.Use = "appname"
cmd.Version = "1.0.1"

cli.Run()
```

Then you will see something like this:

```sh title="Terminal"
$ go run ./demo --help
Usage:
  appname [flags]

Flags:
  -h, --help            help for appname
  -p, --port int         (default 8888)
  -v, --version         version for appname

$ go run ./demo --version
appname version 1.0.1
```

## Dive Deeper

-   Tutorial
    -   [Service Configuration Tutorial](../tutorial/service-configuration.md) includes a working CLI example
-   How-To
    -   [Graceful Shutdown](../how-to/graceful-shutdown.md) on service stop
-   Reference
    -   [`humacli.CLI`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2/humacli#CLI) the CLI instance
    -   [`humacli.New`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2/humacli#New) creates a new CLI instance
    -   [`humacli.Hooks`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2/humacli#Hooks) for startup / shutdown
    -   [`humacli.WithOptions`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2/humacli#WithOptions) wraps a command with options parsing
    -   [`huma.API`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#API) the API instance
-   External Links
    -   [Cobra](https://cobra.dev/) CLI library
