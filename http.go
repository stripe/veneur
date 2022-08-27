package veneur

import (
	"net/http"
	"net/http/pprof"

	"github.com/stripe/veneur/v14/util/build"
	"github.com/stripe/veneur/v14/util/config"

	"goji.io"
	"goji.io/pat"
)

// Handler returns the Handler responsible for routing request processing.
func (s *Server) Handler() http.Handler {
	mux := goji.NewMux()

	mux.HandleFunc(pat.Get("/healthcheck"), func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok\n"))
	})

	mux.HandleFunc(pat.Get("/builddate"), func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(build.BUILD_DATE))
	})

	mux.HandleFunc(pat.Get("/version"), func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(build.VERSION))
	})

	if s.Config.HTTP.Config {
		mux.HandleFunc(pat.Get("/config/json"), config.HandleConfigJson(s.Config))
		mux.HandleFunc(pat.Get("/config/yaml"), config.HandleConfigYaml(s.Config))
	}

	if s.httpQuit {
		mux.HandleFunc(pat.Post(httpQuitEndpoint), func(w http.ResponseWriter, r *http.Request) {
			s.logger.WithField("endpoint", httpQuitEndpoint).
				Info("Received shutdown request on HTTP quit endpoint")
			w.Write([]byte("Beginning graceful shutdown....\n"))
			s.Shutdown()
		})
	}

	// TODO3.0: Maybe remove this endpoint as it is kinda useless now that tracing is always on.
	mux.HandleFunc(pat.Get("/healthcheck/tracing"), func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok\n"))
	})

	for endpoint, customHandler := range s.HttpCustomHandlers {
		mux.HandleFunc(pat.Get(endpoint), customHandler)
	}

	mux.Handle(pat.Get("/debug/pprof/cmdline"), http.HandlerFunc(pprof.Cmdline))
	mux.Handle(pat.Get("/debug/pprof/profile"), http.HandlerFunc(pprof.Profile))
	mux.Handle(pat.Get("/debug/pprof/symbol"), http.HandlerFunc(pprof.Symbol))
	mux.Handle(pat.Get("/debug/pprof/trace"), http.HandlerFunc(pprof.Trace))
	mux.Handle(pat.Get("/debug/pprof/"), http.HandlerFunc(pprof.Index))

	return mux
}
