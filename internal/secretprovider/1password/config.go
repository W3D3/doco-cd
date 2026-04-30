package onepassword

import (
	"fmt"
	"time"

	"github.com/kimdre/doco-cd/internal/config"
)

type Config struct {
	AccessToken     string        `env:"SECRET_PROVIDER_ACCESS_TOKEN" validate:"nonzero"` // #nosec G117 -- Access token for authenticating with the secret provider
	AccessTokenFile string        `env:"SECRET_PROVIDER_ACCESS_TOKEN_FILE,file"`          // Path to a file containing the access token
	CacheTTL        time.Duration `env:"SECRET_PROVIDER_CACHE_TTL"`                       // Duration to cache resolved secrets in memory (e.g. "5m", "1h"). Set to 0 to disable caching (default).
}

// GetConfig retrieves and parses the configuration for the Bitwarden Secrets Manager from environment variables.
func GetConfig() (*Config, error) {
	cfg := Config{}

	mappings := []config.EnvVarFileMapping{
		{EnvName: "SECRET_PROVIDER_ACCESS_TOKEN", EnvValue: &cfg.AccessToken, FileValue: &cfg.AccessTokenFile, AllowUnset: false},
	}

	err := config.ParseConfigFromEnv(&cfg, &mappings)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", config.ErrParseConfigFailed, err)
	}

	return &cfg, nil
}
