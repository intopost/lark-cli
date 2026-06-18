package ipassguard

import (
	"context"
	"os"
	"strings"

	"github.com/larksuite/cli/extension/platform"
)

func init() {
	if os.Getenv("AIPOWER_POWER_URL") == "" {
		return
	}
	platform.Register(
		platform.NewPlugin("ipass-guard", "1.0.0").
			Wrap("block-local-cmds", platform.All(),
				func(next platform.Handler) platform.Handler {
					return func(ctx context.Context, inv platform.Invocation) error {
						root := strings.SplitN(inv.Cmd().Path(), " ", 2)[0]
						switch root {
						case "auth", "config", "profile", "doctor":
							return &platform.AbortError{
								HookName: "ipass-guard",
								Reason:   "command not available in iPass proxy mode",
							}
						}
						return next(ctx, inv)
					}
				}).
			FailClosed().
			MustBuild())
}
