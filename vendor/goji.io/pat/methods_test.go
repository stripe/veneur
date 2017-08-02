package pat

import "testing"

func TestDelete(t *testing.T) {
	t.Parallel()
	pat := Delete("/")
	if pat.Match(mustReq("GET", "/")) != nil {
		t.Errorf("pattern was DELETE, but matched GET")
	}
	if pat.Match(mustReq("DELETE", "/")) == nil {
		t.Errorf("pattern didn't match DELETE")
	}
}

func TestGet(t *testing.T) {
	t.Parallel()
	pat := Get("/")
	if pat.Match(mustReq("POST", "/")) != nil {
		t.Errorf("pattern was GET, but matched POST")
	}
	if pat.Match(mustReq("GET", "/")) == nil {
		t.Errorf("pattern didn't match GET")
	}
	if pat.Match(mustReq("HEAD", "/")) == nil {
		t.Errorf("pattern didn't match HEAD")
	}
}

func TestHead(t *testing.T) {
	t.Parallel()
	pat := Head("/")
	if pat.Match(mustReq("GET", "/")) != nil {
		t.Errorf("pattern was HEAD, but matched GET")
	}
	if pat.Match(mustReq("HEAD", "/")) == nil {
		t.Errorf("pattern didn't match HEAD")
	}
}

func TestOptions(t *testing.T) {
	t.Parallel()
	pat := Options("/")
	if pat.Match(mustReq("GET", "/")) != nil {
		t.Errorf("pattern was OPTIONS, but matched GET")
	}
	if pat.Match(mustReq("OPTIONS", "/")) == nil {
		t.Errorf("pattern didn't match OPTIONS")
	}
}

func TestPatch(t *testing.T) {
	t.Parallel()
	pat := Patch("/")
	if pat.Match(mustReq("GET", "/")) != nil {
		t.Errorf("pattern was PATCH, but matched GET")
	}
	if pat.Match(mustReq("PATCH", "/")) == nil {
		t.Errorf("pattern didn't match PATCH")
	}
}

func TestPost(t *testing.T) {
	t.Parallel()
	pat := Post("/")
	if pat.Match(mustReq("GET", "/")) != nil {
		t.Errorf("pattern was POST, but matched GET")
	}
	if pat.Match(mustReq("POST", "/")) == nil {
		t.Errorf("pattern didn't match POST")
	}
}

func TestPut(t *testing.T) {
	t.Parallel()
	pat := Put("/")
	if pat.Match(mustReq("GET", "/")) != nil {
		t.Errorf("pattern was PUT, but matched GET")
	}
	if pat.Match(mustReq("PUT", "/")) == nil {
		t.Errorf("pattern didn't match PUT")
	}
}
