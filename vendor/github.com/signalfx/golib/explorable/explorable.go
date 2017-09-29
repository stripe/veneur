package explorable

import (
	"fmt"
	"github.com/signalfx/golib/log"
	"github.com/signalfx/golib/logkey"
	"html"
	"net/http"
	"reflect"
	"runtime"
	"strconv"
	"strings"
)

// DefaultLogger is used by explorable if a handler hasn't set a logger
var DefaultLogger = log.Logger(log.DefaultLogger.CreateChild())

// Result is the crude explorable representation of an object returned by ExploreObject
type Result struct {
	Result   interface{}
	Children []string
	Desc     string
}

// Handler allows you to serve an exportable object for debugging over HTTP
type Handler struct {
	Val      interface{}
	BasePath string
	Logger   log.Logger
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	logger := h.Logger
	if logger == nil {
		logger = DefaultLogger
	}
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, h.BasePath), "/")
	nonEmptyParts := []string{}
	for _, p := range pathParts {
		if p != "" {
			nonEmptyParts = append(nonEmptyParts, p)
		}
	}
	logger.Log(logkey.ExplorableParts, fmt.Sprintf("%v", nonEmptyParts), logkey.URL, r.URL, "Exploring object")
	o := ExploreObject(reflect.ValueOf(h.Val), nonEmptyParts)

	parent := ""
	if len(nonEmptyParts) > 0 {
		parent = fmt.Sprintf(`
		<h1>
			<a href="%s">Parent</a>
		</h1>
		`, h.BasePath+strings.TrimPrefix("/"+strings.Join(nonEmptyParts[0:len(nonEmptyParts)-1], "/"), "/"))
	}

	childTable := ""
	if len(o.Children) > 0 {
		childTable += "<table>\n"
		for _, c := range o.Children {
			link := h.BasePath + strings.TrimPrefix("/"+strings.Join(append(nonEmptyParts, c), "/"), "/")
			childTable += fmt.Sprintf(`
<tr>
	<td><a href="%s">%s</a></td>
</tr>
`, link, html.EscapeString(c))
		}
		childTable += "</table>\n"
	}
	s :=
		fmt.Sprintf(`
<!DOCTYPE html>
<html>
	<head>
		<title>Explorable object</title>
	</head>
	<body>
		<h1>
			%s
		</h1>
		%s
		%s
	</body>
</html>`, html.EscapeString(o.Desc), parent, childTable)

	_, err := rw.Write([]byte(s))
	log.IfErr(h.Logger, err)
}

func checkConsts(t reflect.Value) *Result {
	ret := &Result{}
	if !t.IsValid() {
		ret.Desc = "<INVALID>"
		return ret
	}
	kind := t.Kind()
	if kind >= reflect.Int && kind <= reflect.Int64 {
		ret.Desc = strconv.FormatInt(t.Int(), 10)
		return ret
	}
	if kind >= reflect.Uint && kind <= reflect.Uint64 {
		ret.Desc = strconv.FormatUint(t.Uint(), 10)
		return ret
	}
	if kind >= reflect.Float32 && kind <= reflect.Float64 {
		ret.Desc = strconv.FormatFloat(t.Float(), byte('f'), 10, 64)
		return ret
	}
	return nil
}

func exploreArray(t reflect.Value, path []string) *Result {
	ret := &Result{}
	if len(path) == 0 {
		ret.Desc = fmt.Sprintf("array-len(%d of %d)", t.Len(), t.Cap())
		ret.Children = make([]string, t.Len())
		for i := 0; i < t.Len(); i++ {
			ret.Children[i] = strconv.FormatInt(int64(i), 10)
		}
		return ret
	}
	index, err := strconv.ParseInt(path[0], 10, 64)
	if err != nil {
		ret.Desc = err.Error()
		return ret
	}
	// TODO: Catch panics here
	return ExploreObject(t.Index(int(index)), path[1:])
}

func exploreFunc(t reflect.Value, path []string) *Result {
	ret := &Result{}
	if t.IsNil() {
		ret.Desc = "<NIL function>"
		return ret
	}
	f := runtime.FuncForPC(t.Pointer())
	if f == nil {
		ret.Desc = "<UNKNOWN FUNCTION>"
		return ret
	}
	ret.Desc = f.Name()
	return ret
}

func exploreSlice(t reflect.Value, path []string) *Result {
	ret := &Result{}
	if t.IsNil() {
		ret.Desc = "<NIL>"
		return ret
	}
	if len(path) == 0 {
		ret.Desc = fmt.Sprintf("slice-len(%d of %d)", t.Len(), t.Cap())
		ret.Children = make([]string, t.Len())
		for i := 0; i < t.Len(); i++ {
			ret.Children[i] = strconv.FormatInt(int64(i), 10)
		}
		return ret
	}
	index, err := strconv.ParseInt(path[0], 10, 64)
	if err != nil {
		ret.Desc = err.Error()
		return ret
	}
	// TODO: Catch panics here
	return ExploreObject(t.Index(int(index)), path[1:])
}

func exploreMap(t reflect.Value, path []string) *Result {
	ret := &Result{}
	if t.IsNil() {
		ret.Desc = "<NIL>"
		return ret
	}
	if len(path) == 0 {
		ret.Desc = fmt.Sprintf("map-len(%d)", t.Len())
		keys := t.MapKeys()
		ret.Children = make([]string, len(keys))
		for i, k := range keys {
			// TODO: Better index?
			ret.Children[i] = keyMapString(k)
		}
		return ret
	}
	mkey := keyMapType(t.Type().Key().Kind(), path[0])
	if !mkey.IsValid() {
		ret.Desc = fmt.Sprintf("<INVALID MAP KEY %s>", path[0])
		return ret
	}

	v := t.MapIndex(mkey)
	if !v.IsValid() {
		ret.Desc = fmt.Sprintf("<NOT FOUND MAP KEY %s>", path[0])
		return ret
	}
	return ExploreObject(v, path[1:])
}

func exploreStruct(t reflect.Value, path []string) *Result {
	ret := &Result{}
	if len(path) == 0 {
		ret.Desc = t.Type().Name()
		ret.Children = make([]string, t.Type().NumField())
		for i := 0; i < t.Type().NumField(); i++ {
			ret.Children[i] = t.Type().Field(i).Name
		}
		return ret
	}
	val := t.FieldByName(path[0])
	if !val.IsValid() {
		ret.Desc = fmt.Sprintf("<Invalid path %s>", path[0])
		return ret
	}
	return ExploreObject(val, path[1:])
}

func exploreChan(t reflect.Value, path []string) *Result {
	ret := &Result{}
	if t.IsNil() {
		ret.Desc = "<NIL>"
		return ret
	}
	ret.Desc = fmt.Sprintf("chan-len(%d of %d)", t.Len(), t.Cap())
	return ret
}

func explorePtr(t reflect.Value, path []string) *Result {
	ret := &Result{}
	if t.IsNil() {
		ret.Desc = "<NIL>"
		return ret
	}
	return ExploreObject(t.Elem(), path)
}

func exploreInterface(t reflect.Value, path []string) *Result {
	ret := &Result{}
	if t.IsNil() {
		ret.Desc = "<NIL>"
		return ret
	}
	return ExploreObject(t.Elem(), path)
}

// ExploreObject is a crude public way to explore an object's values via reflection
func ExploreObject(t reflect.Value, path []string) *Result {
	if ret := checkConsts(t); ret != nil {
		return ret
	}
	ret := &Result{}
	switch t.Kind() {
	case reflect.Bool:
		ret.Desc = fmt.Sprintf("%t", t.Bool())
		return ret
	case reflect.String:
		ret.Desc = t.String()
		return ret
	}
	c := map[reflect.Kind](func(reflect.Value, []string) *Result){
		reflect.Array:     exploreArray,
		reflect.Func:      exploreFunc,
		reflect.Slice:     exploreSlice,
		reflect.Map:       exploreMap,
		reflect.Struct:    exploreStruct,
		reflect.Chan:      exploreChan,
		reflect.Ptr:       explorePtr,
		reflect.Interface: exploreInterface,
	}
	callback, exists := c[t.Kind()]
	if exists {
		return callback(t, path)
	}
	ret.Desc = "<Unsupported>"
	return ret
}

func stringToIntType(path string) reflect.Value {
	i, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		return reflect.Value{}
	}
	return reflect.ValueOf(int(i))
}

func stringToInt8Type(path string) reflect.Value {
	i, err := strconv.ParseInt(path, 10, 8)
	if err != nil {
		return reflect.Value{}
	}
	return reflect.ValueOf(int8(i))
}

func stringToInt16Type(path string) reflect.Value {
	i, err := strconv.ParseInt(path, 10, 16)
	if err != nil {
		return reflect.Value{}
	}
	return reflect.ValueOf(int16(i))
}

func stringToInt32Type(path string) reflect.Value {
	i, err := strconv.ParseInt(path, 10, 32)
	if err != nil {
		return reflect.Value{}
	}
	return reflect.ValueOf(int32(i))
}

func stringToInt64Type(path string) reflect.Value {
	i, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		return reflect.Value{}
	}
	return reflect.ValueOf(i)
}

func stringToUIntType(path string) reflect.Value {
	i, err := strconv.ParseUint(path, 10, 64)
	if err != nil {
		return reflect.Value{}
	}
	return reflect.ValueOf(uint(i))
}

func stringToUInt8Type(path string) reflect.Value {
	i, err := strconv.ParseUint(path, 10, 8)
	if err != nil {
		return reflect.Value{}
	}
	return reflect.ValueOf(uint8(i))
}

func stringToUInt16Type(path string) reflect.Value {
	i, err := strconv.ParseUint(path, 10, 16)
	if err != nil {
		return reflect.Value{}
	}
	return reflect.ValueOf(uint16(i))
}

func stringToUInt32Type(path string) reflect.Value {
	i, err := strconv.ParseUint(path, 10, 32)
	if err != nil {
		return reflect.Value{}
	}
	return reflect.ValueOf(uint32(i))
}

func stringToUInt64Type(path string) reflect.Value {
	i, err := strconv.ParseUint(path, 10, 64)
	if err != nil {
		return reflect.Value{}
	}
	return reflect.ValueOf(i)
}

func stringToFloat32Type(path string) reflect.Value {
	i, err := strconv.ParseFloat(path, 32)
	if err != nil {
		return reflect.Value{}
	}
	return reflect.ValueOf(float32(i))
}

func stringToFloat64Type(path string) reflect.Value {
	i, err := strconv.ParseFloat(path, 64)
	if err != nil {
		return reflect.Value{}
	}
	return reflect.ValueOf(i)
}

func stringToStringType(path string) reflect.Value {
	return reflect.ValueOf(path)
}

func keyMapType(mapKeyKind reflect.Kind, path string) reflect.Value {
	m := map[reflect.Kind](func(string) reflect.Value){
		reflect.Int:     stringToIntType,
		reflect.Int8:    stringToInt8Type,
		reflect.Int16:   stringToInt16Type,
		reflect.Int32:   stringToInt32Type,
		reflect.Int64:   stringToInt64Type,
		reflect.Uint:    stringToUIntType,
		reflect.Uint8:   stringToUInt8Type,
		reflect.Uint16:  stringToUInt16Type,
		reflect.Uint32:  stringToUInt32Type,
		reflect.Uint64:  stringToUInt64Type,
		reflect.Float32: stringToFloat32Type,
		reflect.Float64: stringToFloat64Type,
		reflect.String:  stringToStringType,
	}
	f, e := m[mapKeyKind]
	if e {
		return f(path)
	}
	return reflect.Value{}
}

func keyMapString(t reflect.Value) string {
	o := ExploreObject(t, []string{})
	return o.Desc
}
