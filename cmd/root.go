package cmd

import (
	"dfile-secondary-node/account"
	"fmt"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "dfile-secondary-node",
	Short: "dfile-secondary-node provides unused space for users that need it",
	Long: `dfile-secondary-node is a CLI application that allows a user (miner) 
	to connect to the DeNet decentralized network and mine DFile tokens by granting access to their avaliable space for other users.`,
	Run: func(cmd *cobra.Command, args []string) {
		accs := account.List()

		if len(accs) == 0 {
			accountCreateCmd.Run(accountCreateCmd, []string{})
		} else if len(accs) == 1 {
			accountLoginCmd.Run(accountLoginCmd, []string{accs[0]})
		} else {
			fmt.Println("Please enter account address you want to log in:")
			accountListCmd.Run(accountListCmd, []string{})
			accountLoginCmd.Run(accountLoginCmd, []string{})
		}

	},
}

func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}

func init() {
}
