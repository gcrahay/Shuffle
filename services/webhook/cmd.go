package webhook

import (
	"fmt"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// WorkerCmd represents the Worker command
var WebhookCmd = &cobra.Command{
	Use:   "webhook",
	Short: "Shuffle Web hook listener",
	Long: `Web hook processes receive data from external apps.`,
	Run: func(cmd *cobra.Command, args []string) {
		server := viper.GetString("server")
		port := viper.GetUint("port")
		apiKey := viper.GetString("api-key")
		hookID := viper.GetString("id")
		uri := viper.GetString("uri")
		webhook(uri, fmt.Sprintf("%d", port), server, apiKey, hookID)
	},
}

func init() {
	WebhookCmd.Flags().StringP("server", "s", "http://127.0.0.1:5001", "API server base URL")
	viper.BindPFlag("server", WebhookCmd.Flags().Lookup("server"))
	viper.SetDefault("server", "http://127.0.0.1:5001")

	WebhookCmd.Flags().Uint16P("port", "p", 5001, "listening port")
	viper.BindPFlag("port", WebhookCmd.Flags().Lookup("port"))
	viper.SetDefault("port", 5002)

	WebhookCmd.Flags().StringP("api-key", "a", "", "API key")
	viper.BindPFlag("api-key", WebhookCmd.Flags().Lookup("api-key"))
	viper.SetDefault("api-key", "")

	WebhookCmd.Flags().StringP("id", "i", "", "hook ID")
	viper.BindPFlag("id", WebhookCmd.Flags().Lookup("id"))
	viper.SetDefault("id", "")

	WebhookCmd.Flags().StringP("uri", "u", "/", "hook URI (path)")
	viper.BindPFlag("uri", WebhookCmd.Flags().Lookup("uri"))
	viper.SetDefault("uri", "/")
}

