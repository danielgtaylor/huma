package main

import (
	"context"
	"fmt"
	"net/http"
	"reflect"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/danielgtaylor/huma/v2/humacli"
	"github.com/go-chi/chi/v5"
	"github.com/google/go-github/v69/github"

	_ "github.com/danielgtaylor/huma/v2/formats/cbor"
)

// Options for the CLI.
type Options struct {
	Port int `help:"Port to listen on" short:"p" default:"8888"`
}

type User struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type GetUserInput struct {
	ID int64 `path:"id" example:"1" doc:"User ID"`
}

type GetUserOutput struct {
	Body User
}

type GetGithubRepositoryInput struct {
	Owner string `path:"owner" example:"danielgtaylor" doc:"Repository owner"`
	Repo  string `path:"repo" example:"huma" doc:"Repository name"`
}

type GetGithubRepositoryOutput struct {
	Body *github.Repository
}

func aliasConflictingTypes(api huma.API) {
	registry := api.OpenAPI().Components.Schemas

	type GitHubUser struct {
		*github.User
	}

	// rename github.User to GithubUser
	registry.RegisterTypeAlias(reflect.TypeOf(github.User{}), reflect.TypeOf(GitHubUser{}))
}

func main() {
	// Create a CLI app which takes a port option.
	cli := humacli.New(func(hooks humacli.Hooks, options *Options) {
		// Create a new router & API
		router := chi.NewMux()
		api := humachi.New(router, huma.DefaultConfig("My API", "1.0.0"))

		// main.User and github.User names are conflicting, we need to rename them
		aliasConflictingTypes(api)

		// Register GET /user/{id}
		huma.Register(api, huma.Operation{
			OperationID: "get-user",
			Summary:     "Get user by id",
			Method:      http.MethodGet,
			Path:        "/user/{id}",
		}, func(ctx context.Context, input *GetUserInput) (*GetUserOutput, error) {
			resp := &GetUserOutput{}
			resp.Body.ID = input.ID
			resp.Body.Name = "John"
			return resp, nil
		})

		// Register GET /github/{owner}/{repo}
		huma.Register(api, huma.Operation{
			OperationID: "get-github-repository",
			Summary:     "Get GitHub repository",
			Method:      http.MethodGet,
			Path:        "/github/{owner}/{repo}",
		}, func(ctx context.Context, input *GetGithubRepositoryInput) (*GetGithubRepositoryOutput, error) {
			client := github.NewClient(nil)

			repo, _, err := client.Repositories.Get(ctx, input.Owner, input.Repo)
			if err != nil {
				return nil, err
			}

			resp := &GetGithubRepositoryOutput{}
			resp.Body = repo
			return resp, nil
		})

		// Tell the CLI how to start your router.
		hooks.OnStart(func() {
			http.ListenAndServe(fmt.Sprintf(":%d", options.Port), router)
		})
	})

	// Run the CLI. When passed no commands, it starts the server.
	cli.Run()
}
