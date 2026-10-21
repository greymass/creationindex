package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/pebble"
)

const endowmentTTL = 10 * time.Second

type endowmentJSON struct {
	ProgramAccount string `json:"program_account"`
	RAMQuota       int64  `json:"ram_quota"`
	RAMUsage       int64  `json:"ram_usage"`
	AvailableBytes int64  `json:"available_bytes"`
	ReadAt         string `json:"read_at"`
}

type endowmentCache struct {
	mu     sync.Mutex
	client http.Client
	last   *endowmentJSON
	at     time.Time
}

type chainAccount struct {
	RAMQuota int64 `json:"ram_quota"`
	RAMUsage int64 `json:"ram_usage"`
}

type chainError struct {
	Error struct {
		Name string `json:"name"`
	} `json:"error"`
}

func (s *Server) getEndowment(_ pebble.Reader, _ params) (any, error) {
	if s.chainURL == "" {
		return nil, &httpError{http.StatusServiceUnavailable, "chain-url is not set"}
	}
	if s.programAccount == "" {
		return nil, &httpError{http.StatusServiceUnavailable, "program-account is not set"}
	}
	c := &s.endowment
	c.mu.Lock()
	defer c.mu.Unlock()
	now := s.now()
	if c.last != nil && now.Sub(c.at) < endowmentTTL {
		return c.last, nil
	}

	body, _ := json.Marshal(map[string]string{"account_name": s.programAccount})
	resp, err := c.client.Post(strings.TrimSuffix(s.chainURL, "/")+"/v1/chain/get_account", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, chainUnavailable(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var ce chainError
		json.NewDecoder(resp.Body).Decode(&ce)
		if ce.Error.Name == "account_query_exception" {
			return nil, notFound("program account %s does not exist", s.programAccount)
		}
		return nil, chainUnavailable(fmt.Errorf("get_account status %d", resp.StatusCode))
	}
	var a chainAccount
	if err := json.NewDecoder(resp.Body).Decode(&a); err != nil {
		return nil, chainUnavailable(err)
	}
	c.last = &endowmentJSON{
		ProgramAccount: s.programAccount,
		RAMQuota:       a.RAMQuota,
		RAMUsage:       a.RAMUsage,
		AvailableBytes: a.RAMQuota - a.RAMUsage,
		ReadAt:         now.UTC().Format(timeLayout),
	}
	c.at = now
	return c.last, nil
}

func chainUnavailable(err error) error {
	return &httpError{http.StatusBadGateway, "chain API unavailable: " + err.Error()}
}
