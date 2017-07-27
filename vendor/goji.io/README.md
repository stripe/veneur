Goji
====

[![GoDoc](https://godoc.org/goji.io?status.svg)](https://godoc.org/goji.io) [![Build Status](https://travis-ci.org/goji/goji.svg?branch=master)](https://travis-ci.org/goji/goji)

Goji is a HTTP request multiplexer, similar to [`net/http.ServeMux`][servemux].
It compares incoming requests to a list of registered [Patterns][pattern], and
dispatches to the [Handler][handler] that corresponds to the first matching
Pattern. Goji also supports [Middleware][middleware] (composable shared
functionality applied to every request) and uses the de facto standard
[`x/net/context`][context] to store request-scoped values.

[servemux]: https://golang.org/pkg/net/http/#ServeMux
[pattern]: https://godoc.org/goji.io#Pattern
[handler]: https://godoc.org/goji.io#Handler
[middleware]: https://godoc.org/goji.io#Mux.Use
[context]: https://godoc.org/golang.org/x/net/context


Quick Start
-----------

```go
package main

import (
        "fmt"
        "net/http"

        "goji.io"
        "goji.io/pat"
        "golang.org/x/net/context"
)

func hello(ctx context.Context, w http.ResponseWriter, r *http.Request) {
        name := pat.Param(ctx, "name")
        fmt.Fprintf(w, "Hello, %s!", name)
}

func main() {
        mux := goji.NewMux()
        mux.HandleFuncC(pat.Get("/hello/:name"), hello)

        http.ListenAndServe("localhost:8000", mux)
}
```

Please refer to [Goji's GoDoc Documentation][godoc] for a full API reference.

[godoc]: https://godoc.org/goji.io


Stability
---------

Goji's API is stable, and guarantees to never break compatibility with existing
code (under similar rules to the Go project's [guidelines][compat]). Goji is
suitable for use in production.

One possible exception to the above compatibility guarantees surrounds the
inclusion of the `x/net/context` package in the standard library for Go 1.7.
When this happens, Goji may switch its package imports to use the standard
library's version of the package. Note that, while this is a backwards
incompatible change, the impact on clients is expected to be minimal:
applications will simply have to change the import path of the `context`
package. More discussion about this migration can be found on the Goji mailing
list.

[compat]: https://golang.org/doc/go1compat


Community / Contributing
------------------------

Goji maintains a mailing list, [gojiberries][berries], where you should feel
welcome to ask questions about the project (no matter how simple!), to announce
projects or libraries built on top of Goji, or to talk about Goji more
generally. Goji's author (Carl Jackson) also loves to hear from users directly
at his personal email address, which is available on his GitHub profile page.

Contributions to Goji are welcome, however please be advised that due to Goji's
stability guarantees interface changes are unlikely to be accepted.

All interactions in the Goji community will be held to the high standard of the
broader Go community's [Code of Conduct][conduct].

[berries]: https://groups.google.com/forum/#!forum/gojiberries
[conduct]: https://golang.org/conduct
