package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/greymass/creationindex/internal/store"
	"github.com/greymass/roborovski/libraries/chain"
	"github.com/greymass/roborovski/libraries/logger"
	"github.com/greymass/roborovski/libraries/server"
)

const (
	Prefix       = "/v1/creationindex/"
	defaultLimit = 100
	maxLimit     = 1000
	maxDays      = 366
)

type handler func(s *Server, r pebble.Reader, p params) (any, error)

var routes = map[string]handler{
	"get_creation":       (*Server).getCreation,
	"get_creations":      (*Server).getCreations,
	"get_daily":          (*Server).getDaily,
	"get_day_creators":   (*Server).getDayCreators,
	"get_program_daily":  (*Server).getProgramDaily,
	"get_quota":          (*Server).getQuota,
	"get_program_events": (*Server).getProgramEvents,
	"get_endowment":      (*Server).getEndowment,
	"health":             (*Server).health,
}

type Server struct {
	db             *store.Store
	programAccount string
	chainURL       string
	now            func() time.Time
	endowment      endowmentCache
}

func New(db *store.Store, programAccount, chainURL string) *Server {
	s := &Server{db: db, programAccount: programAccount, chainURL: chainURL, now: time.Now}
	s.endowment.client.Timeout = 5 * time.Second
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	h, ok := routes[strings.TrimPrefix(r.URL.Path, Prefix)]
	if !ok || !strings.HasPrefix(r.URL.Path, Prefix) {
		server.WriteJSON(w, http.StatusNotFound, errorBody{"not found"})
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		server.WriteJSON(w, http.StatusMethodNotAllowed, errorBody{"method not allowed"})
		return
	}
	p, err := readParams(r)
	if err != nil {
		writeError(w, r, err)
		return
	}

	snap := s.db.DB().NewSnapshot()
	defer snap.Close()
	out, err := h(s, snap, p)
	if err != nil {
		writeError(w, r, err)
		return
	}
	server.WriteJSON(w, http.StatusOK, out)
}

type errorBody struct {
	Error string `json:"error"`
}

type httpError struct {
	status int
	msg    string
}

func (e *httpError) Error() string { return e.msg }

func badRequest(format string, args ...any) error {
	return &httpError{http.StatusBadRequest, fmt.Sprintf(format, args...)}
}

func notFound(format string, args ...any) error {
	return &httpError{http.StatusNotFound, fmt.Sprintf(format, args...)}
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var he *httpError
	if errors.As(err, &he) {
		server.WriteJSON(w, he.status, errorBody{he.msg})
		return
	}
	logger.Printf("http", "%s %s: %v", r.Method, r.URL.Path, err)
	server.WriteJSON(w, http.StatusInternalServerError, errorBody{"internal error"})
}

type params map[string]string

func readParams(r *http.Request) (params, error) {
	p := params{}
	if r.Method == http.MethodGet {
		for k, v := range r.URL.Query() {
			if len(v) > 0 {
				p[k] = v[0]
			}
		}
		return p, nil
	}
	var body map[string]any
	dec := json.NewDecoder(r.Body)
	dec.UseNumber()
	if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		return nil, badRequest("request body must be a JSON object")
	}
	for k, v := range body {
		switch v := v.(type) {
		case string:
			p[k] = v
		case json.Number:
			p[k] = v.String()
		case nil:
		default:
			return nil, badRequest("%s must be a string or a number", k)
		}
	}
	return p, nil
}

func (p params) name(key string) (uint64, bool, error) {
	v := p[key]
	if v == "" {
		return 0, false, nil
	}
	n := chain.StringToName(v)
	if chain.NameToString(n) != v {
		return 0, false, badRequest("%s is not a valid account name", key)
	}
	return n, true, nil
}

func (p params) uint(key string, def uint64) (uint64, error) {
	v := p[key]
	if v == "" {
		return def, nil
	}
	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return 0, badRequest("%s must be a non-negative integer", key)
	}
	return n, nil
}

func (p params) limit() (int, error) {
	n, err := p.uint("limit", defaultLimit)
	if err != nil {
		return 0, err
	}
	if n == 0 || n > maxLimit {
		return 0, badRequest("limit must be between 1 and %d", maxLimit)
	}
	return int(n), nil
}

func (p params) day(key string) (uint32, bool, error) {
	v := p[key]
	if v == "" {
		return 0, false, nil
	}
	t, err := time.Parse(time.DateOnly, v)
	if err != nil || t.Unix() < 0 {
		return 0, false, badRequest("%s must be a day as YYYY-MM-DD", key)
	}
	return uint32(t.Unix() / 86400), true, nil
}

func (p params) dayRange() (from, to uint32, err error) {
	from, ok, err := p.day("from")
	if err != nil {
		return 0, 0, err
	}
	if !ok {
		return 0, 0, badRequest("from is required")
	}
	to, ok, err = p.day("to")
	if err != nil {
		return 0, 0, err
	}
	if !ok {
		return 0, 0, badRequest("to is required")
	}
	if to < from {
		return 0, 0, badRequest("to is before from")
	}
	if to-from >= maxDays {
		return 0, 0, badRequest("at most %d days per request", maxDays)
	}
	return from, to, nil
}

func (p params) after() (uint64, error) {
	after, err := p.uint("after", 0)
	if err == nil && after == math.MaxUint64 {
		return 0, badRequest("after is out of range")
	}
	return after, err
}
