# Developing in Veneur

This document gives a quick overview of how to run veneur for
exploration, and trying out new features under development.

If you're interested in writing (or reading!) code in the veneur code
base, please see [internals.md](internals.md)!

## Running veneur in development

While veneur has decent test coverage, sometimes you'll want to run it
on a development machine (as opposed to your production infra). Since
veneur is written in go, you can use `go run` in the veneur project
root directory to start a development instance, with the
provided [dev.yaml](dev.yaml) file:

``` sh
go run ./cmd/veneur/main.go -f docs/dev.yaml
```

This will start a veneur that listens for statsd packets on UDP port
8200, and for framed SSF data on the UNIX domain socket
`/tmp/veneur.sock`.

Note that veneur does *no* hot code reloading - if you change source
code, you have to Ctrl-C the running veneur server and restart it.

## Submitting data to a veneur instance

In order to see how your veneur instance behaves in development (and
to allow your production shell scripts to submit data too), we ship a
tool called `veneur-emit`. Much like veneur itself, you don't need to
compile it before running either:

``` sh
go run ./cmd/veneur-emit/main.go -ssf -hostport unix:///tmp/veneur.sock -trace_id 999 -parent_span_id 9999 -name hi.there -span_service veneur_ssf_investigation -debug -command /usr/bin/true
 ```

This will time the command `/usr/bin/true` and submit an SSF span
concerning the run time of this process to veneur.

`veneur-emit` has a bunch more options, check out the usage for it
with `go run ./cmd/veneur-emit/main.go -help`!

## Adding test data files

When your tests depend on data files, put them into a `testdata`
directory next to the test file, and use relative paths to refer to
them.
