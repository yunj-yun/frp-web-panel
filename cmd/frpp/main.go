package main

import (
	"embed"

	"github.com/yunj-yun/frp-web-panel/cmd/frpp/shared"
	"github.com/yunj-yun/frp-web-panel/utils/logger"
	"github.com/fatedier/golib/crypto"
	"github.com/spf13/cobra"
)

//go:embed all:out
var fs embed.FS

func main() {
	crypto.DefaultSalt = "frp"
	logger.InitLogger()
	cobra.MousetrapHelpText = ""

	rootCmd := shared.BuildCommand(fs)
	shared.SetMasterCommandIfNonePresent(rootCmd)
	rootCmd.Execute()
}
