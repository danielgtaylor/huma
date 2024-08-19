<a href="#">
	<picture>
		<source media="(prefers-color-scheme: dark)" srcset="https://huma.rocks/huma-dark.png" />
		<source media="(prefers-color-scheme: light)" srcset="https://huma.rocks/huma.png" />
		<img alt="Huma Logo" src="https://huma.rocks/huma.png" />
	</picture>
</a>

[![HUMA Powered](https://img.shields.io/badge/Powered%20By-HUMA-f40273)](https://huma.rocks/) [![CI](https://github.com/danielgtaylor/huma/workflows/CI/badge.svg?branch=main)](https://github.com/danielgtaylor/huma/actions?query=workflow%3ACI+branch%3Amain++) [![codecov](https://codecov.io/gh/danielgtaylor/huma/branch/main/graph/badge.svg)](https://codecov.io/gh/danielgtaylor/huma) [![Docs](https://godoc.org/github.com/danielgtaylor/huma/v2?status.svg)](https://pkg.go.dev/github.com/danielgtaylor/huma/v2?tab=doc) [![Go Report Card](https://goreportcard.com/badge/github.com/danielgtaylor/huma/v2)](https://goreportcard.com/report/github.com/danielgtaylor/huma/v2)

[**🌎English Documentation**](./README.md)

- [什么是 huma?](#intro)
- [安装](#install)
- [样例](#example)
- [文档](#documentation)

<a name="intro"></a>
一个现代、简单、快速且灵活的微框架，用于在 OpenAPI 3 和 JSON Schema 支持的 Go 中构建 HTTP REST/RPC API。国际音标发音：[/'hjuːmɑ/](https://en.wiktionary.org/wiki/Wiktionary:International_Phonetic_Alphabet)。该项目的目标是提供：

- 拥有现有服务的团队逐步采用
  - 带上您自己的路由器（包括 Go 1.22+）、中间件和日志记录/指标
  - 可扩展的 OpenAPI 和 JSON Schema 层来记录现有路由
- 适合 Go 开发人员的现代 REST 或 HTTP RPC API 后端框架
  - [由OpenAPI 3.1](https://github.com/OAI/OpenAPI-Specification/blob/master/versions/3.1.0.md)和[JSON Schema](https://json-schema.org/)描述
- 防止常见错误的护栏
- 不会过时的文档
- 生成的高质量开发人员工具

特点包括：

- 您选择的路由器之上的声明式接口：
  - 操作和模型文档
  - 请求参数（路径、查询、标头或 cookie）
  - 请求正文
  - 响应（包括错误）
  - 响应标头
- [使用RFC9457](https://datatracker.ietf.org/doc/html/rfc9457)和默认情况下的JSON 错误`application/problem+json`（但可以更改）
- 每个操作的请求大小限制与合理的默认值
- 服务器和客户端之间的 [内容协商](https://developer.mozilla.org/en-US/docs/Web/HTTP/Content_negotiation)
  - 通过默认配置的标头支持 JSON ( [RFC 8259](https://tools.ietf.org/html/rfc8259) ) 和可选的 CBOR ( [RFC 7049](https://tools.ietf.org/html/rfc7049) ) 内容类型。`Accept`
- 条件请求支持，例如`If-Match`或`If-Unmodified-Since`header 实用程序。
- 可选的自动生成 `PATCH` 操作支持：
  - [RFC 7386](https://www.rfc-editor.org/rfc/rfc7386) JSON 合并补丁
  - [RFC 6902](https://www.rfc-editor.org/rfc/rfc6902) JSON 补丁
  - [速记](https://github.com/danielgtaylor/shorthand)补丁
- 输入和输出模型的带注释的 Go 类型
  - 从 Go 类型生成 JSON 模式
  - 路径/查询/标头参数、正文、响应标头等的静态类型。
  - 自动输入模型验证和错误处理
- [使用Stoplight Elements](https://stoplight.io/open-source/elements)生成文档
- 可选的内置 CLI，通过参数或环境变量进行配置
  - `-p 8000`通过例如、`--port=8000`、 或设置`SERVICE_PORT=8000`
  - 内置启动操作和正常关闭
- 生成 OpenAPI 以访问丰富的工具生态系统
  - 使用[API Sprout](https://github.com/danielgtaylor/apisprout)或[Prism进行模拟](https://stoplight.io/open-source/prism)
  - [带有OpenAPI Generator](https://github.com/OpenAPITools/openapi-generator)或[oapi-codegen 的](https://github.com/deepmap/oapi-codegen)SDK
  - CLI 与[Restish](https://rest.sh/)
  - 还有[更多](https://openapi.tools/) 
- 使用可选`describedby`链接关系标头以及`$schema`返回对象中的可选属性为每个资源生成 JSON 架构，这些属性集成到编辑器中以进行验证和完成。

该项目的灵感来自[FastAPI](https://fastapi.tiangolo.com/)。标志和品牌由卡里·泰勒设计。

## 赞助商

非常感谢我们现任和前任的赞助商：

- [@bclements](https://github.com/bclements)
- [@bekabaz](https://github.com/bekabaz)
- [@victoraugustolls](https://github.com/victoraugustolls)

## 感言

> 这是迄今为止我最喜欢的 Go Web 框架。它受到 FastAPI 的启发，这也很棒，并且符合常见 Web 事物的许多 RFC...我真的很喜欢这个功能集，它[可以使用] Chi，而且它在某种程度上仍然相对简单使用。我尝试过其他框架，但它们并没有给我带来快乐。 - [Jeb_Jenky](https://www.reddit.com/r/golang/comments/zhitcg/comment/izmg6vk/?utm_source=reddit&utm_medium=web2x&context=3)

> 使用 #Golang 一年多后，我偶然发现了 Huma，一个受 #FastAPI 启发的 Web 框架。这就是我一直期盼的圣诞奇迹！这个框架什么都有！- [Hana Mohan](https://twitter.com/unamashana/status/1733088066053583197)

> 我爱胡玛。真诚地感谢您提供这个很棒的包裹。我已经使用它有一段时间了，效果非常好！ - [plscott](https://www.reddit.com/r/golang/comments/1aoshey/comment/kq6hcpd/?utm_source=reddit&utm_medium=web2x&context=3)

> 谢谢丹尼尔为胡玛。非常有用的项目，并且由于 OpenAPI gen 为我们节省了大量的时间和麻烦——类似于 Python 中的 FastAPI。 - [WolvesOfAllStreets](https://www.reddit.com/r/golang/comments/1aqj99d/comment/kqfqcml/?utm_source=reddit&utm_medium=web2x&context=3)

> Huma 很棒，我最近开始使用它，很高兴，所以非常感谢您的努力🙏  - [callmemicah](https://www.reddit.com/r/golang/comments/1b32ts4/comment/ksvr9h7/?utm_source=reddit&utm_medium=web2x&context=3)

# 安装

通过安装`go get`。请注意，需要 Go 1.21 或更高版本。

```sh
# After: go mod init ...
go get -u github.com/danielgtaylor/huma/v2
```

# 例子

这是 Huma 中的一个完整的基本 hello world 示例，展示了如何使用 CLI 初始化 Huma 应用程序、声明资源操作并定义其处理函数。

```go
package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/danielgtaylor/huma/v2/humacli"
	"github.com/go-chi/chi/v5"

	_ "github.com/danielgtaylor/huma/v2/formats/cbor"
)

// Options for the CLI. Pass `--port` or set the `SERVICE_PORT` env var.
type Options struct {
	Port int `help:"Port to listen on" short:"p" default:"8888"`
}

// GreetingOutput represents the greeting operation response.
type GreetingOutput struct {
	Body struct {
		Message string `json:"message" example:"Hello, world!" doc:"Greeting message"`
	}
}

func main() {
	// Create a CLI app which takes a port option.
	cli := humacli.New(func(hooks humacli.Hooks, options *Options) {
		// Create a new router & API
		router := chi.NewMux()
		api := humachi.New(router, huma.DefaultConfig("My API", "1.0.0"))

		// Add the operation handler to the API.
		huma.Get(api, "/greeting/{name}", func(ctx context.Context, input *struct{
			Name string `path:"name" maxLength:"30" example:"world" doc:"Name to greet"`
		}) (*GreetingOutput, error) {
			resp := &GreetingOutput{}
			resp.Body.Message = fmt.Sprintf("Hello, %s!", input.Name)
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
```

> [!TIP]
> 替换`chi.NewMux()`→`http.NewServeMux()`和`humachi.New`→`humago.New`以使用 Go 1.22+ 中的标准库路由器。只需确保其中列出了您的或更新的`go.mod`版本即可。`go 1.22`其他一切都保持不变！当你准备好时就切换。

你可以用 `go run greet.go` 测试它（可选地传递 '--port' 来更改默认值），并使用 [Restish](https://rest.sh/)（或 `curl`） 发出示例请求：

```sh
# Get the message from the server
$ restish :8888/greeting/world
HTTP/1.1 200 OK
...
{
	$schema: "http://localhost:8888/schemas/GreetingOutputBody.json",
	message: "Hello, world!"
}
```

尽管示例很小，您也可以在 http://localhost:8888/docs 上看到一些生成的文档。生成的 OpenAPI 可在 http://localhost:8888/openapi.json 或 http://localhost:8888/openapi.yaml 获取。

查看[Huma 教程](https://huma.rocks/tutorial/installation/)，获取入门分步指南。

# 文档

请参阅 https://huma.rocks/ 网站，获取演示文稿中的完整文档，该演示文稿比本自述文件更易于导航和搜索。您可以在`docs`此存储库的目录中找到该站点的源代码。

官方 Go 包文档始终可以在 https://pkg.go.dev/github.com/danielgtaylor/huma/v2 找到。

# 文章和提及

- [Go 中的 API 与 Huma 2.0](https://dgt.hashnode.dev/apis-in-go-with-huma-20)
- [减少 Go 依赖：Huma 中减少依赖的案例研究](https://dgt.hashnode.dev/reducing-go-dependencies)
- [在 Twitter/X 上分享的 Golang 新闻、库和工作](https://twitter.com/golangch/status/1752175499701264532)
- [Go Weekly #495](https://golangweekly.com/issues/495) & [#498](https://golangweekly.com/issues/498)精选
- [Bump.sh 从 Huma 部署文档](https://docs.bump.sh/guides/bump-sh-tutorials/huma/)
- [使用泛型的可组合 HTTP 处理程序](https://www.willem.dev/articles/generic-http-handlers/)中提到

如果您觉得该项目有用，请务必为该项目加注星标！

<a href="https://star-history.com/#danielgtaylor/huma&Date">
	<picture>
		<source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=danielgtaylor/huma&type=Date&theme=dark" />
		<source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=danielgtaylor/huma&type=Date" />
		<img alt="Star History Chart" src="https://api.star-history.com/svg?repos=danielgtaylor/huma&type=Date" />
	</picture>
</a>
