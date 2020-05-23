package worker

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// WorkerCmd represents the Worker command
var WorkerCmd = &cobra.Command{
	Use:   "worker",
	Short: "Shuffle Worker",
	Long: `Worker processes execute workflows.`,
	Run: func(cmd *cobra.Command, args []string) {
		server := viper.GetString("server")
		environment := viper.GetString("environment")
		authorization := viper.GetString("authorization")
		execution := viper.GetString("execution")
		runWorker(server, environment, authorization, execution)
	},
}

func init() {
	WorkerCmd.Flags().StringP("server", "s", "http://127.0.0.1:5001", "API server base URL")
	viper.BindPFlag("server", WorkerCmd.Flags().Lookup("server"))
	viper.SetDefault("server", "http://127.0.0.1:5001")
	WorkerCmd.Flags().StringP("environment", "e", "onprem", "execution environment")
	viper.BindPFlag("environment", WorkerCmd.Flags().Lookup("environment"))
	viper.SetDefault("environment", "onprem")
	WorkerCmd.Flags().StringP("authorization", "a", "", "authorization param")
	viper.BindPFlag("authorization", WorkerCmd.Flags().Lookup("authorization"))
	viper.SetDefault("authorization", "")
	WorkerCmd.Flags().StringP("execution", "i", "", "execution ID")
	viper.BindPFlag("execution", WorkerCmd.Flags().Lookup("execution"))
	viper.SetDefault("execution", "")
}

