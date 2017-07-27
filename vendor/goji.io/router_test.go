package goji

import (
	"net/http"
	"reflect"
	"testing"

	"goji.io/internal"
	"golang.org/x/net/context"
)

func TestNoMatch(t *testing.T) {
	t.Parallel()

	var rt router
	rt.add(boolPattern(false), HandlerFunc(func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
		t.Fatal("did not expect handler to be called")
	}))
	_, r := wr()
	ctx := context.Background()
	ctx = context.WithValue(ctx, internal.Pattern, boolPattern(true))
	ctx = context.WithValue(ctx, internal.Pattern, boolPattern(true))
	ctx = context.WithValue(ctx, "answer", 42)

	ctx = context.WithValue(ctx, internal.Path, "/")
	ctx = rt.route(ctx, r)

	if p := ctx.Value(internal.Pattern); p != nil {
		t.Errorf("unexpected pattern %v", p)
	}
	if h := ctx.Value(internal.Handler); h != nil {
		t.Errorf("unexpected handler %v", h)
	}
	if h := ctx.Value("answer"); h != 42 {
		t.Errorf("context didn't work: got %v, wanted %v", h, 42)
	}
}

/*
These are meant to be end-to-end torture tests of Goji's routing semantics. We
generate a list of patterns that can be turned off incrementally with a global
"high water mark." We then run a sequence of requests through the router N
times, incrementing the mark each time. The net effect is that we can compile
the entire set of routes Goji would attempt for every request, ensuring that the
router is picking routes in the correct order.
*/

var TestRoutes = []testPattern{
	testPattern{methods: nil, prefix: "/"},
	testPattern{methods: nil, prefix: "/a"},
	testPattern{methods: []string{"POST", "PUT"}, prefix: "/a"},
	testPattern{methods: []string{"GET", "POST"}, prefix: "/a"},
	testPattern{methods: []string{"GET"}, prefix: "/b"},
	testPattern{methods: nil, prefix: "/ab"},
	testPattern{methods: []string{"POST", "PUT"}, prefix: "/"},
	testPattern{methods: nil, prefix: "/ba"},
	testPattern{methods: nil, prefix: "/"},
	testPattern{methods: []string{}, prefix: "/"},
	testPattern{methods: nil, prefix: "/carl"},
	testPattern{methods: []string{"PUT"}, prefix: "/car"},
	testPattern{methods: nil, prefix: "/cake"},
	testPattern{methods: nil, prefix: "/car"},
	testPattern{methods: []string{"GET"}, prefix: "/c"},
	testPattern{methods: []string{"POST"}, prefix: "/"},
	testPattern{methods: []string{"PUT"}, prefix: "/"},
}

var RouterTests = []struct {
	method, path string
	results      []int
}{
	{"GET", "/", []int{0, 8, 8, 8, 8, 8, 8, 8, 8, -1, -1, -1, -1, -1, -1, -1, -1}},
	{"POST", "/", []int{0, 6, 6, 6, 6, 6, 6, 8, 8, 15, 15, 15, 15, 15, 15, 15, -1}},
	{"PUT", "/", []int{0, 6, 6, 6, 6, 6, 6, 8, 8, 16, 16, 16, 16, 16, 16, 16, 16}},
	{"HEAD", "/", []int{0, 8, 8, 8, 8, 8, 8, 8, 8, -1, -1, -1, -1, -1, -1, -1, -1}},
	{"GET", "/a", []int{0, 1, 3, 3, 8, 8, 8, 8, 8, -1, -1, -1, -1, -1, -1, -1, -1}},
	{"POST", "/a", []int{0, 1, 2, 3, 6, 6, 6, 8, 8, 15, 15, 15, 15, 15, 15, 15, -1}},
	{"PUT", "/a", []int{0, 1, 2, 6, 6, 6, 6, 8, 8, 16, 16, 16, 16, 16, 16, 16, 16}},
	{"HEAD", "/a", []int{0, 1, 8, 8, 8, 8, 8, 8, 8, -1, -1, -1, -1, -1, -1, -1, -1}},
	{"GET", "/b", []int{0, 4, 4, 4, 4, 8, 8, 8, 8, -1, -1, -1, -1, -1, -1, -1, -1}},
	{"POST", "/b", []int{0, 6, 6, 6, 6, 6, 6, 8, 8, 15, 15, 15, 15, 15, 15, 15, -1}},
	{"GET", "/ba", []int{0, 4, 4, 4, 4, 7, 7, 7, 8, -1, -1, -1, -1, -1, -1, -1, -1}},
	{"GET", "/c", []int{0, 8, 8, 8, 8, 8, 8, 8, 8, 14, 14, 14, 14, 14, 14, -1, -1}},
	{"POST", "/c", []int{0, 6, 6, 6, 6, 6, 6, 8, 8, 15, 15, 15, 15, 15, 15, 15, -1}},
	{"GET", "/ab", []int{0, 1, 3, 3, 5, 5, 8, 8, 8, -1, -1, -1, -1, -1, -1, -1, -1}},
	{"POST", "/ab", []int{0, 1, 2, 3, 5, 5, 6, 8, 8, 15, 15, 15, 15, 15, 15, 15, -1}},
	{"GET", "/carl", []int{0, 8, 8, 8, 8, 8, 8, 8, 8, 10, 10, 13, 13, 13, 14, -1, -1}},
	{"POST", "/carl", []int{0, 6, 6, 6, 6, 6, 6, 8, 8, 10, 10, 13, 13, 13, 15, 15, -1}},
	{"HEAD", "/carl", []int{0, 8, 8, 8, 8, 8, 8, 8, 8, 10, 10, 13, 13, 13, -1, -1, -1}},
	{"PUT", "/carl", []int{0, 6, 6, 6, 6, 6, 6, 8, 8, 10, 10, 11, 13, 13, 16, 16, 16}},
	{"GET", "/cake", []int{0, 8, 8, 8, 8, 8, 8, 8, 8, 12, 12, 12, 12, 14, 14, -1, -1}},
	{"PUT", "/cake", []int{0, 6, 6, 6, 6, 6, 6, 8, 8, 12, 12, 12, 12, 16, 16, 16, 16}},
	{"OHAI", "/carl", []int{0, 8, 8, 8, 8, 8, 8, 8, 8, 10, 10, 13, 13, 13, -1, -1, -1}},
}

func TestRouter(t *testing.T) {
	t.Parallel()

	var rt router
	mark := new(int)
	for i, p := range TestRoutes {
		i := i
		p.index = i
		p.mark = mark
		rt.add(p, intHandler(i))
	}

	for i, test := range RouterTests {
		r, err := http.NewRequest(test.method, test.path, nil)
		if err != nil {
			panic(err)
		}
		ctx := context.WithValue(context.Background(), internal.Path, test.path)

		var out []int
		for *mark = 0; *mark < len(TestRoutes); *mark++ {
			ctx := rt.route(ctx, r)
			if h := ctx.Value(internal.Handler); h != nil {
				out = append(out, int(h.(intHandler)))
			} else {
				out = append(out, -1)
			}
		}
		if !reflect.DeepEqual(out, test.results) {
			t.Errorf("[%d] expected %v got %v", i, test.results, out)
		}
	}
}
