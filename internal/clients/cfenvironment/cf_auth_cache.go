package environments

import (
	"strings"
	"sync"

	"github.com/cloudfoundry/go-cfclient/v3/config"
)

// cfConfigCache caches authenticated go-cfclient configs per credential bundle.
//
// cfenvironment builds a fresh go-cfclient on every Observe (newOrganizationClient),
// and config.New performs an eager password-grant login against the CF UAA. So each
// Observe logs in again. Concurrent per-identity logins are the documented lockout
// trigger for the shared technical user, so we reuse one authenticated config per
// credential instead of logging in every reconcile. The config carries a reused
// oauth2 token source, so the token also refreshes across reconciles.
//
// The lock is held across config.New (an eager login) so a concurrent stampede for
// one credential produces a single login. No TTL: the token source refreshes
// internally on expiry. Credential rotation changes the key, leaving the old entry
// until process restart (same trade-off as btp.clientCache).
var (
	cfCacheMu     sync.Mutex
	cfConfigCache = map[string]*config.Config{}
)

func cachedCFConfig(url, username, password, origin string) (*config.Config, error) {
	key := strings.Join([]string{url, username, password, origin}, "\x00")

	cfCacheMu.Lock()
	defer cfCacheMu.Unlock()

	if c, ok := cfConfigCache[key]; ok {
		return c, nil
	}

	opts := []config.Option{config.UserPassword(username, password)}
	if origin != "" {
		opts = append(opts, config.Origin(origin))
	}
	cfg, err := config.New(url, opts...)
	if err != nil {
		// Do not cache a failed login; the next reconcile retries (controller-runtime
		// already applies exponential backoff to failing reconciles).
		return nil, err
	}
	cfConfigCache[key] = cfg
	return cfg, nil
}
