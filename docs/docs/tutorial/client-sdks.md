# Client SDKs

[Several tools](https://openapi.tools/#sdk) can be used to create SDKs from an OpenAPI spec. Let's use the [`oapi-codegen`](https://github.com/deepmap/oapi-codegen) Go code generator to create a Go SDK, and then build a client using that SDK.

## Generate the SDK

First, grab the OpenAPI spec. Then install and use the generator to create the SDK.

{{ asciinema("/terminal/build-sdk.cast", rows="14") }}

## Build the Client

Next, we can use the SDK by writing a small client script.

```go title="client/client.go"
package main

import (
	"context"
	"fmt"

	"github.com/my-user/my-api/sdk"
)

func main() {
	ctx := context.Background()

	// Initialize an SDK client.
	client, _ := sdk.NewClientWithResponses("http://localhost:8888")

	// Make the greeting request.
	greeting, err := client.GetGreetingWithResponse(ctx, "world")
	if err != nil {
		panic(err)
	}

	if greeting.StatusCode() > 200 {
		panic(greeting.ApplicationproblemJSONDefault)
	}

	// Everything was successful, so print the message.
	fmt.Println(greeting.JSON200.Message)
}
```

## Run the Client

Now you're ready to run the client:

{{ asciinema("/terminal/sdk-client.cast", rows="8") }}

## Review

Congratulations! You just learned:

-   How to install an SDK generator
-   How to generate a Go SDK for your API
-   How to build a client using the SDK to call the API

## Dive Deeper

Want to learn more about OpenAPI tooling like SDK generators and how to use them? Check these out next:

-   SDK Generators
    -   [`oapi-codegen`](https://github.com/deepmap/oapi-codegen)
    -   [OpenAPI Generator](https://openapi-generator.tech/)
-   OpenAPI Tool Directories
    -   [openapi.tools](https://openapi.tools/)
    -   [tools.openapis.org](https://tools.openapis.org/)
