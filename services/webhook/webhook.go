package webhook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"shuffle/model"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

type hookConfig struct {
	UriPath     string
	IP string
	Port        string
	CallbackURL string
	ApiKey      string
	HookID      string
}

var config hookConfig

var hook model.Hook

func handleWorkflowAction(request *http.Request, action model.HookAction) error {
	//log.Printf("WORKFLOW!: %#v", action)
	log.Printf("Should execute workflow %s", action.Id)

	fullUrl := fmt.Sprintf("%s/api/v1/workflows/%s/execute", config.CallbackURL, action.Id)

	// ret = requests.post(fullurl, headers=headers, json=data)
	//if ret.status_code != 202:
	//	print(ret.text)
	//	print(ret.status_code)
	//	print("Exiting workflows - run queue")
	//	exit()

	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		return err
	}

	// Execute a workflow
	client := &http.Client{}
	req, err := http.NewRequest(
		"POST",
		fullUrl,
		bytes.NewBuffer(body),
	)

	if err != nil {
		log.Printf("Error making http request: %s", req)
		return err
	}

	req.Header.Add("Authorization", fmt.Sprintf(`Bearer %s`, config.ApiKey))
	req.Header.Add("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error in http request: %s", req)
	}

	log.Printf("%#v", resp)
	return nil
}

// FIXME - refresh hook information once in a while. Compare timestamps or something
func callback(resp http.ResponseWriter, request *http.Request) {
	//apikey = os.Getenv("APIKEY")
	//hookId = os.Getenv("HOOKID")

	handledWorkflowIds := []string{}
	for _, item := range hook.Actions {
		if item.Type == "" {
			log.Printf("CONTINUE AAS EMPTY ITEM: %#v", item)
			continue
		}

		if item.Type == "workflow" {
			found := false
			for _, workflowId := range handledWorkflowIds {
				if item.Id == workflowId {
					found = true
					break
				}
			}

			if found {
				continue
			}

			handledWorkflowIds = append(handledWorkflowIds, item.Id)
			err := handleWorkflowAction(request, item)
			if err != nil {
				log.Printf("Error in workflow exec: %s", err)
			}
		}
	}

	// FIXME - send the webhookdata to a logging service? Idk
	//body, err := ioutil.ReadAll(request.Body)
	//if err != nil {
	//	log.Println("Failed reading body")
	//	resp.WriteHeader(401)
	//	resp.Write([]byte(fmt.Sprintf(`{"success": false}`)))
	//	return
	//}

	//callback, err := http.Post(callbackUrl, "application/json", bytes.NewBuffer(body))
	//if err != nil {
	//	log.Printf("Failed sending callback to %s", callbackUrl)
	//}

	resp.WriteHeader(200)
	resp.Write([]byte(fmt.Sprintf(`{"success": true}`)))
	return
}

func loadConfiguration(fullUrl string, apikey string) error {
	client := &http.Client{}

	req, err := http.NewRequest(
		"GET",
		fullUrl,
		nil,
	)

	if err != nil {
		log.Printf("Error making http request: %s", req)
		return err
	}

	req.Header.Add("Authorization", fmt.Sprintf(`Bearer %s`, apikey))
	req.Header.Add("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error in http request: %s", req)
		return err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response: %s", req)
		return err
	}

	err = json.Unmarshal(body, &hook)
	if err != nil {
		log.Printf("Failed unmarshaling hook API", req)
		return err
	}

	log.Printf("%#v", hook)
	log.Println(hook.Actions)
	return nil
}

func webhook(uriPath, basePort, callbackUrl, apiKey, hookId string) {
	// FIXME - remove static
	ip := "0.0.0.0"

	if len(uriPath) == 0 {
		log.Println("Env URIPATH not set")
		os.Exit(3)
	}

	if len(basePort) == 0 {
		log.Println("Env HOOKPORT not set")
		os.Exit(3)
	}

	if len(callbackUrl) == 0 {
		log.Println("Env CALLBACKURL not set")
		os.Exit(3)
	}

	if len(apiKey) == 0 {
		log.Println("Env APIKEY not set")
		os.Exit(3)
	}

	if len(hookId) == 0 {
		log.Println("Env HOOKID not set")
		os.Exit(3)
	}

	config = hookConfig{
		UriPath:     uriPath,
		IP:          ip,
		Port:        basePort,
		CallbackURL: callbackUrl,
		ApiKey:      apiKey,
		HookID:      hookId,
	}

	log.Println("Loading hook configuration")
	err := loadConfiguration(
		fmt.Sprintf("%s/api/v1/hooks/%s", callbackUrl, hookId),
		apiKey,
	)

	if err != nil {
		log.Fatalf("Error loading config: %s", err)
	}

	// Optional
	// if len(callbackOpts) == 0 {
	// 	log.Println("Env CALLBACKOPTS not set")
	// 	os.Exit(3)
	// }

	port := fmt.Sprintf(":%s", basePort)
	log.Printf("Starting webhook on %s%s with path %s", ip, port, uriPath)

	// Routing
	mux := mux.NewRouter()
	mux.SkipClean(true)

	// FIXME - Add path for updating the hook? Can be a specific POST requeuest from backend
	mux.HandleFunc(uriPath, callback).Methods("POST")

	handlers.LoggingHandler(os.Stdout, mux)
	loggedRouter := handlers.LoggingHandler(os.Stdout, mux)

	err = http.ListenAndServe(
		port,
		loggedRouter,
	)

	if err != nil {
		log.Fatal("ListenAndServer: ", err)
	}
}

func F(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(r.Header.Get("X-Forwarded-For")))
}

