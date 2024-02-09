Goji
====

[![GoDoc](https://godoc.org/goji.io?status.svg)](https://godoc.org/goji.io) [![Build Status](https://travis-ci.org/goji/goji.svg?branch=master)](https://travis-ci.org/goji/goji)

Goji is a HTTP request multiplexer, similar to [`net/http.ServeMux`][servemux].
It compares incoming requests to a list of registered [Patterns][pattern], and
dispatches to the [http.Handler][handler] that corresponds to the first matching
Pattern. Goji also supports [Middleware][middleware] (composable shared
functionality applied to every request) and uses the standard
[`context`][context] package to store request-scoped values.

[servemux]: https://golang.org/pkg/net/http/#ServeMux
[pattern]: https://godoc.org/goji.io#Pattern
[handler]: https://golang.org/pkg/net/http/#Handler
[middleware]: https://godoc.org/goji.io#Mux.Use
[context]: https://golang.org/pkg/context


Quick Start
-----------

```go
package main

import (
        "fmt"
        "net/http"

        "goji.io"
        "goji.io/pat"
)

func hello(w http.ResponseWriter, r *http.Request) {
        name := pat.Param(r, "name")
        fmt.Fprintf(w, "Hello, %s!", name)
}

func main() {
        mux := goji.NewMux()
        mux.HandleFunc(pat.Get("/hello/:name"), hello)

        http.ListenAndServe("localhost:8000", mux)
}
```

Please refer to [Goji's GoDoc Documentation][godoc] for a full API reference.

[godoc]: https://godoc.org/goji.io


Stability
---------

Goji's API was recently updated to use the new `net/http` and `context`
integration, and is therefore some of its interfaces are in a state of flux. We
don't expect any further changes to the API, and expect to be able to announce
API stability soon. Goji is suitable for use in production.

Prior to Go 1.7, Goji promised API stability with a different API to the one
that is offered today. The author broke this promise, and does not take this
breach of trust lightly. While stability is obviously extremely important, the
author and community have decided to follow the broader Go community in
standardizing on the standard library copy of the `context` package.

Users of the old API can find that familiar API on the `net-context` branch. The
author promises to maintain both the `net-context` branch and `master` for the
forseeable future.


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
