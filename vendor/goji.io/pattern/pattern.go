/*
Package pattern contains utilities for Goji Pattern authors.

Goji users should not import this package. Instead, use the utilities provided
by your Pattern package. If you are looking for an implementation of Pattern,
try Goji's pat subpackage, which contains a simple domain specific language for
specifying routes.

For Pattern authors, use of this subpackage is entirely optional. Nevertheless,
authors who wish to take advantage of Goji's PathPrefix optimization or who wish
to standardize on a few common interfaces may find this package useful.
*/
package pattern

import (
	"context"

	"goji.io/internal"
)

/*
Variable is a standard type for the names of Pattern-bound variables, e.g.
variables extracted from the URL. Pass the name of a variable, cast to this
type, to context.Context.Value to retrieve the value bound to that name.
*/
type Variable string

type allVariables struct{}

/*
AllVariables is a standard value which, when passed to context.Context.Value,
returns all variable bindings present in the context, with bindings in newer
contexts overriding values deeper in the stack. The concrete type

	map[Variable]interface{}

is used for this purpose. If no variables are bound, nil should be returned
instead of an empty map.
*/
var AllVariables = allVariables{}

/*
Path returns the path that the Goji router uses to perform the PathPrefix
optimization. While this function does not distinguish between the absence of a
path and an empty path, Goji will automatically extract a path from the request
if none is present.

By convention, paths are stored in their escaped form (i.e., the value returned
by net/url.URL.EscapedPath, and not URL.Path) to ensure that Patterns have as
much discretion as possible (e.g., to behave differently for '/' and '%2f').
*/
func Path(ctx context.Context) string {
	pi := ctx.Value(internal.Path)
	if pi == nil {
		return ""
	}
	return pi.(string)
}

/*
SetPath returns a new context in which the given path is used by the Goji Router
when performing the PathPrefix optimization. See Path for more information about
the intended semantics of this path.
*/
func SetPath(ctx context.Context, path string) context.Context {
	return context.WithValue(ctx, internal.Path, path)
}
