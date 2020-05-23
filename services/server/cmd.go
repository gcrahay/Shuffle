package server

import (
	"fmt"
	"github.com/asdine/storm/v3"
	"github.com/asdine/storm/v3/codec/gob"
	newscheduler "github.com/carlescere/scheduler"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/gorilla/mux"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"shuffle/cmd"
	"shuffle/model"
	"strings"
)

var dbClient *storm.DB

func openDatabase(dbPath string) (*storm.DB, error) {
	return storm.Open(dbPath, storm.Codec(gob.Codec))
}

func setupRouter() *mux.Router {
	r := mux.NewRouter()
	r.HandleFunc("/api/v1/_ah/health", healthCheckHandler)

	// Sends an email if the right things are specified
	r.HandleFunc("/functions/sendmail", handleSendalert).Methods("POST", "OPTIONS")
	r.HandleFunc("/functions/outlook/register", handleNewOutlookRegister).Methods("GET", "OPTIONS")
	r.HandleFunc("/functions/outlook/getFolders", handleGetOutlookFolders).Methods("GET", "OPTIONS")

	// General
	r.HandleFunc("/api/v1/login", handleLogin).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/v1/logout", handleLogout).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/v1/register", handleRegister).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/v1/checkusers", checkAdminLogin).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/v1/getusers", handleGetUsers).Methods("GET", "OPTIONS")

	r.HandleFunc("/api/v1/getenvironments", handleGetEnvironments).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/v1/setenvironments", handleSetEnvironments).Methods("PUT", "OPTIONS")
	r.HandleFunc("/api/v1/getinfo", handleInfo).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/v1/getsettings", handleSettings).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/v1/generateapikey", handleApiGeneration).Methods("GET", "OPTIONS")

	r.HandleFunc("/api/v1/docs", getDocList).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/v1/docs/{key}", getDocs).Methods("GET", "OPTIONS")

	// Queuebuilder and Workflow streams. First is to update a stream, second to get a stream
	// Changed from workflows/streams to streams, as appengine was messing up
	// This does not increase the API counter
	r.HandleFunc("/api/v1/workflows/queue", handleGetWorkflowqueue).Methods("GET")
	r.HandleFunc("/api/v1/workflows/queue/confirm", handleGetWorkflowqueueConfirm).Methods("POST")
	r.HandleFunc("/api/v1/streams", handleWorkflowQueue).Methods("POST")
	r.HandleFunc("/api/v1/streams/results", handleGetStreamResults).Methods("POST", "OPTIONS")

	// Apps
	r.HandleFunc("/api/v1/apps/get_existing", loadExistingApps).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/v1/apps/get_existing/{appname}", loadExistingApps).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/v1/apps/validate", validateAppInput).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/v1/apps/{appId}", deleteWorkflowApp).Methods("DELETE", "OPTIONS")
	r.HandleFunc("/api/v1/apps/{appId}/config", getWorkflowAppConfig).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/v1/apps", getWorkflowApps).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/v1/apps", setNewWorkflowApp).Methods("PUT", "OPTIONS")

	// Legacy things
	r.HandleFunc("/api/v1/workflows/apps/validate", validateAppInput).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/v1/workflows/apps", getWorkflowApps).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/v1/workflows/apps", setNewWorkflowApp).Methods("PUT", "OPTIONS")

	// Workflows
	// FIXME - implement the queue counter lol
	/* Everything below here increases the counters*/
	r.HandleFunc("/api/v1/workflows", getWorkflows).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/v1/workflows", setNewWorkflow).Methods("POST", "OPTIONS")
	//r.HandleFunc("/api/v1/workflows/{key}/execute_fs", executeWorkflowFS)
	r.HandleFunc("/api/v1/workflows/{key}/execute", executeWorkflow).Methods("GET", "POST", "OPTIONS")
	r.HandleFunc("/api/v1/workflows/{key}/schedule", scheduleWorkflow).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/v1/workflows/{key}/schedule/{schedule}", stopSchedule).Methods("DELETE", "OPTIONS")
	r.HandleFunc("/api/v1/workflows/{key}/outlook", createOutlookSub).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/v1/workflows/{key}/outlook/{triggerId}", handleDeleteOutlookSub).Methods("DELETE", "OPTIONS")
	r.HandleFunc("/api/v1/workflows/{key}/executions", getWorkflowExecutions).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/v1/workflows/{key}/executions/{key}/abort", abortExecution).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/v1/workflows/{key}", getSpecificWorkflow).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/v1/workflows/{key}", saveWorkflow).Methods("PUT", "OPTIONS")
	r.HandleFunc("/api/v1/workflows/{key}", deleteWorkflow).Methods("DELETE", "OPTIONS")

	// Triggers
	// Webhook redirect to the correct cloud function
	r.HandleFunc("/api/v1/hooks/new", handleNewHook).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/v1/hooks/{key}", handleWebhookCallback).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/v1/hooks/{key}/delete", handleDeleteHook).Methods("DELETE", "OPTIONS")

	// Trigger hmm
	r.HandleFunc("/api/v1/triggers/{key}", handleGetSpecificTrigger).Methods("GET", "OPTIONS")

	r.HandleFunc("/api/v1/stats/{key}", handleGetSpecificStats).Methods("GET", "OPTIONS")

	// OpenAPI configuration
	r.HandleFunc("/api/v1/verify_swagger", verifySwagger).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/v1/verify_openapi", verifySwagger).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/v1/get_openapi_uri", echoOpenapiData).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/v1/validate_openapi", validateSwagger).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/v1/get_openapi/{key}", getOpenapi).Methods("GET", "OPTIONS")

	r.HandleFunc("/api/v1/execution_cleanup", cleanupExecutions).Methods("GET", "OPTIONS")

	return r
}

func SetupServer(dbPath string) (*mux.Router, error) {
	var err error

	log.Printf("Running INIT process")

	dbClient, err = openDatabase(dbPath)
	if err != nil {
		log.Fatalf("DB error during init: %s", err)
	}

	err = increaseStatisticsField("backend_executions", "", 1)
	if err != nil {
		log.Printf("Failed increasing local stats: %s", err)
	}

	// Gets environments and inits if it doesn't exist
	count, err := getEnvironmentCount()
	if err != nil {
		log.Printf("Cannot get environment count: %s", err)
	}
	if count == 0 && err == nil {
		item := model.Environment{
			Name: "Shuffle",
			Type: "onprem",
		}

		err = setEnvironment(&item)
		if err != nil {
			log.Printf("Failed setting up new environment")
		}
	}

	// Getting apps to see if we should initialize a test
	workflowapps, err := getAllWorkflowApps()
	if err != nil {
		log.Printf("Failed getting apps: %s", err)
	} else if err == nil && len(workflowapps) == 0 {
		log.Printf("Apps: loading TEST")
		fs := memfs.New()
		storer := memory.NewStorage()
		r, err := git.Clone(storer, fs, &git.CloneOptions{
			URL: "https://github.com/frikky/shuffle-apps",
		})

		if err != nil {
			log.Printf("Failed loading repo into memory: %s", err)
		}

		dir, err := fs.ReadDir("")
		if err != nil {
			log.Printf("FAiled reading folder: %s", err)
		}
		_ = r
		iterateAppGithubFolders(fs, dir, "", "testing")
	}

	// Gets schedules and starts them
	schedules, err := getAllSchedules()
	if err != nil {
		log.Printf("Failed getting schedules during service init: %s", err)
	} else {
		log.Printf("Setting up %d schedule(s)", len(schedules))
		for _, schedule := range schedules {
			job := func() {
				request := &http.Request{
					Method: "POST",
					Body:   ioutil.NopCloser(strings.NewReader(schedule.Argument)),
				}

				_, _, err := handleExecution(schedule.WorkflowId, model.Workflow{}, request)
				if err != nil {
					log.Printf("Failed to execute: %s", err)
				}
			}

			jobret, err := newscheduler.Every(schedule.Seconds).Seconds().NotImmediately().Run(job)
			if err != nil {
				log.Printf("Failed to schedule workflow: %s", err)
				// FIXME: what now? lol:w
			}

			scheduledJobs[schedule.Id] = jobret
		}
	}

	log.Printf("Finished INIT")
	return setupRouter(), err
}


// serverCmd represents the server command
var serverCmd = &cobra.Command{
	Use:   "server",
	Aliases: []string{"backend"},
	Short: "Shuffle API server",
	Long: `The backend server provides the core feature to the Shuffle platform..`,
	Run: func(cmd *cobra.Command, args []string) {
		dbPath := viper.GetString("database")

		router, err := SetupServer(dbPath)
		if err != nil {
			fmt.Printf("Cannot setup server: %s", err)
			return
		}

		hostname, err := os.Hostname()
		if err != nil {
			hostname = "MISSING-HOSTNAME"
		}

		http.Handle("/", router)

		innerPort := viper.GetUint("port")
		if err != nil {
			fmt.Printf("Cannot get listening port: %s", err)
			return
		}
		log.Printf("Running on %s:%d", hostname, innerPort)
		log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", innerPort), nil))
	},
}

func init() {

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// serverCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	serverCmd.Flags().StringP("database", "d", "shuffle.db", "database file")
	viper.BindPFlag("database", serverCmd.Flags().Lookup("database"))
	viper.SetDefault("database", "shuffle.db")
	serverCmd.Flags().StringP("listen", "l", "", "API server listening address or host")
	viper.BindPFlag("listen", serverCmd.Flags().Lookup("listen"))
	viper.SetDefault("listen", "")
	serverCmd.Flags().Uint16P("port", "p", 5001, "API server listening port")
	viper.BindPFlag("port", serverCmd.Flags().Lookup("port"))
	viper.SetDefault("port", 5001)

	cmd.RootCmd.AddCommand(serverCmd)
}
