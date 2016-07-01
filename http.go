package veneur

import (
	"net/http"

	"goji.io"
	"goji.io/pat"
	"golang.org/x/net/context"
)

func (s *Server) Handler() http.Handler {
	mux := goji.NewMux()
	mux.HandleFuncC(pat.Get("/healthcheck"), func(c context.Context, w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok\n"))
	})
	return mux
}
