package pat

/*
Delete returns a Pat route that only matches the DELETE HTTP method.
*/
func Delete(pat string) *Pattern {
	return newWithMethods(pat, "DELETE")
}

/*
Get returns a Pat route that only matches the GET and HEAD HTTP method. HEAD
requests are handled transparently by net/http.
*/
func Get(pat string) *Pattern {
	return newWithMethods(pat, "GET", "HEAD")
}

/*
Head returns a Pat route that only matches the HEAD HTTP method.
*/
func Head(pat string) *Pattern {
	return newWithMethods(pat, "HEAD")
}

/*
Options returns a Pat route that only matches the OPTIONS HTTP method.
*/
func Options(pat string) *Pattern {
	return newWithMethods(pat, "OPTIONS")
}

/*
Patch returns a Pat route that only matches the PATCH HTTP method.
*/
func Patch(pat string) *Pattern {
	return newWithMethods(pat, "PATCH")
}

/*
Post returns a Pat route that only matches the POST HTTP method.
*/
func Post(pat string) *Pattern {
	return newWithMethods(pat, "POST")
}

/*
Put returns a Pat route that only matches the PUT HTTP method.
*/
func Put(pat string) *Pattern {
	return newWithMethods(pat, "PUT")
}
