package internal

// ContextKey is a type used for Goji's context.Context keys.
type ContextKey int

var (
	// Path is the context key used to store the path Goji uses for its
	// PathPrefix optimization.
	Path interface{} = ContextKey(0)
	// Pattern is the context key used to store the Pattern that Goji last
	// matched.
	Pattern interface{} = ContextKey(1)
	// Handler is the context key used to store the Handler that Goji last
	// mached (and will therefore dispatch to at the end of the middleware
	// stack).
	Handler interface{} = ContextKey(2)
)
