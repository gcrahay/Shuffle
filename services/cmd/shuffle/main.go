package main

import (
	"os"
	"shuffle/cmd"
	"shuffle/orborus"
	_ "shuffle/server"
	"shuffle/webhook"
	"shuffle/worker"
	"strings"
)

func init() {
	cmd.RootCmd.AddCommand(orborus.OrborusCmd)
	cmd.RootCmd.AddCommand(worker.WorkerCmd)
	cmd.RootCmd.AddCommand(webhook.WebhookCmd)
}

func main() {
		command := strings.ToLower(os.Getenv("SHUFFLE_COMMAND"))
		if len(command) > 0 {
			var commandToRun string
			subCommands := cmd.RootCmd.Commands()
			for _, sub := range subCommands {
				if command == sub.Name() {
					commandToRun = sub.Name()
					break
				}
				for _, alias := range sub.Aliases {
					if command == alias {
						commandToRun = sub.Name()
						break
					}
				}
				if len(commandToRun) > 0 {
					break
				}
			}
			if len(commandToRun) > 0 {
				cmd.RootCmd.SetArgs([]string{commandToRun})
			}
		}
	cmd.Execute()
}