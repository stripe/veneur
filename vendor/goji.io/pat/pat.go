/*
Package pat is a URL-matching domain-specific language for Goji.


Quick Reference

The following table gives an overview of the language this package accepts. See
the subsequent sections for a more detailed explanation of what each pattern
does.

	Pattern			Matches			Does Not Match

	/			/			/hello

	/hello			/hello			/hi
							/hello/

	/user/:name		/user/carl		/user/carl/photos
				/user/alice		/user/carl/
							/user/

	/:file.:ext		/data.json		/.json
				/info.txt		/data.
				/data.tar.gz		/data.json/download

	/user/*			/user/			/user
				/user/carl
				/user/carl/photos


Static Paths

Most URL paths may be specified directly: the pattern "/hello" matches URLs with
precisely that path ("/hello/", for instance, is treated as distinct).

Note that this package operates on raw (i.e., escaped) paths (see the
documentation for net/url.URL.EscapedPath). In order to match a character that
can appear escaped in a URL path, use its percent-encoded form.


Named Matches

Named matches allow URL paths to contain any value in a particular path segment.
Such matches are denoted by a leading ":", for example ":name" in the rule
"/user/:name", and permit any non-empty value in that position. For instance, in
the previous "/user/:name" example, the path "/user/carl" is matched, while
"/user/" or "/user/carl/" (note the trailing slash) are not matched. Pat rules
can contain any number of named matches.

Named matches set URL variables by comparing pattern names to the segments they
matched. In our "/user/:name" example, a request for "/user/carl" would bind the
"name" variable to the value "carl". Use the Param function to extract these
variables from the request context. Variable names in a single pattern must be
unique.

Matches are ordinarily delimited by slashes ("/"), but several other characters
are accepted as delimiters (with slightly different semantics): the period
("."), semicolon (";"), and comma (",") characters. For instance, given the
pattern "/:file.:ext", the request "/data.json" would match, binding "file" to
"data" and "ext" to "json". Note that these special characters are treated
slightly differently than slashes: the above pattern also matches the path
"/data.tar.gz", with "ext" getting set to "tar.gz"; and the pattern "/:file"
matches names with dots in them (like "data.json").


Prefix Matches

Pat can also match prefixes of routes using wildcards. Prefix wildcard routes
end with "/*", and match just the path segments preceding the asterisk. For
instance, the pattern "/user/*" will match "/user/" and "/user/carl/photos" but
not "/user" (note the lack of a trailing slash).

The unmatched suffix, including the leading slash ("/"), are placed into the
request context, which allows subsequent routing (e.g., a subrouter) to continue
from where this pattern left off. For instance, in the "/user/*" pattern from
above, a request for "/user/carl/photos" will consume the "/user" prefix,
leaving the path "/carl/photos" for subsequent patterns to handle. A subrouter
pattern for "/:name/photos" would match this remaining path segment, for
instance.
*/
package pat

import (
	"net/http"
	"regexp"
	"sort"
	"strings"

	"goji.io/pattern"
)

type patNames []struct {
	name pattern.Variable
	idx  int
}

func (p patNames) Len() int {
	return len(p)
}
func (p patNames) Less(i, j int) bool {
	return p[i].name < p[j].name
}
func (p patNames) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

/*
Pattern implements goji.Pattern using a path-matching domain specific language.
See the package documentation for more information about the semantics of this
object.
*/
type Pattern struct {
	raw     string
	methods map[string]struct{}
	// These are parallel arrays of each pattern string (sans ":"), the
	// breaks each expect afterwords (used to support e.g., "." dividers),
	// and the string literals in between every pattern. There is always one
	// more literal than pattern, and they are interleaved like this:
	// <literal> <pattern> <literal> <pattern> <literal> etc...
	pats     patNames
	breaks   []byte
	literals []string
	wildcard bool
}

// "Break characters" are characters that can end patterns. They are not allowed
// to appear in pattern names. "/" was chosen because it is the standard path
// separator, and "." was chosen because it often delimits file extensions. ";"
// and "," were chosen because Section 3.3 of RFC 3986 suggests their use.
const bc = "/.;,"

var patternRe = regexp.MustCompile(`[` + bc + `]:([^` + bc + `]+)`)

/*
New returns a new Pattern from the given Pat route. See the package
documentation for more information about what syntax is accepted by this
function.
*/
func New(pat string) *Pattern {
	p := &Pattern{raw: pat}

	if strings.HasSuffix(pat, "/*") {
		pat = pat[:len(pat)-1]
		p.wildcard = true
	}

	matches := patternRe.FindAllStringSubmatchIndex(pat, -1)
	numMatches := len(matches)
	p.pats = make(patNames, numMatches)
	p.breaks = make([]byte, numMatches)
	p.literals = make([]string, numMatches+1)

	n := 0
	for i, match := range matches {
		a, b := match[2], match[3]
		p.literals[i] = pat[n : a-1] // Need to leave off the colon
		p.pats[i].name = pattern.Variable(pat[a:b])
		p.pats[i].idx = i
		if b == len(pat) {
			p.breaks[i] = '/'
		} else {
			p.breaks[i] = pat[b]
		}
		n = b
	}
	p.literals[numMatches] = pat[n:]

	sort.Sort(p.pats)

	return p
}

func newWithMethods(pat string, methods ...string) *Pattern {
	p := New(pat)

	methodSet := make(map[string]struct{}, len(methods))
	for _, method := range methods {
		methodSet[method] = struct{}{}
	}
	p.methods = methodSet

	return p
}

/*
Match runs the Pat pattern on the given request, returning a non-nil output
request if the input request matches the pattern.

This function satisfies goji.Pattern.
*/
func (p *Pattern) Match(r *http.Request) *http.Request {
	if p.methods != nil {
		if _, ok := p.methods[r.Method]; !ok {
			return nil
		}
	}

	// Check Path
	ctx := r.Context()
	path := pattern.Path(ctx)
	var scratch []string
	if p.wildcard {
		scratch = make([]string, len(p.pats)+1)
	} else if len(p.pats) > 0 {
		scratch = make([]string, len(p.pats))
	}

	for i := range p.pats {
		sli := p.literals[i]
		if !strings.HasPrefix(path, sli) {
			return nil
		}
		path = path[len(sli):]

		m := 0
		bc := p.breaks[i]
		for ; m < len(path); m++ {
			if path[m] == bc || path[m] == '/' {
				break
			}
		}
		if m == 0 {
			// Empty strings are not matches, otherwise routes like
			// "/:foo" would match the path "/"
			return nil
		}
		scratch[i] = path[:m]
		path = path[m:]
	}

	// There's exactly one more literal than pat.
	tail := p.literals[len(p.pats)]
	if p.wildcard {
		if !strings.HasPrefix(path, tail) {
			return nil
		}
		scratch[len(p.pats)] = path[len(tail)-1:]
	} else if path != tail {
		return nil
	}

	for i := range p.pats {
		var err error
		scratch[i], err = unescape(scratch[i])
		if err != nil {
			// If we encounter an encoding error here, there's
			// really not much we can do about it with our current
			// API, and I'm not really interested in supporting
			// clients that misencode URLs anyways.
			return nil
		}
	}

	return r.WithContext(&match{ctx, p, scratch})
}

/*
PathPrefix returns a string prefix that the Paths of all requests that this
Pattern accepts must contain.

This function satisfies goji's PathPrefix Pattern optimization.
*/
func (p *Pattern) PathPrefix() string {
	return p.literals[0]
}

/*
HTTPMethods returns a set of HTTP methods that all requests that this
Pattern matches must be in, or nil if it's not possible to determine
which HTTP methods might be matched.

This function satisfies goji's HTTPMethods Pattern optimization.
*/
func (p *Pattern) HTTPMethods() map[string]struct{} {
	return p.methods
}

/*
String returns the pattern string that was used to create this Pattern.
*/
func (p *Pattern) String() string {
	return p.raw
}

/*
Param returns the bound parameter with the given name. For instance, given the
route:

	/user/:name

and the URL Path:

	/user/carl

a call to Param(r, "name") would return the string "carl". It is the caller's
responsibility to ensure that the variable has been bound. Attempts to access
variables that have not been set (or which have been invalidly set) are
considered programmer errors and will trigger a panic.
*/
func Param(r *http.Request, name string) string {
	return r.Context().Value(pattern.Variable(name)).(string)
}
