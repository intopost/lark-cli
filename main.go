package main

import (
	"os"

	"github.com/larksuite/cli/cmd"

	_ "lark-cli-ipass/ipasscred"
	_ "lark-cli-ipass/ipassguard"
	_ "lark-cli-ipass/ipasstrans"
)

func main() {
	os.Exit(cmd.Execute())
}
