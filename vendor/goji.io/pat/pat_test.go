package pat

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"goji.io/pattern"
)

func mustReq(method, path string) *http.Request {
	req, err := http.NewRequest(method, path, nil)
	if err != nil {
		panic(err)
	}
	ctx := pattern.SetPath(context.Background(), req.URL.EscapedPath())
	return req.WithContext(ctx)
}

type PatTest struct {
	pat   string
	req   string
	match bool
	vars  map[pattern.Variable]interface{}
	path  string
}

type pv map[pattern.Variable]interface{}

var PatTests = []PatTest{
	{"/", "/", true, nil, ""},
	{"/", "/hello", false, nil, ""},
	{"/hello", "/hello", true, nil, ""},

	{"/:name", "/carl", true, pv{"name": "carl"}, ""},
	{"/:name", "/carl/", false, nil, ""},
	{"/:name", "/", false, nil, ""},
	{"/:name/", "/carl/", true, pv{"name": "carl"}, ""},
	{"/:name/", "/carl/no", false, nil, ""},
	{"/:name/hi", "/carl/hi", true, pv{"name": "carl"}, ""},
	{"/:name/:color", "/carl/red", true, pv{"name": "carl", "color": "red"}, ""},
	{"/:name/:color", "/carl/", false, nil, ""},
	{"/:name/:color", "/carl.red", false, nil, ""},

	{"/:file.:ext", "/data.json", true, pv{"file": "data", "ext": "json"}, ""},
	{"/:file.:ext", "/data.tar.gz", true, pv{"file": "data", "ext": "tar.gz"}, ""},
	{"/:file.:ext", "/data", false, nil, ""},
	{"/:file.:ext", "/data.", false, nil, ""},
	{"/:file.:ext", "/.gitconfig", false, nil, ""},
	{"/:file.:ext", "/data.json/", false, nil, ""},
	{"/:file.:ext", "/data/json", false, nil, ""},
	{"/:file.:ext", "/data;json", false, nil, ""},
	{"/hello.:ext", "/hello.json", true, pv{"ext": "json"}, ""},
	{"/:file.json", "/hello.json", true, pv{"file": "hello"}, ""},
	{"/:file.json", "/hello.world.json", false, nil, ""},
	{"/file;:version", "/file;1", true, pv{"version": "1"}, ""},
	{"/file;:version", "/file,1", false, nil, ""},
	{"/file,:version", "/file,1", true, pv{"version": "1"}, ""},
	{"/file,:version", "/file;1", false, nil, ""},

	{"/*", "/", true, nil, "/"},
	{"/*", "/hello", true, nil, "/hello"},
	{"/users/*", "/", false, nil, ""},
	{"/users/*", "/users", false, nil, ""},
	{"/users/*", "/users/", true, nil, "/"},
	{"/users/*", "/users/carl", true, nil, "/carl"},
	{"/users/*", "/profile/carl", false, nil, ""},
	{"/:name/*", "/carl", false, nil, ""},
	{"/:name/*", "/carl/", true, pv{"name": "carl"}, "/"},
	{"/:name/*", "/carl/photos", true, pv{"name": "carl"}, "/photos"},
	{"/:name/*", "/carl/photos%2f2015", true, pv{"name": "carl"}, "/photos%2f2015"},
}

func TestPat(t *testing.T) {
	t.Parallel()

	for _, test := range PatTests {
		pat := New(test.pat)

		if str := pat.String(); str != test.pat {
			t.Errorf("[%q %q] String()=%q, expected=%q", test.pat, test.req, str, test.pat)
		}

		req := pat.Match(mustReq("GET", test.req))
		if (req != nil) != test.match {
			t.Errorf("[%q %q] match=%v, expected=%v", test.pat, test.req, req != nil, test.match)
		}
		if req == nil {
			continue
		}

		ctx := req.Context()
		if path := pattern.Path(ctx); path != test.path {
			t.Errorf("[%q %q] path=%q, expected=%q", test.pat, test.req, path, test.path)
		}

		vars := ctx.Value(pattern.AllVariables)
		if (vars != nil) != (test.vars != nil) {
			t.Errorf("[%q %q] vars=%#v, expected=%#v", test.pat, test.req, vars, test.vars)
		}
		if vars == nil {
			continue
		}
		if tvars := vars.(map[pattern.Variable]interface{}); !reflect.DeepEqual(tvars, test.vars) {
			t.Errorf("[%q %q] vars=%v, expected=%v", test.pat, test.req, tvars, test.vars)
		}
	}
}

func TestBadPathEncoding(t *testing.T) {
	t.Parallel()

	// This one is hard to fit into the table-driven test above since Go
	// refuses to have anything to do with poorly encoded URLs.
	ctx := pattern.SetPath(context.Background(), "/%nope")
	r, _ := http.NewRequest("GET", "/", nil)
	if New("/:name").Match(r.WithContext(ctx)) != nil {
		t.Error("unexpected match")
	}
}

var PathPrefixTests = []struct {
	pat    string
	prefix string
}{
	{"/", "/"},
	{"/hello/:world", "/hello/"},
	{"/users/:name/profile", "/users/"},
	{"/users/*", "/users/"},
}

func TestPathPrefix(t *testing.T) {
	t.Parallel()

	for _, test := range PathPrefixTests {
		pat := New(test.pat)
		if prefix := pat.PathPrefix(); prefix != test.prefix {
			t.Errorf("%q.PathPrefix() = %q, expected %q", test.pat, prefix, test.prefix)
		}
	}
}

func TestHTTPMethods(t *testing.T) {
	t.Parallel()

	pat := New("/foo")
	if methods := pat.HTTPMethods(); methods != nil {
		t.Errorf("expected nil with no methods, got %v", methods)
	}

	pat = Get("/boo")
	expect := map[string]struct{}{"GET": {}, "HEAD": {}}
	if methods := pat.HTTPMethods(); !reflect.DeepEqual(expect, methods) {
		t.Errorf("methods=%v, expected %v", methods, expect)
	}
}

func TestParam(t *testing.T) {
	t.Parallel()

	pat := New("/hello/:name")
	req := pat.Match(mustReq("GET", "/hello/carl"))
	if req == nil {
		t.Fatal("expected a match")
	}
	if name := Param(req, "name"); name != "carl" {
		t.Errorf("name=%q, expected %q", name, "carl")
	}
}
