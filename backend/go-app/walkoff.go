package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/asdine/storm/v3"
	"github.com/asdine/storm/v3/q"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"shuffle/model"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/scheduler/apiv1"
	gyaml "github.com/ghodss/yaml"
	"github.com/h2non/filetype"
	"github.com/satori/go.uuid"
	"google.golang.org/api/cloudfunctions/v1"
	schedulerpb "google.golang.org/genproto/googleapis/cloud/scheduler/v1"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"

	newscheduler "github.com/carlescere/scheduler"
	"github.com/go-git/go-git/v5/storage/memory"
	//"github.com/gorilla/websocket"
	//"google.golang.org/appengine"
	//"google.golang.org/appengine/memcache"
	//"cloud.google.com/go/firestore"
	// "google.golang.org/api/option"
)

var localBase = "http://localhost:5001"

var cloudname = "cloud"

var defaultLocation = "europe-west2"
var scheduledJobs = map[string]*newscheduler.Job{}


//var upgrader = websocket.Upgrader{
//	ReadBufferSize:  1024,
//	WriteBufferSize: 1024,
//	CheckOrigin: func(r *http.Request) bool {
//		return true
//	},
//}

// This might be... a bit off, but that's fine :)
// This might also be stupid, as we want timelines and such
// Anyway, these are super basic stupid stats.
func increaseStatisticsField(fieldname, id string, amount int64) error {

	// 1. Get current stats
	// 2. Increase field(s)
	// 3. Put new stats

	statisticsItem := model.StatisticsItem{}
	newData := model.StatisticsData{
		Timestamp: int64(time.Now().Unix()),
		Amount:    amount,
		Id:        id,
	}

	if err := dbClient.One("Fieldname", fieldname, &statisticsItem); err != nil {
		// Should init
		if err == storm.ErrNotFound {
			statisticsItem = model.StatisticsItem{
				Total:     amount,
				Fieldname: fieldname,
				Data: []model.StatisticsData{
					newData,
				},
			}

			if err := dbClient.Save(&statisticsItem); err != nil {
				log.Printf("Error setting base stats: %s", err)
				return err
			}

			return nil
		}
		//log.Printf("STATSERR: %s", err)

		return err
	}

	statisticsItem.Total += amount
	statisticsItem.Data = append(statisticsItem.Data, newData)

	// New struct, to not add body, author etc
	if err := dbClient.Save(&statisticsItem); err != nil {
		log.Printf("Error stats to %s: %s", fieldname, err)
		return err
	}

	//log.Printf("Stats: %#v", statisticsItem)

	return nil
}

func setWorkflowQueue(executionRequests model.ExecutionRequestWrapper, id string) error {
	// New struct, to not add body, author etc
	executionRequests.ID = id
	if err := dbClient.Save(&executionRequests); err != nil {
		log.Printf("Error adding workflow queue: %s", err)
		return err
	}

	return nil
}

func getWorkflowQueue(id string) (model.ExecutionRequestWrapper, error) {
	workflows := model.ExecutionRequestWrapper{}

	if err := dbClient.One("ID", id, &workflows); err != nil {
		return model.ExecutionRequestWrapper{}, err
	}

	return workflows, nil
}

//func setWorkflowqueuetest(id string) {
//	data := ExecutionRequestWrapper{
//		Data: []ExecutionRequest{
//			ExecutionRequest{
//				ExecutionId:   "2349bf96-51ad-68d2-5ca6-75ef8f7ee814",
//				WorkflowId:    "8e344a2e-db51-448f-804c-eb959a32c139",
//				Authorization: "wut",
//			},
//		},
//	}
//
//	err := setWorkflowQueue(data, id)
//	if err != nil {
//		log.Printf("Fail: %s", err)
//	}
//}

// Frequency = cronjob OR minutes between execution
func createSchedule(ctx context.Context, scheduleId, workflowId, name, frequency string, body []byte) error {
	var err error
	testSplit := strings.Split(frequency, "*")
	cronJob := ""
	newfrequency := 0

	if len(testSplit) > 5 {
		cronJob = frequency
	} else {
		newfrequency, err = strconv.Atoi(frequency)
		if err != nil {
			log.Printf("Failed to parse time: %s", err)
			return err
		}

		//if int(newfrequency) < 60 {
		//	cronJob = fmt.Sprintf("*/%s * * * *")
		//} else if int(newfrequency) <
	}

	// Reverse. Can't handle CRON, only numbers
	if len(cronJob) > 0 {
		return errors.New("cronJob isn't formatted correctly")
	}

	if newfrequency < 1 {
		return errors.New("Frequency has to be more than 0")
	}

	//log.Printf("CRON: %s, body: %s", cronJob, string(body))

	// FIXME:
	// This may run multiple places if multiple servers,
	// but that's a future problem
	job := func() {
		request := &http.Request{
			Method: "POST",
			Body:   ioutil.NopCloser(strings.NewReader(string(body))),
		}

		_, _, err := handleExecution(workflowId, model.Workflow{}, request)
		if err != nil {
			log.Printf("Failed to execute: %s", err)
		}
	}

	log.Printf("Starting frequency: %d", newfrequency)
	jobret, err := newscheduler.Every(newfrequency).Seconds().NotImmediately().Run(job)
	if err != nil {
		log.Printf("Failed to schedule workflow: %s", err)
		return err
	}

	//scheduledJobs = append(scheduledJobs, jobret)
	scheduledJobs[scheduleId] = jobret

	// Doesn't need running/not running. If stopped, we just delete it.
	timeNow := int64(time.Now().Unix())
	schedule := model.ScheduleOld{
		Id:                   scheduleId,
		WorkflowId:           workflowId,
		Argument:             string(body),
		Seconds:              newfrequency,
		CreationTime:         timeNow,
		LastModificationtime: timeNow,
		LastRuntime:          timeNow,
	}

	err = setSchedule(schedule)
	if err != nil {
		log.Printf("Failed to set schedule: %s", err)
		return err
	}

	// FIXME - Create a real schedule based on cron:
	// 1. Parse the cron in a function to match this schedule
	// 2. Make main init check for schedules that aren't running

	return nil
}

func handleGetWorkflowqueueConfirm(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	id := request.Header.Get("Org-Id")
	if len(id) == 0 {
		log.Printf("No Org-Id header set - confirm")
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Specify the org-id header."}`)))
		return
	}

	//setWorkflowqueuetest(id)
	executionRequests, err := getWorkflowQueue(id)
	if err != nil {
		log.Printf("(1) Failed reading body for workflowqueue: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Entity parsing error - confirm"}`)))
		return
	}

	if len(executionRequests.Data) == 0 {
		log.Printf("No requests to fix. Why did this request occur?")
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Some error"}`)))
		return
	}

	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		log.Println("Failed reading body for stream result queue")
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "%s"}`, err)))
		return
	}

	// Getting from the request
	//log.Println(string(body))
	var removeExecutionRequests model.ExecutionRequestWrapper
	err = json.Unmarshal(body, &removeExecutionRequests)
	if err != nil {
		log.Printf("Failed executionrequest in queue unmarshaling: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "%s"}`, err)))
		return
	}

	if len(removeExecutionRequests.Data) == 0 {
		log.Printf("No requests to fix remove")
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Some removal error"}`)))
		return
	}

	// remove items from DB
	var newExecutionRequests model.ExecutionRequestWrapper
	for _, execution := range executionRequests.Data {
		found := false
		for _, removeExecution := range removeExecutionRequests.Data {
			if removeExecution.ExecutionId == execution.ExecutionId && removeExecution.WorkflowId == execution.WorkflowId {
				found = true
				break
			}
		}

		if !found {
			newExecutionRequests.Data = append(newExecutionRequests.Data, execution)
		}
	}

	// Push only the remaining to the DB (remove)
	if len(executionRequests.Data) != len(newExecutionRequests.Data) {
		err := setWorkflowQueue(newExecutionRequests, id)
		if err != nil {
			log.Printf("Fail: %s", err)
		}
	}

	//newjson, err := json.Marshal(removeExecutionRequests)
	//if err != nil {
	//	resp.WriteHeader(401)
	//	resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Failed unpacking workflow execution"}`)))
	//	return
	//}

	resp.WriteHeader(200)
	resp.Write([]byte("OK"))
}

func handleGetWorkflowqueue(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	id := request.Header.Get("Org-Id")
	if len(id) == 0 {
		log.Printf("No org-id header set")
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Specify the org-id header."}`)))
		return
	}

	executionRequests, err := getWorkflowQueue(id)
	if err != nil {
		// Skipping as this comes up over and over
		//log.Printf("(2) Failed reading body for workflowqueue: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "%s"}`, err)))
		return
	}

	if len(executionRequests.Data) == 0 {
		executionRequests.Data = []model.ExecutionRequest{}
	}

	newjson, err := json.Marshal(executionRequests)
	if err != nil {
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Failed unpacking workflow execution"}`)))
		return
	}

	resp.WriteHeader(200)
	resp.Write(newjson)
}

func handleGetStreamResults(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		log.Println("Failed reading body for stream result queue")
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "%s"}`, err)))
		return
	}

	var actionResult model.ActionResult
	err = json.Unmarshal(body, &actionResult)
	if err != nil {
		log.Printf("Failed ActionResult unmarshaling: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "%s"}`, err)))
		return
	}

	workflowExecution, err := getWorkflowExecution(actionResult.ExecutionId)
	if err != nil {
		log.Printf("Failed getting execution (streamresult) %s: %s", actionResult.ExecutionId, err)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Bad authorization key or execution_id might not exist."}`)))
		return
	}

	// Authorization is done here
	if workflowExecution.Authorization != actionResult.Authorization {
		log.Printf("Bad authorization key when getting stream results %s.", actionResult.ExecutionId)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Bad authorization key or execution_id might not exist."}`)))
		return
	}

	//for _, action := range workflowExecution.Workflow.Actions {
	//	log.Printf("Name: %s, Env: %s", action.Name, action.Environment)
	//}

	newjson, err := json.Marshal(workflowExecution)
	if err != nil {
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Failed unpacking workflow execution"}`)))
		return
	}

	resp.WriteHeader(200)
	resp.Write(newjson)

}

func handleWorkflowQueue(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		log.Println("(3) Failed reading body for workflowqueue")
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "%s"}`, err)))
		return
	}

	var actionResult model.ActionResult
	err = json.Unmarshal(body, &actionResult)
	if err != nil {
		log.Printf("Failed ActionResult unmarshaling: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "%s"}`, err)))
		return
	}

	// 1. Get the WorkflowExecution(ExecutionId) from the database
	// 2. if ActionResult.Authentication != WorkflowExecution.Authentication -> exit
	// 3. Add to and update actionResult in workflowExecution
	// 4. Push to dbClient
	// IF FAIL: Set executionstatus: abort or cancel


	workflowExecution, err := getWorkflowExecution(actionResult.ExecutionId)
	if err != nil {
		log.Printf("Failed getting execution (workflowqueue) %s: %s", actionResult.ExecutionId, err)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Failed getting execution ID %s because it doesn't exist."}`, actionResult.ExecutionId)))
		return
	}

	if workflowExecution.Authorization != actionResult.Authorization {
		log.Printf("Bad authorization key when updating node (workflowQueue) %s. Want: %s, Have: %s", actionResult.ExecutionId, workflowExecution.Authorization, actionResult.Authorization)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Bad authorization key"}`)))
		return
	}

	if workflowExecution.Status == "FINISHED" {
		log.Printf("Workflowexecution is already FINISHED. No further action can be taken")

		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Workflowexecution is already finished because of %s with status %s"}`, workflowExecution.LastNode, workflowExecution.Status)))
		return
	}

	// Not sure what's up here
	// FIXME - remove comment
	if workflowExecution.Status == "ABORTED" || workflowExecution.Status == "FAILURE" {

		log.Printf("Workflowexecution is already aborted. No further action can be taken")
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Workflowexecution is aborted because of %s with result %s and status %s"}`, workflowExecution.LastNode, workflowExecution.Result, workflowExecution.Status)))
		return
	}

	if actionResult.Status == "ABORTED" || actionResult.Status == "FAILURE" {
		log.Printf("Actionresult is %s. Should set workflowExecution and exit all running functions", actionResult.Status)
		workflowExecution.Status = actionResult.Status
		workflowExecution.LastNode = actionResult.Action.ID

		// Cleans up aborted, and always gives a result
		lastResult := ""
		newResults := []model.ActionResult{}
		// type ActionResult struct {
		for _, result := range workflowExecution.Results {
			if result.Status == "EXECUTING" {
				result.Status = actionResult.Status
				result.Result = "Aborted because of an unknown error"
			}

			if len(result.Result) > 0 {
				lastResult = result.Result
			}

			newResults = append(newResults, result)
		}

		workflowExecution.Result = lastResult
		workflowExecution.Results = newResults

		if workflowExecution.Status == "ABORTED" {
			err = increaseStatisticsField("workflow_executions_aborted", workflowExecution.Workflow.ID, 1)
			if err != nil {
				log.Printf("Failed to increase aborted execution stats: %s", err)
			}
		} else if workflowExecution.Status == "FAILURE" {
			err = increaseStatisticsField("workflow_executions_failure", workflowExecution.Workflow.ID, 1)
			if err != nil {
				log.Printf("Failed to increase failure execution stats: %s", err)
			}
		}
	}

	// This means it should continue I think :)
	if actionResult.Status == "SKIPPED" {
		// How the fuck do I do this tho
		// Parse _all_ children of the skipped and add them to "finished"
		//
		log.Printf("Find out how to handle skipped items, as there might be more branches to continue anyway")
		// FIXME - simulate that every subnode is skipped
		// Check worker, as it contains this code
		// Children of children of children...
		// Recurse, woo
		//for _, item := range children {

		//}
	}

	// FIXME rebuild to be like this or something
	// workflowExecution/ExecutionId/Nodes/NodeId
	// Find the appropriate action
	if len(workflowExecution.Results) > 0 {
		// FIXME
		found := false
		outerindex := 0
		for index, item := range workflowExecution.Results {
			if item.Action.ID == actionResult.Action.ID {
				found = true
				outerindex = index
				break
			}
		}

		if found {
			// FIXME - this is broken, but why
			//if workflowExecution.Results[outerindex].Status == actionResult.Status {
			//	log.Printf("Status of %s is already %s", actionResult.Action.ID, actionResult.Status)
			//	resp.WriteHeader(401)
			//	resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Status of %s is already %s"}`, actionResult.Action.ID, actionResult.Status)))
			//	return
			//}

			log.Printf("Updating %s in %s from %s to %s", actionResult.Action.ID, workflowExecution.ExecutionId, workflowExecution.Results[outerindex].Status, actionResult.Status)
			workflowExecution.Results[outerindex] = actionResult
		} else {
			log.Printf("Setting value of %s in %s to %s", actionResult.Action.ID, workflowExecution.ExecutionId, actionResult.Status)
			workflowExecution.Results = append(workflowExecution.Results, actionResult)
		}
	} else {
		log.Printf("Setting value of %s in %s to %s", actionResult.Action.ID, workflowExecution.ExecutionId, actionResult.Status)
		workflowExecution.Results = append(workflowExecution.Results, actionResult)
	}

	extraInputs := 0
	for _, result := range workflowExecution.Results {
		if result.Action.Name == "User Input" && result.Action.AppName == "User Input" {
			extraInputs += 1
		}
	}

	//log.Printf("Checking results %d vs %d", len(workflowExecution.Results), len(workflowExecution.Workflow.Actions)+extraInputs)
	if len(workflowExecution.Results) == len(workflowExecution.Workflow.Actions)+extraInputs {
		finished := true
		lastResult := ""
		for _, result := range workflowExecution.Results {
			if result.Status != "SUCCESS" && result.Status != "FINISHED" {
				finished = false
				break
			}

			lastResult = result.Result
		}

		if finished {
			log.Printf("Execution of %s finished.", workflowExecution.ExecutionId)
			//log.Println("Might be finished based on length of results and everything being SUCCESS or FINISHED - VERIFY THIS. Setting status to finished.")

			workflowExecution.Result = lastResult
			workflowExecution.Status = "FINISHED"
			workflowExecution.CompletedAt = int64(time.Now().Unix())
			if workflowExecution.LastNode == "" {
				workflowExecution.LastNode = actionResult.Action.ID
			}

			err = increaseStatisticsField("workflow_executions_success", workflowExecution.Workflow.ID, 1)
			if err != nil {
				log.Printf("Failed to increase success execution stats: %s", err)
			}
		}
	}

	// FIXME - why isn't this how it works otherwise, wtf?
	//workflow, err := getWorkflow(workflowExecution.Workflow.ID)
	//newActions := []Action{}
	//for _, action := range workflowExecution.Workflow.Actions {
	//	log.Printf("Name: %s, Env: %s", action.Name, action.Environment)
	//}

	err = setWorkflowExecution(*workflowExecution)
	if err != nil {
		log.Printf("Error saving workflow execution actionresult setting: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Failed setting workflowexecution actionresult: %s"}`, err)))
		return
	}

	resp.WriteHeader(200)
	resp.Write([]byte(fmt.Sprintf(`{"success": true}`)))
}

func getWorkflows(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	user, err := handleApiAuthentication(resp, request)
	if err != nil {
		log.Printf("Api authentication failed in getworkflows: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	//memcacheName := fmt.Sprintf("%s_workflows", user.Username)
	//if item, err := memcache.Get(ctx, memcacheName); err == memcache.ErrCacheMiss {
	//	// Not in cache
	//	//log.Printf("Workflows not in cache.")
	//} else if err != nil {
	//	log.Printf("Error getting item: %v", err)
	//} else {
	//	// FIXME - verify if value is ok? Can unmarshal etc.
	//	resp.WriteHeader(200)
	//	resp.Write(item.Value)
	//	return
	//}

	// With user, do a search for workflows with user or user's org attached
	var workflows []model.Workflow
	err = dbClient.Find("Owner", user.Id, &workflows)
	if err == storm.ErrNotFound {
		resp.WriteHeader(200)
		resp.Write([]byte("[]"))
		return
	}
	if err != nil {
		log.Printf("Failed getting workflows for user %s: %s", user.Username, err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	if len(workflows) == 0 {
		resp.WriteHeader(200)
		resp.Write([]byte("[]"))
		return
	}

	newjson, err := json.Marshal(workflows)
	if err != nil {
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Failed unpacking workflows"}`)))
		return
	}

	//item := &memcache.Item{
	//	Key:        memcacheName,
	//	Value:      newjson,
	//	Expiration: time.Minute * 10,
	//}
	//if err := memcache.Add(ctx, item); err == memcache.ErrNotStored {
	//	if err := memcache.Set(ctx, item); err != nil {
	//		log.Printf("Error setting item: %v", err)
	//	}
	//} else if err != nil {
	//	log.Printf("Error adding item: %v", err)
	//} else {
	//	//log.Printf("Set cache for %s", item.Key)
	//}

	resp.WriteHeader(200)
	resp.Write(newjson)
}

// FIXME - add to actual database etc
func setNewWorkflow(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	user, err := handleApiAuthentication(resp, request)
	if err != nil {
		log.Printf("Api authentication failed in set new workflowhandler: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		log.Printf("Error with body read: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	var workflow model.Workflow
	err = json.Unmarshal(body, &workflow)
	if err != nil {
		log.Printf("Failed unmarshaling: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	workflow.ID = uuid.NewV4().String()
	workflow.Owner = user.Id
	workflow.Sharing = "private"

	err = setWorkflow(workflow, workflow.ID)
	if err != nil {
		log.Printf("Failed setting workflow: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	log.Printf("Saved new workflow %s with name %s", workflow.ID, workflow.Name)
	err = increaseStatisticsField("total_workflows", workflow.ID, 1)
	if err != nil {
		log.Printf("Failed to increase total workflows: %s", err)
	}

	if len(workflow.Actions) == 0 {
		workflow.Actions = []model.Action{}
	}
	if len(workflow.Branches) == 0 {
		workflow.Branches = []model.Branch{}
	}
	if len(workflow.Triggers) == 0 {
		workflow.Triggers = []model.Trigger{}
	}
	if len(workflow.Errors) == 0 {
		workflow.Errors = []string{}
	}

	newActions := []model.Action{}
	for _, action := range workflow.Actions {
		if action.Environment == "" {
			//action.Environment = baseEnvironment
			action.IsValid = true
		}

		newActions = append(newActions, action)
	}

	workflow.Actions = newActions
	workflow.IsValid = true

	workflowjson, err := json.Marshal(workflow)
	if err != nil {
		log.Printf("Failed workflow json setting marshalling: %s", err)
		resp.WriteHeader(http.StatusInternalServerError)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	//memcacheName := fmt.Sprintf("%s_workflows", user.Username)
	//memcache.Delete(ctx, memcacheName)

	resp.WriteHeader(200)
	//log.Println(string(workflowjson))
	resp.Write(workflowjson)
}

func deleteWorkflow(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	user, err := handleApiAuthentication(resp, request)
	if err != nil {
		log.Printf("Api authentication failed in deleting workflow: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	location := strings.Split(request.URL.String(), "/")

	var fileId string
	if location[1] == "api" {
		if len(location) <= 4 {
			resp.WriteHeader(401)
			resp.Write([]byte(`{"success": false}`))
			return
		}

		fileId = location[4]
	}

	if len(fileId) != 36 {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Workflow ID to delete is not valid"}`))
		return
	}

	workflow, err := getWorkflow(fileId)
	if err != nil {
		log.Printf("Failed getting the workflow locally: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	// FIXME - have a check for org etc too..
	if user.Id != workflow.Owner && user.Role != "admin" {
		log.Printf("Wrong user (%s) for workflow %s", user.Username, workflow.ID)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	// Clean up triggers and executions
	for _, item := range workflow.Triggers {
		if item.TriggerType == "SCHEDULE" {
			err = deleteSchedule(item.ID)
			if err != nil {
				log.Printf("Failed to delete schedule: %s", err)
			}
		} else if item.TriggerType == "WEBHOOK" {
			err = removeWebhookFunction(item.ID)
			if err != nil {
				log.Printf("Failed to delete webhook: %s", err)
			}
		} else if item.TriggerType == "EMAIL" {
			err = handleOutlookSubRemoval(workflow.ID, item.ID)
			if err != nil {
				log.Printf("Failed to delete email sub: %s", err)
			}
		}

		err = increaseStatisticsField("total_workflow_triggers", workflow.ID, -1)
		if err != nil {
			log.Printf("Failed to increase total workflows: %s", err)
		}
	}

	// FIXME - maybe delete workflow executions
	log.Printf("Should delete workflow %s", fileId)
	err = dbClient.DeleteStruct(workflow)
	if err != nil {
		log.Printf("Failed deleting key %s", fileId)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Failed deleting key"}`))
		return
	}

	err = increaseStatisticsField("total_workflows", fileId, -1)
	if err != nil {
		log.Printf("Failed to increase total workflows: %s", err)
	}

	//memcacheName := fmt.Sprintf("%s_%s", user.Username, fileId)
	//memcache.Delete(ctx, memcacheName)
	//memcacheName = fmt.Sprintf("%s_workflows", user.Username)
	//memcache.Delete(ctx, memcacheName)

	resp.WriteHeader(200)
	resp.Write([]byte(`{"success": true}`))
}

// FIXME - check whether all nodes has a branch, otherwise go back
func saveWorkflow(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	log.Println("Start")
	user, userErr := handleApiAuthentication(resp, request)
	if userErr != nil {
		log.Printf("Api authentication failed in edit workflow: %s", userErr)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	log.Println("PostUser")
	location := strings.Split(request.URL.String(), "/")

	var fileId string
	if location[1] == "api" {
		if len(location) <= 4 {
			resp.WriteHeader(401)
			resp.Write([]byte(`{"success": false}`))
			return
		}

		fileId = location[4]
	}

	if len(fileId) != 36 {
		log.Printf(`ID %s is not valid`, fileId)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Workflow ID to save is not valid"}`))
		return
	}

	// Here to check access rights
	log.Println("GetWorkflow start")

	tmpworkflow, err := getWorkflow(fileId)
	if err != nil {
		log.Printf("Failed getting the workflow locally: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	log.Println("GetWorkflow end")

	// FIXME - have a check for org etc too..
	if user.Id != tmpworkflow.Owner && user.Role != "admin" {
		log.Printf("Wrong user (%s) for workflow %s (save)", user.Username, tmpworkflow.ID)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	//Actions           []Action   `json:"actions" datastore:"actions,noindex"`

	log.Printf("Hello")
	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		log.Printf("Failed hook unmarshaling: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	log.Printf("Hello2")
	var workflow model.Workflow
	err = json.Unmarshal([]byte(body), &workflow)
	//log.Printf(string(body))
	if err != nil {
		log.Printf("Failed workflow unmarshaling: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	// FIXME - auth and check if they should have access
	if fileId != workflow.ID {
		log.Printf("Path and request ID are not matching: %s:%s.", fileId, workflow.ID)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	// FIXME - this shouldn't be necessary with proper API checks
	newActions := []model.Action{}
	allNodes := []string{}
	log.Println("Pre")
	for _, action := range workflow.Actions {
		allNodes = append(allNodes, action.ID)
		if action.Environment == "" {
			resp.WriteHeader(401)
			resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "An environment for %s is required"}`, action.Label)))
			return
			action.IsValid = true
		}
		newActions = append(newActions, action)
	}

	workflow.Actions = newActions

	for _, trigger := range workflow.Triggers {
		log.Println("TRIGGERS")
		allNodes = append(allNodes, trigger.ID)
	}

	if len(workflow.Actions) == 0 {
		workflow.Actions = []model.Action{}
	}
	if len(workflow.Branches) == 0 {
		workflow.Branches = []model.Branch{}
	}
	if len(workflow.Triggers) == 0 {
		workflow.Triggers = []model.Trigger{}
	}
	if len(workflow.Errors) == 0 {
		workflow.Errors = []string{}
	}

	// FIXME - do actual checks ROFL
	// FIXME - minor issues with e.g. hello world and self.console_logger
	// Nodechecks
	foundNodes := []string{}
	for _, node := range allNodes {
		for _, branch := range workflow.Branches {
			log.Println("branch")
			//log.Println(node)
			//log.Println(branch.DestinationID)
			if node == branch.DestinationID || node == branch.SourceID {
				foundNodes = append(foundNodes, node)
				break
			}
		}
	}

	// FIXME - append all nodes (actions, triggers etc) to one single array here
	if len(foundNodes) != len(allNodes) || len(workflow.Actions) <= 0 {
		// This shit takes a few seconds lol
		if !workflow.IsValid {
			oldworkflow, err := getWorkflow(fileId)
			if err != nil {
				log.Printf("Workflow %s doesn't exist - oldworkflow.", fileId)
				resp.WriteHeader(401)
				resp.Write([]byte(`{"success": false, "reason": "Item already exists."}`))
				return
			}

			oldworkflow.IsValid = false
			err = setWorkflow(*oldworkflow, fileId)
			if err != nil {
				log.Printf("Failed saving workflow to database: %s", err)
				resp.WriteHeader(401)
				resp.Write([]byte(`{"success": false}`))
				return
			}
		}

		// FIXME - more checks here - force reload of data or something
		//if len(allNodes) == 0 {
		//	resp.WriteHeader(401)
		//	resp.Write([]byte(`{"success": false, "reason": "Please insert a node"}`))
		//	return
		//}

		// Allowed with only a start node
		//if len(allNodes) != 1 {
		//	resp.WriteHeader(401)
		//	resp.Write([]byte(`{"success": false, "reason": "There are nodes with no branches"}`))
		//	return
		//}
	}

	// FIXME - might be a sploit to run someone elses app if getAllWorkflowApps
	// doesn't check sharing=true
	// Have to do it like this to add the user's apps
	log.Println("Apps set starting")
	workflowApps := []model.WorkflowApp{}
	//memcacheName = "all_apps"
	//if item, err := memcache.Get(ctx, memcacheName); err == memcache.ErrCacheMiss {
	//	// Not in cache
	//	log.Printf("Apps not in cache.")
	workflowApps, err = getAllWorkflowApps()
	if err != nil {
		log.Printf("Failed getting all workflow apps from database: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}
	//} else if err != nil {
	//	log.Printf("Error getting item: %v", err)
	//} else {
	//	// FIXME - verify if value is ok? Can unmarshal etc.
	//	err = json.Unmarshal(item.Value, &workflowApps)
	//	if err != nil {
	//		log.Printf("Failed unmarshaling allworkflowapps from memcache: %s", err)
	//		resp.WriteHeader(401)
	//		resp.Write([]byte(`{"success": false}`))
	//		return
	//	}

	//	if userErr == nil && len(user.PrivateApps) > 0 {
	//		workflowApps = append(workflowApps, user.PrivateApps...)
	//	}
	//}

	// Started getting the single apps, but if it's weird, this is faster
	log.Println("Apps set done")

	// Check every app action and param to see whether they exist
	newActions = []model.Action{}
	for _, action := range workflow.Actions {
		reservedApps := []string{
			"0ca8887e-b4af-4e3e-887c-87e9d3bc3d3e",
		}

		builtin := false
		for _, id := range reservedApps {
			if id == action.AppID {
				builtin = true
				break
			}
		}

		if builtin {
			newActions = append(newActions, action)
		} else {
			curapp := model.WorkflowApp{}
			// FIXME - can this work with ONLY AppID?
			for _, app := range workflowApps {
				if app.ID == action.AppID {
					curapp = app
					break
				}

				if app.Name == action.AppName && app.AppVersion == action.AppVersion {
					curapp = app
					break
				}
			}

			// Check to see if the whole app is valid
			if curapp.Name != action.AppName {
				log.Printf("App %s doesn't exist.", action.AppName)
				resp.WriteHeader(401)
				resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "App %s doesn't exist"}`, action.AppName)))
				return
			}

			// Check tosee if the appaction is valid
			curappaction := model.WorkflowAppAction{}
			for _, curAction := range curapp.Actions {
				if action.Name == curAction.Name {
					curappaction = curAction
					break
				}
				log.Println(action.Name, curAction.Name)
			}

			// Check to see if the action is valid
			if curappaction.Name != action.Name {
				log.Printf("Appaction %s doesn't exist.", action.Name)
				resp.WriteHeader(401)
				resp.Write([]byte(`{"success": false}`))
				return
			}

			// FIXME - check all parameters to see if they're valid
			// Includes checking required fields

			newParams := []model.WorkflowAppActionParameter{}
			for _, param := range curappaction.Parameters {
				found := false

				// Handles check for parameter exists + value not empty in used fields
				for _, actionParam := range action.Parameters {
					if actionParam.Name == param.Name {
						found = true

						if actionParam.Value == "" && actionParam.Variant == "STATIC_VALUE" && actionParam.Required == true {
							log.Printf("Appaction %s with required param '%s' is empty.", action.Name, param.Name)
							resp.WriteHeader(401)
							resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Appaction %s with required param '%s' is empty."}`, action.Name, param.Name)))
							return

						}

						if actionParam.Variant == "" {
							actionParam.Variant = "STATIC_VALUE"
						}

						newParams = append(newParams, actionParam)
					}
				}

				// Handles check for required params
				if !found && param.Required {
					log.Printf("Appaction %s with required param %s doesn't exist.", action.Name, param.Name)
					resp.WriteHeader(401)
					resp.Write([]byte(`{"success": false}`))
					return
				}

			}

			action.Parameters = newParams
			newActions = append(newActions, action)
		}
	}

	workflow.Actions = newActions
	workflow.IsValid = true

	err = setWorkflow(workflow, fileId)
	if err != nil {
		log.Printf("Failed saving workflow to database: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	totalOldActions := len(tmpworkflow.Actions)
	totalNewActions := len(workflow.Actions)
	err = increaseStatisticsField("total_workflow_actions", workflow.ID, int64(totalNewActions-totalOldActions))
	if err != nil {
		log.Printf("Failed to change total actions data: %s", err)
	}

	log.Printf("Saved new version of workflow %s", fileId)
	resp.WriteHeader(200)
	resp.Write([]byte(`{"success": true}`))
}

func getWorkflowLocal(fileId string, request *http.Request) ([]byte, error) {
	fullUrl := fmt.Sprintf("%s/api/v1/workflows/%s", localBase, fileId)
	client := &http.Client{}
	req, err := http.NewRequest(
		"GET",
		fullUrl,
		nil,
	)

	if err != nil {
		return []byte{}, err
	}

	for key, value := range request.Header {
		req.Header.Add(key, strings.Join(value, ";"))
	}

	newresp, err := client.Do(req)
	if err != nil {
		return []byte{}, err
	}

	body, err := ioutil.ReadAll(newresp.Body)
	if err != nil {
		return []byte{}, err
	}

	// Temporary solution
	if strings.Contains(string(body), "reason") && strings.Contains(string(body), "false") {
		return []byte{}, errors.New(fmt.Sprintf("Failed getting workflow %s with message %s", fileId, string(body)))
	}

	return body, nil
}

func abortExecution(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	user, err := handleApiAuthentication(resp, request)
	if err != nil {
		log.Printf("Api authentication failed in abort workflow: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	location := strings.Split(request.URL.String(), "/")

	var fileId string
	if location[1] == "api" {
		if len(location) <= 4 {
			resp.WriteHeader(401)
			resp.Write([]byte(`{"success": false}`))
			return
		}

		fileId = location[4]
	}

	if len(fileId) != 36 {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Workflow ID to abort is not valid"}`))
		return
	}

	executionId := location[6]
	if len(executionId) != 36 {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "ExecutionID not valid"}`))
		return
	}

	workflowExecution, err := getWorkflowExecution(executionId)
	if err != nil {
		log.Printf("Failed getting execution (abort) %s: %s", executionId, err)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Failed getting execution ID %s because it doesn't exist (abort)."}`, executionId)))
		return
	}

	// FIXME - have a check for org etc too..
	if user.Id != workflowExecution.Workflow.Owner && user.Role != "admin" {
		log.Printf("Wrong user (%s) for workflowexecution workflow %s", user.Username, workflowExecution.Workflow.ID)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	if workflowExecution.Status == "ABORTED" || workflowExecution.Status == "FAILURE" || workflowExecution.Status == "FINISHED" {
		log.Printf("Stopped execution of %s with status %s", executionId, workflowExecution.Status)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Status for %s is %s, which can't be aborted."}`, executionId, workflowExecution.Status)))
		return
	}

	topic := "workflowexecution"

	workflowExecution.CompletedAt = int64(time.Now().Unix())
	workflowExecution.Status = "ABORTED"

	lastResult := ""
	newResults := []model.ActionResult{}
	// type ActionResult struct {
	for _, result := range workflowExecution.Results {
		if result.Status == "EXECUTING" {
			result.Status = "ABORTED"
			result.Result = "Aborted because of an unknown error"
		}

		if len(result.Result) > 0 {
			lastResult = result.Result
		}

		newResults = append(newResults, result)
	}

	workflowExecution.Results = newResults
	if len(workflowExecution.Result) == 0 {
		workflowExecution.Result = lastResult
	}

	err = setWorkflowExecution(*workflowExecution)
	if err != nil {
		log.Printf("Error saving workflow execution for updates when aborting %s: %s", topic, err)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Failed setting workflowexecution status to abort"}`)))
		return
	}

	err = increaseStatisticsField("workflow_executions_aborted", workflowExecution.Workflow.ID, 1)
	if err != nil {
		log.Printf("Failed to increase aborted execution stats: %s", err)
	}

	// FIXME - allowed to edit it? idk
	resp.WriteHeader(200)
	resp.Write([]byte(fmt.Sprintf(`{"success": true}`)))

	// Not sure what's up here
	//if workflowExecution.Status == "ABORTED" || workflowExecution.Status == "FAILURE" {
	//	log.Printf("Workflowexecution is already aborted. No further action can be taken")
	//	resp.WriteHeader(401)
	//	resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Workflowexecution is aborted because of %s with result %s and status %s"}`, workflowExecution.LastNode, workflowExecution.Result, workflowExecution.Status)))
	//	return
	//}
}

//// New execution with firestore

func cleanupExecutions(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	user, err := handleApiAuthentication(resp, request)
	if err != nil {
		log.Printf("Api authentication failed in execute workflow: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "message": "Not authenticated"}`))
		return
	}

	//if user.Role != "admin" {
	//	resp.WriteHeader(401)
	//	resp.Write([]byte(`{"success": false, "message": "Insufficient permissions"}`))
	//	return
	//}

	log.Printf("CLEANUP!")
	log.Printf("%#v", user)

	// Removes three months from today
	timestamp := int64(time.Now().AddDate(0, -2, 0).Unix())
	log.Println(timestamp)
	var workflowExecutions []model.WorkflowExecution
	err = dbClient.Select(q.Lt("StartedAt", timestamp)).Find(&workflowExecutions)
	if err != nil {
		log.Printf("Error getting workflowexec: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Failed getting all workflowexecutions"}`)))
		return
	}

	log.Println(len(workflowExecutions))

	resp.WriteHeader(200)
	resp.Write([]byte("OK"))
}

func handleExecution(id string, workflow model.Workflow, request *http.Request) (model.WorkflowExecution, string, error) {
	if workflow.ID == "" || workflow.ID != id {
		tmpworkflow, err := getWorkflow(id)
		if err != nil {
			log.Printf("Failed getting the workflow locally: %s", err)
			return model.WorkflowExecution{}, "Failed getting workflow", err
		}

		workflow = *tmpworkflow
	}

	if len(workflow.Actions) == 0 {
		workflow.Actions = []model.Action{}
	}
	if len(workflow.Branches) == 0 {
		workflow.Branches = []model.Branch{}
	}
	if len(workflow.Triggers) == 0 {
		workflow.Triggers = []model.Trigger{}
	}
	if len(workflow.Errors) == 0 {
		workflow.Errors = []string{}
	}

	if !workflow.IsValid {
		log.Printf("Stopped execution as workflow %s is not valid.", workflow.ID)
		return model.WorkflowExecution{}, fmt.Sprintf(`workflow %s is invalid`, workflow.ID), errors.New("Failed getting workflow")
	}

	workflowBytes, err := json.Marshal(workflow)
	if err != nil {
		log.Printf("Failed workflow unmarshal in execution: %s", err)
		return model.WorkflowExecution{}, "", err
	}

	//log.Println(workflow)
	var workflowExecution model.WorkflowExecution
	err = json.Unmarshal(workflowBytes, &workflowExecution.Workflow)
	if err != nil {
		log.Printf("Failed execution unmarshaling: %s", err)
		return model.WorkflowExecution{}, "Failed unmarshal during execution", err
	}

	makeNew := true
	if request.Method == "POST" {
		body, err := ioutil.ReadAll(request.Body)
		if err != nil {
			log.Printf("Failed request POST read: %s", err)
			return model.WorkflowExecution{}, "Failed getting body", err
		}

		var execution model.ExecutionRequest
		err = json.Unmarshal(body, &execution)
		if err != nil {
			log.Printf("Failed execution POST unmarshaling: %s", err)
			return model.WorkflowExecution{}, "", err
		}

		if execution.Start == "" && len(body) > 0 {
			execution.ExecutionArgument = string(body)
		}

		// FIXME - this should have "execution_argument" from executeWorkflow frontend
		if len(execution.ExecutionArgument) > 0 {
			workflowExecution.ExecutionArgument = execution.ExecutionArgument
		}

		//log.Printf("Execution data: %#v", execution)
		if len(execution.Start) == 36 {
			log.Printf("SHOULD START ON NODE %s", execution.Start)
			workflow.Start = execution.Start
		}

		if len(execution.ExecutionId) == 36 {
			workflowExecution.ExecutionId = execution.ExecutionId
		} else {
			sessionToken := uuid.NewV4()
			workflowExecution.ExecutionId = sessionToken.String()
		}
	} else {
		// Check for parameters of start and ExecutionId
		start, startok := request.URL.Query()["start"]
		answer, answerok := request.URL.Query()["answer"]
		referenceId, referenceok := request.URL.Query()["reference_execution"]
		if answerok && referenceok {
			// If answer is false, reference execution with result
			log.Printf("Answer is OK AND reference is OK!")
			if answer[0] == "false" {
				log.Printf("Should update reference and return, no need for further execution!")

				// Get the reference execution
				oldExecution, err := getWorkflowExecution(referenceId[0])
				if err != nil {
					log.Printf("Failed getting execution (execution) %s: %s", referenceId[0], err)
					return model.WorkflowExecution{}, fmt.Sprintf("Failed getting execution ID %s because it doesn't exist.", referenceId[0]), err
				}

				if oldExecution.Workflow.ID != id {
					log.Println("Wrong workflowid!")
					return model.WorkflowExecution{}, fmt.Sprintf("Bad ID %s", referenceId), errors.New("Bad ID")
				}

				newResults := []model.ActionResult{}
				//log.Printf("%#v", oldExecution.Results)
				for _, result := range oldExecution.Results {
					log.Printf("%s - %s", result.Action.ID, start[0])
					if result.Action.ID == start[0] {
						note, noteok := request.URL.Query()["note"]
						if noteok {
							result.Result = fmt.Sprintf("User note: %s", note[0])
						} else {
							result.Result = fmt.Sprintf("User clicked %s", answer[0])
						}

						// Stopping the whole thing
						result.CompletedAt = int64(time.Now().Unix())
						result.Status = "ABORTED"
						oldExecution.Status = result.Status
						oldExecution.Result = result.Result
						oldExecution.LastNode = result.Action.ID
					}

					newResults = append(newResults, result)
				}

				oldExecution.Results = newResults
				err = setWorkflowExecution(*oldExecution)
				if err != nil {
					log.Printf("Error saving workflow execution actionresult setting: %s", err)
					return model.WorkflowExecution{}, fmt.Sprintf("Failed setting workflowexecution actionresult in execution: %s", err), err
				}

				return model.WorkflowExecution{}, "", nil
			}
		}

		if referenceok {
			log.Printf("Handling an old execution continuation!")
			// Will use the old name, but still continue with NEW ID
			oldExecution, err := getWorkflowExecution(referenceId[0])
			if err != nil {
				log.Printf("Failed getting execution (execution) %s: %s", referenceId[0], err)
				return model.WorkflowExecution{}, fmt.Sprintf("Failed getting execution ID %s because it doesn't exist.", referenceId[0]), err
			}

			workflowExecution = *oldExecution
		}

		if len(workflowExecution.ExecutionId) == 0 {
			sessionToken := uuid.NewV4()
			workflowExecution.ExecutionId = sessionToken.String()
		} else {
			log.Printf("Using the same executionId as before: %s", workflowExecution.ExecutionId)
			makeNew = false
		}

		if startok {
			log.Printf("Setting start to %s based on query!", start[0])
			workflowExecution.Workflow.Start = start[0]
			workflowExecution.Start = start[0]
		}

	}

	// FIXME - regex uuid, and check if already exists?
	if len(workflowExecution.ExecutionId) != 36 {
		log.Printf("Invalid uuid: %s", workflowExecution.ExecutionId)
		return model.WorkflowExecution{}, "Invalid uuid", err
	}

	// FIXME - find owner of workflow
	// FIXME - get the actual workflow itself and build the request
	// MAYBE: Don't send the workflow within the pubsub, as this requires more data to be sent
	// Check if a worker already exists for company, else run one with:
	// locations, project IDs and subscription names

	// When app is executed:
	// Should update with status execution (somewhere), which will trigger the next node
	// IF action.type == internal, we need the internal watcher to be running and executing
	// This essentially means the WORKER has to be the responsible party for new actions in the INTERNAL landscape
	// Results are ALWAYS posted back to cloud@execution_id?
	if makeNew {
		workflowExecution.Type = "workflow"
		//workflowExecution.Stream = "tmp"
		//workflowExecution.WorkflowQueue = "tmp"
		//workflowExecution.SubscriptionNameNodestream = "testcompany-nodestream"
		workflowExecution.ProjectId = gceProject
		workflowExecution.Locations = []string{"europe-west2"}
		workflowExecution.WorkflowId = workflow.ID
		workflowExecution.StartedAt = int64(time.Now().Unix())
		workflowExecution.CompletedAt = 0
		workflowExecution.Authorization = uuid.NewV4().String()

		// Status for the entire workflow.
		workflowExecution.Status = "EXECUTING"
	}
	// Local authorization for this single workflow used in workers.

	// FIXME: Used for cloud
	//mappedData, err := json.Marshal(workflowExecution)
	//if err != nil {
	//	log.Printf("Failed workflowexecution marshalling: %s", err)
	//	resp.WriteHeader(http.StatusInternalServerError)
	//	resp.Write([]byte(`{"success": false}`))
	//	return
	//}

	//log.Println(string(mappedData))
	topic := "workflows"
	// FIXME - remove this?
	newActions := []model.Action{}
	for _, action := range workflowExecution.Workflow.Actions {
		action.LargeImage = ""
		//log.Println(action.Environment)

		if action.Environment == "" {
			return model.WorkflowExecution{}, fmt.Sprintf("Environment is not defined for %s", action.Name), errors.New("Environment not defined!")
		}
		newActions = append(newActions, action)
	}
	workflowExecution.Workflow.Actions = newActions

	//log.Printf("%#v", workflowExecution.Workflow.Actions)

	// Verification for execution environments
	onpremExecution := false
	environments := []string{}
	for _, action := range workflowExecution.Workflow.Actions {
		if action.Environment != cloudname {
			found := false
			for _, env := range environments {
				if env == action.Environment {
					found = true
					break
				}
			}

			if !found {
				environments = append(environments, action.Environment)
			}

			onpremExecution = true
		}
	}

	err = setWorkflowExecution(workflowExecution)
	if err != nil {
		log.Printf("Error saving workflow execution for updates %s: %s", topic, err)
		return model.WorkflowExecution{}, "Failed getting workflowexecution", err
	}

	// Adds queue for onprem execution
	// FIXME - add specifics to executionRequest, e.g. specific environment (can run multi onprem)
	if onpremExecution {
		// FIXME - tmp name based on future companyname-companyId
		for _, environment := range environments {
			log.Printf("EXECUTION: %s should execute onprem with execution environment \"%s\"", workflowExecution.ExecutionId, environment)

			executionRequest := model.ExecutionRequest{
				ExecutionId:   workflowExecution.ExecutionId,
				WorkflowId:    workflowExecution.Workflow.ID,
				Authorization: workflowExecution.Authorization,
				Environments:  environments,
			}

			executionRequestWrapper, err := getWorkflowQueue(environment)
			if err != nil {
				executionRequestWrapper = model.ExecutionRequestWrapper{
					Data: []model.ExecutionRequest{executionRequest},
				}
			} else {
				executionRequestWrapper.Data = append(executionRequestWrapper.Data, executionRequest)
			}

			//log.Printf("Execution request: %#v", executionRequest)

			err = setWorkflowQueue(executionRequestWrapper, environment)
			if err != nil {
				log.Printf("Failed adding to dbClient: %s", err)
			}
		}
	}

	err = increaseStatisticsField("workflow_executions", workflow.ID, 1)
	if err != nil {
		log.Printf("Failed to increase stats execution stats: %s", err)
	}

	return workflowExecution, "", nil
}

func executeWorkflow(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	user, err := handleApiAuthentication(resp, request)
	if err != nil {
		log.Printf("Api authentication failed in execute workflow: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	location := strings.Split(request.URL.String(), "/")

	var fileId string
	if location[1] == "api" {
		if len(location) <= 4 {
			resp.WriteHeader(401)
			resp.Write([]byte(`{"success": false}`))
			return
		}

		fileId = location[4]
	}

	if len(fileId) != 36 {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Workflow ID to execute is not valid"}`))
		return
	}

	//memcacheName := fmt.Sprintf("%s_%s", user.Username, fileId)
	workflow, err := getWorkflow(fileId)
	if err != nil {
		log.Printf("Failed getting the workflow locally: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	// FIXME - have a check for org etc too..
	// FIXME - admin check like this? idk
	if user.Id != workflow.Owner && user.Role != "admin" && user.Role != "scheduler" && user.Role != fmt.Sprintf("workflow_%s", fileId) {
		log.Printf("Wrong user (%s) for workflow %s (execute)", user.Username, workflow.ID)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	workflowExecution, executionResp, err := handleExecution(fileId, *workflow, request)

	if err == nil {
		resp.WriteHeader(200)
		resp.Write([]byte(fmt.Sprintf(`{"success": true, "execution_id": "%s", "authorization": "%s"}`, workflowExecution.ExecutionId, workflowExecution.Authorization)))
		return
	}

	resp.WriteHeader(500)
	resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "%s"}`, executionResp)))
}

func stopSchedule(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	user, err := handleApiAuthentication(resp, request)
	if err != nil {
		log.Printf("Api authentication failed in schedule workflow: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	location := strings.Split(request.URL.String(), "/")

	var fileId string
	var scheduleId string
	if location[1] == "api" {
		if len(location) <= 6 {
			resp.WriteHeader(401)
			resp.Write([]byte(`{"success": false}`))
			return
		}

		fileId = location[4]
		scheduleId = location[6]
	}

	if len(fileId) != 36 {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Workflow ID to stop schedule is not valid"}`))
		return
	}

	if len(scheduleId) != 36 {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Schedule ID not valid"}`))
		return
	}

	workflow, err := getWorkflow(fileId)
	if err != nil {
		log.Printf("Failed getting the workflow locally: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	// FIXME - have a check for org etc too..
	// FIXME - admin check like this? idk
	if user.Id != workflow.Owner && user.Role != "admin" && user.Role != "scheduler" {
		log.Printf("Wrong user (%s) for workflow %s (stop schedule)", user.Username, workflow.ID)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	if len(workflow.Actions) == 0 {
		workflow.Actions = []model.Action{}
	}
	if len(workflow.Branches) == 0 {
		workflow.Branches = []model.Branch{}
	}
	if len(workflow.Triggers) == 0 {
		workflow.Triggers = []model.Trigger{}
	}
	if len(workflow.Errors) == 0 {
		workflow.Errors = []string{}
	}

	err = deleteSchedule(scheduleId)
	if err != nil {
		if strings.Contains(err.Error(), "Job not found") {
			resp.WriteHeader(200)
			resp.Write([]byte(fmt.Sprintf(`{"success": true}`)))
		} else {
			resp.WriteHeader(401)
			resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Failed stopping schedule"}`)))
		}
		return
	}

	resp.WriteHeader(200)
	resp.Write([]byte(fmt.Sprintf(`{"success": true}`)))
	return
}

func stopScheduleGCP(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	user, err := handleApiAuthentication(resp, request)
	if err != nil {
		log.Printf("Api authentication failed in schedule workflow: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	location := strings.Split(request.URL.String(), "/")

	var fileId string
	var scheduleId string
	if location[1] == "api" {
		if len(location) <= 6 {
			resp.WriteHeader(401)
			resp.Write([]byte(`{"success": false}`))
			return
		}

		fileId = location[4]
		scheduleId = location[6]
	}

	if len(fileId) != 36 {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Workflow ID to stop schedule is not valid"}`))
		return
	}

	if len(scheduleId) != 36 {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Schedule ID not valid"}`))
		return
	}

	workflow, err := getWorkflow(fileId)
	if err != nil {
		log.Printf("Failed getting the workflow locally: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	// FIXME - have a check for org etc too..
	// FIXME - admin check like this? idk
	if user.Id != workflow.Owner && user.Role != "admin" && user.Role != "scheduler" {
		log.Printf("Wrong user (%s) for workflow %s (stop schedule)", user.Username, workflow.ID)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	if len(workflow.Actions) == 0 {
		workflow.Actions = []model.Action{}
	}
	if len(workflow.Branches) == 0 {
		workflow.Branches = []model.Branch{}
	}
	if len(workflow.Triggers) == 0 {
		workflow.Triggers = []model.Trigger{}
	}
	if len(workflow.Errors) == 0 {
		workflow.Errors = []string{}
	}

	err = deleteSchedule(scheduleId)
	if err != nil {
		if strings.Contains(err.Error(), "Job not found") {
			resp.WriteHeader(200)
			resp.Write([]byte(fmt.Sprintf(`{"success": true}`)))
		} else {
			resp.WriteHeader(401)
			resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Failed stopping schedule"}`)))
		}
		return
	}

	resp.WriteHeader(200)
	resp.Write([]byte(fmt.Sprintf(`{"success": true}`)))
	return
}

func deleteSchedule(id string) error {
	log.Printf("Should stop schedule %s!", id)
	//newscheduler "github.com/carlescere/scheduler"
	log.Printf("Schedules: %#v", scheduledJobs)
	if value, exists := scheduledJobs[id]; exists {
		log.Printf("STOP THIS ONE: %s", value)
		// Looks like this does the trick? Hurr
		value.Lock()
		schedule, err := getSchedule(id)
		if err != nil {
			log.Printf("Failed to fetch schedule to delete: %s", err)
			return err
		}
		err = dbClient.DeleteStruct(schedule)
		if err != nil {
			log.Printf("Failed to delete schedule: %s", err)
			return err
		}
	} else {
		// FIXME - allow it to kind of stop anyway?
		return errors.New("Can't find the schedule.")
	}

	return nil
}

func deleteScheduleGCP(ctx context.Context, id string) error {
	c, err := scheduler.NewCloudSchedulerClient(ctx)
	if err != nil {
		log.Printf("%s", err)
		return err
	}

	req := &schedulerpb.DeleteJobRequest{
		Name: fmt.Sprintf("projects/%s/locations/europe-west2/jobs/schedule_%s", gceProject, id),
	}

	err = c.DeleteJob(ctx, req)
	if err != nil {
		log.Printf("%s", err)
		return err
	}

	return nil
}

func scheduleWorkflow(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	user, err := handleApiAuthentication(resp, request)
	if err != nil {
		log.Printf("Api authentication failed in schedule workflow: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	location := strings.Split(request.URL.String(), "/")

	var fileId string
	if location[1] == "api" {
		if len(location) <= 4 {
			resp.WriteHeader(401)
			resp.Write([]byte(`{"success": false}`))
			return
		}

		fileId = location[4]
	}

	if len(fileId) != 36 {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Workflow ID to start schedule is not valid"}`))
		return
	}

	ctx := context.Background()
	workflow, err := getWorkflow(fileId)
	if err != nil {
		log.Printf("Failed getting the workflow locally: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	// FIXME - have a check for org etc too..
	// FIXME - admin check like this? idk
	if user.Id != workflow.Owner && user.Role != "admin" && user.Role != "scheduler" {
		log.Printf("Wrong user (%s) for workflow %s", user.Username, workflow.ID)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	if len(workflow.Actions) == 0 {
		workflow.Actions = []model.Action{}
	}
	if len(workflow.Branches) == 0 {
		workflow.Branches = []model.Branch{}
	}
	if len(workflow.Triggers) == 0 {
		workflow.Triggers = []model.Trigger{}
	}
	if len(workflow.Errors) == 0 {
		workflow.Errors = []string{}
	}

	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		log.Printf("Failed hook unmarshaling: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	var schedule model.Schedule
	err = json.Unmarshal(body, &schedule)
	if err != nil {
		log.Printf("Failed schedule POST unmarshaling: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	if len(schedule.Id) != 36 {
		log.Printf("ID length is not 36 for schedule: %s", err)
		resp.WriteHeader(http.StatusInternalServerError)
		resp.Write([]byte(`{"success": false, "reason": "Invalid data"}`))
		return
	}

	if len(schedule.Name) == 0 {
		log.Printf("Empty name.")
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Schedule name can't be empty"}`))
		return
	}

	if len(schedule.Frequency) == 0 {
		log.Printf("Empty frequency.")
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Frequency can't be empty"}`))
		return
	}

	scheduleArg, err := json.Marshal(schedule.ExecutionArgument)
	if err != nil {
		log.Printf("Failed scheduleArg marshal: %s", err)
		resp.WriteHeader(http.StatusInternalServerError)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	log.Printf("Schedulearg: %s", string(scheduleArg))

	err = createSchedule(
		ctx,
		schedule.Id,
		workflow.ID,
		schedule.Name,
		schedule.Frequency,
		scheduleArg,
	)

	// FIXME - real error message lol
	if err != nil {
		log.Printf("Failed creating schedule: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Invalid argument. Try cron */15 * * * *"}`)))
		return
	}

	workflow.Schedules = append(workflow.Schedules, schedule)
	err = setWorkflow(*workflow, workflow.ID)
	if err != nil {
		log.Printf("Failed setting workflow for schedule: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	resp.WriteHeader(200)
	resp.Write([]byte(fmt.Sprintf(`{"success": true}`)))
	return
}

// FIXME - add to actual database etc
func getSpecificWorkflow(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	user, err := handleApiAuthentication(resp, request)
	if err != nil {
		log.Printf("Api authentication failed in getting specific workflow: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	location := strings.Split(request.URL.String(), "/")

	var fileId string
	if location[1] == "api" {
		if len(location) <= 4 {
			resp.WriteHeader(401)
			resp.Write([]byte(`{"success": false}`))
			return
		}

		fileId = location[4]
	}

	if strings.Contains(fileId, "?") {
		fileId = strings.Split(fileId, "?")[0]
	}

	if len(fileId) != 36 {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Workflow ID when getting workflow is not valid"}`))
		return
	}

	//memcacheName := fmt.Sprintf("%s_%s", user.Username, fileId)
	//if item, err := memcache.Get(ctx, memcacheName); err == memcache.ErrCacheMiss {
	//	// Not in cache
	//	log.Printf("User %s not in cache.", memcacheName)
	//} else if err != nil {
	//	log.Printf("Error getting item: %v", err)
	//} else {
	//	log.Printf("Got workflow %s from cache", fileId)
	//	// FIXME - verify if value is ok? Can unmarshal etc.
	//	resp.WriteHeader(200)
	//	resp.Write(item.Value)
	//	return
	//}

	workflow, err := getWorkflow(fileId)
	if err != nil {
		log.Printf("Workflow %s doesn't exist.", fileId)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Item already exists."}`))
		return
	}

	// CHECK orgs of user, or if user is owner
	// FIXME - add org check too, and not just owner
	// Check workflow.Sharing == private / public / org  too
	if user.Id != workflow.Owner && user.Role != "admin" {
		log.Printf("Wrong user (%s) for workflow %s (get workflow)", user.Username, workflow.ID)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	if len(workflow.Actions) == 0 {
		workflow.Actions = []model.Action{}
	}
	if len(workflow.Branches) == 0 {
		workflow.Branches = []model.Branch{}
	}
	if len(workflow.Triggers) == 0 {
		workflow.Triggers = []model.Trigger{}
	}
	if len(workflow.Errors) == 0 {
		workflow.Errors = []string{}
	}

	// Only required for individuals I think
	//newactions := []Action{}
	//for _, item := range workflow.Actions {
	//	item.LargeImage = ""
	//	item.SmallImage = ""
	//	newactions = append(newactions, item)
	//}
	//workflow.Actions = newactions

	//newtriggers := []Trigger{}
	//for _, item := range workflow.Triggers {
	//	item.LargeImage = ""
	//	newtriggers = append(newtriggers, item)
	//}
	//workflow.Triggers = newtriggers

	body, err := json.Marshal(workflow)
	if err != nil {
		log.Printf("Failed workflow GET marshalling: %s", err)
		resp.WriteHeader(http.StatusInternalServerError)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	//item := &memcache.Item{
	//	Key:        memcacheName,
	//	Value:      body,
	//	Expiration: time.Minute * 60,
	//}
	//if err := memcache.Add(ctx, item); err == memcache.ErrNotStored {
	//	if err := memcache.Set(ctx, item); err != nil {
	//		log.Printf("Error setting item: %v", err)
	//	}
	//} else if err != nil {
	//	log.Printf("error adding item: %v", err)
	//} else {
	//	//log.Printf("Set cache for %s", item.Key)
	//}

	resp.WriteHeader(200)
	resp.Write(body)
}

//func setWorkflowExecutionFS(ctx context.Context, reference string, workflowExecution WorkflowExecution) error {
//	if len(workflowExecution.ExecutionId) == 0 {
//		log.Printf("Workflowexeciton executionId can't be empty.")
//		return errors.New("ExecutionId can't be empty.")
//	}
//
//	firestoreClient, err := firestore.NewClient(ctx, shuffleTestProject, option.WithCredentialsFile(shuffleTestPath))
//	if err != nil {
//		return err
//	}
//
//	executionRef := firestoreClient.Doc(reference)
//	_, err = executionRef.Set(ctx, workflowExecution)
//	if err != nil {
//		return err
//	}
//
//	return nil
//}

func setWorkflowExecution(workflowExecution model.WorkflowExecution) error {
	if len(workflowExecution.ExecutionId) == 0 {
		log.Printf("Workflowexeciton executionId can't be empty.")
		return errors.New("ExecutionId can't be empty.")
	}

	// New struct, to not add body, author etc
	if err := dbClient.Save(&workflowExecution); err != nil {
		log.Printf("Error adding workflow_execution: %s", err)
		return err
	}

	return nil
}

func getWorkflowExecution(id string) (*model.WorkflowExecution, error) {
	workflowExecution := &model.WorkflowExecution{}
	if err := dbClient.One("ExecutionId", id, workflowExecution); err != nil {
		return &model.WorkflowExecution{}, err
	}

	return workflowExecution, nil
}

func getApp(id string) (*model.WorkflowApp, error) {
	workflowApp := &model.WorkflowApp{}
	if err := dbClient.One("ID", id, workflowApp); err != nil {
		return &model.WorkflowApp{}, err
	}

	return workflowApp, nil
}

func getWorkflow(id string) (*model.Workflow, error) {
	workflow := &model.Workflow{}
	if err := dbClient.One("ID",id, workflow); err != nil {
		return &model.Workflow{}, err
	}

	return workflow, nil
}

func getAllWorkflows() ([]model.Workflow, error) {
	var allworkflows []model.Workflow

	err := dbClient.All(&allworkflows)
	if err != nil {
		return []model.Workflow{}, err
	}

	return allworkflows, nil
}

// Hmm, so I guess this should use uuid :(
// Consistency PLX
func setWorkflow(workflow model.Workflow, id string) error {
	workflow.ID = id
	// New struct, to not add body, author etc
	if err := dbClient.Save(&workflow); err != nil {
		log.Printf("Error adding workflow: %s", err)
		return err
	}

	return nil
}

func deleteWorkflowApp(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	user, userErr := handleApiAuthentication(resp, request)
	if userErr != nil {
		log.Printf("Api authentication failed in edit workflow: %s", userErr)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	location := strings.Split(request.URL.String(), "/")
	log.Printf("%#v", location)
	var fileId string
	if location[1] == "api" {
		if len(location) <= 4 {
			resp.WriteHeader(401)
			resp.Write([]byte(`{"success": false}`))
			return
		}

		fileId = location[4]
	}

	log.Printf("ID: %s", fileId)
	app, err := getApp(fileId)
	if err != nil {
		log.Printf("Error getting app %s: %s", app.Name, err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	// FIXME - check whether it's in use and maybe restrict again for later?
	// FIXME - actually delete other than private apps too..
	private := false
	if app.Downloaded {
		log.Printf("Deleting downloaded app (authenticated users can do this)")
	} else if user.Id != app.Owner && user.Role != "admin" {
		log.Printf("Wrong user (%s) for app %s (delete)", user.Username, app.Name)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	} else {
		private = true
	}

	// Not really deleting it, just removing from user cache
	if private {
		log.Printf("Deleting private app")
		var privateApps []model.WorkflowApp
		for _, item := range user.PrivateApps {
			log.Println(item.ID, fileId)
			if item.ID == fileId {
				continue
			}

			privateApps = append(privateApps, item)
		}

		user.PrivateApps = privateApps
		err = setUser(&user)
		if err != nil {
			log.Printf("Failed removing %s app for user %s: %s", app.Name, user.Username, err)
			resp.WriteHeader(401)
			resp.Write([]byte(fmt.Sprintf(`{"success": true"}`)))
			return
		}
	} else {
		log.Printf("Deleting public app")
		app, err := getApp(fileId)
		if err != nil {
			log.Printf("Failed getting workflowapp")
			resp.WriteHeader(401)
			resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Failed getting workflow app"}`)))
			return
		}
		err = dbClient.DeleteStruct(app)
		if err != nil {
			log.Printf("Failed deleting workflowapp")
			resp.WriteHeader(401)
			resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Failed deleting workflow app"}`)))
			return
		}
	}

	err = increaseStatisticsField("total_apps_deleted", fileId, 1)
	if err != nil {
		log.Printf("Failed to increase total apps loaded stats: %s", err)
	}
	//err = memcache.Delete(request.Context(), sessionToken)
	resp.WriteHeader(200)
	resp.Write([]byte(`{"success": true}`))
}

func getWorkflowAppConfig(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	user, userErr := handleApiAuthentication(resp, request)
	if userErr != nil {
		log.Printf("Api authentication failed in edit workflow: %s", userErr)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	location := strings.Split(request.URL.String(), "/")
	log.Printf("%#v", location)
	var fileId string
	if location[1] == "api" {
		if len(location) <= 4 {
			resp.WriteHeader(401)
			resp.Write([]byte(`{"success": false}`))
			return
		}

		fileId = location[4]
	}

	app, err := getApp(fileId)
	if err != nil {
		log.Printf("Error getting app: %s", app.Name)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	if user.Id != app.Owner && user.Role != "admin" {
		log.Printf("Wrong user (%s) for app %s", user.Username, app.Name)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	parsedApi, err := getOpenApiDatastore(fileId)
	if err != nil {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	parsedApi.Success = true
	data, err := json.Marshal(parsedApi)
	if err != nil {
		resp.WriteHeader(422)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Failed marshalling new parsed swagger: %s"}`, err)))
		return
	}

	resp.WriteHeader(200)
	resp.Write(data)
}

func getWorkflowApps(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	// FIXME - set this to be per user IF logged in, as there might exist private and public
	//memcacheName := "all_apps"

	// Just need to be logged in
	// FIXME - need to be logged in?
	user, userErr := handleApiAuthentication(resp, request)
	if userErr != nil {
		log.Printf("Api authentication failed in get all apps: %s", userErr)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	//if item, err := memcache.Get(ctx, memcacheName); err == memcache.ErrCacheMiss {
	//	// Not in cache
	//	log.Printf("Apps not in cache.")
	//} else if err != nil {
	//	log.Printf("Error getting item: %v", err)
	//} else {
	//	// FIXME - verify if value is ok? Can unmarshal etc.
	//	allApps := item.Value

	//	if userErr == nil && len(user.PrivateApps) > 0 {
	//		var parsedApps []WorkflowApp
	//		err = json.Unmarshal(allApps, &parsedApps)
	//		if err == nil {
	//			log.Printf("Shouldve added %d apps", len(user.PrivateApps))
	//			user.PrivateApps = append(user.PrivateApps, parsedApps...)

	//			tmpApps, err := json.Marshal(user.PrivateApps)
	//			if err == nil {
	//				allApps = tmpApps
	//			}
	//		}
	//	}

	//	resp.WriteHeader(200)
	//	resp.Write(allApps)
	//	return
	//}

	workflowapps, err := getAllWorkflowApps()
	if err != nil {
		log.Printf("Failed getting apps: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}
	//log.Printf("Length: %d", len(workflowapps))

	// FIXME - this is really garbage, but is here to protect again null values etc.
	newapps := []model.WorkflowApp{}
	baseApps := []model.WorkflowApp{}

	if len(user.PrivateApps) > 0 {
		newapps = append(newapps, user.PrivateApps...)
	}

	for _, workflowapp := range workflowapps {
		if !workflowapp.Sharing {
			continue
		}

		//workflowapp.Environment = "cloud"
		newactions := []model.WorkflowAppAction{}
		for _, action := range workflowapp.Actions {
			//action.Environment = workflowapp.Environment
			if len(action.Parameters) == 0 {
				action.Parameters = []model.WorkflowAppActionParameter{}
			}

			newactions = append(newactions, action)
		}

		workflowapp.Actions = newactions
		newapps = append(newapps, workflowapp)
		baseApps = append(baseApps, workflowapp)
	}

	// Double unmarshal because of user apps
	newbody, err := json.Marshal(newapps)
	//newbody, err := json.Marshal(workflowapps)
	if err != nil {
		log.Printf("Failed unmarshalling all newapps: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Failed unpacking workflow apps"}`)))
		return
	}

	//basebody, err := json.Marshal(baseApps)
	////newbody, err := json.Marshal(workflowapps)
	//if err != nil {
	//	log.Printf("Failed unmarshalling all baseapps: %s", err)
	//	resp.WriteHeader(401)
	//	resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Failed unpacking workflow apps"}`)))
	//	return
	//}

	// Refreshed every hour
	//item := &memcache.Item{
	//	Key:        memcacheName,
	//	Value:      basebody,
	//	Expiration: time.Minute * 60,
	//}
	//if err := memcache.Add(ctx, item); err == memcache.ErrNotStored {
	//	if err := memcache.Set(ctx, item); err != nil {
	//		log.Printf("Error setting item: %v", err)
	//	}
	//} else if err != nil {
	//	log.Printf("error adding item: %v", err)
	//} else {
	//	log.Printf("Set cache for %s", item.Key)
	//}

	//log.Println(string(body))
	//log.Println(string(newbody))
	resp.WriteHeader(200)
	resp.Write(newbody)
}

// Bad check for workflowapps :)
// FIXME - use tags and struct reflection
func checkWorkflowApp(workflowApp model.WorkflowApp) error {
	// Validate fields
	if workflowApp.Name == "" {
		return errors.New("App field name doesn't exist")
	}

	if workflowApp.Description == "" {
		return errors.New("App field description doesn't exist")
	}

	if workflowApp.AppVersion == "" {
		return errors.New("App field app_version doesn't exist")
	}

	if workflowApp.ContactInfo.Name == "" {
		return errors.New("App field contact_info.name doesn't exist")
	}

	return nil
}

func handleGetfile(resp http.ResponseWriter, request *http.Request) ([]byte, error) {
	// Upload file here first
	request.ParseMultipartForm(32 << 20)
	file, _, err := request.FormFile("file")
	if err != nil {
		log.Printf("Error parsing: %s", err)
		return []byte{}, err
	}
	defer file.Close()

	buf := bytes.NewBuffer(nil)
	if _, err := io.Copy(buf, file); err != nil {
		return []byte{}, err
	}

	return buf.Bytes(), nil
}

func validateAppInput(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	// Just need to be logged in
	// FIXME - should have some permissions?
	_, err := handleApiAuthentication(resp, request)
	if err != nil {
		log.Printf("Api authentication failed in set new app: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	filebytes, err := handleGetfile(resp, request)
	if err != nil {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	kind, err := filetype.Match(filebytes)
	if err != nil {
		log.Printf("Failed parsing filetype")
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	//fmt.Printf("File type: %s. MIME: %s\n", kind.Extension, kind.MIME.Value)
	if kind == filetype.Unknown {
		fmt.Println("Unknown file type")
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	if kind.MIME.Value != "application/zip" {
		fmt.Println("Not zip, can't unzip")
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	// FIXME - validate folderstructure, Dockerfile, python scripts, api.yaml, requirements.txt, src/

	resp.WriteHeader(200)
	resp.Write([]byte(fmt.Sprintf(`{"success": true}`)))
}

// Deploy to google cloud function :)
func deployCloudFunctionPython(ctx context.Context, name, localization, applocation string, environmentVariables map[string]string) error {
	service, err := cloudfunctions.NewService(ctx)
	if err != nil {
		return err
	}

	// ProjectsLocationsListCall
	projectsLocationsFunctionsService := cloudfunctions.NewProjectsLocationsFunctionsService(service)
	location := fmt.Sprintf("projects/%s/locations/%s", gceProject, localization)
	functionName := fmt.Sprintf("%s/functions/%s", location, name)

	cloudFunction := &cloudfunctions.CloudFunction{
		AvailableMemoryMb:    128,
		EntryPoint:           "authorization",
		EnvironmentVariables: environmentVariables,
		HttpsTrigger:         &cloudfunctions.HttpsTrigger{},
		MaxInstances:         0,
		Name:                 functionName,
		Runtime:              "python37",
		SourceArchiveUrl:     applocation,
	}

	//getCall := projectsLocationsFunctionsService.Get(fmt.Sprintf("%s/functions/function-5", location))
	//resp, err := getCall.Do()

	createCall := projectsLocationsFunctionsService.Create(location, cloudFunction)
	_, err = createCall.Do()
	if err != nil {
		log.Printf("Failed creating new function. SKIPPING patch, as it probably already exists: %s", err)

		// FIXME - have patching code or nah?
		createCall := projectsLocationsFunctionsService.Patch(fmt.Sprintf("%s/functions/%s", location, name), cloudFunction)
		_, err = createCall.Do()
		if err != nil {
			log.Println("Failed patching function")
			return err
		}

		log.Printf("Successfully patched %s to %s", name, localization)
	} else {
		log.Printf("Successfully deployed %s to %s", name, localization)
	}

	// FIXME - use response to define the HTTPS entrypoint. It's default to an easy one tho

	return nil
}

// Deploy to google cloud function :)
func deployCloudFunctionGo(ctx context.Context, name, localization, applocation string, environmentVariables map[string]string) error {
	service, err := cloudfunctions.NewService(ctx)
	if err != nil {
		return err
	}

	// ProjectsLocationsListCall
	projectsLocationsFunctionsService := cloudfunctions.NewProjectsLocationsFunctionsService(service)
	location := fmt.Sprintf("projects/%s/locations/%s", gceProject, localization)
	functionName := fmt.Sprintf("%s/functions/%s", location, name)

	cloudFunction := &cloudfunctions.CloudFunction{
		AvailableMemoryMb:    128,
		EntryPoint:           "Authorization",
		EnvironmentVariables: environmentVariables,
		HttpsTrigger:         &cloudfunctions.HttpsTrigger{},
		MaxInstances:         1,
		Name:                 functionName,
		Runtime:              "go111",
		SourceArchiveUrl:     applocation,
	}

	//getCall := projectsLocationsFunctionsService.Get(fmt.Sprintf("%s/functions/function-5", location))
	//resp, err := getCall.Do()

	createCall := projectsLocationsFunctionsService.Create(location, cloudFunction)
	_, err = createCall.Do()
	if err != nil {
		log.Println("Failed creating new function. Attempting patch, as it might exist already")

		createCall := projectsLocationsFunctionsService.Patch(fmt.Sprintf("%s/functions/%s", location, name), cloudFunction)
		_, err = createCall.Do()
		if err != nil {
			log.Println("Failed patching function")
			return err
		}

		log.Printf("Successfully patched %s to %s", name, localization)
	} else {
		log.Printf("Successfully deployed %s to %s", name, localization)
	}

	// FIXME - use response to define the HTTPS entrypoint. It's default to an easy one tho

	return nil
}

// Deploy to google cloud function :)
func deployWebhookFunction(ctx context.Context, name, localization, applocation string, environmentVariables map[string]string) error {
	service, err := cloudfunctions.NewService(ctx)
	if err != nil {
		return err
	}

	// ProjectsLocationsListCall
	projectsLocationsFunctionsService := cloudfunctions.NewProjectsLocationsFunctionsService(service)
	location := fmt.Sprintf("projects/%s/locations/%s", gceProject, localization)
	functionName := fmt.Sprintf("%s/functions/%s", location, name)

	cloudFunction := &cloudfunctions.CloudFunction{
		AvailableMemoryMb:    128,
		EntryPoint:           "Authorization",
		EnvironmentVariables: environmentVariables,
		HttpsTrigger:         &cloudfunctions.HttpsTrigger{},
		MaxInstances:         1,
		Name:                 functionName,
		Runtime:              "go111",
		SourceArchiveUrl:     applocation,
	}

	//getCall := projectsLocationsFunctionsService.Get(fmt.Sprintf("%s/functions/function-5", location))
	//resp, err := getCall.Do()

	createCall := projectsLocationsFunctionsService.Create(location, cloudFunction)
	_, err = createCall.Do()
	if err != nil {
		log.Println("Failed creating new function. Attempting patch, as it might exist already")

		createCall := projectsLocationsFunctionsService.Patch(fmt.Sprintf("%s/functions/%s", location, name), cloudFunction)
		_, err = createCall.Do()
		if err != nil {
			log.Println("Failed patching function")
			return err
		}

		log.Printf("Successfully patched %s to %s", name, localization)
	} else {
		log.Printf("Successfully deployed %s to %s", name, localization)
	}

	// FIXME - use response to define the HTTPS entrypoint. It's default to an easy one tho

	return nil
}

func loadExistingApps(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	// Just need to be logged in
	// FIXME - should have some permissions?
	_, err := handleApiAuthentication(resp, request)
	if err != nil {
		log.Printf("Api authentication failed in load apps: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	fs := memfs.New()
	storer := memory.NewStorage()
	r, err := git.Clone(storer, fs, &git.CloneOptions{
		URL: "https://github.com/frikky/shuffle-apps",
	})

	if err != nil {
		log.Printf("Failed loading repo into memory: %s", err)
	}

	dir, err := fs.ReadDir("/")
	if err != nil {
		log.Printf("FAiled reading folder: %s", err)
	}
	_ = r
	iterateAppGithubFolders(fs, dir, "", "")

	resp.WriteHeader(200)
	resp.Write([]byte(fmt.Sprintf(`{"success": true}`)))
}

// Onlyname is used to
func iterateAppGithubFolders(fs billy.Filesystem, dir []os.FileInfo, extra string, onlyname string) error {
	var err error
	runUpload := false
	for _, file := range dir {
		if len(onlyname) > 0 && file.Name() != onlyname {
			continue
		}

		// Folder?
		switch mode := file.Mode(); {
		case mode.IsDir():
			tmpExtra := fmt.Sprintf("%s%s/", extra, file.Name())
			dir, err := fs.ReadDir(tmpExtra)
			if err != nil {
				log.Printf("Failed to read dir: %s", err)
				break
			}

			// Go routine? Hmm, this can be super quick I guess
			err = iterateAppGithubFolders(fs, dir, tmpExtra, "")
			if err != nil {
				break
			}
		case mode.IsRegular():
			// Check the file
			filename := file.Name()
			if filename == "Dockerfile" {
				log.Printf("Handle Dockerfile in location %s", extra)

				extraSplit := strings.Split(extra, "/")
				tags := []string{}
				if len(extraSplit) > 1 {
					tags = []string{
						fmt.Sprintf("%s:%s_%s", baseDockerName, strings.ReplaceAll(extraSplit[0], " ", "-"), extraSplit[1]),
						// Version = folder of last part of extra
						// Name = first folder of extra
					}
				} else {
					// Skip
					runUpload = false
					log.Printf("Skipping folder %s because the extra variable is empty~", extra)
					break
					//return nil
				}

				/// Only upload if successful and no errors
				err := buildImageMemory(fs, tags, extra)
				if err != nil {
					log.Printf("Failed image build memory: %s", err)
					runUpload = false
				} else {
					runUpload = true
				}
			}
		}
	}

	// Done sequentailly to prevent bad uploads
	if runUpload && err == nil {
		for _, file := range dir {
			if file.Name() == "api.yaml" || file.Name() == "api.yaml" {
				log.Printf("Run update of %sapi.yaml in backend if it doesn't exist!!", extra)
				fullPath := fmt.Sprintf("%s%s", extra, file.Name())

				fileReader, err := fs.Open(fullPath)
				if err != nil {
					return err
				}

				readFile, err := ioutil.ReadAll(fileReader)
				if err != nil {
					log.Printf("Filereader error: %s", err)
					return err
				}

				var workflowapp model.WorkflowApp
				err = gyaml.Unmarshal(readFile, &workflowapp)
				if err != nil {
					log.Printf("Failed api.yaml unmarshal: %s", err)
					return err
				}

				log.Printf("APIName: %s", workflowapp.Name)
				extraSplit := strings.Split(extra, "/")
				appName := fmt.Sprintf("%s_%s", strings.ReplaceAll(extraSplit[0], " ", "-"), extraSplit[1])

				allapps, err := getAllWorkflowApps()
				if err != nil {
					log.Printf("Failed getting apps to verify: %s", err)
					return err
				}

				log.Printf("APPS: %d", len(allapps))

				for _, app := range allapps {
					if app.Name == workflowapp.Name && app.AppVersion == workflowapp.AppVersion {
						log.Printf("App upload for %s:%s already exists.", app.Name, app.AppVersion)
						return errors.New(fmt.Sprintf("App %s already exists. ", appName))
					}
				}

				err = checkWorkflowApp(workflowapp)
				if err != nil {
					log.Printf("%s for app %s:%s", err, workflowapp.Name, workflowapp.AppVersion)
					return err
				}

				//if workflowapp.Environment == "" {
				//	workflowapp.Environment = baseEnvironment
				//}

				workflowapp.ID = uuid.NewV4().String()
				workflowapp.IsValid = true
				workflowapp.Verified = true
				workflowapp.Sharing = true
				workflowapp.Downloaded = true

				err = setWorkflowAppDatastore(workflowapp, workflowapp.ID)
				if err != nil {
					log.Printf("Failed setting workflowapp: %s", err)
					return err
				}

				err = increaseStatisticsField("total_apps_created", workflowapp.ID, 1)
				if err != nil {
					log.Printf("Failed to increase total apps created stats: %s", err)
				}

				err = increaseStatisticsField("total_apps_loaded", workflowapp.ID, 1)
				if err != nil {
					log.Printf("Failed to increase total apps loaded stats: %s", err)
				}

				log.Printf("Added %s:%s to the database", workflowapp.Name, workflowapp.AppVersion)
				//memcache.Delete(ctx, "all_apps")
				//os.Exit(3)
			}
		}
	}

	return err
}

func setNewWorkflowApp(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	// Just need to be logged in
	// FIXME - should have some permissions?
	_, err := handleApiAuthentication(resp, request)
	if err != nil {
		log.Printf("Api authentication failed in set new app: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		log.Printf("Error with body read: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	var workflowapp model.WorkflowApp
	err = json.Unmarshal(body, &workflowapp)
	if err != nil {
		log.Printf("Failed unmarshaling: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	allapps, err := getAllWorkflowApps()
	if err != nil {
		log.Printf("Failed getting apps to verify: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	appfound := false
	for _, app := range allapps {
		if app.Name == workflowapp.Name && app.AppVersion == workflowapp.AppVersion {
			log.Printf("App upload for %s:%s already exists.", app.Name, app.AppVersion)
			appfound = true
			break
		}
	}

	if appfound {
		log.Printf("App %s:%s already exists. Bump the version.", workflowapp.Name, workflowapp.AppVersion)
		resp.WriteHeader(409)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "App %s:%s already exists."}`, workflowapp.Name, workflowapp.AppVersion)))
		return
	}

	err = checkWorkflowApp(workflowapp)
	if err != nil {
		log.Printf("%s for app %s:%s", err, workflowapp.Name, workflowapp.AppVersion)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "%s for app %s:%s"}`, err, workflowapp.Name, workflowapp.AppVersion)))
		return
	}

	//if workflowapp.Environment == "" {
	//	workflowapp.Environment = baseEnvironment
	//}

	workflowapp.ID = uuid.NewV4().String()
	workflowapp.IsValid = true
	workflowapp.Generated = false

	err = setWorkflowAppDatastore(workflowapp, workflowapp.ID)
	if err != nil {
		log.Printf("Failed setting workflowapp: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	} else {
		log.Printf("Added %s:%s to the database", workflowapp.Name, workflowapp.AppVersion)
	}

	//memcache.Delete(ctx, "all_apps")

	resp.WriteHeader(200)
	resp.Write([]byte(fmt.Sprintf(`{"success": true}`)))
}

func getWorkflowExecutions(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	user, err := handleApiAuthentication(resp, request)
	if err != nil {
		log.Printf("Api authentication failed in getting specific workflow: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	location := strings.Split(request.URL.String(), "/")

	var fileId string
	if location[1] == "api" {
		if len(location) <= 4 {
			resp.WriteHeader(401)
			resp.Write([]byte(`{"success": false}`))
			return
		}

		fileId = location[4]
	}

	if len(fileId) != 36 {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Workflow ID when getting workflow executions is not valid"}`))
		return
	}

	workflow, err := getWorkflow(fileId)
	if err != nil {
		log.Printf("Failed getting the workflow locally: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	// FIXME - have a check for org etc too..
	if user.Id != workflow.Owner && user.Role != "admin" {
		log.Printf("Wrong user (%s) for workflow %s (get execution)", user.Username, workflow.ID)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	// Query for the specifci workflowId
	var workflowExecutions []model.WorkflowExecution
	err = dbClient.Find("WorkflowId", fileId, &workflowExecutions, storm.Limit(50))
	if err != nil {
		log.Printf("Error getting workflowexec: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Failed getting all workflowexecutions for %s"}`, fileId)))
		return
	}

	if len(workflowExecutions) == 0 {
		resp.Write([]byte("[]"))
		resp.WriteHeader(200)
		return
	}

	newjson, err := json.Marshal(workflowExecutions)
	if err != nil {
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Failed unpacking workflow executions"}`)))
		return
	}

	resp.WriteHeader(200)
	resp.Write(newjson)
}

func getAllSchedules() ([]model.ScheduleOld, error) {
	var schedules []model.ScheduleOld

	err := dbClient.All(&schedules)
	if err != nil {
		return []model.ScheduleOld{}, err
	}

	return schedules, nil
}

func getAllWorkflowApps() ([]model.WorkflowApp, error) {
	var allworkflowapps []model.WorkflowApp

	err := dbClient.All(&allworkflowapps)
	if err != nil {
		return []model.WorkflowApp{}, err
	}

	return allworkflowapps, nil
}

// Hmm, so I guess this should use uuid :(
// Consistency PLX
func setWorkflowAppDatastore(workflowapp model.WorkflowApp, id string) error {
	workflowapp.ID = id
	// New struct, to not add body, author etc
	if err := dbClient.Save(&workflowapp); err != nil {
		log.Printf("Error adding workflow app: %s", err)
		return err
	}

	return nil
}

// Starts a new webhook
func handleStopHook(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	user, err := handleApiAuthentication(resp, request)
	if err != nil {
		log.Printf("Api authentication failed in set new workflowhandler: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	location := strings.Split(request.URL.String(), "/")

	var fileId string
	if location[1] == "api" {
		if len(location) <= 4 {
			resp.WriteHeader(401)
			resp.Write([]byte(`{"success": false}`))
			return
		}

		fileId = location[4]
	}

	if len(fileId) != 32 {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Workflow ID when stopping hook is not valid"}`))
		return
	}

	hook, err := getHook(fileId)
	if err != nil {
		log.Printf("Failed getting hook: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	if user.Id != hook.Owner && user.Role != "admin" {
		log.Printf("Wrong user (%s) for workflow %s", user.Username, hook.Id)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	log.Printf("Status: %s", hook.Status)
	log.Printf("Running: %t", hook.Running)
	if !hook.Running {
		message := fmt.Sprintf("Error: %s isn't running", hook.Id)
		log.Println(message)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "%s"}`, message)))
		return
	}

	hook.Status = "stopped"
	hook.Running = false
	hook.Actions = []model.HookAction{}
	err = setHook(*hook)
	if err != nil {
		log.Printf("Failed setting hook: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	image := "webhook"

	// This is here to force stop and remove the old webhook
	// FIXME
	err = removeWebhookFunction(fileId)
	if err != nil {
		log.Printf("Container stop issue for %s-%s: %s", image, fileId, err)
	}

	resp.WriteHeader(200)
	resp.Write([]byte(`{"success": true, "reason": "Stopped webhook"}`))
}

func handleDeleteHook(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	user, err := handleApiAuthentication(resp, request)
	if err != nil {
		log.Printf("Api authentication failed in set new workflowhandler: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	location := strings.Split(request.URL.String(), "/")

	var fileId string
	if location[1] == "api" {
		if len(location) <= 4 {
			resp.WriteHeader(401)
			resp.Write([]byte(`{"success": false}`))
			return
		}

		fileId = location[4]
	}

	if len(fileId) != 36 {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Workflow ID when deleting hook is not valid"}`))
		return
	}

	hook, err := getHook(fileId)
	if err != nil {
		log.Printf("Failed getting hook: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	if user.Id != hook.Owner && user.Role != "admin" {
		log.Printf("Wrong user (%s) for workflow %s", user.Username, hook.Id)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	if len(hook.Workflows) > 0 {
		err = increaseStatisticsField("total_workflow_triggers", hook.Workflows[0], -1)
		if err != nil {
			log.Printf("Failed to increase total workflows: %s", err)
		}
	}

	hook.Status = "stopped"
	err = setHook(*hook)
	if err != nil {
		log.Printf("Failed setting hook: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	// This is here to force stop and remove the old webhook
	//image := "webhook"
	//err = removeWebhookFunction(ctx, fileId)
	//if err != nil {
	//	log.Printf("Function removal issue for %s-%s: %s", image, fileId, err)
	//	if strings.Contains(err.Error(), "does not exist") {
	//		resp.WriteHeader(200)
	//		resp.Write([]byte(`{"success": true, "reason": "Stopped webhook"}`))

	//	} else {
	//		resp.WriteHeader(401)
	//		resp.Write([]byte(`{"success": false, "reason": "Couldn't stop webhook, please try again later"}`))
	//	}

	//	return
	//}

	log.Printf("Successfully deleted webhook %s", fileId)
	resp.WriteHeader(200)
	resp.Write([]byte(`{"success": true, "reason": "Stopped webhook"}`))
}

func removeWebhookFunction(hookid string) error {
	// TODO: GC: Webhook functions on prem
	/*
	service, err := cloudfunctions.NewService(ctx)
	if err != nil {
		return err
	}

	// ProjectsLocationsListCall
	projectsLocationsFunctionsService := cloudfunctions.NewProjectsLocationsFunctionsService(service)
	location := fmt.Sprintf("projects/%s/locations/%s", gceProject, defaultLocation)
	functionName := fmt.Sprintf("%s/functions/webhook_%s", location, hookid)

	deleteCall := projectsLocationsFunctionsService.Delete(functionName)
	resp, err := deleteCall.Do()
	if err != nil {
		log.Printf("Failed to delete %s from %s: %s", hookid, defaultLocation, err)
		return err
	} else {
		log.Printf("Successfully deleted %s from %s", hookid, defaultLocation)
	}

	_ = resp
	return nil
	*/
	return nil
}

func handleStartHook(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	user, err := handleApiAuthentication(resp, request)
	if err != nil {
		log.Printf("Api authentication failed in set new workflowhandler: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	location := strings.Split(request.URL.String(), "/")

	var fileId string
	if location[1] == "api" {
		if len(location) <= 4 {
			resp.WriteHeader(401)
			resp.Write([]byte(`{"success": false}`))
			return
		}

		fileId = location[4]
	}

	if len(fileId) != 36 {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Workflow ID when starting hook is not valid"}`))
		return
	}

	hook, err := getHook(fileId)
	if err != nil {
		log.Printf("Failed getting hook: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	if user.Id != hook.Owner && user.Role != "admin" {
		log.Printf("Wrong user (%s) for workflow %s", user.Username, hook.Id)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	log.Printf("Status: %s", hook.Status)
	log.Printf("Running: %t", hook.Running)
	if hook.Running || hook.Status == "Running" {
		message := fmt.Sprintf("Error: %s is already running", hook.Id)
		log.Println(message)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "%s"}`, message)))
		return
	}

	environmentVariables := map[string]string{
		"FUNCTION_APIKEY": user.ApiKey,
		"CALLBACKURL":     "https://shuffler.io",
		"HOOKID":          fileId,
	}

	applocation := fmt.Sprintf("gs://%s/triggers/webhook.zip", bucketName)
	hookname := fmt.Sprintf("webhook_%s", fileId)
	ctx := context.Background()
	err = deployWebhookFunction(ctx, hookname, "europe-west2", applocation, environmentVariables)
	if err != nil {
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "%s"}`, err)))
		return
	}

	hook.Status = "running"
	hook.Running = true
	err = setHook(*hook)
	if err != nil {
		log.Printf("Failed setting hook: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	log.Printf("Starting function %s?", fileId)
	resp.WriteHeader(200)
	resp.Write([]byte(`{"success": true, "reason": "Started webhook"}`))
	return
}

func removeOutlookTriggerFunction(ctx context.Context, triggerId string) error {
	service, err := cloudfunctions.NewService(ctx)
	if err != nil {
		return err
	}

	// ProjectsLocationsListCall
	projectsLocationsFunctionsService := cloudfunctions.NewProjectsLocationsFunctionsService(service)
	location := fmt.Sprintf("projects/%s/locations/%s", gceProject, defaultLocation)
	functionName := fmt.Sprintf("%s/functions/outlooktrigger_%s", location, triggerId)

	deleteCall := projectsLocationsFunctionsService.Delete(functionName)
	resp, err := deleteCall.Do()
	if err != nil {
		log.Printf("Failed to delete %s from %s: %s", triggerId, defaultLocation, err)
		return err
	} else {
		log.Printf("Successfully deleted %s from %s", triggerId, defaultLocation)
	}

	_ = resp
	return nil
}
