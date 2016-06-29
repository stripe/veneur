/*
Package internal is a private package that allows Goji to expose a less
confusing interface to its users. This package must not be used outside of Goji;
every piece of its functionality has been exposed by one of Goji's subpackages.

The problem this package solves is to allow Goji to internally agree on types
and secret values between its packages without introducing import cycles. Goji
needs to agree on these types and values in order to organize its public API
into audience-specific subpackages (for instance, a package for pattern authors,
a package for middleware authors, and a main package for routing users) without
exposing implementation details in any of the packages.
*/
package internal
