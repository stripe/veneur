package pat

import (
	"context"
	"sort"

	"goji.io/internal"
	"goji.io/pattern"
)

type match struct {
	context.Context
	pat     *Pattern
	matches []string
}

func (m match) Value(key interface{}) interface{} {
	switch key {
	case pattern.AllVariables:
		var vs map[pattern.Variable]interface{}
		if vsi := m.Context.Value(key); vsi == nil {
			if len(m.pat.pats) == 0 {
				return nil
			}
			vs = make(map[pattern.Variable]interface{}, len(m.matches))
		} else {
			vs = vsi.(map[pattern.Variable]interface{})
		}

		for _, p := range m.pat.pats {
			vs[p.name] = m.matches[p.idx]
		}
		return vs
	case internal.Path:
		if len(m.matches) == len(m.pat.pats)+1 {
			return m.matches[len(m.matches)-1]
		}
		return ""
	}

	if k, ok := key.(pattern.Variable); ok {
		i := sort.Search(len(m.pat.pats), func(i int) bool {
			return m.pat.pats[i].name >= k
		})
		if i < len(m.pat.pats) && m.pat.pats[i].name == k {
			return m.matches[m.pat.pats[i].idx]
		}
	}

	return m.Context.Value(key)
}
