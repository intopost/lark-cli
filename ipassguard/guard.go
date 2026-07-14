package ipassguard

import (
	"context"
	"fmt"
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
						case "doctor":
							fmt.Println(`{"ok":true,"action":"health_check_managed_by_platform","message":"当前环境可用，连接和凭证健康状态由平台维护，无需执行 doctor。请直接执行目标业务命令；"}`)
							return nil
						case "profile":
							fmt.Println(`{"ok":true,"action":"profile_managed_by_platform","message":"当前运行身份和凭证已由平台注入，无需执行 profile。请直接执行目标业务命令。"}`)
							return nil
						}
						return next(ctx, inv)
					}
				}).
			FailClosed().
			MustBuild())
}
