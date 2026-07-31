package probe

import (
	"errors"
	"net"
	"net/url"
	"regexp"
	"strings"
)

const (
	EnvAPIURL         = "FEATBIT_TEST_API_URL"
	EnvServiceToken   = "FEATBIT_TEST_SERVICE_TOKEN"
	EnvPersonalToken  = "FEATBIT_TEST_PERSONAL_TOKEN"
	EnvTarget         = "FEATBIT_TEST_TARGET"
	EnvResourcePrefix = "FEATBIT_TEST_RESOURCE_PREFIX"
)

type Target string

const (
	TargetCloudCurrent  Target = "cloud-current"
	TargetSelfHostedMin Target = "selfhosted-min"
)

const cloudAPIHost = "app-api.featbit.co"

var resourcePrefixPattern = regexp.MustCompile(`^tfp0-[a-z0-9](?:[a-z0-9-]{5,42}[a-z0-9])$`)

// Config is populated exclusively through the Phase 0 environment variables.
// Token fields must never be formatted, logged, or serialized.
type Config struct {
	APIURL         *url.URL
	ServiceToken   string
	PersonalToken  string
	Target         Target
	ResourcePrefix string
}

type LookupEnv func(string) (string, bool)

// Presence reports configuration availability without exposing values.
type Presence struct {
	APIURL         bool
	ServiceToken   bool
	PersonalToken  bool
	Target         bool
	ResourcePrefix bool
}

func LoadConfig(lookup LookupEnv) (Config, Presence, error) {
	var cfg Config
	var presence Presence

	rawURL, apiURLPresent := lookup(EnvAPIURL)
	presence.APIURL = apiURLPresent
	cfg.ServiceToken, presence.ServiceToken = lookup(EnvServiceToken)
	cfg.PersonalToken, presence.PersonalToken = lookup(EnvPersonalToken)
	rawTarget, targetPresent := lookup(EnvTarget)
	presence.Target = targetPresent
	cfg.ResourcePrefix, presence.ResourcePrefix = lookup(EnvResourcePrefix)
	cfg.Target = Target(rawTarget)

	if presence.APIURL {
		parsed, err := parseAPIURL(rawURL)
		if err != nil {
			return Config{}, presence, err
		}
		cfg.APIURL = parsed
	}

	return cfg, presence, nil
}

func (c Config) ValidateReadOnly() error {
	if c.APIURL == nil {
		return errors.New("test API URL is required")
	}
	if c.Target != TargetCloudCurrent && c.Target != TargetSelfHostedMin {
		return errors.New("test target must be cloud-current or selfhosted-min")
	}
	if c.ServiceToken == "" && c.PersonalToken == "" {
		return errors.New("a test access token is required")
	}
	return nil
}

// ValidateMutation fails closed unless the base URL and resource prefix prove
// that the caller selected an approved disposable target.
func (c Config) ValidateMutation() error {
	if err := c.ValidateReadOnly(); err != nil {
		return err
	}
	if err := ValidateResourcePrefix(c.ResourcePrefix); err != nil {
		return err
	}

	host := strings.ToLower(c.APIURL.Hostname())
	switch c.Target {
	case TargetCloudCurrent:
		if c.APIURL.Scheme != "https" || host != cloudAPIHost || c.APIURL.Port() != "" {
			return errors.New("cloud-current mutations require the approved FeatBit Cloud API host over HTTPS")
		}
	case TargetSelfHostedMin:
		if !isLocalOrPrivateHost(host) {
			return errors.New("selfhosted-min mutations require a loopback or private target")
		}
		if c.APIURL.Scheme != "http" && c.APIURL.Scheme != "https" {
			return errors.New("selfhosted-min URL must use HTTP or HTTPS")
		}
	default:
		return errors.New("unapproved mutation target")
	}

	return nil
}

func ValidateResourcePrefix(prefix string) error {
	if prefix == "" {
		return errors.New("test resource prefix is required for mutations")
	}
	if !resourcePrefixPattern.MatchString(prefix) {
		return errors.New("test resource prefix must start with tfp0- and contain 12-48 lowercase letters, digits, or hyphens")
	}
	return nil
}

func parseAPIURL(raw string) (*url.URL, error) {
	if strings.TrimSpace(raw) != raw || raw == "" {
		return nil, errors.New("test API URL must be a non-empty absolute URL without surrounding whitespace")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, errors.New("test API URL is invalid")
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, errors.New("test API URL must be absolute")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("test API URL must not contain user information, query parameters, or a fragment")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("test API URL must use HTTP or HTTPS")
	}

	switch strings.TrimSuffix(u.EscapedPath(), "/") {
	case "", "/api/v1":
		u.Path = "/api/v1"
		u.RawPath = ""
	default:
		return nil, errors.New("test API URL path must be empty or /api/v1")
	}
	return u, nil
}

func isLocalOrPrivateHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate())
}
