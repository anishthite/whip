package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/context-labs/whip/internal/codexauth"
	"github.com/context-labs/whip/internal/config"
)

func TestLoginCodexShowsDeviceInstructions(t *testing.T) {
	t.Setenv("WHIP_HOME", t.TempDir())
	expires := time.Date(2030, time.January, 2, 3, 4, 5, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/accounts/deviceauth/usercode":
			w.Write([]byte(`{"device_auth_id":"device-id","user_code":"ABCD-1234","interval":"1"}`))
		case "/api/accounts/deviceauth/token":
			w.Write([]byte(`{"authorization_code":"authorization-code","code_verifier":"verifier"}`))
		case "/oauth/token":
			w.Write([]byte(`{"access_token":"new-access","refresh_token":"new-refresh","id_token":"` + testJWT(t, expires, "account") + `"}`))
		case "/codex/models":
			if r.Header.Get("Authorization") != "Bearer new-access" || r.Header.Get("ChatGPT-Account-ID") != "account" {
				http.Error(w, "missing Codex auth", http.StatusUnauthorized)
				return
			}
			w.Write([]byte(`{"models":[
  {"slug":"gpt-5.6-sol","supported_in_api":true,"context_window":1050000,"supported_reasoning_levels":[{"effort":"low"},{"effort":"high"}],"input_modalities":["text","image"]},
  {"slug":"gpt-5.6-terra","supported_in_api":true,"context_window":1050000,"input_modalities":["text","image"]}
]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	source := &codexauth.Source{HomeDir: t.TempDir(), HTTP: srv.Client(), IssuerURL: srv.URL}
	if err := loginCodexAt(context.Background(), source, &out, srv.URL); err != nil {
		t.Fatal(err)
	}
	printed := out.String()
	for _, want := range []string{
		srv.URL + "/codex/device",
		"ABCD-1234",
		"ctrl+c",
		"saved to ~/.codex/auth.json",
		"2 account models are ready in /model",
	} {
		if !strings.Contains(printed, want) {
			t.Fatalf("login output missing %q:\n%s", want, printed)
		}
	}
	if strings.Contains(printed, "new-access") || strings.Contains(printed, "new-refresh") {
		t.Fatalf("login output leaked a token:\n%s", printed)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Providers[config.CodexProviderName]; !ok {
		t.Fatal("successful login did not configure the codex provider")
	}
	route := cfg.Models[config.CodexDefaultModel]
	if len(route.Providers) != 1 || route.Providers[0] != config.CodexProviderName {
		t.Fatalf("successful login did not configure the codex model route: %+v", route)
	}
	cat, ok := config.LoadCatalogs()[config.CodexProviderName]
	if !ok || len(cat.Models) != 2 || cat.ContextLength("gpt-5.6-sol") != 1050000 {
		t.Fatalf("successful login did not prefetch the Codex model catalog: %+v", cat)
	}
}

func TestLoginCLIUsage(t *testing.T) {
	if err := loginCLI([]string{"openai"}); err == nil || err.Error() != "usage: whip login <codex|claude>" {
		t.Fatalf("error = %v", err)
	}
}

func TestAuthCodexCLIUsage(t *testing.T) {
	if err := authCodexCLI([]string{"unexpected"}); err == nil || err.Error() != "usage: whip auth codex" {
		t.Fatalf("error = %v", err)
	}
}

func testJWT(t *testing.T, expires time.Time, account string) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"exp": expires.Unix(),
		"https://api.openai.com/auth": map[string]string{
			"chatgpt_account_id": account,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`)) + "." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}
