package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"time"

	"github.com/redhatinsights/module-update-router/identity"

	log "github.com/sirupsen/logrus"
	"github.com/slok/go-http-metrics/metrics"
	httpmetrics "github.com/slok/go-http-metrics/metrics/prometheus"
	"github.com/slok/go-http-metrics/middleware"

	request "github.com/redhatinsights/platform-go-middlewares/request_id"
)

var r metrics.Recorder = httpmetrics.NewRecorder(httpmetrics.Config{})

// Server is the application's HTTP server. It is comprised of an HTTP
// multiplexer for routing HTTP requests to appropriate handlers and a database
// handle for looking up application data.
type Server struct {
	mux    *http.ServeMux
	db     *DB
	addr   string
	events *chan []byte
}

// NewServer creates a new instance of the application, configured with the
// provided addr, API roots and database handle.
func NewServer(addr string, apiroots []string, db *DB, events *chan []byte) (*Server, error) {
	srv := &Server{
		mux:    &http.ServeMux{},
		db:     db,
		addr:   addr,
		events: events,
	}
	srv.routes(apiroots...)
	return srv, nil
}

func (s Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// ListenAndServe simply calls http.ListenAndServe with the configured TCP
// address and s as the handler.
func (s Server) ListenAndServe() error {
	return http.ListenAndServe(s.addr, s)
}

// Close closes the database handle.
func (s *Server) Close() error {
	return s.db.Close()
}

// routes registers handlerFuncs for the server paths under the given prefixes.
func (s *Server) routes(prefixes ...string) {
	s.mux.HandleFunc("/ping", s.handlePing())
	for _, prefix := range prefixes {
		s.mux.HandleFunc(prefix+"/", s.metrics(s.requestID(s.log(s.auth(s.handleAPI(prefix))))))
	}
}

// handlePing creates an http.HandlerFunc that handles the health check endpoint
// /ping.
func (s *Server) handlePing() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte(`OK`)); err != nil {
			log.Errorf("cannot write HTTP response: %v", err)
		}
	}
}

// handleAPI creates an http.HandlerFunc that creates handlerFuncs for
// operations under the API root.
func (s *Server) handleAPI(prefix string) http.HandlerFunc {
	m := http.ServeMux{}

	m.HandleFunc(path.Join(prefix, "channel"), s.handleChannel())
	m.HandleFunc(path.Join(prefix, "event"), s.handleEvent())

	return func(w http.ResponseWriter, r *http.Request) {
		m.ServeHTTP(w, r)
	}
}

// handleChannel creates an http.HandlerFunc for the API endpoint /channel.
func (s *Server) handleChannel() http.HandlerFunc {
	type response struct {
		URL string `json:"url"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		module := r.URL.Query().Get("module")
		if len(module) < 1 {
			formatJSONError(w, http.StatusBadRequest, "missing required parameter: 'module'")
			return
		}

		resp := response{
			URL: "/release",
		}
		id, err := identity.GetIdentity(r)
		if err != nil {
			formatJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if id.Identity.OrgID == "" {
			formatJSONError(w, http.StatusBadRequest, "missing org_id identity field")
			return
		}
		count, err := s.db.Count(module, id.Identity.OrgID)
		if err != nil {
			log.Error(err)
		}
		if count > 0 {
			resp.URL = "/testing"
		}
		data, err := json.Marshal(resp)
		if err != nil {
			formatJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		incRequests(resp.URL)
		w.Header().Add("Content-Type", "application/json")
		if _, err := w.Write(data); err != nil {
			log.Errorf("cannot write HTTP response: %v", err)
		}
	}
}

// handleEvent creates an http.HandlerFunc for the API endpoint /event.
func (s *Server) handleEvent() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.WriteHeader(http.StatusCreated)
		case http.MethodGet:
			id, err := identity.GetIdentity(r)
			if err != nil {
				formatJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
			if *id.Identity.Type != "Associate" {
				formatJSONError(w, http.StatusUnauthorized, "")
				return
			}

			params, err := url.ParseQuery(r.URL.RawQuery)
			if err != nil {
				formatJSONError(w, http.StatusBadRequest, err.Error())
				return
			}
			var limit, offset int64
			{
				var err error
				p := params.Get("limit")
				if p == "" {
					p = "-1"
				}
				limit, err = strconv.ParseInt(p, 10, 64)
				if err != nil {
					formatJSONError(w, http.StatusBadRequest, err.Error())
					return
				}
			}
			{
				var err error
				p := params.Get("offset")
				if p == "" {
					p = "0"
				}
				offset, err = strconv.ParseInt(p, 10, 64)
				if err != nil {
					formatJSONError(w, http.StatusBadRequest, err.Error())
					return
				}
			}

			events, err := s.db.GetEvents(int(limit), int(offset))
			if err != nil {
				formatJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
			data, err := json.Marshal(&events)
			if err != nil {
				formatJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
			w.Header().Add("Content-Type", "application/json")
			if _, err := w.Write(data); err != nil {
				log.Errorf("cannot write HTTP response: %v", err)
			}
		default:
			formatJSONError(w, http.StatusMethodNotAllowed, fmt.Sprintf("error: '%s' not allowed", r.Method))
			return
		}
	}
}

// log is an http HandlerFunc middlware handler that creates a responseWriter
// and logs details about the HandlerFunc it wraps.
func (s *Server) log(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rr := newResponseRecorder(w)
		start := time.Now()

		next(rr, r)

		var level log.Level
		switch {
		case rr.Code >= 400:
			level = log.WarnLevel
		case rr.Code >= 500:
			level = log.ErrorLevel
		default:
			level = log.InfoLevel
		}

		responseBody := rr.Body.String()
		if len(responseBody) > 1024 {
			responseBody = responseBody[:1024]
		}

		log.WithFields(log.Fields{
			"ident":      r.Host,
			"method":     r.Method,
			"referer":    r.Referer(),
			"url":        r.URL.String(),
			"user-agent": r.UserAgent(),
			"status":     rr.Code,
			"response":   responseBody,
			"duration":   time.Since(start),
			"request-id": r.Header.Get("X-Request-Id"),
		}).Log(level)
	}
}

// requestID is an http HandlerFunc middleware handler that creates a request ID
// and writes it to the response header map.
func (s *Server) requestID(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		request.RequestID(next).ServeHTTP(w, r)
	}
}

// auth is an http HandlerFunc middleware handler that ensures a valid
// X-Rh-Identity header is present in the request.
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity.Identify(next).ServeHTTP(w, r)
	}
}

// metrics is an http HandlerFunc middleware handler that creates and enables
// a metrics recorder.
func (s *Server) metrics(next http.HandlerFunc) http.HandlerFunc {
	m := middleware.New(middleware.Config{
		Recorder: r,
	})
	return func(w http.ResponseWriter, r *http.Request) {
		m.Handler("", http.Handler(next)).ServeHTTP(w, r)
	}
}
