package oauth2

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	jsoniter "github.com/json-iterator/go"
	"github.com/patrickmn/go-cache"
	"golang.org/x/oauth2"
	"gopkg.in/gorilla/securecookie.v1"
)

// codebeat:disable[TOO_MANY_IVARS]

// Config holds package options and constants
type Config struct {
	MyURL       string `long:"my_url" description:"Own host URL (autodetect if empty)"`
	CallBackURL string `long:"cb_url" default:"/login" description:"URL for Auth server's redirect"`

	//nolint:staticcheck // Multiple struct tag "choice" is allowed
	Type      string `long:"type" env:"TYPE" default:"gitea"  choice:"gitea" choice:"mmost" description:"Authorization Server type (gitea|mmost)"`
	Do401     bool   `long:"do401" env:"DO401" description:"Do not redirect with http.StatusUnauthorized, process it"`
	Host      string `long:"host" env:"HOST" default:"http://gitea:8080" description:"Authorization Server host"`
	Team      string `long:"team" env:"TEAM" default:"dcape" description:"Authorization Server team which members has access to resource"`
	ClientID  string `long:"client_id" env:"CLIENT_ID" description:"Authorization Server Client ID"`
	ClientKey string `long:"client_key" env:"CLIENT_KEY" description:"Authorization Server Client key"`

	AuthHeader     string `long:"auth_header" default:"X-narra-token" description:"Use token from this header if given"`
	CookieDomain   string `long:"cookie_domain"  description:"Auth cookie domain"`
	CookieName     string `long:"cookie_name" default:"narra_token" description:"Auth cookie name"`
	CookieSignKey  string `long:"cookie_sign" env:"COOKIE_SIGN_KEY" description:"Cookie sign key (32 or 64 bytes)"`
	CookieCryptKey string `long:"cookie_crypt" env:"COOKIE_CRYPT_KEY" description:"Cookie crypt key (16, 24, or 32 bytes)"`

	UserHeader string `long:"user_header" env:"USER_HEADER" default:"X-Username" description:"HTTP Response Header for username"`

	BasicRealm     string `long:"basic_realm" default:"narra" description:"Basic Auth realm"`
	BasicUser      string `long:"basic_username" default:"token" description:"Basic Auth user name"`
	BasicUserAgent string `long:"basic_useragent" default:"docker/" description:"UserAgent which requires Basic Auth"`
}

// ProviderConfig holds Authorization Server properties
type ProviderConfig struct {
	Auth        string
	Token       string
	User        string
	Team        string
	TokenPrefix string
	TeamName    string
}

// codebeat:enable[TOO_MANY_IVARS]

// Service holds service attributes
type Service struct {
	Config        Config
	api           *oauth2.Config
	cookie        *securecookie.SecureCookie
	cache         *cache.Cache
	provider      *ProviderConfig
	lock          sync.Mutex
	lockableMyURL string
}

var (
	// ErrNoTeam holds error: User is not in required team
	ErrNoTeam = errors.New("User is not in required team")
	// ErrAuthNotGranted holds error: Auth not granted
	ErrAuthNotGranted = errors.New("Auth not granted")
	// ErrStateUnknown holds error: Unknown state
	ErrStateUnknown = errors.New("Unknown state")
	// ErrBasicTokenExpected holds error when username <> token
	ErrBasicTokenExpected = errors.New("Basuc Auth username does not match")
	// ErrBasicAuthRequired holds 401 for docker client
	ErrBasicAuthRequired = errors.New("Basuc Auth is required")
)

// Package debug level
var DL = 1

// Functional options
// https://github.com/tmrts/go-patterns/blob/master/idiom/functional-options.md

// Option is a functional options return type
type Option func(*Service)

// Cache allows to change default cache lib
func Cache(cache *cache.Cache) Option {
	return func(srv *Service) {
		srv.cache = cache
	}
}

// Cookie allows to change default cookie lib
func Cookie(cookie *securecookie.SecureCookie) Option {
	return func(srv *Service) {
		srv.cookie = cookie
	}
}

// Provider allows to change authorization server config
func Provider(prov *ProviderConfig) Option {
	return func(srv *Service) {
		srv.provider = prov
	}
}

var (
	// Providers holds supported Authorization Servers properties
	Providers = map[string]*ProviderConfig{
		"gitea": {
			Auth:        "/login/oauth/authorize",
			Token:       "/login/oauth/access_token",
			User:        "/api/v1/user",
			Team:        "/api/v1/user/orgs",
			TokenPrefix: "token ",
			TeamName:    "username",
		},
		"mmost": {
			Auth:        "/oauth/authorize",
			Token:       "/oauth/access_token",
			User:        "/api/v4/users/me",
			Team:        "/api/v4/users/%s/teams",
			TokenPrefix: "Bearer ",
			TeamName:    "name",
		},
	}
)

// New creates service
func New(cfg Config, options ...Option) *Service {
	srv := &Service{
		Config: cfg,
	}
	for _, option := range options {
		option(srv)
	}
	if srv.cookie == nil {
		srv.cookie = securecookie.New([]byte(cfg.CookieSignKey), []byte(cfg.CookieCryptKey))
	}
	if srv.cache == nil {
		srv.cache = cache.New(5*time.Minute, 10*time.Minute)
	}
	if srv.provider == nil {
		srv.provider = Providers[cfg.Type]
	}
	if strings.HasSuffix(srv.Config.Host, "/") {
		// some users asked to autoremove
		srv.Config.Host = strings.TrimSuffix(srv.Config.Host, "/")
	}
	srv.api = &oauth2.Config{
		ClientID:     srv.Config.ClientID,
		ClientSecret: srv.Config.ClientKey,
		Scopes:       []string{srv.Config.BasicRealm},
		Endpoint: oauth2.Endpoint{
			TokenURL: srv.Config.Host + srv.provider.Token,
			AuthURL:  srv.Config.Host + srv.provider.Auth,
		},
	}
	if srv.Config.MyURL != "" {
		// given in config
		srv.api.RedirectURL = srv.Config.MyURL + srv.Config.CallBackURL
		// disable autodetect
		srv.lockableMyURL = srv.Config.MyURL
	}
	return srv
}

// IsMyURLEmpty check if app URL autodetect requested
func (srv *Service) IsMyURLEmpty() bool {
	srv.lock.Lock()
	defer srv.lock.Unlock()
	return srv.lockableMyURL == ""
}

// SetMyURL changes app URL
func (srv *Service) SetMyURL(scheme, host string) {
	//Use mutex here
	srv.lock.Lock()
	srv.lockableMyURL = fmt.Sprintf("%s://%s", scheme, host)
	srv.api.RedirectURL = srv.lockableMyURL + srv.Config.CallBackURL
	srv.lock.Unlock()
}

// AuthIsOK returns true if request is allowed to proceed
func (srv *Service) AuthIsOK(w http.ResponseWriter, r *http.Request) bool {
	// Use the custom HTTP client when requesting a token.
	var ids *[]string
	var auth string
	log := logr.FromContextOrDiscard(r.Context())

	scheme := "http"
	if r.TLS != nil {
		scheme += "s"
	}
	if srv.IsMyURLEmpty() {
		srv.SetMyURL(scheme, r.Host)
	}

	if u, p, ok := r.BasicAuth(); ok {
		log.V(DL).Info("Basic Auth requested", "user", u)
		if u != srv.Config.BasicUser {
			warn(w, log, ErrBasicTokenExpected, srv.Config.BasicUser, http.StatusUnauthorized)
			return false
		}
		auth = p
	} else {
		auth = r.Header.Get(srv.Config.AuthHeader)
	}

	if auth != "" {
		// server token
		httpClient := &http.Client{Timeout: 2 * time.Second}
		ctx := context.WithValue(context.Background(), oauth2.HTTPClient, httpClient)
		client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(&oauth2.Token{
			AccessToken: auth,
			TokenType:   "Bearer",
		}))
		var err error
		ids, err = srv.getMeta(client)
		if err != nil {
			warn(w, log, fmt.Errorf("Get meta by header (%v) error: %w", r.Header, err), "", http.StatusUnauthorized)
			return false
		}
		log.V(DL).Info("User meta", "tags", ids)

	} else {
		// No header => check others

		// Basic auth
		ua := r.Header.Get("User-Agent")
		if strings.HasPrefix(ua, srv.Config.BasicUserAgent) {
			log.V(DL).Info("This ua requires Basic Auth", "ua", ua)
			w.Header().Set("WWW-Authenticate", fmt.Sprintf("Basic realm=%q", srv.Config.BasicRealm))
			http.Error(w, ErrBasicAuthRequired.Error(), http.StatusUnauthorized)
			return false
		}

		// Own cookie
		cookie, err := r.Cookie(srv.Config.CookieName)
		errMsg := "Cookie read error"
		if err == nil {
			err = srv.cookie.Decode(srv.Config.CookieName, cookie.Value, &ids)
			errMsg = "Cookie decode error"
		}
		if err != nil {
			log.V(DL).Info(errMsg, "error", err.Error())
			r.Header.Set("X-Forwarded-Proto", scheme)
			r.Header.Set("X-Forwarded-Host", r.Host)
			r.Header.Set("X-Forwarded-Uri", r.RequestURI)
			if srv.Config.Do401 && r.Header.Get("Accept") != "application/json" {
				// traefik wants redirect to provider
				srv.Stage1Handler().ServeHTTP(w, r)
			} else {
				// nginx and js wants 401
				http.Error(w, err.Error(), http.StatusUnauthorized)
			}
			return false
		}
	}
	if len(srv.Config.Team) == 0 || stringExists(ids, srv.Config.Team) {
		log.V(DL).Info("User authorized", "user", (*ids)[0])
		r.Header.Add(srv.Config.UserHeader, (*ids)[0])
		return true
	}
	warn(w, log, fmt.Errorf("User %s Team %s: %w", (*ids)[0], srv.Config.Team, ErrNoTeam), "", http.StatusForbidden)
	return false
}

// HTTP handler pattern, see
// https://www.alexedwards.net/blog/a-recap-of-request-handling

// AuthHandler is a Nginx auth_request handler
func (srv *Service) AuthHandler() http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		if srv.AuthIsOK(w, r) {
			w.WriteHeader(http.StatusOK)
		}
	}
	return http.HandlerFunc(fn)
}

// Stage1Handler handles 401 error & redirects user to auth server
func (srv *Service) Stage1Handler() http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		log := logr.FromContextOrDiscard(r.Context())
		uuid, err := uuid.NewRandom()
		if err != nil {
			warn(w, log, err, "UUID Generate error", http.StatusServiceUnavailable)
			return
		}
		url := fmt.Sprintf("%s://%s%s",
			r.Header.Get("X-Forwarded-Proto"),
			r.Header.Get("X-Forwarded-Host"),
			r.Header.Get("X-Forwarded-Uri"),
		)
		log.V(DL).Info("Got UUID", "uuid", uuid.String(), "url", url)
		srv.cache.Set(uuid.String(), url, cache.DefaultExpiration)
		redirect := srv.api.AuthCodeURL(uuid.String(), oauth2.AccessTypeOffline)

		log.V(DL).Info("Redirect", "url", redirect)
		w.Header().Add("Content-type", "application/json")
		http.Redirect(w, r, redirect, http.StatusFound)
	}
	return http.HandlerFunc(fn)
}

// Stage2Handler handles redirect from auth provider,
// fetches token & user info
func (srv *Service) Stage2Handler() http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		log := logr.FromContextOrDiscard(r.Context())
		url, ids, err := srv.processMeta(r)
		if err != nil {
			warn(w, log, err, "Meta processing failed", http.StatusServiceUnavailable)
			return
		}

		log.V(DL).Info("Got Meta", "ids", ids)
		// store ids in cookie
		if encoded, err := srv.cookie.Encode(srv.Config.CookieName, &ids); err == nil {
			cookie := &http.Cookie{
				Name:  srv.Config.CookieName,
				Value: encoded,
				Path:  "/",
			}
			if srv.Config.CookieDomain != "" {
				cookie.Domain = srv.Config.CookieDomain
			}
			http.SetCookie(w, cookie)
			log.V(DL).Info("All OK, set cookie", "domain", srv.Config.CookieDomain, "redirect", url)
			http.Redirect(w, r, url, http.StatusFound)
		} else {
			warn(w, log, err, "Cookie encode error", http.StatusServiceUnavailable)
		}
	}
	return http.HandlerFunc(fn)
}

// LogoutHandler handles auth cookie clearing
func (srv *Service) LogoutHandler() http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		cookie := &http.Cookie{
			Name:    srv.Config.CookieName,
			Value:   "",
			Path:    "/",
			Expires: time.Unix(0, 0),
		}
		if srv.Config.CookieDomain != "" {
			cookie.Domain = srv.Config.CookieDomain
		}
		http.SetCookie(w, cookie)
		http.Redirect(w, r, "/", http.StatusFound)
	}
	return http.HandlerFunc(fn)
}

// request processes requests to Auth service
func (srv *Service) request(client *http.Client, url string, data interface{}) error {
	req, err := http.NewRequest("GET", srv.Config.Host+url, nil)
	if err != nil {
		return fmt.Errorf("Request create error: %w", err)
	}
	req.Header.Add("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Request error: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Not OK with request, status: %d", resp.StatusCode)
	}
	defer resp.Body.Close()
	err = jsoniter.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		return fmt.Errorf("Parse response error: %w", err)
	}
	return nil
}

// getMeta fetches user metadata from auth server
func (srv *Service) getMeta(client *http.Client) (*[]string, error) {
	// get username
	var user map[string]interface{}
	err := srv.request(client, srv.provider.User, &user)
	if err != nil {
		return nil, fmt.Errorf("Get user metadata: %w", err)
	}
	tags := []string{user["username"].(string)}

	if len(srv.Config.Team) == 0 {
		// no team check
		return &tags, nil
	}
	// get user teams
	url := srv.provider.Team
	if strings.Contains(url, "%s") {
		// mattermost wants user id in URL
		url = fmt.Sprintf(url, user["id"])
	}

	var orgs []map[string]interface{}
	err = srv.request(client, url, &orgs)
	if err != nil {
		return nil, fmt.Errorf("Get team metadata: %w", err)
	}

	for _, o := range orgs {
		tags = append(tags, o[srv.provider.TeamName].(string))
	}
	return &tags, nil
}

// processMeta fetches user's metadata at auth stage 2
func (srv *Service) processMeta(r *http.Request) (url string, ids *[]string, err error) {
	log := logr.FromContextOrDiscard(r.Context())
	code := r.FormValue("code")
	state := r.FormValue("state")
	// TODO: r.FormValue("error")
	// error=invalid_request&error_description
	log.V(DL).Info("Auth data", "code", code, "state", state)
	if code == "" || state == "" {
		err = ErrAuthNotGranted
		return
	}
	urlIface, found := srv.cache.Get(state)
	if !found {
		err = ErrStateUnknown
		return
	}
	srv.cache.Delete(state)
	url = urlIface.(string)

	// Use the custom HTTP client when requesting a token.
	httpClient := &http.Client{Timeout: 2 * time.Second}
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, httpClient)

	tok, err := srv.api.Exchange(ctx, code)
	if err != nil {
		err = fmt.Errorf("Token fetch failed: %w", err)
		return
	}

	log.V(DL+1).Info("API token", "token", tok)
	client := srv.api.Client(ctx, tok)

	// load usernames from provider
	ids, err = srv.getMeta(client)
	log.V(DL).Info("User meta", "tags", ids)
	return
}

// stringExists checks if str exists in strings slice
func stringExists(strings *[]string, str string) bool {
	if len(*strings) > 0 {
		for _, s := range *strings {
			if str == s {
				return true
			}
		}
	}
	return false
}

// warn prints warning to log and http
func warn(w http.ResponseWriter, log logr.Logger, e error, msg string, status int) {
	log.Error(e, msg)
	http.Error(w, e.Error(), status)
}
