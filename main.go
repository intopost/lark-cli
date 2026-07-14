package main

import (
	"fmt"
	"os"

	"github.com/larksuite/cli/cmd"
	_ "github.com/intopost/lark-cli/cred"
	_ "github.com/intopost/lark-cli/ipasscred"
	_ "github.com/intopost/lark-cli/ipassguard"
	_ "github.com/intopost/lark-cli/ipasstrans"
)

func main() {
	if len(os.Args) >= 2 {
		rootCmd := os.Args[1]
		if rootCmd == "auth" || rootCmd == "config" {
			fmt.Println(`{"ok":false,"error":{"type":"validation","subtype":"invalid_argument","message":"\"auth\" is not supported: credentials are provided externally and do not support interactive management"}}`)
			os.Exit(0)
		}
	}
	os.Exit(cmd.Execute())
}
