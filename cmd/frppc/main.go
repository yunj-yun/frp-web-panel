package main

import (
	"github.com/yunj-yun/frp-web-panel/cmd/frpp/shared"
	"github.com/yunj-yun/frp-web-panel/conf"
	"github.com/yunj-yun/frp-web-panel/utils/logger"
	"github.com/fatedier/golib/crypto"
	"github.com/spf13/cobra"
)

func main() {
	crypto.DefaultSalt = "frp"
	logger.InitLogger()
	cobra.MousetrapHelpText = ""

	rootCmd := shared.NewRootCmd(
		shared.NewClientCmd(conf.NewConfig()),
		shared.NewJoinCmd(),
		shared.NewInstallServiceCmd(),
		shared.NewUninstallServiceCmd(),
		shared.NewStartServiceCmd(),
		shared.NewStopServiceCmd(),
		shared.NewRestartServiceCmd(),
		shared.NewVersionCmd(),
	)

	shared.SetClientCommandIfNonePresent(rootCmd)
	rootCmd.Execute()
}
