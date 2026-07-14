package main

import (
	"fmt"
	"os"

	_ "github.com/intopost/lark-cli/cred"
	_ "github.com/intopost/lark-cli/ipasscred"
	_ "github.com/intopost/lark-cli/ipassguard"
	_ "github.com/intopost/lark-cli/ipasstrans"
	"github.com/larksuite/cli/cmd"
)

func main() {
	// 当前分发包由 @intopost/lark-cli 负责升级，关闭官方包的升级提示。
	_ = os.Setenv("LARKSUITE_CLI_NO_UPDATE_NOTIFIER", "1")

	if len(os.Args) >= 2 {
		rootCmd := os.Args[1]
		if rootCmd == "auth" || rootCmd == "config" || rootCmd == "update" {
			action := "managed_by_platform"
			message := ""
			switch rootCmd {
			case "auth":
				action = "platform_authorization_required"
				message = "当前环境可用，无需在 CLI 中执行 auth。请通过平台完成授权，授权成功后直接重试原业务命令。"
			case "config":
				action = "no_configuration_required"
				message = "当前环境已由平台完成配置，无需执行 config。请直接执行目标业务命令。"
			case "update":
				action = "manual_update_required"
				message = "当前环境可正常使用，无需执行内置 update。如需更新，请运行 bun add -g @intopost/lark-cli@latest。"
			}
			fmt.Printf(`{"ok":true,"action":%q,"message":%q}`+"\n", action, message)
			os.Exit(0)
		}
	}
	os.Exit(cmd.Execute())
}
