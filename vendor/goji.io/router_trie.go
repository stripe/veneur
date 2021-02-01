// +build !goji_router_simple

package goji

import (
	"net/http"
	"sort"
	"strings"

	"goji.io/internal"
)

type router struct {
	routes   []route
	methods  map[string]*trieNode
	wildcard trieNode
}

type route struct {
	Pattern
	http.Handler
}

type child struct {
	prefix string
	node   *trieNode
}

type trieNode struct {
	routes   []int
	children []child
}

func (rt *router) add(p Pattern, h http.Handler) {
	i := len(rt.routes)
	rt.routes = append(rt.routes, route{p, h})

	var prefix string
	if pp, ok := p.(pathPrefix); ok {
		prefix = pp.PathPrefix()
	}

	var methods map[string]struct{}
	if hm, ok := p.(httpMethods); ok {
		methods = hm.HTTPMethods()
	}
	if methods == nil {
		rt.wildcard.add(prefix, i)
		for _, sub := range rt.methods {
			sub.add(prefix, i)
		}
	} else {
		if rt.methods == nil {
			rt.methods = make(map[string]*trieNode)
		}

		for method := range methods {
			if _, ok := rt.methods[method]; !ok {
				rt.methods[method] = rt.wildcard.clone()
			}
			rt.methods[method].add(prefix, i)
		}
	}
}

func (rt *router) route(r *http.Request) *http.Request {
	tn := &rt.wildcard
	if tn2, ok := rt.methods[r.Method]; ok {
		tn = tn2
	}

	ctx := r.Context()
	path := ctx.Value(internal.Path).(string)
	for path != "" {
		i := sort.Search(len(tn.children), func(i int) bool {
			return path[0] <= tn.children[i].prefix[0]
		})
		if i == len(tn.children) || !strings.HasPrefix(path, tn.children[i].prefix) {
			break
		}

		path = path[len(tn.children[i].prefix):]
		tn = tn.children[i].node
	}
	for _, i := range tn.routes {
		if r2 := rt.routes[i].Match(r); r2 != nil {
			return r2.WithContext(&match{
				Context: r2.Context(),
				p:       rt.routes[i].Pattern,
				h:       rt.routes[i].Handler,
			})
		}
	}
	return r.WithContext(&match{Context: ctx})
}

// We can be a teensy bit more efficient here: we're maintaining a sorted list,
// so we know exactly where to insert the new element. But since that involves
// more bookkeeping and makes the code messier, let's cross that bridge when we
// come to it.
type byPrefix []child

func (b byPrefix) Len() int {
	return len(b)
}
func (b byPrefix) Less(i, j int) bool {
	return b[i].prefix < b[j].prefix
}
func (b byPrefix) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}

func longestPrefix(a, b string) string {
	mlen := len(a)
	if len(b) < mlen {
		mlen = len(b)
	}
	for i := 0; i < mlen; i++ {
		if a[i] != b[i] {
			return a[:i]
		}
	}
	return a[:mlen]
}

func (tn *trieNode) add(prefix string, idx int) {
	if len(prefix) == 0 {
		tn.routes = append(tn.routes, idx)
		for i := range tn.children {
			tn.children[i].node.add(prefix, idx)
		}
		return
	}

	ch := prefix[0]
	i := sort.Search(len(tn.children), func(i int) bool {
		return ch <= tn.children[i].prefix[0]
	})

	if i == len(tn.children) || ch != tn.children[i].prefix[0] {
		routes := append([]int(nil), tn.routes...)
		tn.children = append(tn.children, child{
			prefix: prefix,
			node:   &trieNode{routes: append(routes, idx)},
		})
	} else {
		lp := longestPrefix(prefix, tn.children[i].prefix)

		if tn.children[i].prefix == lp {
			tn.children[i].node.add(prefix[len(lp):], idx)
			return
		}

		split := new(trieNode)
		split.children = []child{
			{tn.children[i].prefix[len(lp):], tn.children[i].node},
		}
		split.routes = append([]int(nil), tn.routes...)
		split.add(prefix[len(lp):], idx)

		tn.children[i].prefix = lp
		tn.children[i].node = split
	}

	sort.Sort(byPrefix(tn.children))
}

func (tn *trieNode) clone() *trieNode {
	clone := new(trieNode)
	clone.routes = append(clone.routes, tn.routes...)
	clone.children = append(clone.children, tn.children...)
	for i := range clone.children {
		clone.children[i].node = tn.children[i].node.clone()
	}
	return clone
}
