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
)

func TestLoginCodexShowsDeviceInstructions(t *testing.T) {
	expires := time.Date(2030, time.January, 2, 3, 4, 5, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/accounts/deviceauth/usercode":
			w.Write([]byte(`{"device_auth_id":"device-id","user_code":"ABCD-1234","interval":"1"}`))
		case "/api/accounts/deviceauth/token":
			w.Write([]byte(`{"authorization_code":"authorization-code","code_verifier":"verifier"}`))
		case "/oauth/token":
			w.Write([]byte(`{"access_token":"new-access","refresh_token":"new-refresh","id_token":"` + testJWT(t, expires, "account") + `"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	source := &codexauth.Source{HomeDir: t.TempDir(), HTTP: srv.Client(), IssuerURL: srv.URL}
	if err := loginCodex(context.Background(), source, &out); err != nil {
		t.Fatal(err)
	}
	printed := out.String()
	for _, want := range []string{srv.URL + "/codex/device", "ABCD-1234", "ctrl+c", "saved to ~/.codex/auth.json"} {
		if !strings.Contains(printed, want) {
			t.Fatalf("login output missing %q:\n%s", want, printed)
		}
	}
	if strings.Contains(printed, "new-access") || strings.Contains(printed, "new-refresh") {
		t.Fatalf("login output leaked a token:\n%s", printed)
	}
}

func TestLoginCLIUsage(t *testing.T) {
	if err := loginCLI([]string{"openai"}); err == nil || err.Error() != "usage: whip login codex" {
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
