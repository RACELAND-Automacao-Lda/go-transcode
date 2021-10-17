package hlsproxy

import (
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// how often should be cache cleanup called
const cacheCleanupPeriod = 4 * time.Second

const segmentExpiration = 60 * time.Second

const playlistExpiration = 1 * time.Second

type Cache struct {
	Data    []byte
	Expires time.Time
}

type ManagerCtx struct {
	logger  zerolog.Logger
	mu      sync.Mutex
	baseUrl string
	prefix  string

	cache   map[string]Cache
	cacheMu sync.RWMutex

	shutdown chan struct{}
}

func New(baseUrl string, prefix string) *ManagerCtx {
	// ensure it ends with slash
	baseUrl = strings.TrimSuffix(baseUrl, "/")
	baseUrl += "/"

	return &ManagerCtx{
		logger:   log.With().Str("module", "hlsproxy").Str("submodule", "manager").Logger(),
		baseUrl:  baseUrl,
		prefix:   prefix,
		cache:    map[string]Cache{},
		shutdown: make(chan struct{}),
	}
}

func (m *ManagerCtx) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.shutdown = make(chan struct{})

	// periodic cleanup
	go func() {
		ticker := time.NewTicker(cacheCleanupPeriod)
		defer ticker.Stop()

		for {
			select {
			case <-m.shutdown:
				return
			case <-ticker.C:
				m.logger.Debug().Msg("performing cleanup")
				m.clearCache()
			}
		}
	}()

	return nil
}

func (m *ManagerCtx) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	close(m.shutdown)
}

func (m *ManagerCtx) ServePlaylist(w http.ResponseWriter, r *http.Request) {
	url := m.baseUrl + strings.TrimPrefix(r.URL.String(), m.prefix)

	body, ok := m.getFromCache(url)
	if ok {
		w.Write(body)
		return
	}

	resp, err := http.Get(url)
	if err != nil {
		log.Err(err).Msg("unable to get HTTP")
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Err(err).Msg("unadle to read response body")
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	var re = regexp.MustCompile(`(?m:^(https?\:\/\/[^\/]+)?\/)`)
	text := re.ReplaceAllString(string(buf), m.prefix)

	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.WriteHeader(resp.StatusCode)

	m.saveToCache(url, body, playlistExpiration)
	w.Write([]byte(text))
}

func (m *ManagerCtx) ServeMedia(w http.ResponseWriter, r *http.Request) {
	url := m.baseUrl + strings.TrimPrefix(r.URL.String(), m.prefix)

	body, ok := m.getFromCache(url)
	if ok {
		w.Write(body)
		return
	}

	resp, err := http.Get(url)
	if err != nil {
		log.Err(err).Msg("unable to get HTTP")
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		log.Err(err).Msg("unadle to read response body")
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "video/MP2T")
	w.WriteHeader(resp.StatusCode)

	m.saveToCache(url, body, segmentExpiration)
	w.Write(body)
}
