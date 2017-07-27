package pat

import (
	"net/http"
	"reflect"
	"testing"

	"goji.io/pattern"
	"golang.org/x/net/context"
)

func TestExistingContext(t *testing.T) {
	t.Parallel()

	pat := New("/hi/:c/:a/:r/:l")
	req, err := http.NewRequest("GET", "/hi/foo/bar/baz/quux", nil)
	if err != nil {
		panic(err)
	}
	ctx := context.Background()
	ctx = pattern.SetPath(ctx, req.URL.EscapedPath())
	ctx = context.WithValue(ctx, pattern.AllVariables, map[pattern.Variable]interface{}{
		"hello": "world",
		"c":     "nope",
	})
	ctx = context.WithValue(ctx, "user", "carl")

	ctx = pat.Match(ctx, req)
	if ctx == nil {
		t.Fatalf("expected pattern to match")
	}

	expected := map[pattern.Variable]interface{}{
		"c": "foo",
		"a": "bar",
		"r": "baz",
		"l": "quux",
	}
	for k, v := range expected {
		if p := Param(ctx, string(k)); p != v {
			t.Errorf("expected %s=%q, got %q", k, v, p)
		}
	}

	expected["hello"] = "world"
	all := ctx.Value(pattern.AllVariables).(map[pattern.Variable]interface{})
	if !reflect.DeepEqual(all, expected) {
		t.Errorf("expected %v, got %v", expected, all)
	}

	if path := pattern.Path(ctx); path != "" {
		t.Errorf("expected path=%q, got %q", "", path)
	}

	if user := ctx.Value("user"); user != "carl" {
		t.Errorf("expected user=%q, got %q", "carl", user)
	}
}
