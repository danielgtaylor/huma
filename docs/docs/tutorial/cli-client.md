---
description: Level up your API with a simple but powerful generated command-line client.
---

# CLI Client

It's useful to have a terminal or command-line client for your API, so you can test it out and see how it works.

While Huma doesn't include this functionality built-in, you can utilize [Restish](https://rest.sh/) to quickly get a CLI up and running. Restish provides a nicer high-level interface to your API than just using `curl` or `httpie` directly, by providing commands for each operation, converting inputs into command-line arguments and options, and generating useful help documentation.

## Install Restish

First, install [Restish](https://rest.sh/):

=== "Mac"

    Install using Homebrew, Go, or [download a release](https://github.com/danielgtaylor/restish/releases).

    ```bash title="Terminal"
    # Homebrew
    $ brew tap danielgtaylor/restish
    $ brew install restish

    # Go (requires Go 1.18+)
    $ go install github.com/danielgtaylor/restish@latest
    ```

=== "Linux"

    Install using Go, Homebrew for Linux, or [download a release](https://github.com/danielgtaylor/restish/releases).

    ```bash title="Terminal"
    # Go (requires Go 1.18+)
    $ go install github.com/danielgtaylor/restish@latest

    # Homebrew for Linux
    $ brew tap danielgtaylor/restish
    $ brew install restish
    ```

=== "Windows"

    Install using Go or [download a release](https://github.com/danielgtaylor/restish/releases).

    ```bash title="Terminal"
    # Go (requires Go 1.18+)
    $ go install github.com/danielgtaylor/restish@latest
    ```

Also consider setting up [shell command-line completion](https://rest.sh/#/guide?id=shell-command-line-completion) for Restish.

## Configure your API

Next, we need to tell Restish about your API and give it a short name, which we'll call `tutorial`. Do this using the `api configure` command. This only needs to be done one time. Make sure your API is running and accessible before continuing, as this pulls the OpenAPI spec from the service.

{{ asciinema("../../terminal/restish-config.cast", rows="8") }}

## Calling the API

Once configured, you can call the API operations using high-level commands generated from the OpenAPI operation IDs:

{{ asciinema("../../terminal/restish-call.cast", rows="20") }}

See the help commands like `restish tutorial --help` or `restish tutorial get-greeting --help` for more details. If you set up command-line completion, you can also use tab to see all available commands.

## Review

Congratulations! You just learned:

-   How to install Restish
-   How to configure Restish for your API
-   How to call your API using Restish
-   How to pass parameters and body content to Restish

## Dive Deeper

Want to learn more about how Restish works and how to use it? Check these out next:

-   [Restish](https://rest.sh/)
-   [Restish OpenAPI 3 support](https://rest.sh/#/openapi)
-   [Restish input](https://rest.sh/#/input)
-   [A CLI for REST APIs](https://dev.to/danielgtaylor/a-cli-for-rest-apis-part-1-104b)
