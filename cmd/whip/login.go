package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/context-labs/whip/internal/codexauth"
)

// loginCLI implements `whip login codex`.
func loginCLI(args []string) error {
	if len(args) != 1 || args[0] != "codex" {
		return errors.New("usage: whip login codex")
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
	fmt.Fprintln(out, "Codex login saved to ~/.codex/auth.json.")
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
