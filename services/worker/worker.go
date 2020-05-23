package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"shuffle/model"

	//"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
)

var baseimagename = "frikky/shuffle"
var sleepTime = 2

var baseUrl, environment string

// removes every container except itself (worker)
func shutdown(executionId, workflowId string) {
	dockercli, err := dockerclient.NewEnvClient()
	if err != nil {
		log.Printf("Unable to create docker client: %s", err)
		os.Exit(3)
	}

	containerOptions := types.ContainerListOptions{
		All: true,
	}

	containers, err := dockercli.ContainerList(context.Background(), containerOptions)
	if err != nil {
		panic(err)
	}
	_ = containers

	for _, container := range containers {
		for _, name := range container.Names {
			if strings.Contains(name, executionId) {
				// FIXME - reinstate - not here for debugging
				//err = removeContainer(container.ID)
				//if err != nil {
				//	log.Printf("Failed removing %s before shutdown.", name)
				//}

				break
			}
		}

	}

	// FIXME: Add an API call to the backend
	// fmt.Sprintf("AUTHORIZATION=%s", workflowExecution.Authorization),

	fullUrl := fmt.Sprintf("%s/api/v1/workflows/%s/executions/%s/abort", baseUrl, workflowId, executionId)
	req, err := http.NewRequest(
		"GET",
		fullUrl,
		nil,
	)

	if err != nil {
		log.Println("Failed building request: %s", err)
	}

	req.Header.Add("Content-Type", "application/json")
	//req.Header.Add("Authorization", authorization)
	client := &http.Client{}
	_, err = client.Do(req)
	if err != nil {
		log.Printf("Failed abort request: %s", err)
	}

	log.Printf("Finished shutdown.")
	os.Exit(3)
}

// Deploys the internal worker whenever something happens
func deployApp(cli *dockerclient.Client, image string, identifier string, env []string) error {
	hostConfig := &container.HostConfig{
		LogConfig: container.LogConfig{
			Type:   "json-file",
			Config: map[string]string{},
		},
	}

	config := &container.Config{
		Image: image,
		Env:   env,
	}

	cont, err := cli.ContainerCreate(
		context.Background(),
		config,
		hostConfig,
		nil,
		identifier,
	)

	if err != nil {
		log.Println(err)
		return err
	}

	cli.ContainerStart(context.Background(), cont.ID, types.ContainerStartOptions{})
	fmt.Printf("\n")
	log.Printf("Container %s is created", cont.ID)
	return nil
}

func removeContainer(containername string) error {
	ctx := context.Background()

	cli, err := dockerclient.NewEnvClient()
	if err != nil {
		log.Printf("Unable to create docker client: %s", err)
		return err
	}

	// FIXME - ucnomment
	//	containers, err := cli.ContainerList(ctx, types.ContainerListOptions{
	//		All: true,
	//	})

	_ = ctx
	_ = cli
	//if err := cli.ContainerStop(ctx, containername, nil); err != nil {
	//	log.Printf("Unable to stop container %s - running removal anyway, just in case: %s", containername, err)
	//}

	removeOptions := types.ContainerRemoveOptions{
		RemoveVolumes: true,
		Force:         true,
	}

	// FIXME - remove comments etc
	_ = removeOptions
	//if err := cli.ContainerRemove(ctx, containername, removeOptions); err != nil {
	//	log.Printf("Unable to remove container: %s", err)
	//}

	return nil
}

func runFilter(workflowExecution model.WorkflowExecution, action model.Action) {
	// 1. Get the parameter $.#.id
	if action.Label == "filter_cases" && len(action.Parameters) > 0 {
		if action.Parameters[0].Variant == "ACTION_RESULT" {
			//param := action.Parameters[0]
			//value := param.Value

			// Loop cases.. Hmm, that's tricky
		}
	} else {
		log.Printf("No handler for filter %s with %d params", action.Label, len(action.Parameters))
	}

}

func handleExecution(client *http.Client, req *http.Request, workflowExecution model.WorkflowExecution) error {
	// if no onprem runs (shouldn't happen, but extra check), exit
	// if there are some, load the images ASAP for the app
	dockercli, err := dockerclient.NewEnvClient()
	if err != nil {
		log.Printf("Unable to create docker client: %s", err)
		shutdown(workflowExecution.ExecutionId, workflowExecution.Workflow.ID)
	}

	onpremApps := []string{}
	startAction := workflowExecution.Workflow.Start
	toExecuteOnprem := []string{}
	parents := map[string][]string{}
	children := map[string][]string{}

	// source = parent, dest = child
	// parent can have more children, child can have more parents
	for _, branch := range workflowExecution.Workflow.Branches {
		parents[branch.DestinationID] = append(parents[branch.DestinationID], branch.SourceID)
		children[branch.SourceID] = append(children[branch.SourceID], branch.DestinationID)
	}

	for _, action := range workflowExecution.Workflow.Actions {
		if action.Environment != environment {
			continue
		}

		toExecuteOnprem = append(toExecuteOnprem, action.ID)
		actionName := fmt.Sprintf("%s:%s_%s", baseimagename, action.AppName, action.AppVersion)
		found := false
		for _, app := range onpremApps {
			if actionName == app {
				found = true
			}
		}

		if !found {
			onpremApps = append(onpremApps, actionName)
		}
	}

	if len(onpremApps) == 0 {
		return errors.New("No apps to handle onprem")
	}

	pullOptions := types.ImagePullOptions{}
	for _, image := range onpremApps {
		log.Printf("Image: %s", image)
		if strings.Contains(image, " ") {
			image = strings.ReplaceAll(image, " ", "-")
		}

		reader, err := dockercli.ImagePull(context.Background(), image, pullOptions)
		if err != nil {
			log.Printf("Failed getting %s. The app is missing or some other issue", image)
			//shutdown(workflowExecution.ExecutionId)
		}

		//io.Copy(os.Stdout, reader)
		_ = reader
		log.Printf("Successfully downloaded and built %s", image)
	}

	// Process the parents etc. How?
	// while queue:
	// while len(self.in_process) > 0 or len(self.parallel_in_process) > 0:
	// check if its their own turn to continue
	// visited = {self.start_action}
	visited := []string{}
	nextActions := []string{}
	queueNodes := []string{}

	for {
		//if len(queueNodes) > 0 {
		//	log.Println(queueNodes)
		//	nextActions = queueNodes
		//} else {
		//	nextActions := []string{}
		//}
		// FIXME - this might actually work, but probably not
		//queueNodes = []string{}

		if len(workflowExecution.Results) == 0 {
			nextActions = []string{startAction}
		} else {
			for _, item := range workflowExecution.Results {
				visited = append(visited, item.Action.ID)
				nextActions = children[item.Action.ID]
				// FIXME: check if nextActions items are finished?
			}
		}

		if len(nextActions) == 0 {
			log.Println("No next action. Finished?")
			//shutdown(workflowExecution.ExecutionId)
		}

		for _, node := range nextActions {
			nodeChildren := children[node]
			for _, child := range nodeChildren {
				if !arrayContains(queueNodes, child) {
					queueNodes = append(queueNodes, child)
				}
			}
		}

		//log.Println(queueNodes)

		// IF NOT VISITED && IN toExecuteOnPrem
		// SKIP if it's not onprem
		// FIXME: Find next node(s)
		//for _, result := range workflowExecution.Results {
		//	log.Println(result.Status)
		//}

		for _, nextAction := range nextActions {
			action := getAction(workflowExecution, nextAction)
			// FIXME - remove this. Should always need to be valid.
			//if action.IsValid == false {
			//	log.Printf("%#v", action)
			//	log.Printf("Action %s (%s) isn't valid. Exiting, BUT SHOULD CALLBACK TO SET FAILURE.", action.ID, action.Name)
			//	os.Exit(3)
			//}

			// check visited and onprem
			if arrayContains(visited, nextAction) {
				log.Printf("ALREADY VISITIED: %s", nextAction)
				continue
			}

			// Not really sure how this edgecase happens.

			// FIXME
			// Execute, as we don't really care if env is not set? IDK
			if action.Environment != environment { //&& action.Environment != "" {
				log.Printf("Bad environment: %s", action.Environment)
				continue
			}

			// check whether the parent is finished executing
			//log.Printf("%s has %d parents", nextAction, len(parents[nextAction]))

			continueOuter := true
			if action.IsStartNode {
				continueOuter = false
			} else if len(parents[nextAction]) > 0 {
				// FIXME - wait for parents to finishe executing
				fixed := 0
				for _, parent := range parents[nextAction] {
					parentResult := getResult(workflowExecution, parent)
					if parentResult.Status == "FINISHED" || parentResult.Status == "SUCCESS" {
						fixed += 1
					}
				}

				if fixed == len(parents[nextAction]) {
					continueOuter = false
				}
			} else {
				continueOuter = false
			}

			if continueOuter {
				log.Printf("Parents of %s aren't finished: %s", nextAction, strings.Join(parents[nextAction], ", "))
				continue
			}

			// get action status
			actionResult := getResult(workflowExecution, nextAction)
			if actionResult.Action.ID == action.ID {
				log.Printf("%s already has status %s.", action.ID, actionResult.Status)
				continue
			} else {
				log.Printf("%s:%s has no status result yet. Should execute.", action.Name, action.ID)
			}

			appname := action.AppName
			appversion := action.AppVersion
			appname = strings.Replace(appname, ".", "-", -1)
			appversion = strings.Replace(appversion, ".", "-", -1)

			image := fmt.Sprintf("%s:%s_%s", baseimagename, action.AppName, action.AppVersion)
			if strings.Contains(image, " ") {
				image = strings.ReplaceAll(image, " ", "-")
			}

			identifier := fmt.Sprintf("%s_%s_%s_%s", appname, appversion, action.ID, workflowExecution.ExecutionId)
			if strings.Contains(identifier, " ") {
				identifier = strings.ReplaceAll(identifier, " ", "-")
			}

			// FIXME - check whether it's running locally yet too
			stats, err := dockercli.ContainerInspect(context.Background(), identifier)
			if err != nil || stats.ContainerJSONBase.State.Status != "running" {
				// REMOVE
				if err == nil {
					log.Printf("Status: %s, should kill: %s", stats.ContainerJSONBase.State.Status, identifier)
					err = removeContainer(identifier)
					if err != nil {
						log.Printf("Error killing container: %s", err)
					}
				} else {
					//log.Printf("WHAT TO DO HERE?: %s", err)
				}
			} else if stats.ContainerJSONBase.State.Status == "running" {
				continue
			}

			if len(action.Parameters) == 0 {
				action.Parameters = []model.WorkflowAppActionParameter{}
			}

			if len(action.Errors) == 0 {
				action.Errors = []string{}
			}

			// marshal action and put it in there rofl
			log.Printf("Time to execute %s with app %s:%s, function %s, env %s with %d parameters.", action.ID, action.AppName, action.AppVersion, action.Name, action.Environment, len(action.Parameters))

			actionData, err := json.Marshal(action)
			if err != nil {
				log.Printf("Failed unmarshalling action: %s", err)
				continue
			}

			if action.AppID == "0ca8887e-b4af-4e3e-887c-87e9d3bc3d3e" {
				log.Printf("\nShould run filter: %#v\n\n", action)
				runFilter(workflowExecution, action)
				continue
			}

			//log.Println(string(actionData))
			// FIXME - add proper FUNCTION_APIKEY from user definition
			env := []string{
				fmt.Sprintf("ACTION=%s", string(actionData)),
				fmt.Sprintf("EXECUTIONID=%s", workflowExecution.ExecutionId),
				fmt.Sprintf("FUNCTION_APIKEY=%s", "asdasd"),
				fmt.Sprintf("AUTHORIZATION=%s", workflowExecution.Authorization),
				fmt.Sprintf("CALLBACK_URL=%s", baseUrl),
			}

			err = deployApp(dockercli, image, identifier, env)
			if err != nil {
				log.Printf("Failed deploying %s from image %s: %s", identifier, image, err)
				log.Printf("Should send status and exit the entire thing?")
				//shutdown(workflowExecution.ExecutionId)
			}

			visited = append(visited, action.ID)
			//log.Printf("%#v", action)
		}

		//log.Println(nextAction)
		//log.Println(startAction, children[startAction])

		// FIXME - new request here
		// FIXME - clean up stopped (remove) containers with this execution id
		newresp, err := client.Do(req)
		if err != nil {
			log.Printf("Failed making request: %s", err)
			time.Sleep(time.Duration(sleepTime) * time.Second)
			continue
		}

		body, err := ioutil.ReadAll(newresp.Body)
		if err != nil {
			log.Printf("Failed reading body: %s", err)
			time.Sleep(time.Duration(sleepTime) * time.Second)
			continue
		}

		if newresp.StatusCode != 200 {
			log.Printf("Err: %s\nStatusCode: %d", string(body), newresp.StatusCode)
			time.Sleep(time.Duration(sleepTime) * time.Second)
			continue
		}

		err = json.Unmarshal(body, &workflowExecution)
		if err != nil {
			log.Printf("Failed workflowExecution unmarshal: %s", err)
			time.Sleep(time.Duration(sleepTime) * time.Second)
			continue
		}

		if workflowExecution.Status == "FINISHED" || workflowExecution.Status == "SUCCESS" {
			log.Printf("Workflow %s is finished. Exiting worker.", workflowExecution.ExecutionId)
			shutdown(workflowExecution.ExecutionId, workflowExecution.Workflow.ID)
		}

		log.Printf("Status: %s, Results: %d, actions: %d", workflowExecution.Status, len(workflowExecution.Results), len(workflowExecution.Workflow.Actions))
		if workflowExecution.Status != "EXECUTING" {
			log.Printf("Exiting as worker execution has status %s!", workflowExecution.Status)
			shutdown(workflowExecution.ExecutionId, workflowExecution.Workflow.ID)
		}

		if len(workflowExecution.Results) == len(workflowExecution.Workflow.Actions) {
			shutdownCheck := true
			ctx := context.Background()
			for _, result := range workflowExecution.Results {
				if result.Status == "EXECUTING" {
					// Cleaning up executing stuff
					shutdownCheck = false
					// Check status

					containers, err := dockercli.ContainerList(ctx, types.ContainerListOptions{
						All: true,
					})
					if err != nil {
						log.Printf("Failed listing containers: %s", err)
						continue
					}

					stopContainers := []string{}
					removeContainers := []string{}
					for _, container := range containers {
						for _, name := range container.Names {
							if !strings.Contains(name, result.Action.ID) {
								continue
							}

							if container.State != "running" {
								removeContainers = append(removeContainers, container.ID)
								stopContainers = append(stopContainers, container.ID)
							}
						}
					}

					// FIXME - add killing of apps with same execution ID too
					// FIXME - stahp
					//for _, containername := range stopContainers {
					//	if err := dockercli.ContainerStop(ctx, containername, nil); err != nil {
					//		log.Printf("Unable to stop container: %s", err)
					//	} else {
					//		log.Printf("Stopped container %s", containername)
					//	}
					//}

					removeOptions := types.ContainerRemoveOptions{
						RemoveVolumes: true,
						Force:         true,
					}

					_ = removeOptions

					// FIXME - this
					//for _, containername := range removeContainers {
					//	if err := dockercli.ContainerRemove(ctx, containername, removeOptions); err != nil {
					//		log.Printf("Unable to remove container: %s", err)
					//	} else {
					//		log.Printf("Removed container %s", containername)
					//	}
					//}

					//  FIXME - send POST request to kill the container
					log.Printf("Should remove (POST request) stopped containers")
					//ret = requests.post("%s%s" % (self.url, stream_path), headers=headers, json=action_result)
				}
			}

			if shutdownCheck {
				log.Println("BREAKING BECAUSE RESULTS IS SAME LENGTH AS ACTIONS. SHOULD CHECK ALL RESULTS FOR WHETHER THEY'RE DONE")
				shutdown(workflowExecution.ExecutionId, workflowExecution.Workflow.ID)
			}
		}
		time.Sleep(time.Duration(sleepTime) * time.Second)
	}

	return nil
}

func arrayContains(visited []string, id string) bool {
	found := false
	for _, item := range visited {
		if item == id {
			found = true
		}
	}

	return found
}

func getResult(workflowExecution model.WorkflowExecution, id string) model.ActionResult {
	for _, actionResult := range workflowExecution.Results {
		if actionResult.Action.ID == id {
			return actionResult
		}
	}

	return model.ActionResult{}
}

func getAction(workflowExecution model.WorkflowExecution, id string) model.Action {
	for _, action := range workflowExecution.Workflow.Actions {
		if action.ID == id {
			return action
		}
	}

	return model.Action{}
}

// Initial loop etc
func runWorker(baseUrlParam string, environmentParam string, authorization string, executionId string) {
	log.Printf("Setting up worker environment")
	baseUrl = baseUrlParam
	environment = environmentParam
	sleepTime := 5
	client := &http.Client{}

	if len(authorization) == 0 {
		log.Println("No AUTHORIZATION key set in env")
		shutdown(executionId, "")
	}

	if len(executionId) == 0 {
		log.Println("No EXECUTIONID key set in env")
		shutdown(executionId, "")
	}

	// FIXME - tmp
	data := fmt.Sprintf(`{"execution_id": "%s", "authorization": "%s"}`, executionId, authorization)
	fullUrl := fmt.Sprintf("%s/api/v1/streams/results", baseUrl)
	req, err := http.NewRequest(
		"POST",
		fullUrl,
		bytes.NewBuffer([]byte(data)),
	)

	if err != nil {
		log.Println("Failed making request builder")
		shutdown(executionId, "")
	}

	for {
		newresp, err := client.Do(req)
		if err != nil {
			log.Printf("Failed request: %s", err)
			time.Sleep(time.Duration(sleepTime) * time.Second)
			continue
		}

		body, err := ioutil.ReadAll(newresp.Body)
		if err != nil {
			log.Printf("Failed reading body: %s", err)
			time.Sleep(time.Duration(sleepTime) * time.Second)
			continue
		}

		if newresp.StatusCode != 200 {
			log.Printf("Err: %s\nStatusCode: %d", string(body), newresp.StatusCode)
			time.Sleep(time.Duration(sleepTime) * time.Second)
			continue
		}

		var workflowExecution model.WorkflowExecution
		err = json.Unmarshal(body, &workflowExecution)
		if err != nil {
			log.Printf("Failed workflowExecution unmarshal: %s", err)
			time.Sleep(time.Duration(sleepTime) * time.Second)
			continue
		}

		if workflowExecution.Status == "FINISHED" || workflowExecution.Status == "SUCCESS" {
			log.Printf("Workflow %s is finished. Exiting worker.", workflowExecution.ExecutionId)
			shutdown(executionId, workflowExecution.Workflow.ID)
		}

		if workflowExecution.Status == "EXECUTING" || workflowExecution.Status == "RUNNING" {
			//log.Printf("Status: %s", workflowExecution.Status)
			err = handleExecution(client, req, workflowExecution)
			if err != nil {
				log.Printf("Workflow %s is finished: %s", workflowExecution.ExecutionId, err)
				shutdown(executionId, workflowExecution.Workflow.ID)
			}
		} else {
			log.Printf("Workflow %s has status %s. Exiting worker.", workflowExecution.ExecutionId, workflowExecution.Status)
			shutdown(executionId, workflowExecution.Workflow.ID)
		}

		//log.Println(string(body))
		time.Sleep(time.Duration(sleepTime) * time.Second)
	}
}
