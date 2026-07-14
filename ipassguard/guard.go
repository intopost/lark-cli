package ipassguard

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/larksuite/cli/extension/platform"
)

func init() {
	platform.Register(
		platform.NewPlugin("ipass-guard", "1.0.0").
			Wrap("block-local-cmds", platform.All(),
				func(next platform.Handler) platform.Handler {
					return func(ctx context.Context, inv platform.Invocation) error {
						root := strings.SplitN(inv.Cmd().Path(), "/", 2)[0]
						switch root {
						case "doctor", "update", "profile":
							fmt.Println(`{"ok":false,"error":{"type":"validation","subtype":"invalid_argument","message":"command not available in iPass proxy mode"}}`)
							os.Exit(-1)
							return nil
						}
						return next(ctx, inv)
					}
				}).
			FailClosed().
			MustBuild())
}

