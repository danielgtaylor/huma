---
description: Benchmarks for Go & Huma show that it's fast and low memory. It's a great choice over Node.js, FastAPI, or Django.
---

# Benchmarks

You should always perform your own benchmarking, as your use-case may not be identical to the use-cases of others. However, here are some general benchmarks to get you started.

## Go Performance

Go is fast. When compared to Node.js and Python, Go is often much faster while using less memory and handling concurrency better. The [Techempower benchmarks](https://www.techempower.com/benchmarks/#section=data-r21&l=zijmkf-6bj&p=0-0-0-0-1kw&f=0-0-0-0-0-0-0-194hs-0-0-mhhc-0-o-0&test=composite) are a good place to start for a general comparison:

![Techempower benchmarks](./techempower.png)

Notice that all the top frameworks are written in Go. Huma's performance and features will vary based on the specific router or framework used, but it is generally on par with the other top Go frameworks.

Highlighting just a few is telling, and this ignores the improvements in memory use which likely means cheaper hardware and container costs:

| Framework | Language   | JSON Req/s | Percentage | Fortunes Req/s | Percentage |
| --------- | ---------- | ---------: | ---------: | -------------: | ---------: |
| Chi       | Go         |       520K |       100% |           150K |       100% |
| Node.js   | Javascript |       377K |        73% |            80K |        53% |
| FastAPI   | Python     |       168K |        32% |            50K |        33% |
| Django    | Python     |        73K |        14% |            15K |        10% |

### Takeways

Here are a few takeaways of the above, other benchmarks, and our time with Go:

-   Go (and thus Huma) is **fast** and **low memory**.
-   Go is **simple** and can be picked up by a team **quickly**.
    -   Its complexity is **on par** or less than Javascript or Python
-   Huma is a **good choice** over Node.js, FastAPI, or Django.

## Micro Benchmarks

Significant performance improvements have been made since Huma v1, as shown by the following basic benchmark operation with a few input parameters, a small input body, some output headers and an output body (see [`adapters/humachi/humachi_test.go`](https://github.com/danielgtaylor/huma/blob/main/adapters/humachi/humachi_test.go)).

```sh
# Huma v1
BenchmarkHumaV1Chi-10         16285  112086 ns/op  852209 B/op  258 allocs/op

# Huma v2
BenchmarkHumaV2Chi-10        431028    2777 ns/op    1718 B/op   29 allocs/op

# Chi without Huma (raw)
BenchmarkRawChi-10           552764    2143 ns/op    2370 B/op   29 allocs/op
```

These improvements are due to a number of factors, including changes to the Huma API, precomputation of reflection data when possible, low or zero-allocation validation & URL parsing, using shared buffer pools to limit garbage collector pressure, and more.

Since you bring your own router, you are free to "escape" Huma by using the router directly, but as you can see above it's rarely needed with v2.
