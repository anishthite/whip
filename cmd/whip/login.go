package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/context-labs/whip/internal/claudeauth"
	"github.com/context-labs/whip/internal/codexauth"
	"github.com/context-labs/whip/internal/config"
	"github.com/context-labs/whip/internal/llm"
)

// loginCLI implements the compatible `whip login <subscription>` aliases.
func loginCLI(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: whip login <codex|claude>")
	}
	switch args[0] {
	case "codex":
		return authCodexCLI(nil)
	case "claude":
		return authClaudeCLI(nil)
	default:
		return errors.New("usage: whip login <codex|claude>")
	}
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
	return loginCodexAt(ctx, source, out, config.CodexBaseURL)
}

// loginCodexAt is loginCodex with an injectable backend for the authenticated
// catalog test. Production always uses the fixed ChatGPT Codex endpoint.
func loginCodexAt(ctx context.Context, source *codexauth.Source, out io.Writer, baseURL string) error {
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

	client := llm.NewCodex(baseURL, source)
	if source.HTTP != nil { // lets device-login tests use their fake backend
		client.HTTP = source.HTTP
	}
	infos, catalogErr := client.Models(ctx)
	if catalogErr != nil {
		fmt.Fprintln(out, "Codex login saved to ~/.codex/auth.json. gpt-5.4 @ codex is ready in /model.")
		fmt.Fprintln(out, "Codex model catalog could not be fetched yet; run /model refresh after starting Whip:", catalogErr)
		return nil
	}
	if err := saveCatalog(config.CodexProviderName, baseURL, infos); err != nil {
		fmt.Fprintln(out, "Codex login saved to ~/.codex/auth.json. gpt-5.4 @ codex is ready in /model.")
		fmt.Fprintln(out, "Codex model catalog could not be cached; /model refresh will retry:", err)
		return nil
	}
	fmt.Fprintf(out, "Codex login saved to ~/.codex/auth.json. %d account models are ready in /model.\n", len(infos))
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

// authClaudeCLI implements `whip auth claude`; login claude delegates here.
func authClaudeCLI(args []string) error {
	if len(args) != 0 {
		return errors.New("usage: whip auth claude")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return loginClaude(ctx, &claudeauth.Source{}, os.Stdout)
}

func loginClaude(ctx context.Context, source *claudeauth.Source, out io.Writer) error {
	err := source.Login(ctx, func(loginURL string) {
		fmt.Fprintf(out, "\nOpen this URL in a browser and sign in to Claude:\n  %s\n\nContinue only if you started this login in Whip. Press ctrl+c to cancel.\n\n", loginURL)
	})
	if errors.Is(err, context.Canceled) {
		return errors.New("Claude login cancelled")
	}
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("configure Claude provider: %w", err)
	}
	cfg.UpsertClaude()
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("configure Claude provider: %w", err)
	}
	fmt.Fprintln(out, "Claude login saved to ~/.whip/claude.json. claude-sonnet-4-6 @ claude is ready in /model.")
	return nil
}
