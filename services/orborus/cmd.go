package orborus

import (
	"fmt"
	"github.com/spf13/cobra"
)

// orborusCmd represents the Orborus command
var OrborusCmd = &cobra.Command{
	Use:   "orborus",
	Short: "Shuffle Orborus server",
	Long: `Orborus exists to listen for new workflow executions and deploy workers.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("orborus called")
		flags := cmd.Flags()
		server, err := flags.GetString("server")
		if err != nil {
			fmt.Printf("Invalid `server` argument: %s", err)
			return
		}
		organization, err := flags.GetString("organization")
		if err != nil {
			fmt.Printf("Invalid `organization` argument: %s", err)
			return
		}
		environment, err := flags.GetString("environment")
		if err != nil {
			fmt.Printf("Invalid `environment` argument: %s", err)
			return
		}
		dockerApiVersion, err := flags.GetString("docker-api-version")
		if err != nil {
			fmt.Printf("Invalid `docker_api_version` argument: %s", err)
			return
		}
		runOrborus(server, organization, environment, dockerApiVersion)
	},
}

func init() {

	OrborusCmd.Flags().StringP("server", "s", "http://127.0.0.1:5001", "API server base URL")
	OrborusCmd.Flags().StringP("organization", "o", "Shuffle", "organization ID")
	OrborusCmd.Flags().StringP("environment", "e", "onprem", "execution environment")
	OrborusCmd.Flags().StringP("docker-api-version", "a", "1.40", "Docker API version")
}

