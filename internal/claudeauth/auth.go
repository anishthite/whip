// Package claudeauth manages the local OAuth state used by Claude subscriptions.
package claudeauth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	clientID          = "9d1c250a-e6a1-44d9-88ed-5944d1962f5e"
	defaultAuthorize  = "https://claude.ai/oauth/authorize"
	defaultToken      = "https://platform.claude.com/v1/oauth/token"
	defaultCallback   = "127.0.0.1:53692"
	callbackPath      = "/callback"
	claudeOAuthScopes = "org:create_api_key user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload"
)

var ErrLoginRequired = errors.New("claude authentication not found; run whip auth claude (or whip login claude), or pi /login anthropic")

// Credentials intentionally excludes the refresh token, which never needs to
// leave this package and must not be exposed to callers that may log values.
type Credentials struct{ AccessToken string }

// Source reads Whip's OAuth state first and Pi's Anthropic state as a fallback.
// Its exported endpoints make the browser flow testable without a live login.
type Source struct {
	HomeDir      string
	HTTP         *http.Client
	AuthorizeURL string
	TokenURL     string
	CallbackAddr string

	mu  sync.Mutex
	now func() time.Time
}

type credential struct {
	Access  string `json:"access"`
	Refresh string `json:"refresh"`
	Expires int64  `json:"expires"`
}

type candidate struct {
	path string
	root map[string]json.RawMessage
	key  string
	credential
}

func (s *Source) Available() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.load()
	return err
}

func (s *Source) Credentials(ctx context.Context) (Credentials, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.load()
	if err != nil {
		return Credentials{}, err
	}
	if c.Access != "" && (c.Expires == 0 || time.Unix(c.Expires, 0).After(s.clock().Add(5*time.Minute))) {
		return Credentials{AccessToken: c.Access}, nil
	}
	if c.Refresh == "" {
		return Credentials{}, ErrLoginRequired
	}
	if err := s.refresh(ctx, c); err != nil {
		return Credentials{}, err
	}
	return Credentials{AccessToken: c.Access}, nil
}

// Login starts Pi's browser PKCE flow. show receives an authorization URL but
// never credential material. ctx owns the callback listener and exchange.
func (s *Source) Login(ctx context.Context, show func(string)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	verifier, challenge, err := pkce()
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", s.callbackAddr())
	if err != nil {
		return fmt.Errorf("start Claude login callback: %w", err)
	}
	redirect := "http://" + listener.Addr().String() + callbackPath
	if s.CallbackAddr == "" || s.CallbackAddr == defaultCallback {
		redirect = "http://localhost:53692" + callbackPath
	}

	type result struct {
		code string
		err  error
	}
	resultCh := make(chan result, 1)
	sendResult := func(result result) {
		select {
		case resultCh <- result:
		default:
		}
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != callbackPath {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("state") != verifier {
			http.Error(w, "OAuth state mismatch", http.StatusBadRequest)
			sendResult(result{err: errors.New("Claude OAuth state mismatch")})
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "Missing OAuth authorization code", http.StatusBadRequest)
			sendResult(result{err: errors.New("Claude OAuth callback missing authorization code")})
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "Claude sign-in completed. You can close this window.")
		sendResult(result{code: code})
	})}
	go func() { _ = server.Serve(listener) }()
	defer func() { _ = server.Close() }()

	params := url.Values{
		"code":                  {"true"},
		"client_id":             {clientID},
		"response_type":         {"code"},
		"redirect_uri":          {redirect},
		"scope":                 {claudeOAuthScopes},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {verifier},
	}
	if show != nil {
		show(s.authorizeURL() + "?" + params.Encode())
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case result := <-resultCh:
		if result.err != nil {
			return result.err
		}
		c, err := s.exchange(ctx, result.code, verifier, redirect)
		if err != nil {
			return err
		}
		return s.saveWhip(c)
	}
}

func (s *Source) load() (*candidate, error) {
	home, err := s.homeDir()
	if err != nil {
		return nil, ErrLoginRequired
	}
	for _, source := range []struct{ path, key string }{
		{filepath.Join(home, ".whip", "claude.json"), ""},
		{filepath.Join(home, ".pi", "agent", "auth.json"), "anthropic"},
	} {
		c, err := loadCandidate(source.path, source.key)
		if err == nil && (c.Access != "" || c.Refresh != "") {
			return c, nil
		}
	}
	return nil, ErrLoginRequired
}

func loadCandidate(path, key string) (*candidate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	root := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &root); err != nil || root == nil {
		return nil, errors.New("Claude auth file must contain a JSON object")
	}
	raw := json.RawMessage(data)
	if key != "" {
		raw = root[key]
	}
	var c credential
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	return &candidate{path: path, root: root, key: key, credential: c}, nil
}

func (s *Source) exchange(ctx context.Context, code, verifier, redirect string) (credential, error) {
	body, _ := json.Marshal(map[string]string{
		"grant_type":    "authorization_code",
		"client_id":     clientID,
		"code":          code,
		"redirect_uri":  redirect,
		"code_verifier": verifier,
	})
	return s.token(ctx, body)
}

func (s *Source) refresh(ctx context.Context, c *candidate) error {
	body, _ := json.Marshal(map[string]string{
		"grant_type":    "refresh_token",
		"client_id":     clientID,
		"refresh_token": c.Refresh,
	})
	fresh, err := s.token(ctx, body)
	if err != nil {
		return fmt.Errorf("refresh Claude login: %w", err)
	}
	c.credential = fresh
	return c.save()
}

func (s *Source) token(ctx context.Context, body []byte) (credential, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.tokenURL(), bytes.NewReader(body))
	if err != nil {
		return credential{}, fmt.Errorf("prepare Claude OAuth token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := s.httpClient().Do(req)
	if err != nil {
		return credential{}, fmt.Errorf("Claude OAuth token request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return credential{}, fmt.Errorf("Claude OAuth token request returned %s", resp.Status)
	}
	var response struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&response); err != nil || response.AccessToken == "" || response.RefreshToken == "" || response.ExpiresIn <= 0 {
		return credential{}, errors.New("Claude OAuth token response was invalid")
	}
	return credential{Access: response.AccessToken, Refresh: response.RefreshToken, Expires: s.clock().Add(time.Duration(response.ExpiresIn) * time.Second).Unix()}, nil
}

func (s *Source) saveWhip(c credential) error {
	home, err := s.homeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, ".whip", "claude.json")
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, append(data, '\n'))
}

func (c *candidate) save() error {
	var (
		data []byte
		err  error
	)
	if c.key == "" {
		data, err = json.MarshalIndent(c.credential, "", "  ")
	} else {
		entry, err := json.Marshal(c.credential)
		if err != nil {
			return err
		}
		c.root[c.key] = entry
		data, err = json.MarshalIndent(c.root, "", "  ")
	}
	if err != nil {
		return err
	}
	return writeAtomic(c.path, append(data, '\n'))
}

func pkce() (verifier, challenge string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generate Claude OAuth PKCE verifier: %w", err)
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".whip-claude-auth-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func (s *Source) homeDir() (string, error) {
	if s.HomeDir != "" {
		return s.HomeDir, nil
	}
	return os.UserHomeDir()
}

func (s *Source) callbackAddr() string {
	if s.CallbackAddr != "" {
		return s.CallbackAddr
	}
	return defaultCallback
}

func (s *Source) authorizeURL() string {
	if s.AuthorizeURL != "" {
		return strings.TrimRight(s.AuthorizeURL, "/")
	}
	return defaultAuthorize
}

func (s *Source) tokenURL() string {
	if s.TokenURL != "" {
		return s.TokenURL
	}
	return defaultToken
}

func (s *Source) httpClient() *http.Client {
	if s.HTTP != nil {
		return s.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (s *Source) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}
