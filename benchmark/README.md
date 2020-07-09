# Benchmarks

Huma makes use of some reflection. Reflection is slow. It's natural to think that this may make servers built with Huma slow.

This folder contains four implementations of the same service using the following libraries:

- [FastAPI](https://github.com/tiangolo/fastapi) (Python, popular framework)
- [Gin](https://github.com/gin-gonic/gin) (Go, popular framework)
- [Echo](https://echo.labstack.com/) (Go, popular framework)
- [Huma](https://github.com/istreamlabs/huma) (Go, this project)

The [wrk](https://github.com/wg/wrk) benchmarking tool is used to make requests against each implementation for 10 seconds with 10 concurrent workers. The results on a 2017 MacBook Pro are shown below:

| Implementation |  Req/s | Avg Req (ms) |  Transfer | Relative Speed |
| -------------- | -----: | -----------: | --------: | -------------: |
| FastAPI        |  7,052 |        1.440 |  1.35MB/s |             8% |
| Gin            | 85,892 |        0.118 | 14.91MB/s |           100% |
| Echo           | 84,532 |        0.120 | 14.67MB/s |            98% |
| Huma           | 80,478 |        0.136 | 13.97MB/s |            94% |

<details>
  <summary>Benchmark Details</summary>

First install the benchmark tool, e.g:

```sh
$ brew install wrk
```

For Python, install [Pipenv](https://pipenv.pypa.io/en/latest/). Then:

```sh
$ cd benchmark/fastapi
$ pipenv install
$ pipenv run uvicorn --workers=8 --no-access-log main:app
```

In another tab:

```sh
$ wrk -t 10 -d 10 -H "Authorization: bearer 123" http://localhost:8000/items/123
Running 10s test @ http://localhost:8000/items/123
  10 threads and 10 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency     1.44ms  748.88us  25.13ms   93.33%
    Req/Sec   713.35    232.62     1.28k    59.02%
  71237 requests in 10.10s, 13.59MB read
Requests/sec:   7051.84
Transfer/sec:      1.35MB
```

For the Gin benchmark:

```sh
$ go run ./benchmark/gin/main.go
```

In another tab:

```sh
$ wrk -t 10 -d 10 -H "Authorization: bearer 123" http://localhost:8888/items/123
Running 10s test @ http://localhost:8888/items/123
  10 threads and 10 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency   118.42us  101.92us   5.00ms   96.21%
    Req/Sec     8.65k   411.81    11.28k    84.23%
  867478 requests in 10.10s, 150.57MB read
Requests/sec:  85892.15
Transfer/sec:     14.91MB
```

For the Echo benchmark:

```sh
$ go run ./benchmark/echo/main.go
```

In another tab:

```sh
$ wrk -t 10 -d 10 -H "Authorization: bearer 123" http://localhost:1323/items/123
Running 10s test @ http://localhost:1323/items/123
  10 threads and 10 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency   120.17us   94.26us   5.66ms   95.71%
    Req/Sec     8.50k   496.66    10.33k    78.71%
  853731 requests in 10.10s, 148.18MB read
Requests/sec:  84532.91
Transfer/sec:     14.67MB
```

For Huma:

```sh
$ go run ./benchmarks/huma/main.go
```

In another tab:

```sh
$ wrk -t 10 -d 10 -H "Authorization: bearer 123" http://localhost:8888/items/123
Running 10s test @ http://localhost:8888/items/123
  10 threads and 10 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency   136.38us  163.89us   8.27ms   94.67%
    Req/Sec     8.09k   456.92    10.07k    79.60%
  812830 requests in 10.10s, 141.08MB read
Requests/sec:  80478.11
Transfer/sec:     13.97MB
```

</details>

Does reflection slow Huma down? Yes. Does it matter for 99% of use-cases? Probably not. For that small slowdown you get a lot of nice features built-in.

Also, if you like FastAPI, you can get a **massive** speedup (over 10x) by switching to Huma while keeping most of the same feature set.
