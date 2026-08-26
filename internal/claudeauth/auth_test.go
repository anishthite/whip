package claudeauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoginSavesWhipCredentials(t *testing.T) {
	token := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "wrong request", http.StatusBadRequest)
			return
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["grant_type"] != "authorization_code" || body["code"] != "approved" || body["code_verifier"] == "" {
			http.Error(w, "wrong exchange", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"access","refresh_token":"refresh","expires_in":3600}`))
	}))
	defer token.Close()

	source := &Source{HomeDir: t.TempDir(), HTTP: token.Client(), TokenURL: token.URL, CallbackAddr: "127.0.0.1:0"}
	loginURL := make(chan string, 1)
	done := make(chan error, 1)
	go func() { done <- source.Login(context.Background(), func(u string) { loginURL <- u }) }()

	u := <-loginURL
	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatal(err)
	}
	redirect, err := url.Parse(parsed.Query().Get("redirect_uri"))
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Get(redirect.String() + "?code=approved&state=" + url.QueryEscape(parsed.Query().Get("state")))
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(source.HomeDir, ".whip", "claude.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "approved") || !strings.Contains(string(data), "access") {
		t.Fatalf("stored credentials = %s", data)
	}
	if credentials, err := source.Credentials(context.Background()); err != nil || credentials.AccessToken != "access" {
		t.Fatalf("credentials = %+v, %v", credentials, err)
	}
}

func TestCredentialsRefreshesPiFallback(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".pi", "agent", "auth.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"anthropic":{"type":"oauth","access":"old","refresh":"refresh","expires":1},"other":{"keep":true}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	token := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["grant_type"] != "refresh_token" || body["refresh_token"] != "refresh" {
			http.Error(w, "wrong refresh", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"new","refresh_token":"rotated","expires_in":3600}`))
	}))
	defer token.Close()
	source := &Source{HomeDir: home, HTTP: token.Client(), TokenURL: token.URL}

	credentials, err := source.Credentials(context.Background())
	if err != nil || credentials.AccessToken != "new" {
		t.Fatalf("credentials = %+v, %v", credentials, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"access": "new"`) || !strings.Contains(string(data), `"other"`) {
		t.Fatalf("refreshed Pi auth = %s", data)
	}
}

func TestLoginCancellationClosesCallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	source := &Source{HomeDir: t.TempDir(), CallbackAddr: "127.0.0.1:0"}
	ready := make(chan string, 1)
	done := make(chan error, 1)
	go func() { done <- source.Login(ctx, func(u string) { ready <- u }) }()
	<-ready
	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("login did not stop after cancellation")
	}
}
