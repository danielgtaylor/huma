# Benchmarks

Huma makes use of some reflection. Reflection is slow. It's natural to think that this may make servers built with Huma slow.

This folder contains three implementations of the same service using the following libraries:

- [FastAPI](https://github.com/tiangolo/fastapi) (Python, popular framework)
- [Gin](https://github.com/gin-gonic/gin) (Go, popular framework)
- [Huma](https://github.com/danielgtaylor/huma) (Go, this project)

A Go implementation of wrk called [go-wrk](https://github.com/tsliwowicz/go-wrk) is used to make requests against each implementation for 10 seconds with 10 concurrent workers. The results on a 2017 MacBook Pro are shown below:

| Implementation |  Req/s | Avg Req (ms) | Relative Speed |
| -------------- | -----: | -----------: | -------------: |
| FastAPI        |  6,160 |        1.623 |            11% |
| Gin            | 56,130 |        0.178 |           100% |
| Huma           | 52,106 |        0.192 |            93% |

<details>
  <summary>Benchmark Details</summary>

First install the benchmark tool:

```sh
$ go get github.com/tsliwowicz/go-wrk
```

For Python, install [Pipenv](). Then:

```sh
$ cd benchmark/fastapi
$ pipenv install
$ pipenv run uvicorn --workers=8 --no-access-log main:app
```

In another tab:

```sh
$ go-wrk -H "Authorization: bearer abc123" http://localhost:8000/items/123
Running 10s test @ http://localhost:8000/items/123
  10 goroutine(s) running concurrently
61360 requests in 9.961536365s, 10.71MB read
Requests/sec:		6159.69
Transfer/sec:		1.08MB
Avg Req Time:		1.623457ms
Fastest Request:	628.285µs
Slowest Request:	32.805386ms
Number of Errors:	0
```

For the Gin benchmark:

```sh
$ go run ./benchmark/gin/main.go
```

In another tab:

```sh
$ go-wrk -H "Authorization: bearer abc123" http://localhost:8888/items/123
Running 10s test @ http://localhost:8888/items/123
  10 goroutine(s) running concurrently
542121 requests in 9.658220351s, 85.31MB read
Requests/sec:		56130.53
Transfer/sec:		8.83MB
Avg Req Time:		178.156µs
Fastest Request:	57.824µs
Slowest Request:	6.100352ms
Number of Errors:	0
```

For Huma:

```sh
$ go run ./benchmarks/huma/main.go
```

In another tab:

```sh
$ go-wrk -H "Authorization: bearer abc123" http://localhost:8888/items/123
Running 10s test @ http://localhost:8888/items/123
  10 goroutine(s) running concurrently
504191 requests in 9.676117336s, 79.34MB read
Requests/sec:		52106.75
Transfer/sec:		8.20MB
Avg Req Time:		191.913µs
Fastest Request:	59.686µs
Slowest Request:	5.727491ms
Number of Errors:	0
```

</details>

Does reflection slow Huma down? Yes. Does it matter for 99% of use-cases? Probably not. For that small slowdown you get a lot of nice features built-in.

Also, if you like FastAPI, you can get a **massive** speedup (almost 10x) by switching to Huma while keeping most of the same feature set.
