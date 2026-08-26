package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/context-labs/whip/internal/codexauth"
	"github.com/context-labs/whip/internal/config"
)

// loginCLI implements `whip login codex`.
func loginCLI(args []string) error {
	if len(args) != 1 || args[0] != "codex" {
		return errors.New("usage: whip login codex")
	}
	return authCodexCLI(nil)
}

// authCodexCLI implements `whip auth codex`; login codex delegates here so
// both spellings authenticate and configure the ready-to-use route alike.
func authCodexCLI(args []string) error {
	if len(args) != 0 {
		return errors.New("usage: whip auth codex")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return loginCodex(ctx, &codexauth.Source{}, os.Stdout)
}

func loginCodex(ctx context.Context, source *codexauth.Source, out io.Writer) error {
	err := source.DeviceLogin(ctx, func(code codexauth.DeviceCode) {
		fmt.Fprint(out, deviceLoginPrompt(code))
	})
	if errors.Is(err, context.Canceled) {
		return errors.New("Codex login cancelled")
	}
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("configure Codex provider: %w", err)
	}
	cfg.UpsertCodex()
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("configure Codex provider: %w", err)
	}
	fmt.Fprintln(out, "Codex login saved to ~/.codex/auth.json. Codex is ready in /model as gpt-5.4 @ codex.")
	return nil
}

func deviceLoginPrompt(code codexauth.DeviceCode) string {
	return fmt.Sprintf(`
Open this URL in any browser and sign in to ChatGPT:
  %s

Enter this one-time code (expires in 15 minutes):
  %s

Continue only if you started this login in Whip. Press ctrl+c to cancel.

`, code.VerificationURL, code.UserCode)
}
