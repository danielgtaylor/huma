---
description: Migrate your code from Huma v1 to Huma v2 in a few simple steps.
---

# Migrating From Huma V1

1. Import `github.com/danielgtaylor/huma/v2` instead of `github.com/danielgtaylor/huma`.
1. Use the `humachi.NewV4` adapter as Huma v1 uses Chi v4 under the hood
1. Attach your middleware to the `chi` instance.
1. Replace resource & operation creation with `huma.Register`
1. Rewrite handlers to be like `func(context.Context, *Input) (*Output, error)`
    1. Return errors instead of `ctx.WriteError(...)`
    1. Return instances instead of `ctx.WriteModel(...)`
1. Define options via a struct and use `humacli.New` to wrap the service

Note that GraphQL support from Huma v1 has been removed. Take a look at alternative tools like [https://www.npmjs.com/package/openapi-to-graphql](https://www.npmjs.com/package/openapi-to-graphql) which will automatically generate a GraphQL endpoint from Huma's generated OpenAPI spec.
