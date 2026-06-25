package main

import (
	"os"

	"github.com/larksuite/cli/cmd"

	_ "github.com/intopost/lark-cli/ipasscred"
	_ "github.com/intopost/lark-cli/ipassguard"
	_ "github.com/intopost/lark-cli/ipasstrans"
)

func main() {
	os.Exit(cmd.Execute())
}
