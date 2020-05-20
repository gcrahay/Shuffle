package main

import (
	"bufio"
	"github.com/asdine/storm/v3"
	"github.com/asdine/storm/v3/codec/gob"
	"shuffle/model"

	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"

	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	//"regexp"
	"strconv"
	"strings"
	"time"

	// Google cloud
	"google.golang.org/appengine/mail"

	"github.com/getkin/kin-openapi/openapi2"
	"github.com/getkin/kin-openapi/openapi2conv"
	"github.com/getkin/kin-openapi/openapi3"

	"github.com/google/go-github/v28/github"
	"golang.org/x/oauth2"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/storage/memory"

	// Random
	xj "github.com/basgys/goxml2json"
	newscheduler "github.com/carlescere/scheduler"
	gyaml "github.com/ghodss/yaml"
	"github.com/satori/go.uuid"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"

	// Web
	// "github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	// Old items (cloud)
	// "google.golang.org/appengine"
	// "google.golang.org/appengine/memcache"
	// applog "google.golang.org/appengine/log"
	//cloudrun "google.golang.org/api/run/v1"
)

// This is used to handle onprem vs offprem databases etc
var gceProject = "shuffle"
var bucketName = "shuffler.appspot.com"
var baseAppPath = "/home/frikky/git/shaffuru/tmp/apps"
var baseDockerName = "frikky/shuffle"

type Userapi struct {
	Username string `datastore:"Username"`
	ApiKey   string `datastore:"apikey"`
}

var dbClient *storm.DB


// "Execution by status"
// Execution history
//type GlobalStatistics struct {
//	BackendExecutions     int64            `json:"backend_executions" datastore:"backend_executions"`
//	WorkflowCount         int64            `json:"workflow_count" datastore:"workflow_count"`
//	ExecutionCount        int64            `json:"execution_count" datastore:"execution_count"`
//	ExecutionSuccessCount int64            `json:"execution_success_count" datastore:"execution_success_count"`
//	ExecutionAbortCount   int64            `json:"execution_abort_count" datastore:"execution_abort_count"`
//	ExecutionFailureCount int64            `json:"execution_failure_count" datastore:"execution_failure_count"`
//	ExecutionPendingCount int64            `json:"execution_pending_count" datastore:"execution_pending_count"`
//	AppUsageCount         int64            `json:"app_usage_count" datastore:"app_usage_count"`
//	TotalAppsCount        int64            `json:"total_apps_count" datastore:"total_apps_count"`
//	SelfMadeAppCount      int64            `json:"self_made_app_count" datastore:"self_made_app_count"`
//	WebhookUsageCount     int64            `json:"webhook_usage_count" datastore:"webhook_usage_count"`
//	Baseline              map[string]int64 `json:"baseline" datastore:"baseline"`
//}

type session struct {
	Username string `datastore:"Username,noindex"`
	Session  string `datastore:"session,noindex"`
}

type loginStruct struct {
	Username string `json:"Username"`
	Password string `json:"password"`
}

func IndexHandler(entrypoint string) func(w http.ResponseWriter, r *http.Request) {
	fn := func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, entrypoint)
	}

	return http.HandlerFunc(fn)
}


func jsonPrettyPrint(in string) string {
	var out bytes.Buffer
	err := json.Indent(&out, []byte(in), "", "\t")
	if err != nil {
		return in
	}
	return out.String()
}


func md5sumfile(filepath string) string {
	dat, err := ioutil.ReadFile(filepath)
	if err != nil {
		log.Printf("Error in dat: %s", err)
	}

	hasher := md5.New()
	hasher.Write(dat)
	newmd5 := hex.EncodeToString(hasher.Sum(nil))

	log.Printf("%s: %s", filepath, newmd5)
	return newmd5
}

func handleApiAuthentication(resp http.ResponseWriter, request *http.Request) (model.User, error) {
	apikey := request.Header.Get("Authorization")
	if len(apikey) > 0 {
		if !strings.HasPrefix(apikey, "Bearer ") {
			log.Printf("Apikey doesn't start with bearer")
			return model.User{}, errors.New("No bearer token for authorization header")
		}

		apikeyCheck := strings.Split(apikey, " ")
		if len(apikeyCheck) != 2 {
			log.Printf("Invalid format for apikey.")
			return model.User{}, errors.New("Invalid format for apikey")
		}

		// fml
		//log.Println(apikeyCheck)

		// This is annoying af and is done because of maxlength lol
		newApikey := apikeyCheck[1]
		if len(newApikey) > 249 {
			newApikey = newApikey[0:248]
		}

		//if item, err := memcache.Get(ctx, newApikey); err == memcache.ErrCacheMiss {
		//	// Not in cache
		//} else if err != nil {
		//	// Error with cache
		//	log.Printf("Error getting item: %v", err)
		//} else {
		//	var Userdata User
		//	err = json.Unmarshal(item.Value, &Userdata)

		//	if err == nil {
		//		if len(Userdata.Username) > 0 {
		//			return Userdata, nil
		//		} else {
		//			return Userdata, errors.New("User is invalid")
		//		}
		//	}
		//}

		// Make specific check for just service user?
		// Get the user based on APIkey here
		//log.Println(apikeyCheck[1])
		Userdata, err := getApikey(apikeyCheck[1])
		if err != nil {
			log.Printf("Apikey %s doesn't exist: %s", apikey, err)
			return model.User{}, err
		}

		// Caching both bad and good apikeys :)
		//b, err := json.Marshal(Userdata)
		//if err != nil {
		//	log.Printf("Failed marshalling: %s", err)
		//	return User{}, err
		//}

		// Add to cache if it doesn't exist
		//item := &memcache.Item{
		//	Key:        newApikey,
		//	Value:      b,
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

		if len(Userdata.Username) > 0 {
			return Userdata, nil
		} else {
			return Userdata, errors.New("User is invalid")
		}
	}

	// One time API keys
	authorizationArr, ok := request.URL.Query()["authorization"]
	if ok {
		authorization := ""
		if len(authorizationArr) > 0 {
			authorization = authorizationArr[0]
		}
		_ = authorization

		//if item, err := memcache.Get(ctx, authorization); err == memcache.ErrCacheMiss {
		//	// Doesn't exist :(
		//	log.Printf("Couldn't find %s in cache!", authorization)
		//	return User{}, err
		//} else if err != nil {
		//	log.Printf("Error getting item: %v", err)
		//	return User{}, err
		//} else {
		//	log.Printf("%#v", item.Value)
		//	var Userdata User

		//	log.Printf("Deleting key %s", authorization)
		//	memcache.Delete(ctx, authorization)
		//	err = json.Unmarshal(item.Value, &Userdata)
		//	if err == nil {
		//		return Userdata, nil
		//	}

		//	return User{}, err
		//}
	}

	c, err := request.Cookie("session_token")
	if err == nil {
		//if item, err := memcache.Get(ctx, c.Value); err == memcache.ErrCacheMiss {
		//	// Not in cache
		//} else if err != nil {
		//	log.Printf("Error getting item: %v", err)
		//} else {
		//	var Userdata User
		//	err = json.Unmarshal(item.Value, &Userdata)
		//	if err == nil {
		//		return Userdata, nil
		//	}
		//}

		// Get session first
		// Should basically never happen
		Userdata, err := getUserBySession(c.Value)
		if err != nil {
			log.Printf("Username %s doesn't exist: %s", Userdata.Username, err)
			return model.User{}, err
		}

		// Means session exists, but
		return *Userdata, nil
	}

	// Key = apikey
	return model.User{}, errors.New("Missing authentication")
}

func handleGetallSchedules(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	var err error
	var limit = 50

	// FIXME - add org search and public / private
	key, ok := request.URL.Query()["limit"]
	if ok {
		limit, err = strconv.Atoi(key[0])
		if err != nil {
			limit = 50
		}
	}

	// Max datastore limit
	if limit > 1000 {
		limit = 1000
	}

	var allSchedules []model.Schedules
	err =  dbClient.All(&allSchedules, storm.Limit(limit))
	if err != nil {
		log.Println(err)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Failed getting schedules"}`)))
		return
	}

	newjson, err := json.Marshal(allSchedules)
	if err != nil {
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Failed unpacking"}`)))
		return
	}

	resp.WriteHeader(200)
	resp.Write(newjson)
}

func redirect(w http.ResponseWriter, req *http.Request) {
	// remove/add not default ports from req.Host
	target := "https://" + req.Host + req.URL.Path
	if len(req.URL.RawQuery) > 0 {
		target += "?" + req.URL.RawQuery
	}
	log.Printf("redirect to: %s", target)
	http.Redirect(w, req, target,
		// see @andreiavrammsd comment: often 307 > 301
		http.StatusTemporaryRedirect)
}

func parseLoginParameters(resp http.ResponseWriter, request *http.Request) (loginStruct, error) {

	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		return loginStruct{}, err
	}

	var t loginStruct

	err = json.Unmarshal(body, &t)
	if err != nil {
		return loginStruct{}, err
	}

	return t, nil
}

// Can check against HIBP etc?
// Removed for localhost
func checkPasswordStrength(password string) error {
	// Check password strength here
	//if len(password) < 10 {
	//	return errors.New("Minimum password length is 10.")
	//}

	//if len(password) > 128 {
	//	return errors.New("Maximum password length is 128.")
	//}

	//re := regexp.MustCompile("[0-9]+")
	//if len(re.FindAllString(password, -1)) == 0 {
	//	return errors.New("Password must contain a number")
	//}

	//re = regexp.MustCompile("[a-z]+")
	//if len(re.FindAllString(password, -1)) == 0 {
	//	return errors.New("Password must contain a lower case char")
	//}

	//re = regexp.MustCompile("[A-Z]+")
	//if len(re.FindAllString(password, -1)) == 0 {
	//	return errors.New("Password must contain an upper case char")
	//}

	return nil
}

// No more emails :)
func checkUsername(Username string) error {
	// Stupid first check of email loool
	//if !strings.Contains(Username, "@") || !strings.Contains(Username, ".") {
	//	return errors.New("Invalid Username")
	//}

	if len(Username) < 4 {
		return errors.New("Minimum Username length is 4")
	}

	return nil
}

func handleRegisterVerification(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	defaultMessage := "Successfully registered"

	var reference string
	location := strings.Split(request.URL.String(), "/")
	if len(location) <= 4 {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	reference = location[4]

	if len(reference) != 36 {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Id when registering verification is not valid"}`))
		return
	}

	// With user, do a search for workflows with user or user's org attached
	// Only giving 200 to not give any suspicion whether they're onto an actual user or not
	var users []model.User
	err := dbClient.Find("verification_token", reference, &users)
	if err != nil {
		log.Printf("Failed getting users for verification token: %s", err)
		resp.WriteHeader(200)
		resp.Write([]byte(fmt.Sprintf(`{"success": true, "reason": "%s"}`, defaultMessage)))
		return
	}

	// FIXME - check reset_timeout
	if len(users) != 1 {
		log.Printf("Error - no user with verification id %s", reference)
		resp.WriteHeader(200)
		resp.Write([]byte(fmt.Sprintf(`{"success": true, "reason": "%s"}`, defaultMessage)))
		return
	}

	Userdata := users[0]

	// FIXME: Not for cloud!
	Userdata.Verified = true
	err = dbClient.Save(&Userdata)
	if err != nil {
		log.Printf("Failed adding verification for user %s: %s", Userdata.Username, err)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": true, "reason": "%s"}`, defaultMessage)))
		return
	}

	resp.WriteHeader(200)
	resp.Write([]byte(fmt.Sprintf(`{"success": true, "reason": "%s"}`, defaultMessage)))
	log.Printf("%s SUCCESSFULLY FINISHED REGISTRATION", Userdata.Username)
}

func handleSetEnvironments(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	// FIXME: Overhaul the top part.
	// Only admin can change environments, but if there are no users, anyone can make (first)
	user, err := handleApiAuthentication(resp, request)
	if err != nil {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Can't register without being admin"}`))
		return
	}

	if user.Role != "admin" {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Can't register without being admin"}`))
		return
	}

	var environments []model.Environment
	err = dbClient.All(&environments)
	if err != nil {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Can't get environments when setting"}`))
		return
	}

	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		log.Println("Failed reading body")
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Failed to read data"}`)))
		return
	}

	var newEnvironments []model.Environment
	err = json.Unmarshal(body, &newEnvironments)
	if err != nil {
		log.Printf("Failed unmarshaling: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Failed to unmarshal data"}`)))
		return
	}

	if len(newEnvironments) < 1 {
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "One environment is required"}`)))
		return
	}

	// Clear old data
	/*for _, item := range environments {
		err = DeleteKey(ctx, "Environments", item.Name)
		if err != nil {
			resp.WriteHeader(401)
			resp.Write([]byte(`{"success": false, "reason": "Error cleaning up environment"}`))
			return
		}
	}

	for _, item := range newEnvironments {
		err = setEnvironment(ctx, &item)
		if err != nil {
			resp.WriteHeader(401)
			resp.Write([]byte(`{"success": false, "reason": "Failed setting environment variable"}`))
			return
		}
	} */

	// FIXME - check which are in use
	log.Printf("FIXME: Set new environments: %#v", newEnvironments)
	log.Printf("DONT DELETE ONES THAT ARE IN USE")

	resp.WriteHeader(200)
	resp.Write([]byte(`{"success": true}`))
}

func handleRegister(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	// FIXME: Overhaul the top part.
	// Only admin can CREATE users, but if there are no users, anyone can make (first)
	count, countErr := getUserCount()
	user, err := handleApiAuthentication(resp, request)
	if err != nil {
		if (countErr == nil && count > 0) || countErr != nil {
			resp.WriteHeader(401)
			resp.Write([]byte(`{"success": false, "reason": "Can't register without being admin"}`))
			return
		}
	}

	//log.Printf("User role: %s", user.Role)
	if err == nil && user.Role != "admin" && count > 0 {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Can't register without being admin (2)"}`))
		return
	}

	// Gets a struct of Username, password
	data, err := parseLoginParameters(resp, request)
	if err != nil {
		log.Printf("Invalid params: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "%s"}`, err)))
		return
	}

	// Returns false if there is an issue
	// Use this for register
	err = checkPasswordStrength(data.Password)
	if err != nil {
		log.Printf("Bad password strength: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "%s"}`, err)))
		return
	}

	err = checkUsername(data.Username)
	if err != nil {
		log.Printf("Bad Username strength: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "%s"}`, err)))
		return
	}

	// FIXME - use it somehow
	ctx := context.Background()
	_, err = getUser(data.Username)
	if err == nil {
		log.Printf("Username %s exists and can't register", data.Username)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Username and/or password is incorrect"}`))
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(data.Password), 8)
	if err != nil {
		log.Printf("Wrong password for %s: %s", data.Username, err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Username and/or password is incorrect"}`))
		return
	}

	newUser := new(model.User)
	newUser.Username = data.Username
	newUser.Password = string(hashedPassword)
	newUser.Verified = false
	newUser.Role = "user"
	newUser.CreationTime = time.Now().Unix()

	// FIXME - Remove this later
	newUser.Role = "admin"

	// set limits
	// WorkflowExecutions > CloudExecutions simply because of onprem
	newUser.Limits.DailyApiUsage = 100
	newUser.Limits.DailyWorkflowExecutions = 1000
	newUser.Limits.DailyCloudExecutions = 100
	newUser.Limits.DailyTriggers = 20
	newUser.Limits.DailyMailUsage = 100
	newUser.Limits.MaxTriggers = 10
	newUser.Limits.MaxWorkflows = 10

	// Set base info for the user
	newUser.Executions.TotalApiUsage = 0
	newUser.Executions.TotalWorkflowExecutions = 0
	newUser.Executions.TotalAppExecutions = 0
	newUser.Executions.TotalCloudExecutions = 0
	newUser.Executions.TotalOnpremExecutions = 0
	newUser.Executions.DailyApiUsage = 0
	newUser.Executions.DailyWorkflowExecutions = 0
	newUser.Executions.DailyAppExecutions = 0
	newUser.Executions.DailyCloudExecutions = 0
	newUser.Executions.DailyOnpremExecutions = 0

	addr := newUser.Username

	verifyToken := uuid.NewV4()
	ID := uuid.NewV4()
	newUser.Id = ID.String()
	newUser.VerificationToken = verifyToken.String()
	err = setUser(newUser)
	if err != nil {
		log.Printf("Error adding User %s: %s", data.Username, err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Username and/or password is incorrect"}`))
		return
	}
	url := fmt.Sprintf("https://shuffler.io/register/%s", verifyToken.String())
	const verifyMessage = `
Registration URL :)

%s
	`

	msg := &mail.Message{
		Sender:  "Shuffle <frikky@shuffler.io>",
		To:      []string{addr},
		Subject: "Verify your username - Shuffle",
		Body:    fmt.Sprintf(verifyMessage, url),
	}

	log.Println(msg.Body)
	if err := mail.Send(ctx, msg); err != nil {
		log.Printf("Couldn't send email: %v", err)
	}

	//sessionToken := uuid.NewV4()

	//// Finally, we set the client cookie for "session_token" as the session token we just generated
	//// we also set an expiry time of 120 seconds, the same as the cache
	//http.SetCookie(resp, &http.Cookie{
	//	Name:    "session_token",
	//	Value:   sessionToken.String(),
	//	Expires: time.Now().Add(1200 * time.Second),
	//})

	//log.Println(Userdata)
	resp.WriteHeader(200)
	resp.Write([]byte(`{"success": true}`))
	log.Printf("%s Successfully registered.", data.Username)

	//err = SetSession(*newUser, sessionToken.String())
	//if err != nil {
	//	log.Printf("Error adding session to database: %s", err)
	//}

	//err = SetApikey(*newUser)
	//if err != nil {
	//	log.Printf("Error adding apikey to database: %s", err)
	//}

	//err = SetSession(*newUser, sessionToken.String())
	//if err != nil {
	//	log.Printf("Error adding apikey to database: %s", err)
	//}
}

func handleCookie(request *http.Request) bool {
	c, err := request.Cookie("session_token")
	if err != nil {
		return false
	}

	if len(c.Value) == 0 {
		return false
	}

	return true
}

func handleLogout(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	// Check cookie
	c, err := request.Cookie("session_token")
	if err != nil {
		resp.WriteHeader(200)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "%s"}`, err)))
		return
	} else {
		log.Printf("Session cookie is set!")
	}

	var Userdata model.User
	//item, err := memcache.Get(ctx, c.Value)
	//// Memcache handling for logout
	//if err == nil {
	//	err = json.Unmarshal(item.Value, &Userdata)
	//	if err != nil {
	//		log.Printf("Failed unmarshaling: %s", err)
	//		resp.WriteHeader(401)
	//		resp.Write([]byte(fmt.Sprintf(`{"success": false}`)))
	//		return
	//	}

	//	sessionToken = Userdata.Session
	//} else {
	//	// Validate with User

	// Get session first
	// Should basically never happen
	_, err = getUserBySession(c.Value)
	if err != nil {
		log.Printf("Session %s doesn't exist: %s", c.Value, err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Username and/or password is incorrect"}`))
		return
	}

	//	Userdata = *tmpdata
	//}

	// FIXME
	// Session might delete someone elses here?
	// No need to think about before possible scale..?
	err = SetSession(Userdata, "")
	if err != nil {
		log.Printf("Error removing session for: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Username and/or password is incorrect"}`))
		return
	}

	resp.WriteHeader(200)
	resp.Write([]byte(`{"success": false, "reason": "Successfully logged out"}`))
	http.SetCookie(resp, c)
}

func generateApikey(userInfo model.User) (model.User, error) {
	// Generate UUID
	// Set uuid to apikey in backend (update)
	apikey := uuid.NewV4()
	userInfo.ApiKey = apikey.String()


	if err := dbClient.Save(userInfo); err != nil {
		log.Printf("Failed updating user: %s", err)
		return userInfo, err
	}

	return userInfo, nil
}

func handleApiGeneration(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	c, err := request.Cookie("session_token")
	if err != nil {
		log.Printf("User doesn't have sessiontoken, on apigen: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "%s"}`, err)))
		return
	}

	// Get session first
	// Should basically never happen
	userInfo, err := getUserBySession(c.Value)
	if err != nil {
		log.Printf("Session %s doesn't exist: %s", c.Value, err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": ""}`))
		return
	}

	newUserInfo, err := generateApikey(*userInfo)
	if err != nil {
		log.Printf("Failed to generate apikey for user %s: %s", userInfo.Username, err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": ""}`))
		return
	}
	userInfo = &newUserInfo

	//memcache.Delete(request.Context(), sessionToken)

	log.Printf("Updated apikey for user %s", userInfo.Username)
	resp.WriteHeader(200)
	resp.Write([]byte(fmt.Sprintf(`{"success": true, "Username": "%s", "verified": %t, "apikey": "%s"}`, userInfo.Username, userInfo.Verified, userInfo.ApiKey)))
}

func handleSettings(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	c, err := request.Cookie("session_token")
	if err != nil {
		log.Printf("User doesn't have sessiontoken, on getsettings: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "%s"}`, err)))
		return
	}

	// Get session first
	// Should basically never happen
	UserInfo, err := getUserBySession(c.Value)
	if err != nil {
		log.Printf("Session %s doesn't exist: %s", c.Value, err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": ""}`))
		return
	}

	resp.WriteHeader(200)
	resp.Write([]byte(fmt.Sprintf(`{"success": true, "Username": "%s", "verified": %t, "apikey": "%s"}`, UserInfo.Username, UserInfo.Verified, UserInfo.ApiKey)))
}

func handleInfo(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	// Should compare with local storage first
	c, err := request.Cookie("session_token")
	if err != nil {
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "%s"}`, err)))
		return
	}

	// FIXME - check memcache here
	// Get the item from the memcache
	//if item, err := memcache.Get(ctx, c.Value); err == memcache.ErrCacheMiss {
	//	// Not in cache
	//} else if err != nil {
	//	log.Printf("Error getting item: %v", err)
	//} else {
	//	var Userdata User
	//	err = json.Unmarshal(item.Value, &Userdata)
	//	if err == nil {
	//		resp.WriteHeader(200)
	//		resp.Write([]byte(`{"success": true, "reason": "OK"}`))
	//		return
	//	}
	//}

	// Get session first
	// Should basically never happen
	UserInfo, err := getUserBySession(c.Value)
	if err != nil {
		log.Printf("Session %s doesn't exist: %s", c.Value, err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": ""}`))
		return
	}

	expiration := time.Now().Add(1200 * time.Second)
	http.SetCookie(resp, &http.Cookie{
		Name:    "session_token",
		Value:   UserInfo.Session,
		Expires: expiration,
	})

	returnData := fmt.Sprintf(`{"success": true, "cookies": [{"key": "session_token", "value": "%s", "expiration": %d}]}`, UserInfo.Session, expiration.Unix())

	//b, err := json.Marshal(UserInfo)
	//if err != nil {
	//	log.Printf("Failed marshalling: %s", err)
	//	resp.WriteHeader(401)
	//	resp.Write([]byte(`{"success": false}`))
	//	return
	//}

	// Adding to cache here
	// Only keeping it in for 24 hours
	//item := &memcache.Item{
	//	Key:        c.Value,
	//	Value:      b,
	//	Expiration: time.Hour * 24,
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

	resp.WriteHeader(200)
	resp.Write([]byte(returnData))
}

type passwordReset struct {
	Password1 string `json:"newpassword"`
	Password2 string `json:"newpassword2"`
	Reference string `json:"reference"`
}

type passwordChange struct {
	Password1 string `json:"newpassword"`
	Password2 string `json:"newpassword2"`
	Password3 string `json:"currentpassword"`
}

func handlePasswordResetMail(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	log.Println("Handling password reset mail")
	defaultMessage := "We have sent you an email :)"

	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		log.Println("Failed reading body")
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "%s"}`, defaultMessage)))
		return
	}

	type passwordReset struct {
		Username string `json:"Username"`
	}

	var t passwordReset
	err = json.Unmarshal(body, &t)
	if err != nil {
		log.Printf("Failed unmarshaling: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "%s"}`, defaultMessage)))
		return
	}

	ctx := context.Background()
	Userdata, err := getUser(t.Username)
	if err != nil {
		log.Printf("Username %s doesn't exist: %s", t.Username, err)
		resp.WriteHeader(200)
		resp.Write([]byte(fmt.Sprintf(`{"success": true}`)))
		return
	}

	resetToken := uuid.NewV4()
	// FIXME:
	// Weakness with this system is that you can spam someone with password resets,
	// and they would never be able to reset, as a new token is always generated
	url := fmt.Sprintf("https://shuffler.io/passwordreset/%s", resetToken.String())

	Userdata.ResetReference = resetToken.String()
	Userdata.ResetTimeout = 0
	err = setUser(Userdata)
	if err != nil {
		log.Printf("Error patching User for mail %s: %s", Userdata.Username, err)
		resp.WriteHeader(200)
		resp.Write([]byte(fmt.Sprintf(`{"success": true}`)))
		return
	}

	log.Printf("%#v", Userdata)
	addr := t.Username
	const confirmMessage = `
Reset URL :)

%s
	`

	msg := &mail.Message{
		Sender:  "Shuffle <frikky@shuffler.io>",
		To:      []string{addr},
		Subject: "Reset your password - Shuffle",
		Body:    fmt.Sprintf(confirmMessage, url),
	}

	log.Println(msg.Body)
	if err := mail.Send(ctx, msg); err != nil {
		log.Printf("Couldn't send email: %v", err)
	}

	// FIXME
	// Generate an email to send
	// Generate a reset code with a reset link
	// Build frontend to handle reset link with "new password" etc.

	resp.WriteHeader(200)
	resp.Write([]byte(fmt.Sprintf(`{"success": true, "reason": "%s"}`, defaultMessage)))
}

func handlePasswordReset(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	log.Println("Handling password reset")
	defaultMessage := "Successfully handled password reset"

	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		log.Println("Failed reading body")
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false}`)))
		return
	}

	var t passwordReset
	err = json.Unmarshal(body, &t)
	if err != nil {
		log.Println("Failed unmarshaling")
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false}`)))
		return
	}

	if t.Password1 != t.Password2 {
		resp.WriteHeader(401)
		err := "Passwords don't match"
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "%s"}`, err)))
		return
	}

	if len(t.Password1) < 10 || len(t.Password2) < 10 {
		resp.WriteHeader(401)
		err := "Passwords don't match - 2"
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "%s"}`, err)))
		return
	}

	// With user, do a search for workflows with user or user's org attached
	// Only giving 200 to not give any suspicion whether they're onto an actual user or not
	var users []model.User
	err = dbClient.Find("reset_reference", t.Reference, &users)
	if err != nil {
		log.Printf("Failed getting users: %s", err)
		resp.WriteHeader(200)
		resp.Write([]byte(fmt.Sprintf(`{"success": true, "reason": "%s"}`, defaultMessage)))
		return
	}

	// FIXME - check reset_timeout
	if len(users) != 1 {
		log.Printf("Error - no user with id %s", t.Reference)
		resp.WriteHeader(200)
		resp.Write([]byte(fmt.Sprintf(`{"success": true, "reason": "%s"}`, defaultMessage)))
		return
	}

	Userdata := users[0]
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(t.Password1), 8)
	if err != nil {
		log.Printf("Wrong password for %s: %s", Userdata.Username, err)
		resp.WriteHeader(200)
		resp.Write([]byte(fmt.Sprintf(`{"success": true, "reason": "%s"}`, defaultMessage)))
		return
	}

	Userdata.Password = string(hashedPassword)
	Userdata.ResetTimeout = 0
	Userdata.ResetReference = ""
	err = setUser(&Userdata)
	if err != nil {
		log.Printf("Error adding User %s: %s", Userdata.Username, err)
		resp.WriteHeader(200)
		resp.Write([]byte(fmt.Sprintf(`{"success": true, "reason": "%s"}`, defaultMessage)))
		return
	}

	// FIXME - maybe send a mail here to say that the password was changed

	resp.WriteHeader(200)
	resp.Write([]byte(fmt.Sprintf(`{"success": true, "reason": "%s"}`, defaultMessage)))
}

func WriteResponse(resp http.ResponseWriter, statusCode int, message string) {
	resp.WriteHeader(statusCode)
	_, err:= resp.Write([]byte(message))
	if err != nil {
		log.Printf("Error while writing response %s: %s\n", message, err)
	}
}

func handlePasswordChange(resp http.ResponseWriter, request *http.Request) {
	log.Println("Handling password change")

	cors := handleCors(resp, request)
	if cors {
		return
	}

	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		log.Println("Failed reading body")
		WriteResponse(resp, 401, fmt.Sprintf(`{"success": false}`))
		return
	}

	var t passwordChange
	err = json.Unmarshal(body, &t)
	if err != nil {
		log.Println("Failed unmarshaling")
		WriteResponse(resp, 401, fmt.Sprintf(`{"success": false}`))
		return
	}

	if t.Password1 != t.Password2 {
		err := "Passwords don't match"
		WriteResponse(resp, 401, fmt.Sprintf(`{"success": false, "reason": "%s"}`, err))
		return
	}

	if len(t.Password1) < 10 || len(t.Password2) < 10 {
		err := "Passwords don't match - 2"
		WriteResponse(resp, 401, fmt.Sprintf(`{"success": false, "reason": "%s"}`, err))
		return
	}

	err = checkPasswordStrength(t.Password3)
	if err != nil {
		log.Printf("Bad password strength: %s", err)
		WriteResponse(resp, 401, fmt.Sprintf(`{"success": false, "reason": "%s"}`, err))
		return
	}

	// Check cookie
	c, err := request.Cookie("session_token")
	if err != nil {
		log.Printf("User doesn't have sessiontoken on pw change: %s", err)
		WriteResponse(resp, 401, fmt.Sprintf(`{"success": false, "reason": "You're not logged in."}`))
		return
	}

	// Get session first
	// Should basically never happen
	Userdata, err := getUserBySession(c.Value)
	if err != nil {
		log.Printf("Session %s doesn't exist: %s", c.Value, err)
		WriteResponse(resp, 401, `{"success": false, "reason": "Username and/or password is incorrect"}`)
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(Userdata.Password), []byte(t.Password1))
	if err != nil {
		log.Printf("Bad password for %s: %s", Userdata.Username, err)
		WriteResponse(resp, 401, `{"success": false, "reason": "Username and/or password is incorrect"}`)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(t.Password3), 8)
	if err != nil {
		log.Printf("Wrong password for %s: %s", Userdata.Username, err)
		WriteResponse(resp, 401, `{"success": false, "reason": "Username and/or password is incorrect"}`)
		return
	}

	Userdata.Password = string(hashedPassword)
	err = setUser(Userdata)
	if err != nil {
		log.Printf("Error adding User %s: %s", Userdata.Username, err)
		WriteResponse(resp, 401, `{"success": false, "reason": "Username and/or password is incorrect"}`)
		return
	}
	WriteResponse(resp, 200, `{"success": true}`)
}

// FIXME - forward this to emails or whatever CRM system in use
func handleContact(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		WriteResponse(resp, 401, fmt.Sprintf(`{"success": false, "reason": "%s"}`, err))
		return
	}

	var t model.Contact
	err = json.Unmarshal(body, &t)
	if err != nil {
		WriteResponse(resp, 401, fmt.Sprintf(`{"success": false, "reason": "%s"}`, err))
		return
	}

	if len(t.Email) < 3 || len(t.Message) == 0 {
		WriteResponse(resp, 401, fmt.Sprintf(`{"success": false, "reason": "Please fill a valid email and message"}`))
		return
	}

	ctx := context.Background()
	mailContent := fmt.Sprintf("Firsname: %s\nLastname: %s\nTitle: %s\nCompanyname: %s\nPhone: %s\nEmail: %s\nMessage: %s", t.Firstname, t.Lastname, t.Title, t.Companyname, t.Phone, t.Email, t.Message)
	log.Printf("Sending contact from %s", t.Email)

	msg := &mail.Message{
		Sender:  "Shuffle <frikky@shuffler.io>",
		To:      []string{"frikky@shuffler.io"},
		Subject: "Shuffler.io - New contact form",
		Body:    mailContent,
	}

	if err := mail.Send(ctx, msg); err != nil {
		log.Printf("Couldn't send email: %v", err)
	}

	WriteResponse(resp, 200, `{"success": true, "message": "Thanks for reaching out. We will contact you soon!"}`)
}

func getEnvironmentCount() (int, error) {
	count, err := dbClient.Count(&model.Environment{})
	if err != nil {
		return 0, err
	}
	return count, nil
}

func getUserCount() (int, error) {
	count, err := dbClient.Count(&model.User{});
	if err != nil {
		return 0, err
	}

	return count, nil
}

func handleGetEnvironments(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	_, err := handleApiAuthentication(resp, request)
	if err != nil {
		log.Printf("Api authentication failed in set new workflowhandler: %s", err)
		WriteResponse(resp, 401, `{"success": false}`)
		return
	}

	var environments []model.Environment
	if err := dbClient.All(&environments); err != nil {
		log.Printf("Cannot get environment: %s", err)
		WriteResponse(resp, 401, `{"success": false, "reason": "Can't get environments"}`)
		return
	}

	newjson, err := json.Marshal(environments)
	if err != nil {
		log.Printf("Failed unmarshal: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Failed unpacking environments"}`)))
		return
	}

	//log.Printf("Existing environments: %s", string(newjson))

	resp.WriteHeader(200)
	resp.Write(newjson)
}

func handleGetUsers(resp http.ResponseWriter, request *http.Request) {
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

	if user.Role != "admin" {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Not admin"}`))
		return
	}

	var users []model.User
	if err := dbClient.All(&users); err != nil {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Can't get users"}`))
		return
	}

	newjson, err := json.Marshal(users)
	if err != nil {
		log.Printf("Failed unmarshal: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Failed unpacking"}`)))
		return
	}

	resp.WriteHeader(200)
	resp.Write(newjson)
}

func checkAdminLogin(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	count, err := getUserCount()
	if err != nil {
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "%s"}`, err)))
	}

	if count == 0 {
		log.Printf("No users - redirecting for management user")
		resp.WriteHeader(200)
		resp.Write([]byte(fmt.Sprintf(`{"success": true, "reason": "stay"}`)))
		return
	}

	resp.WriteHeader(200)
	resp.Write([]byte(fmt.Sprintf(`{"success": true, "reason": "redirect"}`)))
}

func handleLogin(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	// Gets a struct of Username, password
	data, err := parseLoginParameters(resp, request)
	if err != nil {
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "%s"}`, err)))
		return
	}

	log.Printf("Handling login of %s", data.Username)

	err = checkUsername(data.Username)
	if err != nil {
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "%s"}`, err)))
		return
	}

	Userdata, err := getUser(data.Username)
	if err != nil {
		log.Printf("Username %s doesn't exist: %s", data.Username, err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Username and/or password is incorrect"}`))
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(Userdata.Password), []byte(data.Password))
	if err != nil {
		log.Printf("Password for %s is incorrect: %s", data.Username, err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Username and/or password is incorrect"}`))
		return
	}

	log.Printf("%s SUCCESSFULLY LOGGED IN", data.Username)
	//if !Userdata.Verified {
	//	log.Printf("User %s is not verified", data.Username)
	//	resp.WriteHeader(403)
	//	resp.Write([]byte(`{"success": false, "reason": "Successful login, but your email address isn't verified. Check your mailbox."}`))
	//	return
	//}

	loginData := `{"success": true}`

	// FIXME - have timeout here
	if len(Userdata.Session) != 0 {
		//log.Println("Nonexisting session")
		expiration := time.Now().Add(1200 * time.Second)

		http.SetCookie(resp, &http.Cookie{
			Name:    "session_token",
			Value:   Userdata.Session,
			Expires: expiration,
		})

		loginData = fmt.Sprintf(`{"success": true, "cookies": [{"key": "session_token", "value": "%s", "expiration": %d}]}`, Userdata.Session, expiration.Unix())
		log.Printf("SESSION LENGTH MORE THAN 0 IN LOGIN: %s", Userdata.Session)

		err = SetSession(*Userdata, Userdata.Session)
		if err != nil {
			log.Printf("Error adding session to database: %s", err)
		}

		resp.WriteHeader(200)
		resp.Write([]byte(loginData))
		return
	}

	sessionToken := uuid.NewV4()

	http.SetCookie(resp, &http.Cookie{
		Name:    "session_token",
		Value:   sessionToken.String(),
		Expires: time.Now().Add(1200 * time.Second),
	})

	// ADD TO DATABASE
	err = SetSession(*Userdata, sessionToken.String())
	if err != nil {
		log.Printf("Error adding session to database: %s", err)
	}

	resp.WriteHeader(200)
	resp.Write([]byte(loginData))
}

func getApikey(apikey string) (model.User, error) {
	// Query for the specifci workflowId
	var users []model.User
	if err := dbClient.Find("ApiKey", apikey, &users); err != nil {
		log.Printf("Error getting users apikey: %s", err)
		return model.User{}, err
	}

	if len(users) == 0 {
		log.Printf("No users found for apikey %s", apikey)
		return model.User{}, fmt.Errorf("no user found")
	}

	if len(users) > 1 {
		log.Printf("Multiple users found for apikey %s", apikey)
		return model.User{}, fmt.Errorf("too many users found")
	}

	return users[0], nil
}

func getUserBySession(session string) (*model.User, error) {
	var user model.User
	if err := dbClient.One("Session", session, &user); err != nil {
		log.Printf("Error getting user by session %s: %s", session, err)
		return &user, err
	}
	return &user, nil
}

func getUser(Username string) (*model.User, error) {
	curUser := &model.User{}
	if err := dbClient.One("Username", Username, curUser); err != nil {
		return &model.User{}, err
	}

	return curUser, nil
}

// Index = Username
func SetSession(Userdata model.User, value string) error {
	// Non indexed User data
	Userdata.Session = value

	// New struct, to not add body, author etc
	if err := dbClient.Save(&Userdata); err != nil {
		log.Printf("rror adding Usersession: %s", err)
		return err
	}

	if len(Userdata.Session) > 0 {
		// Indexed session data
		sessiondata := new(session)
		sessiondata.Username = Userdata.Username
		sessiondata.Session = Userdata.Session

		if err := dbClient.Save(sessiondata); err != nil {
			log.Printf("Error adding session: %s", err)
			return err
		}
	}

	return nil
}

func setOpenApiDatastore(data model.ParsedOpenApi) error {
	if err := dbClient.Save(&data); err != nil {
		log.Println(err)
		return err
	}

	return nil
}

func getOpenApiDatastore(id string) (model.ParsedOpenApi, error) {
	api := &model.ParsedOpenApi{}
	if err := dbClient.One("ID", id, api); err != nil {
		return model.ParsedOpenApi{}, err
	}
	return *api, nil
}

func setEnvironment(data *model.Environment) error {

	if err := dbClient.Save(data); err != nil {
		log.Println(err)
		return err
	}

	return nil
}

func setUser(data *model.User) error {

	if err := dbClient.Save(data); err != nil {
		log.Printf("Error saving user: %s", err)
		return err
	}
	return nil
}

func handleCors(resp http.ResponseWriter, request *http.Request) bool {

	// FIXME - this is to handle multiple frontends in test rofl
	origin := request.Header["Origin"]
	resp.Header().Set("Vary", "Origin")
	if len(origin) > 0 {
		resp.Header().Set("Access-Control-Allow-Origin", origin[0])
	} else {
		resp.Header().Set("Access-Control-Allow-Origin", "http://localhost:4201")
	}
	//resp.Header().Set("Access-Control-Allow-Origin", "http://localhost:8000")
	resp.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept, X-Requested-With, remember-me")
	resp.Header().Set("Access-Control-Allow-Methods", "POST, GET, PUT, DELETE")
	resp.Header().Set("Access-Control-Allow-Credentials", "true")

	if request.Method == "OPTIONS" {
		resp.WriteHeader(200)
		resp.Write([]byte("OK"))
		return true
	}

	return false
}

func parseWorkflowParameters(resp http.ResponseWriter, request *http.Request) (map[string]interface{}, error) {
	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}

	log.Printf("Parsing data: %s", string(body))
	var t map[string]interface{}
	err = json.Unmarshal(body, &t)
	if err == nil {
		log.Printf("PARSED!! :)")
		return t, nil
	}

	// Translate XML to json in case of an XML blob.
	// FIXME - use Content-Type and Accept headers

	xml := strings.NewReader(string(body))
	curjson, err := xj.Convert(xml)
	if err != nil {
		return t, err
	}

	//fmt.Println(curjson.String())
	//log.Printf("Parsing json a second time: %s", string(curjson.String()))

	err = json.Unmarshal(curjson.Bytes(), &t)
	if err != nil {
		return t, nil
	}

	envelope := t["Envelope"].(map[string]interface{})
	curbody := envelope["Body"].(map[string]interface{})

	//log.Println(curbody)

	// ALWAYS handle strings only
	// FIXME - remove this and get it from config or something
	requiredField := "symptomDescription"
	_, found := SearchNested(curbody, requiredField)

	// Maxdepth
	maxiter := 5

	// Need to look for parent of the item, as that is most likely root
	if found {
		cnt := 0
		var previousDifferentItem map[string]interface{}
		var previousItem map[string]interface{}
		_ = previousItem
		for {
			if cnt == maxiter {
				break
			}

			// Already know it exists
			key, realItem, _ := SearchNestedParent(curbody, requiredField)

			// First should ALWAYS work since we already have recursion checked
			if len(previousDifferentItem) == 0 {
				previousDifferentItem = realItem.(map[string]interface{})
			}

			switch t := realItem.(type) {
			case map[string]interface{}:
				previousItem = realItem.(map[string]interface{})
				curbody = realItem.(map[string]interface{})
			default:
				// Gets here if it's not an object
				_ = t
				//log.Printf("hi %#v", previousItem)
				return previousItem, nil
			}

			_ = key
			cnt += 1
		}
	}

	//key, realItem, found = SearchNestedParent(newbody, requiredField)

	//if !found {
	//	log.Println("NOT FOUND!")
	//}

	////log.Println(realItem[requiredField].(map[string]interface{}))
	//log.Println(realItem[requiredField])
	//log.Printf("FOUND PARENT :): %s", key)

	return t, nil
}

// SearchNested searches a nested structure consisting of map[string]interface{}
// and []interface{} looking for a map with a specific key name.
// If found SearchNested returns the value associated with that key, true
func SearchNestedParent(obj interface{}, key string) (string, interface{}, bool) {
	switch t := obj.(type) {
	case map[string]interface{}:
		if v, ok := t[key]; ok {
			return "", v, ok
		}
		for k, v := range t {
			if _, ok := SearchNested(v, key); ok {
				return k, v, ok
			}
		}
	case []interface{}:
		for _, v := range t {
			if _, ok := SearchNested(v, key); ok {
				return "", v, ok
			}
		}
	}

	return "", nil, false
}

// SearchNested searches a nested structure consisting of map[string]interface{}
// and []interface{} looking for a map with a specific key name.
// If found SearchNested returns the value associated with that key, true
// If the key is not found SearchNested returns nil, false
func SearchNested(obj interface{}, key string) (interface{}, bool) {
	switch t := obj.(type) {
	case map[string]interface{}:
		if v, ok := t[key]; ok {
			return v, ok
		}
		for _, v := range t {
			if result, ok := SearchNested(v, key); ok {
				return result, ok
			}
		}
	case []interface{}:
		for _, v := range t {
			if result, ok := SearchNested(v, key); ok {
				return result, ok
			}
		}
	}
	return nil, false
}

func handleSetHook(resp http.ResponseWriter, request *http.Request) {
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

	var workflowId string
	if location[1] == "api" {
		if len(location) <= 4 {
			resp.WriteHeader(401)
			resp.Write([]byte(`{"success": false}`))
			return
		}

		workflowId = location[4]
	}

	if len(workflowId) != 32 {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "message": "ID not valid"}`))
		return
	}

	// FIXME - check basic authentication
	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		log.Printf("Error with body read: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	log.Println(jsonPrettyPrint(string(body)))

	var hook model.Hook
	err = json.Unmarshal(body, &hook)
	if err != nil {
		log.Printf("Failed unmarshaling: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	if user.Id != hook.Owner && user.Role != "admin" && user.Role != "scheduler" {
		log.Printf("Wrong user (%s) for hook %s", user.Username, hook.Id)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	if hook.Id != workflowId {
		errorstring := fmt.Sprintf(`Id %s != %s`, hook.Id, workflowId)
		log.Printf("Ids not matching: %s", errorstring)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "message": "%s"}`, errorstring)))
		return
	}

	// Verifies the hook JSON. Bad verification :^)
	finished, errorstring := verifyHook(hook)
	if !finished {
		log.Printf("Error with hook: %s", errorstring)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "message": "%s"}`, errorstring)))
		return
	}

	// Get the ID to see whether it exists
	// FIXME - use return and set READONLY fields (don't allow change from User)
	_, err = getHook(workflowId)
	if err != nil {
		log.Printf("Failed getting hook: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "message": "Invalid ID"}`))
		return
	}

	// Update the fields
	err = setHook(hook)
	if err != nil {
		log.Printf("Failed setting hook: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	resp.WriteHeader(200)
	resp.Write([]byte(`{"success": true}`))
}

// FIXME - some fields (e.g. status) shouldn't be writeable.. Meh
func verifyHook(hook model.Hook) (bool, string) {
	// required fields: Id, info.name, type, status, running
	if hook.Id == "" {
		return false, "Missing required field id"
	}

	if hook.Info.Name == "" {
		return false, "Missing required field info.name"
	}

	// Validate type stuff
	validTypes := []string{"webhook"}
	found := false
	for _, key := range validTypes {
		if hook.Type == key {
			found = true
			break
		}
	}

	if !found {
		return false, fmt.Sprintf("Field type is invalid. Allowed: %s", strings.Join(validTypes, ", "))
	}

	// WEbhook specific
	if hook.Type == "webhook" {
		if hook.Info.Url == "" {
			return false, "Missing required field info.url"
		}
	}

	if hook.Status == "" {
		return false, "Missing required field status"
	}

	validStatusFields := []string{"running", "stopped", "uninitialized"}
	found = false
	for _, key := range validStatusFields {
		if hook.Status == key {
			found = true
			break
		}
	}

	if !found {
		return false, fmt.Sprintf("Field status is invalid. Allowed: %s", strings.Join(validStatusFields, ", "))
	}

	// Verify actions
	if len(hook.Actions) > 0 {
		existingIds := []string{}
		for index, action := range hook.Actions {
			if action.Type == "" {
				return false, fmt.Sprintf("Missing required field actions.type at index %d", index)
			}

			if action.Name == "" {
				return false, fmt.Sprintf("Missing required field actions.name at index %d", index)
			}

			if action.Id == "" {
				return false, fmt.Sprintf("Missing required field actions.id at index %d", index)
			}

			// Check for duplicate IDs
			for _, actionId := range existingIds {
				if action.Id == actionId {
					return false, fmt.Sprintf("actions.id %s at index %d already exists", actionId, index)
				}
			}
			existingIds = append(existingIds, action.Id)
		}
	}

	return true, "All items set"
	//log.Printf("%#v", hook)

	//Id         string   `json:"id" datastore:"id"`
	//Info       Info     `json:"info" datastore:"info"`
	//Transforms struct{} `json:"transforms" datastore:"transforms"`
	//Actions    []HookAction `json:"actions" datastore:"actions"`
	//Type       string   `json:"type" datastore:"type"`
	//Status     string   `json:"status" datastore:"status"`
	//Running    bool     `json:"running" datastore:"running"`
}

func setSpecificSchedule(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	location := strings.Split(request.URL.String(), "/")

	var workflowId string
	if location[1] == "api" {
		if len(location) <= 4 {
			resp.WriteHeader(401)
			resp.Write([]byte(`{"success": false}`))
			return
		}

		workflowId = location[4]
	}

	if len(workflowId) != 32 {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "message": "ID not valid"}`))
		return
	}

	// FIXME - check basic authentication
	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		log.Printf("Error with body read: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	jsonPrettyPrint(string(body))
	var schedule model.ScheduleOld
	err = json.Unmarshal(body, &schedule)
	if err != nil {
		log.Printf("Failed unmarshaling: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	// FIXME - check access etc
	err = setSchedule(schedule)
	if err != nil {
		log.Printf("Failed setting schedule: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	// FIXME - get some real data?
	resp.WriteHeader(200)
	resp.Write([]byte(`{"success": true}`))
	return
}

func getSchedule(schedulename string) (*model.ScheduleOld, error) {
	schedule := &model.ScheduleOld{}
	if err := dbClient.One("Id", schedulename, schedule); err != nil {
		return &model.ScheduleOld{}, err
	}

	return schedule, nil
}


func getSpecificWebhook(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	location := strings.Split(request.URL.String(), "/")

	var workflowId string
	if location[1] == "api" {
		if len(location) <= 4 {
			resp.WriteHeader(401)
			resp.Write([]byte(`{"success": false}`))
			return
		}

		workflowId = location[4]
	}

	if len(workflowId) != 32 {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "message": "ID not valid"}`))
		return
	}

	schedule, err := getSchedule(workflowId)
	if err != nil {
		log.Printf("Failed setting schedule: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	//log.Printf("%#v", schedule.Translator[0])

	b, err := json.Marshal(schedule)
	if err != nil {
		log.Printf("Failed marshalling: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	// FIXME - get some real data?
	resp.WriteHeader(200)
	resp.Write([]byte(b))
	return
}

// Starts a new webhook
func handleDeleteSchedule(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	location := strings.Split(request.URL.String(), "/")

	var workflowId string
	if location[1] == "api" {
		if len(location) <= 4 {
			resp.WriteHeader(401)
			resp.Write([]byte(`{"success": false}`))
			return
		}

		workflowId = location[4]
	}

	if len(workflowId) != 32 {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "message": "ID not valid"}`))
		return
	}

	schedule, err := getSchedule(workflowId)
	if err != nil {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "message": "Can't delete"}`))
		return
	}
	err = dbClient.DeleteStruct(schedule)

	if err != nil {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "message": "Can't delete"}`))
		return
	}

	// FIXME - remove schedule too

	resp.WriteHeader(200)
	resp.Write([]byte(`{"success": true, "message": "Deleted webhook"}`))
}

// Starts a new webhook
func handleNewSchedule(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	randomValue := uuid.NewV4()
	h := md5.New()
	io.WriteString(h, randomValue.String())
	newId := strings.ToLower(fmt.Sprintf("%X", h.Sum(nil)))

	// FIXME - timestamp!
	// FIXME - applocation - cloud function?
	timeNow := int64(time.Now().Unix())
	schedule := model.ScheduleOld{
		Id:                   newId,
		AppInfo:              model.AppInfo{},
		BaseAppLocation:      "/home/frikky/git/shaffuru/tmp/apps",
		CreationTime:         timeNow,
		LastModificationtime: timeNow,
		LastRuntime:          timeNow,
	}

	err := setSchedule(schedule)
	if err != nil {
		log.Printf("Failed setting hook: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	log.Println("Generating new schedule")
	resp.WriteHeader(200)
	resp.Write([]byte(`{"success": true, "message": "Created new service"}`))
}

// Does the webhook
func handleWebhookCallback(resp http.ResponseWriter, request *http.Request) {
	// 1. Get callback data
	// 2. Load the configuration
	// 3. Execute the workflow

	path := strings.Split(request.URL.String(), "/")
	if len(path) < 4 {
		resp.WriteHeader(403)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	// 1. Get config with hookId
	//fmt.Sprintf("%s/api/v1/hooks/%s", callbackUrl, hookId)
	location := strings.Split(request.URL.String(), "/")

	var hookId string
	if location[1] == "api" {
		if len(location) <= 4 {
			resp.WriteHeader(401)
			resp.Write([]byte(`{"success": false}`))
			return
		}

		hookId = location[4]
	}

	// ID: webhook_<UID>
	if len(hookId) != 44 {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "message": "ID not valid"}`))
		return
	}

	hookId = hookId[8:len(hookId)]

	log.Printf("HookID: %s", hookId)
	hook, err := getHook(hookId)
	if err != nil {
		log.Printf("Failed getting hook: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	log.Printf("HOOK FOUND: %#v", hook)
	// Execute the workflow
	//executeWorkflow(resp, request)

	//resp.WriteHeader(200)
	//resp.Write([]byte(`{"success": true}`))
	if hook.Status == "stopped" {
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "The webhook isn't running. Click start to start it"}`)))
		return
	}

	if len(hook.Workflows) == 0 {
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "No workflows are defined"}`)))
		return
	}

	for _, item := range hook.Workflows {
		log.Printf("Running for workflow: %s", item)
		workflow := model.Workflow{
			ID: "",
		}

		workflowExecution, executionResp, err := handleExecution(item, workflow, request)

		if err == nil {
			err = increaseStatisticsField("total_webhooks_ran", workflowExecution.Workflow.ID, 1)
			if err != nil {
				log.Printf("Failed to increase total apps loaded stats: %s", err)
			}

			resp.WriteHeader(200)
			resp.Write([]byte(fmt.Sprintf(`{"success": true, "execution_id": "%s", "authorization": "%s"}`, workflowExecution.ExecutionId, workflowExecution.Authorization)))
			return
		}

		resp.WriteHeader(500)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "%s"}`, executionResp)))
	}
}

// Starts a new webhook
func handleNewHook(resp http.ResponseWriter, request *http.Request) {
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

	type requestData struct {
		Type        string `json:"type"`
		Description string `json:"description"`
		Id          string `json:"id"`
		Name        string `json:"name"`
		Workflow    string `json:"workflow"`
	}

	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		log.Printf("Body data error: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	log.Println("Data: %s", string(body))

	var requestdata requestData
	err = yaml.Unmarshal([]byte(body), &requestdata)
	if err != nil {
		log.Printf("Failed unmarshaling inputdata: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}
	log.Printf("%#v", requestdata)

	// CBA making a real thing. Already had some code lol
	newId := requestdata.Id
	if len(newId) != 36 {
		log.Printf("Bad ID")
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Invalid ID"}`))
		return
	}

	if requestdata.Id == "" || requestdata.Name == "" {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Requires fields id and name can't be empty"}`))
		return

	}

	validTypes := []string{
		"webhook",
	}

	isTypeValid := false
	for _, thistype := range validTypes {
		if requestdata.Type == thistype {
			isTypeValid = true
			break
		}
	}

	if !(isTypeValid) {
		log.Printf("Type %s is not valid. Try any of these: %s", requestdata.Type, strings.Join(validTypes, ", "))
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	hook := model.Hook{
		Id:        newId,
		Workflows: []string{requestdata.Workflow},
		Info: model.Info{
			Name:        requestdata.Name,
			Description: requestdata.Description,
			Url:         fmt.Sprintf("https://shuffler.io/functions/webhooks/webhook_%s", newId),
		},
		Type:   "webhook",
		Owner:  user.Username,
		Status: "uninitialized",
		Actions: []model.HookAction{
			model.HookAction{
				Type:  "workflow",
				Name:  requestdata.Name,
				Id:    requestdata.Workflow,
				Field: "",
			},
		},
		Running: false,
	}

	log.Printf("Hello")

	// FIXME: Add cloud function execution?
	//b, err := json.Marshal(hook)
	//if err != nil {
	//	log.Printf("Failed marshalling: %s", err)
	//	resp.WriteHeader(401)
	//	resp.Write([]byte(`{"success": false}`))
	//	return
	//}

	//environmentVariables := map[string]string{
	//	"FUNCTION_APIKEY": user.ApiKey,
	//	"CALLBACKURL":     "https://shuffler.io",
	//	"HOOKID":          hook.Id,
	//}

	//applocation := fmt.Sprintf("gs://%s/triggers/webhook.zip", bucketName)
	//hookname := fmt.Sprintf("webhook_%s", hook.Id)
	//err = deployWebhookFunction(ctx, hookname, defaultLocation, applocation, environmentVariables)
	//if err != nil {
	//	log.Printf("Error deploying hook: %s", err)
	//	resp.WriteHeader(401)
	//	resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Issue with starting hook. Please wait a second and try again"}`)))
	//	return
	//}

	hook.Status = "running"
	hook.Running = true
	err = setHook(hook)
	if err != nil {
		log.Printf("Failed setting hook: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	err = increaseStatisticsField("total_workflow_triggers", requestdata.Workflow, 1)
	if err != nil {
		log.Printf("Failed to increase total workflows: %s", err)
	}

	log.Println("Generating new hook")
	resp.WriteHeader(200)
	resp.Write([]byte(`{"success": true}`))
}

func sendHookResult(resp http.ResponseWriter, request *http.Request) {
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
	_ = user

	location := strings.Split(request.URL.String(), "/")

	var workflowId string
	if location[1] == "api" {
		if len(location) <= 4 {
			resp.WriteHeader(401)
			resp.Write([]byte(`{"success": false}`))
			return
		}

		workflowId = location[4]
	}

	if len(workflowId) != 32 {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "message": "ID not valid"}`))
		return
	}

	hook, err := getHook(workflowId)
	if err != nil {
		log.Printf("Failed getting hook: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		log.Printf("Body data error: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	log.Printf("SET the hook results for %s to %s", workflowId, body)
	// FIXME - set the hook result in the DB somehow as interface{}
	// FIXME - should the hook do the transform? Hmm

	b, err := json.Marshal(hook)
	if err != nil {
		log.Printf("Failed marshalling: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	resp.WriteHeader(200)
	resp.Write([]byte(b))
	return
}

func handleGetHook(resp http.ResponseWriter, request *http.Request) {
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

	var workflowId string
	if location[1] == "api" {
		if len(location) <= 4 {
			resp.WriteHeader(401)
			resp.Write([]byte(`{"success": false}`))
			return
		}

		workflowId = location[4]
	}

	if len(workflowId) != 36 {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "message": "ID not valid"}`))
		return
	}

	hook, err := getHook(workflowId)
	if err != nil {
		log.Printf("Failed getting hook: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	if user.Id != hook.Owner && user.Role != "admin" && user.Role != "scheduler" {
		log.Printf("Wrong user (%s) for hook %s", user.Username, hook.Id)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	b, err := json.Marshal(hook)
	if err != nil {
		log.Printf("Failed marshalling: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	// FIXME - get some real data?
	resp.WriteHeader(200)
	resp.Write([]byte(b))
	return
}

func getSpecificSchedule(resp http.ResponseWriter, request *http.Request) {
	if request.Method != "GET" {
		setSpecificSchedule(resp, request)
		return
	}

	cors := handleCors(resp, request)
	if cors {
		return
	}

	location := strings.Split(request.URL.String(), "/")

	var workflowId string
	if location[1] == "api" {
		if len(location) <= 4 {
			resp.WriteHeader(401)
			resp.Write([]byte(`{"success": false}`))
			return
		}

		workflowId = location[4]
	}

	if len(workflowId) != 32 {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "message": "ID not valid"}`))
		return
	}

	schedule, err := getSchedule(workflowId)
	if err != nil {
		log.Printf("Failed getting schedule: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	//log.Printf("%#v", schedule.Translator[0])

	b, err := json.Marshal(schedule)
	if err != nil {
		log.Printf("Failed marshalling: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	resp.WriteHeader(200)
	resp.Write([]byte(b))
}

func loadYaml(fileLocation string) (model.ApiYaml, error) {
	apiYaml := model.ApiYaml{}

	yamlFile, err := ioutil.ReadFile(fileLocation)
	if err != nil {
		log.Printf("yamlFile.Get err: %s", err)
		return model.ApiYaml{}, err
	}

	err = yaml.Unmarshal([]byte(yamlFile), &apiYaml)
	if err != nil {
		return model.ApiYaml{}, err
	}

	return apiYaml, nil
}

// This should ALWAYS come from an OUTPUT
func executeSchedule(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	location := strings.Split(request.URL.String(), "/")
	var workflowId string

	if location[1] == "api" {
		if len(location) <= 4 {
			resp.WriteHeader(401)
			resp.Write([]byte(`{"success": false}`))
			return
		}

		workflowId = location[4]
	}

	if len(workflowId) != 32 {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "message": "ID not valid"}`))
		return
	}

	log.Printf("EXECUTING %s!", workflowId)
	idConfig, err := getSchedule(workflowId)
	if err != nil {
		log.Printf("Error getting schedule: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "%s"}`, err)))
		return
	}

	// Basically the src app
	inputStrings := map[string]string{}
	for _, item := range idConfig.Translator {
		if item.Dst.Required == "false" {
			log.Println("Skipping not required")
			continue
		}

		if item.Src.Name == "" {
			errorMsg := fmt.Sprintf("Required field %s has no source", item.Dst.Name)
			log.Println(errorMsg)
			resp.WriteHeader(401)
			resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "%s"}`, errorMsg)))
			return
		}

		inputStrings[item.Dst.Name] = item.Src.Name
	}

	configmap := map[string]string{}
	for _, config := range idConfig.AppInfo.SourceApp.Config {
		configmap[config.Key] = config.Value
	}

	// FIXME - this wont work for everything lmao
	functionName := strings.ToLower(idConfig.AppInfo.SourceApp.Action)
	functionName = strings.Replace(functionName, " ", "_", 10)

	cmdArgs := []string{
		fmt.Sprintf("%s/%s/app.py", baseAppPath, "thehive"),
		fmt.Sprintf("--referenceid=%s", workflowId),
		fmt.Sprintf("--function=%s", functionName),
	}

	for key, value := range configmap {
		cmdArgs = append(cmdArgs, fmt.Sprintf("--%s=%s", key, value))
	}

	// FIXME - processname
	baseProcess := "python3"
	log.Printf("Executing: %s %s", baseProcess, strings.Join(cmdArgs, " "))
	execSubprocess(baseProcess, cmdArgs)

	resp.WriteHeader(200)
	resp.Write([]byte(`{"success": true}`))
}

func execSubprocess(cmdName string, cmdArgs []string) error {
	cmd := exec.Command(cmdName, cmdArgs...)
	cmdReader, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error creating StdoutPipe for Cmd", err)
		return err
	}

	scanner := bufio.NewScanner(cmdReader)
	go func() {
		for scanner.Scan() {
			fmt.Printf("Out: %s\n", scanner.Text())
		}
	}()

	err = cmd.Start()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error starting Cmd", err)
		return err
	}

	err = cmd.Wait()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error waiting for Cmd", err)
		return err
	}

	return nil
}

// This should ALWAYS come from an OUTPUT
func uploadWorkflowResult(resp http.ResponseWriter, request *http.Request) {
	// Post to a key with random data?
	location := strings.Split(request.URL.String(), "/")

	var workflowId string
	if location[1] == "api" {
		if len(location) <= 4 {
			resp.WriteHeader(401)
			resp.Write([]byte(`{"success": false}`))
			return
		}

		workflowId = location[4]
	}

	if len(workflowId) != 32 {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "message": "ID not valid"}`))
		return
	}

	// FIXME - check if permission AND whether it exists

	// FIXME - validate ID as well
	schedule, err := getSchedule(workflowId)
	if err != nil {
		log.Printf("Failed setting schedule %s: %s", workflowId, err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	// Should use generic interfaces and parse fields OR
	// build temporary struct based on api.yaml of the app
	data, err := parseWorkflowParameters(resp, request)
	if err != nil {
		log.Printf("Invalid params: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "%s"}`, err)))
		return
	}

	// Get the actual fields
	foldername := schedule.AppInfo.SourceApp.Foldername
	curOutputType := schedule.AppInfo.SourceApp.Name
	curOutputAppOutput := schedule.AppInfo.SourceApp.Action
	curInputType := schedule.AppInfo.DestinationApp.Name
	translatormap := schedule.Translator

	if len(curOutputType) <= 0 {
		log.Printf("Id %s is invalid. Missing sourceapp name", workflowId)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false}`)))
		return
	}

	if len(foldername) == 0 {
		foldername = strings.ToLower(curOutputType)
	}

	if len(curOutputAppOutput) <= 0 {
		log.Printf("Id %s is invalid. Missing source output ", workflowId)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false}`)))
		return
	}

	if len(curInputType) <= 0 {
		log.Printf("Id %s is invalid. Missing destination name", workflowId)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false}`)))
		return
	}

	// Needs to be used for parsing properly
	// Might be dumb to have the yaml as a file too
	yamlpath := fmt.Sprintf("%s/%s/api.yaml", baseAppPath, foldername)
	curyaml, err := loadYaml(yamlpath)
	if err != nil {
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "%s"}`, err)))
		return
	}

	//validFields := []string{}
	requiredFields := []string{}
	optionalFields := []string{}
	for _, output := range curyaml.Output {
		if output.Name != curOutputAppOutput {
			continue
		}

		for _, outputparam := range output.OutputParameters {
			if outputparam.Required == "true" {
				if outputparam.Schema.Type == "string" {
					requiredFields = append(requiredFields, outputparam.Name)
				} else {
					log.Printf("Outputparam schematype %s is not implemented.", outputparam.Schema.Type)
				}
			} else {
				optionalFields = append(optionalFields, outputparam.Name)
			}
		}

		// Wont reach here unless it's the right one
		break
	}

	// Checks whether ALL required fields are filled
	for _, fieldname := range requiredFields {
		if data[fieldname] == nil {
			resp.WriteHeader(401)
			resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Field %s is required"}`, fieldname)))
			return
		} else {
			log.Printf("%s: %s", fieldname, data[fieldname])
		}
	}

	// FIXME
	// Verify whether it can be sent from the source to destination here
	// Save to DB or send it straight? Idk
	// Use e.g. google pubsub if cloud and maybe kafka locally

	// FIXME - add more types :)
	sourcedatamap := map[string]string{}
	for key, value := range data {
		switch v := value.(type) {
		case string:
			sourcedatamap[key] = value.(string)
		default:
			log.Printf("unexpected type %T", v)
		}
	}

	log.Println(data)
	log.Println(requiredFields)
	log.Println(translatormap)
	log.Println(sourcedatamap)

	outputmap := map[string]string{}
	for _, translator := range translatormap {
		if translator.Src.Type == "static" {
			log.Printf("%s = %s", translator.Dst.Name, translator.Src.Value)
			outputmap[translator.Dst.Name] = translator.Src.Value
		} else {
			log.Printf("%s = %s", translator.Dst.Name, translator.Src.Name)
			outputmap[translator.Dst.Name] = sourcedatamap[translator.Src.Name]
		}
	}

	configmap := map[string]string{}
	for _, config := range schedule.AppInfo.DestinationApp.Config {
		configmap[config.Key] = config.Value
	}

	// FIXME - add function to run
	// FIXME - add reference somehow
	// FIXME - add apikey somehow
	// Just package and run really?

	// FIXME - generate from sourceapp
	outputmap["function"] = "create_alert"
	cmdArgs := []string{
		fmt.Sprintf("%s/%s/app.py", baseAppPath, foldername),
	}

	for key, value := range outputmap {
		cmdArgs = append(cmdArgs, fmt.Sprintf("--%s=%s", key, value))
	}

	// COnfig map!
	for key, value := range configmap {
		cmdArgs = append(cmdArgs, fmt.Sprintf("--%s=%s", key, value))
	}
	outputmap["referenceid"] = workflowId

	baseProcess := "python3"
	log.Printf("Executing: %s %s", baseProcess, strings.Join(cmdArgs, " "))
	execSubprocess(baseProcess, cmdArgs)

	resp.WriteHeader(200)
	resp.Write([]byte(`{"success": true}`))
}

// Index = Username
func setSchedule(schedule model.ScheduleOld) error {
	// New struct, to not add body, author etc
	if err := dbClient.Save(&schedule); err != nil {
		log.Printf("Error adding schedule: %s", err)
		return err
	}

	return nil
}

//dst: {name: "title", required: "true", type: "string"}
//
//"title": "symptomDescription",
//"description": "detailedDescription",
//"type": "ticketType",
//"sourceRef": "ticketId"
//"name": "secureworks",
//"id": "e07910a06a086c83ba41827aa00b26ed",
//"description": "I AM SECUREWORKS DESC",
//"action": "Get Tickets",
//"config": {}
//"name": "thehive",
//			"id": "e07910a06a086c83ba41827aa00b26ef",
//			"description": "I AM thehive DESC",
//			"action": "Add ticket",
//			"config": [{
//				"key": "http://localhost:9000",
//				"value": "kZJmmn05j8wndOGDGvKg/D9eKub1itwO"
//			}]

func getAllScheduleApps(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	var err error
	var limit = 50

	// FIXME - add org search and public / private
	key, ok := request.URL.Query()["limit"]
	if ok {
		limit, err = strconv.Atoi(key[0])
		if err != nil {
			limit = 50
		}
	}

	// Max datastore limit
	if limit > 1000 {
		limit = 1000
	}

	// Get URLs from a database index (mapped by orborus)
	var allappschedules model.ScheduleApps

	err = dbClient.All(&allappschedules.Apps, storm.Limit(limit))
	if err != nil {
		log.Printf("Failed getting all apps: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Failed getting apps"}`)))
		return
	}

	newjson, err := json.Marshal(allappschedules)
	if err != nil {
		log.Printf("Failed unmarshal: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Failed unpacking"}`)))
		return
	}

	resp.WriteHeader(200)
	resp.Write(newjson)
}

func findValidScheduleAppFolders(rootAppFolder string) ([]string, error) {
	rootFiles, err := ioutil.ReadDir(rootAppFolder)
	if err != nil {
		return []string{}, err
	}

	invalidRootFiles := []string{}
	invalidRootFolders := []string{}
	invalidAppFolders := []string{}
	validAppFolders := []string{}

	// This is dumb
	allowedLanguages := []string{"py", "go"}

	for _, rootfile := range rootFiles {
		if !rootfile.IsDir() {
			invalidRootFiles = append(invalidRootFiles, rootfile.Name())
			continue
		}

		appFolderLocation := fmt.Sprintf("%s/%s", rootAppFolder, rootfile.Name())
		appFiles, err := ioutil.ReadDir(appFolderLocation)
		if err != nil {
			// Invalid app folder (deleted within a few MS lol)
			log.Printf("%s", err)
			invalidRootFolders = append(invalidRootFolders, rootfile.Name())
			continue
		}

		yamlFileDone := false
		appFileExists := false
		for _, appfile := range appFiles {
			if appfile.Name() == "api.yaml" {
				err := validateAppYaml(
					fmt.Sprintf("%s/%s", appFolderLocation, appfile.Name()),
				)

				if err != nil {
					log.Printf("Error in %s: %s", fmt.Sprintf("%s/%s", rootfile.Name(), appfile.Name()), err)
					break
				}

				log.Printf("YAML FOR %s: %s IS VALID!!", rootfile.Name(), appfile.Name())
				yamlFileDone = true
			}

			for _, language := range allowedLanguages {
				if appfile.Name() == fmt.Sprintf("app.%s", language) {
					log.Printf("Appfile found for %s", rootfile.Name())
					appFileExists = true
					break
				}
			}
		}

		if !yamlFileDone || !appFileExists {
			invalidAppFolders = append(invalidAppFolders, rootfile.Name())
		} else {
			validAppFolders = append(validAppFolders, rootfile.Name())
		}
	}

	log.Printf("Invalid rootfiles: %s", strings.Join(invalidRootFiles, ", "))
	log.Printf("Invalid rootfolders: %s", strings.Join(invalidRootFolders, ", "))
	log.Printf("Invalid appfolders: %s", strings.Join(invalidAppFolders, ", "))
	log.Printf("\n=== VALID appfolders ===\n* %s", strings.Join(validAppFolders, "\n"))

	return validAppFolders, err
}

func validateInputOutputYaml(appType string, apiYaml model.ApiYaml) error {
	if appType == "input" {
		for index, input := range apiYaml.Input {
			if input.Name == "" {
				return errors.New(fmt.Sprintf("YAML field name doesn't exist in index %d of Input", index))
			}
			if input.Description == "" {
				return errors.New(fmt.Sprintf("YAML field description doesn't exist in index %d of Input", index))
			}

			for paramindex, param := range input.InputParameters {
				if param.Name == "" {
					return errors.New(fmt.Sprintf("YAML field name doesn't exist in Input %s with index %d", input.Name, paramindex))
				}

				if param.Description == "" {
					return errors.New(fmt.Sprintf("YAML field description doesn't exist in Input %s with index %d", input.Name, index))
				}

				if param.Schema.Type == "" {
					return errors.New(fmt.Sprintf("YAML field schema.type doesn't exist in Input %s with index %d", input.Name, index))
				}
			}
		}
	}

	return nil
}

func validateAppYaml(fileLocation string) error {
	/*
		Requires:
		name, description, app_version, contact_info (name), types
	*/

	apiYaml, err := loadYaml(fileLocation)
	if err != nil {
		return err
	}

	// Validate fields
	if apiYaml.Name == "" {
		return errors.New("YAML field name doesn't exist")
	}
	if apiYaml.Description == "" {
		return errors.New("YAML field description doesn't exist")
	}

	if apiYaml.AppVersion == "" {
		return errors.New("YAML field app_version doesn't exist")
	}

	if apiYaml.ContactInfo.Name == "" {
		return errors.New("YAML field contact_info.name doesn't exist")
	}

	if len(apiYaml.Types) == 0 {
		return errors.New("YAML field types doesn't exist")
	}

	// Validate types (input/ouput)
	validTypes := []string{"input", "output"}
	for _, appType := range apiYaml.Types {
		// Validate in here lul
		for _, validType := range validTypes {
			if appType == validType {
				err = validateInputOutputYaml(appType, apiYaml)
				if err != nil {
					return err
				}
				break
			}
		}
	}

	return nil
}

func getHook(hookId string) (*model.Hook, error) {
	hook := &model.Hook{}
	if err := dbClient.One("ID", strings.ToLower(hookId), hook); err != nil {
		return &model.Hook{}, err
	}

	return hook, nil
}

func setHook(hook model.Hook) error {
	// New struct, to not add body, author etc
	if err := dbClient.Save(&hook); err != nil {
		log.Printf("Error adding hook: %s", err)
		return err
	}

	return nil
}

func handleGetallHooks(resp http.ResponseWriter, request *http.Request) {
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

	// With user, do a search for workflows with user or user's org attached
	var allhooks []model.Hook
	err = dbClient.Find("Owner", user.Username, &allhooks)
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

	if len(allhooks) == 0 {
		resp.WriteHeader(200)
		resp.Write([]byte("[]"))
		return
	}

	newjson, err := json.Marshal(allhooks)
	if err != nil {
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Failed unpacking"}`)))
		return
	}

	resp.WriteHeader(200)
	resp.Write(newjson)
}

//func deployWebhookCloudrun(ctx context.Context) {
//	service, err := cloudrun.NewService(ctx)
//	_ = err
//
//	projectsLocationsService := cloudrun.NewProjectsLocationsService(service)
//	log.Printf("%#v", projectsLocationsService)
//	projectsLocationsGetCall := projectsLocationsService.Get("webhook")
//	log.Printf("%#v", projectsLocationsGetCall)
//
//	location, err := projectsLocationsGetCall.Do()
//	log.Printf("%#v, err: %s", location, err)
//
//	//func NewProjectsLocationsService(s *Service) *ProjectsLocationsService {
//	//func (r *ProjectsLocationsService) Get(name string) *ProjectsLocationsGetCall {
//	//func (c *ProjectsLocationsGetCall) Do(opts ...googleapi.CallOption) (*Location, error) {
//}

// Finds available ports
func findAvailablePorts(startRange int64, endRange int64) string {
	for i := startRange; i < endRange; i++ {
		s := strconv.FormatInt(i, 10)
		l, err := net.Listen("tcp", ":"+s)

		if err == nil {
			l.Close()
			return s
		}
	}

	return ""
}

func handleSendalert(resp http.ResponseWriter, request *http.Request) {
	user, err := handleApiAuthentication(resp, request)
	if err != nil {
		log.Printf("Api authentication failed in getworkflows: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	if user.Role != "mail" && user.Role != "admin" {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "You don't have access to send mail"}`))
		return
	}

	// ReferenceExecution and below are for execution continuations when user inputs arrive
	type mailcheck struct {
		Targets            []string `json:"targets"`
		Body               string   `json:"body"`
		Subject            string   `json:"subject"`
		Type               string   `json:"type"`
		SenderCompany      string   `json:"sender_company"`
		ReferenceExecution string   `json:"reference_execution"`
		WorkflowId         string   `json:"workflow_id"`
		ExecutionType      string   `json:"execution_type"`
		Start              string   `json:"start"`
	}

	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		log.Printf("Body data error on mail: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	var mailbody mailcheck
	err = json.Unmarshal(body, &mailbody)
	if err != nil {
		log.Printf("Unmarshal error on mail: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	ctx := context.Background()
	confirmMessage := `
You have a new alert from shuffler.io!

%s

Please contact us at shuffler.io or frikky@shuffler.io if there is an issue with this message.`

	parsedBody := fmt.Sprintf(confirmMessage, mailbody.Body)

	// FIXME - Make a continuation email here - might need more info from worker
	// making the request, e.g. what the next start-node is and execution_id for
	// how to make the links
	if mailbody.Type == "User input" {
		authkey := uuid.NewV4().String()

		log.Printf("Should handle differentiator for user input in email!")
		log.Printf("%#v", mailbody)

		url := "https://shuffler.io"
		//url := "http://localhost:5001"
		continueUrl := fmt.Sprintf("%s/api/v1/workflows/%s/execute?authorization=%s&start=%s&reference_execution=%s&answer=true", url, mailbody.WorkflowId, authkey, mailbody.Start, mailbody.ReferenceExecution)
		stopUrl := fmt.Sprintf("%s/api/v1/workflows/%s/execute?authorization=%s&start=%s&reference_execution=%s&answer=false", url, mailbody.WorkflowId, authkey, mailbody.Start, mailbody.ReferenceExecution)

		//item := &memcache.Item{
		//	Key:        authkey,
		//	Value:      []byte(fmt.Sprintf(`{"role": "workflow_%s"}`, mailbody.WorkflowId)),
		//	Expiration: time.Minute * 1200,
		//}

		//if err := memcache.Add(ctx, item); err == memcache.ErrNotStored {
		//	if err := memcache.Set(ctx, item); err != nil {
		//		log.Printf("Error setting new user item: %v", err)
		//	}
		//} else if err != nil {
		//	log.Printf("error adding item: %v", err)
		//} else {
		//	log.Printf("Set cache for %s", item.Key)
		//}

		parsedBody = fmt.Sprintf(`
Action required!
			
%s

If this is TRUE click this: %s

IF THIS IS FALSE, click this: %s

Please contact us at shuffler.io or frikky@shuffler.io if there is an issue with this message.
`, mailbody.Body, continueUrl, stopUrl)

	}

	msg := &mail.Message{
		Sender:  "Shuffle <frikky@shuffler.io>",
		To:      mailbody.Targets,
		Subject: fmt.Sprintf("Shuffle - %s - %s", mailbody.Type, mailbody.Subject),
		Body:    parsedBody,
	}

	log.Println(msg.Body)
	if err := mail.Send(ctx, msg); err != nil {
		log.Printf("Couldn't send email: %v", err)
	}

	resp.WriteHeader(200)
	resp.Write([]byte("OK"))
}

func setBadMemcache(ctx context.Context, path string) {
	// Add to cache if it doesn't exist
	//item := &memcache.Item{
	//	Key:        path,
	//	Value:      []byte(`{"success": false}`),
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
}

func getDocList(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	ctx := context.Background()
	//if item, err := memcache.Get(ctx, "docs_list"); err == memcache.ErrCacheMiss {
	//	// Not in cache
	//} else if err != nil {
	//	// Error with cache
	//	log.Printf("Error getting item: %v", err)
	//} else {
	//	resp.WriteHeader(200)
	//	resp.Write([]byte(item.Value))
	//	return
	//}

	client := github.NewClient(nil)
	_, item1, _, err := client.Repositories.GetContents(ctx, "shaffuru", "shuffle-docs", "docs", nil)
	if err != nil {
		resp.WriteHeader(500)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Error listing directory"`)))
		return
	}

	if len(item1) == 0 {
		resp.WriteHeader(500)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "No docs available."`)))
		return
	}

	names := []string{}
	for _, item := range item1 {
		if !strings.HasSuffix(*item.Name, "md") {
			continue
		}

		names = append(names, (*item.Name)[0:len(*item.Name)-3])
	}

	log.Println(names)

	type Result struct {
		Success bool     `json:"success"`
		Reason  string   `json:"reason"`
		List    []string `json:"list"`
	}

	var result Result
	result.Success = true
	result.Reason = "Success"
	result.List = names
	b, err := json.Marshal(result)
	if err != nil {
		http.Error(resp, err.Error(), 500)
		return
	}

	//item := &memcache.Item{
	//	Key:        "docs_list",
	//	Value:      b,
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

	resp.WriteHeader(200)
	resp.Write(b)
}

// r.HandleFunc("/api/v1/docs/{key}", getDocs).Methods("GET", "OPTIONS")
func getDocs(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	location := strings.Split(request.URL.String(), "/")
	if len(location) != 5 {
		resp.WriteHeader(404)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Bad path. Use e.g. /api/v1/docs/workflows.md"`)))
		return
	}

	//ctx := context.Background()
	docPath := fmt.Sprintf("https://raw.githubusercontent.com/shaffuru/shuffle-docs/master/docs/%s.md", location[4])
	//if item, err := memcache.Get(ctx, docPath); err == memcache.ErrCacheMiss {
	//	// Not in cache
	//} else if err != nil {
	//	// Error with cache
	//	log.Printf("Error getting item: %v", err)
	//} else {
	//	resp.WriteHeader(200)
	//	resp.Write([]byte(item.Value))
	//	return
	//}

	client := &http.Client{}
	req, err := http.NewRequest(
		"GET",
		docPath,
		nil,
	)

	if err != nil {
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Bad path. Use e.g. /api/v1/docs/workflows.md"`)))
		resp.WriteHeader(404)
		//setBadMemcache(ctx, docPath)
		return
	}

	newresp, err := client.Do(req)
	if err != nil {
		resp.WriteHeader(404)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Bad path. Use e.g. /api/v1/docs/workflows.md"`)))
		//setBadMemcache(ctx, docPath)
		return
	}

	body, err := ioutil.ReadAll(newresp.Body)
	if err != nil {
		resp.WriteHeader(500)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Can't parse data"`)))
		//setBadMemcache(ctx, docPath)
		return
	}

	type Result struct {
		Success bool   `json:"success"`
		Reason  string `json:"reason"`
	}

	var result Result
	result.Success = true

	//applog.Infof(ctx, string(body))
	//applog.Infof(ctx, "Url: %s", docPath)
	//applog.Infof(ctx, "Status: %d", newresp.StatusCode)
	//applog.Infof(ctx, "GOT BODY OF LENGTH %d", len(string(body)))

	result.Reason = string(body)
	b, err := json.Marshal(result)
	if err != nil {
		http.Error(resp, err.Error(), 500)
		//setBadMemcache(ctx, docPath)
		return
	}

	// Add to cache if it doesn't exist
	//item := &memcache.Item{
	//	Key:        docPath,
	//	Value:      b,
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

	resp.WriteHeader(200)
	resp.Write(b)
}

type OutlookProfile struct {
	OdataContext      string      `json:"@odata.context"`
	BusinessPhones    []string    `json:"businessPhones"`
	DisplayName       string      `json:"displayName"`
	GivenName         string      `json:"givenName"`
	JobTitle          interface{} `json:"jobTitle"`
	Mail              string      `json:"mail"`
	MobilePhone       interface{} `json:"mobilePhone"`
	OfficeLocation    interface{} `json:"officeLocation"`
	PreferredLanguage interface{} `json:"preferredLanguage"`
	Surname           string      `json:"surname"`
	UserPrincipalName string      `json:"userPrincipalName"`
	ID                string      `json:"id"`
}

type OutlookFolder struct {
	ID               string `json:"id"`
	DisplayName      string `json:"displayName"`
	ParentFolderID   string `json:"parentFolderId"`
	ChildFolderCount int    `json:"childFolderCount"`
	UnreadItemCount  int    `json:"unreadItemCount"`
	TotalItemCount   int    `json:"totalItemCount"`
}

type OutlookFolders struct {
	OdataContext  string          `json:"@odata.context"`
	OdataNextLink string          `json:"@odata.nextLink"`
	Value         []OutlookFolder `json:"value"`
}

func getOutlookFolders(client *http.Client) (OutlookFolders, error) {
	requestUrl := fmt.Sprintf("https://graph.microsoft.com/v1.0/users/frikky@shuffletest.onmicrosoft.com/mailfolders")

	ret, err := client.Get(requestUrl)
	if err != nil {
		log.Printf("FolderErr: %s", err)
		return OutlookFolders{}, err
	}

	if ret.StatusCode != 200 {
		log.Printf("Status folders: %d", ret.StatusCode)
		return OutlookFolders{}, err
	}

	body, err := ioutil.ReadAll(ret.Body)
	if err != nil {
		log.Printf("Body: %s", err)
		return OutlookFolders{}, err
	}

	//log.Printf("Body: %s", string(body))

	mailfolders := OutlookFolders{}
	err = json.Unmarshal(body, &mailfolders)
	if err != nil {
		log.Printf("Unmarshal: %s", err)
		return OutlookFolders{}, err
	}

	//fmt.Printf("%#v", mailfolders)
	// FIXME - recursion for subfolders
	// Recursive struct
	// folderEndpoint := fmt.Sprintf("%s/%s/childfolders?$top=40", requestUrl, parentId)
	//for _, folder := range mailfolders.Value {
	//	log.Println(folder.DisplayName)
	//}

	return mailfolders, nil
}

func getOutlookProfile(client *http.Client) (OutlookProfile, error) {
	requestUrl := fmt.Sprintf("https://graph.microsoft.com/v1.0/me?$select=mail")

	ret, err := client.Get(requestUrl)
	if err != nil {
		log.Printf("FolderErr: %s", err)
		return OutlookProfile{}, err
	}

	log.Printf("Status folders: %d", ret.StatusCode)
	body, err := ioutil.ReadAll(ret.Body)
	if err != nil {
		log.Printf("Body: %s", err)
		return OutlookProfile{}, err
	}

	profile := OutlookProfile{}
	err = json.Unmarshal(body, &profile)
	if err != nil {
		log.Printf("Unmarshal: %s", err)
		return OutlookProfile{}, err
	}

	return profile, nil
}

func handleNewOutlookRegister(resp http.ResponseWriter, request *http.Request) {
	code := request.URL.Query().Get("code")
	if len(code) == 0 {
		log.Println("No code")
		resp.WriteHeader(401)
		return
	}

	url := fmt.Sprintf("http://%s%s", request.Host, request.URL.EscapedPath())
	log.Println(url)
	ctx := context.Background()
	client, accessToken, err := getOutlookClient(ctx, code, OauthToken{}, url)
	if err != nil {
		log.Printf("Oauth client failure - outlook register: %s", err)
		resp.WriteHeader(401)
		return
	}
	// This should be possible, and will also give the actual username
	profile, err := getOutlookProfile(client)
	if err != nil {
		log.Printf("Outlook profile failure: %s", err)
		resp.WriteHeader(401)
		return
	}

	// This is a state workaround, which should really be for CSRF checks lol
	state := request.URL.Query().Get("state")
	if len(state) == 0 {
		log.Println("No state")
		resp.WriteHeader(401)
		return
	}

	stateitems := strings.Split(state, "%26")
	if len(stateitems) == 1 {
		stateitems = strings.Split(state, "&")
	}

	// FIXME - trigger auth
	senderUser := ""
	trigger := TriggerAuth{}
	for _, item := range stateitems {
		itemsplit := strings.Split(item, "%3D")
		if len(itemsplit) == 1 {
			itemsplit = strings.Split(item, "=")
		}

		if len(itemsplit) != 2 {
			continue
		}

		// Do something here
		if itemsplit[0] == "workflow_id" {
			trigger.WorkflowId = itemsplit[1]
		} else if itemsplit[0] == "trigger_id" {
			trigger.Id = itemsplit[1]
		} else if itemsplit[0] == "type" {
			trigger.Type = itemsplit[1]
		} else if itemsplit[0] == "username" {
			trigger.Username = itemsplit[1]
			trigger.Owner = itemsplit[1]
			senderUser = itemsplit[1]
		}
	}

	// THis is an override based on the user in oauth return
	trigger.Username = profile.Mail
	trigger.Code = code
	trigger.OauthToken = OauthToken{
		AccessToken:  accessToken.AccessToken,
		TokenType:    accessToken.TokenType,
		RefreshToken: accessToken.RefreshToken,
		Expiry:       accessToken.Expiry,
	}

	//log.Printf("%#v", trigger)
	if trigger.WorkflowId == "" || trigger.Id == "" || trigger.Username == "" || trigger.Type == "" {
		log.Printf("All oauth items need to contain data to register a new state")
		resp.WriteHeader(401)
		return
	}

	// Should also update the user
	Userdata, err := getUser(senderUser)
	if err != nil {
		log.Printf("Username %s doesn't exist (oauth2): %s", trigger.Username, err)
		resp.WriteHeader(401)
		return
	}

	Userdata.Authentication = append(Userdata.Authentication, model.UserAuth{
		Name:        "Outlook",
		Description: "oauth2",
		Workflows:   []string{trigger.WorkflowId},
		Username:    trigger.Username,
		Fields: []model.UserAuthField{
			model.UserAuthField{
				Key:   "trigger_id",
				Value: trigger.Id,
			},
			model.UserAuthField{
				Key:   "username",
				Value: trigger.Username,
			},
			model.UserAuthField{
				Key:   "code",
				Value: code,
			},
			model.UserAuthField{
				Key:   "type",
				Value: trigger.Type,
			},
		},
	})

	// Set apikey for the user if they don't have one
	if len(Userdata.ApiKey) == 0 {
		newUser, err := generateApikey(*Userdata)
		Userdata = &newUser
		if err != nil {
			log.Printf("Failed to generate apikey for user %s when creating outlook sub: %s", Userdata.Username, err)
			resp.WriteHeader(401)
			resp.Write([]byte(`{"success": false, "reason": ""}`))
			return
		}
	}

	//err = setUser(Userdata)
	//if err != nil {
	//	log.Printf("Failed setting user data for %s: %s", Userdata.Username, err)
	//	resp.WriteHeader(401)
	//	return
	//}

	err = setTriggerAuth(trigger)
	if err != nil {
		log.Printf("Failed to set trigger auth for %s - %s", trigger.Username, err)
		resp.WriteHeader(401)
		return
	}

	// FIXME - not sure if these are good at all :)
	environmentVariables := map[string]string{
		"FUNCTION_APIKEY": Userdata.ApiKey,
		"CALLBACKURL":     "https://shuffler.io",
		"WORKFLOW_ID":     trigger.WorkflowId,
		"TRIGGER_ID":      trigger.Id,
	}

	applocation := fmt.Sprintf("gs://%s/triggers/outlooktrigger.zip", bucketName)
	hookname := fmt.Sprintf("outlooktrigger_%s", trigger.Id)

	err = deployCloudFunctionGo(ctx, hookname, defaultLocation, applocation, environmentVariables)
	if err != nil {
		log.Printf("Error deploying hook: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Issue with starting hook. Please wait a second and try again"}`)))
		return
	}

	resp.WriteHeader(200)
	resp.Write([]byte("OK"))
}

type OauthToken struct {
	AccessToken  string    `json:"AccessToken" datastore:"AccessToken,noindex"`
	TokenType    string    `json:"TokenType" datastore:"TokenType,noindex"`
	RefreshToken string    `json:"RefreshToken" datastore:"RefreshToken,noindex"`
	Expiry       time.Time `json:"Expiry" datastore:"Expiry,noindex"`
}
type TriggerAuth struct {
	Id             string `json:"id" datastore:"id"`
	SubscriptionId string `json:"subscriptionId" datastore:"subscriptionId"`

	Username   string     `json:"username" datastore:"username,noindex"`
	WorkflowId string     `json:"workflow_id" datastore:"workflow_id,noindex"`
	Owner      string     `json:"owner" datastore:"owner"`
	Type       string     `json:"type" datastore:"type"`
	Code       string     `json:"code,omitempty" datastore:"code,noindex"`
	OauthToken OauthToken `json:"oauth_token,omitempty" datastore:"oauth_token"`
}

func getTriggerAuth(id string) (*TriggerAuth, error) {
	triggerauth := &TriggerAuth{}
	if err := dbClient.One("ID", strings.ToLower(id), triggerauth); err != nil {
		return &TriggerAuth{}, err
	}

	return triggerauth, nil
}

func setTriggerAuth(trigger TriggerAuth) error {
	// New struct, to not add body, author etc
	if err := dbClient.Save(&trigger); err != nil {
		log.Printf("Error adding trigger auth: %s", err)
		return err
	}

	return nil
}

// THis all of a sudden became really horrible.. fml
func getOutlookClient(ctx context.Context, code string, accessToken OauthToken, redirectUri string) (*http.Client, *oauth2.Token, error) {

	conf := &oauth2.Config{
		ClientID:     "70e37005-c954-4290-b573-d4b94e484336",
		ClientSecret: ".eNw/A[kQFB5zL.agvRputdEJENeJ392",
		Scopes: []string{
			"Mail.Read",
			"User.Read",
		},
		RedirectURL: redirectUri,
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://login.microsoftonline.com/common/oauth2/authorize",
			TokenURL: "https://login.microsoftonline.com/common/oauth2/token",
		},
	}

	if len(code) > 0 {
		access_token, err := conf.Exchange(ctx, code)
		if err != nil {
			log.Printf("Access_token issue: %s", err)
			return &http.Client{}, access_token, err
		}

		client := conf.Client(ctx, access_token)
		return client, access_token, nil
	} else {
		// Manually recreate the oauthtoken
		access_token := &oauth2.Token{
			AccessToken:  accessToken.AccessToken,
			TokenType:    accessToken.TokenType,
			RefreshToken: accessToken.RefreshToken,
			Expiry:       accessToken.Expiry,
		}

		client := conf.Client(ctx, access_token)
		return client, access_token, nil
	}
}

func handleGetOutlookFolders(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	// Exchange every time hmm
	// FIXME
	// Should really just get the code from the trigger that's being used OR the user
	triggerId := request.URL.Query().Get("trigger_id")
	if len(triggerId) == 0 {
		log.Println("No trigger_id supplied")
		resp.WriteHeader(401)
		return
	}

	trigger, err := getTriggerAuth(triggerId)
	if err != nil {
		log.Printf("Trigger %s doesn't exist - outlook folders.", triggerId)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Trigger doesn't exist."}`))
		return
	}

	// FIXME - should be shuffler in literally every case except testing lol
	redirectDomain := "shuffler.io"
	url := fmt.Sprintf("https://%s/functions/outlook/register", redirectDomain)
	outlookClient, _, err := getOutlookClient(context.TODO(), "", trigger.OauthToken, url)
	if err != nil {
		log.Printf("Oauth client failure - outlook folders: %s", err)
		resp.WriteHeader(401)
		return
	}

	folders, err := getOutlookFolders(outlookClient)
	if err != nil {
		resp.WriteHeader(401)
		return
	}

	b, err := json.Marshal(folders.Value)
	if err != nil {
		log.Println("Failed to marshal folderdata")
		resp.WriteHeader(401)
		return
	}

	resp.WriteHeader(200)
	resp.Write(b)
}

func handleGetSpecificStats(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	_, err := handleApiAuthentication(resp, request)
	if err != nil {
		log.Printf("Api authentication failed in getting specific workflow: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	location := strings.Split(request.URL.String(), "/")

	var statsId string
	if location[1] == "api" {
		if len(location) <= 4 {
			resp.WriteHeader(401)
			resp.Write([]byte(`{"success": false}`))
			return
		}

		statsId = location[4]
	}

	statisticsItem := model.StatisticsItem{}
	if err := dbClient.One("ID", statsId, &statisticsItem); err != nil {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	b, err := json.Marshal(statisticsItem)
	if err != nil {
		log.Println("Failed to marshal data: %s", err)
		resp.WriteHeader(401)
		return
	}

	resp.WriteHeader(200)
	resp.Write([]byte(b))
}

func handleGetSpecificTrigger(resp http.ResponseWriter, request *http.Request) {
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

	var workflowId string
	if location[1] == "api" {
		if len(location) <= 4 {
			resp.WriteHeader(401)
			resp.Write([]byte(`{"success": false}`))
			return
		}

		workflowId = location[4]
	}

	if strings.Contains(workflowId, "?") {
		workflowId = strings.Split(workflowId, "?")[0]
	}

	trigger, err := getTriggerAuth(workflowId)
	if err != nil {
		log.Printf("Trigger %s doesn't exist - specific trigger.", workflowId)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": ""}`))
		return
	}

	if user.Username != trigger.Owner && user.Role != "admin" {
		log.Printf("Wrong user (%s) for trigger %s", user.Username, trigger.Id)
		resp.WriteHeader(401)
		return
	}

	trigger.OauthToken = OauthToken{}
	trigger.Code = ""

	b, err := json.Marshal(trigger)
	if err != nil {
		log.Println("Failed to marshal data")
		resp.WriteHeader(401)
		return
	}

	resp.WriteHeader(200)
	resp.Write(b)
}

func handleDeleteOutlookSub(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	location := strings.Split(request.URL.String(), "/")

	var workflowId string
	var triggerId string
	if location[1] == "api" {
		if len(location) <= 6 {
			resp.WriteHeader(401)
			resp.Write([]byte(`{"success": false}`))
			return
		}

		workflowId = location[4]
		triggerId = location[6]
	}

	if len(workflowId) == 0 || len(triggerId) == 0 {
		log.Printf("Ids can't be zero when deleting %s", workflowId)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	workflow, err := getWorkflow(workflowId)
	if err != nil {
		log.Printf("Failed getting the workflow locally: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	user, err := handleApiAuthentication(resp, request)
	if err != nil {
		log.Printf("Api authentication failed in outlook deploy: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	// FIXME - have a check for org etc too..
	if user.Id != workflow.Owner && user.Role != "admin" {
		log.Printf("Wrong user (%s) for workflow %s when deploying outlook", user.Username, workflow.ID)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	// Check what kind of sub it is
	err = handleOutlookSubRemoval(workflowId, triggerId)
	if err != nil {
		log.Printf("Failed sub removal: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	resp.WriteHeader(200)
	resp.Write([]byte(`{"success": true}`))
}

func removeOutlookSubscription(outlookClient *http.Client, subscriptionId string) error {
	// DELETE https://graph.microsoft.com/v1.0/subscriptions/{id}
	fullUrl := fmt.Sprintf("https://graph.microsoft.com/v1.0/subscriptions/%s", subscriptionId)
	req, err := http.NewRequest(
		"DELETE",
		fullUrl,
		nil,
	)
	req.Header.Add("Content-Type", "application/json")
	res, err := outlookClient.Do(req)
	if err != nil {
		log.Printf("Client: %s", err)
		return err
	}

	if res.StatusCode != 200 && res.StatusCode != 201 && res.StatusCode != 204 {
		return errors.New(fmt.Sprintf("Bad status code when deleting subscription: %d", res.StatusCode))
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Printf("Body: %s", err)
		return err
	}

	_ = body

	return nil
}

// Remove AUTH
// Remove function
// Remove subscription
func handleOutlookSubRemoval(workflowId, triggerId string) error {
	// 1. Get the auth for trigger
	// 2. Stop the subscription
	// 3. Remove the function
	// 4. Remove the database entry for auth
	trigger, err := getTriggerAuth(triggerId)
	if err != nil {
		log.Printf("Trigger auth %s doesn't exist - outlook sub removal.", triggerId)
		return err
	}

	url := fmt.Sprintf("https://shuffler.io")
	outlookClient, _, err := getOutlookClient(context.TODO(), "", trigger.OauthToken, url)
	if err != nil {
		log.Printf("Oauth client failure - triggerauth sub removal: %s", err)
		return err
	}

	notificationURL := fmt.Sprintf("https://%s-%s.cloudfunctions.net/outlooktrigger_%s", defaultLocation, gceProject, trigger.Id)
	curSubscriptions, err := getOutlookSubscriptions(outlookClient)
	if err == nil {
		for _, sub := range curSubscriptions.Value {
			if sub.NotificationURL == notificationURL {
				log.Printf("Removing existing subscription %s", sub.Id)
				removeOutlookSubscription(outlookClient, sub.Id)
			}
		}
	} else {
		log.Printf("Failed to get subscriptions - need to overwrite")
	}

	// FIXME - not removing the function, as the trigger still exists
	//err = removeOutlookTriggerFunction(triggerId)
	//if err != nil {
	//	return err
	//}

	return nil
}

// This sets up the sub with outlook itself
// Parses data from the workflow to see whether access is right to subscribe it
// Creates the cloud function for outlook return
// Wait for it to be available, then schedule a workflow to it
func createOutlookSub(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	location := strings.Split(request.URL.String(), "/")

	var workflowId string
	if location[1] == "api" {
		if len(location) <= 4 {
			resp.WriteHeader(401)
			resp.Write([]byte(`{"success": false}`))
			return
		}

		workflowId = location[4]
	}

	workflow, err := getWorkflow(workflowId)
	if err != nil {
		log.Printf("Failed getting the workflow locally: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	user, err := handleApiAuthentication(resp, request)
	if err != nil {
		log.Printf("Api authentication failed in outlook deploy: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	// FIXME - have a check for org etc too..
	if user.Id != workflow.Owner && user.Role != "admin" {
		log.Printf("Wrong user (%s) for workflow %s when deploying outlook", user.Username, workflow.ID)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	log.Println("Handle outlook subscription for trigger")

	// Should already be authorized at this point, as the workflow is shared
	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		log.Printf("Failed body read for workflow %s", workflow.ID)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	log.Println(string(body))

	// Based on the input data from frontend
	type CurTrigger struct {
		Name    string   `json:"name"`
		Folders []string `json:"folders"`
		ID      string   `json:"id"`
	}

	var curTrigger CurTrigger
	err = json.Unmarshal(body, &curTrigger)
	if err != nil {
		log.Printf("Failed body read unmarshal for trigger %s", workflow.ID)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	if len(curTrigger.Folders) == 0 {
		log.Printf("Error for %s. Choosing folders is required, currently 0", workflow.ID)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	// Now that it's deployed - wait a few seconds before generating:
	// 1. Oauth2 token thingies for outlook.office.com
	// 2. Set the url to have the right mailboxes (probably ID?) ("https://outlook.office.com/api/v2.0/me/mailfolders('inbox')/messages")
	// 3. Set the callback URL to be the new trigger
	// 4. Run subscription test
	// 5. Set the subscriptionId to the trigger object

	// First - lets regenerate an oauth token for outlook.office.com from the original items
	trigger, err := getTriggerAuth(curTrigger.ID)
	if err != nil {
		log.Printf("Trigger %s doesn't exist - outlook sub.", curTrigger.ID)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": ""}`))
		return
	}

	// url doesn't really matter here
	url := fmt.Sprintf("https://shuffler.io")
	outlookClient, _, err := getOutlookClient(context.TODO(), "", trigger.OauthToken, url)
	if err != nil {
		log.Printf("Oauth client failure - triggerauth: %s", err)
		resp.WriteHeader(401)
		return
	}

	// Location +
	notificationURL := fmt.Sprintf("https://%s-%s.cloudfunctions.net/outlooktrigger_%s", defaultLocation, gceProject, curTrigger.ID)
	log.Println(notificationURL)

	// This is here simply to let the function start
	// Usually takes 10 attempts minimum :O
	// 10 * 5 = 50 seconds. That's waaay too much :(
	//notificationURL = "https://europe-west1-shuffler.cloudfunctions.net/outlooktrigger_e2ce43b0-997e-4980-9617-6eadbc68cf88"
	//notificationURL = "https://de4fc12b.ngrok.io"

	curSubscriptions, err := getOutlookSubscriptions(outlookClient)
	if err == nil {
		for _, sub := range curSubscriptions.Value {
			if sub.NotificationURL == notificationURL {
				log.Printf("Removing existing subscription %s", sub.Id)
				removeOutlookSubscription(outlookClient, sub.Id)
			}
		}
	} else {
		log.Printf("Failed to get subscriptions - need to overwrite")
	}

	maxFails := 15
	failCnt := 0
	log.Println(curTrigger.Folders)
	for {
		subId, err := makeOutlookSubscription(outlookClient, curTrigger.Folders, notificationURL)
		if err != nil {
			failCnt += 1
			log.Printf("Failed making oauth subscription, retrying in 5 seconds: %s", err)
			time.Sleep(5 * time.Second)
			if failCnt == maxFails {
				log.Printf("Failed to set up subscription %d times.", maxFails)
				resp.WriteHeader(401)
				return
			}

			continue
		}

		// Set the ID somewhere here
		trigger.SubscriptionId = subId
		err = setTriggerAuth(*trigger)
		if err != nil {
			log.Printf("Failed setting triggerauth: %s", err)
		}

		break
	}

	log.Printf("Successfully handled outlook subscription for trigger %s in workflow %s", curTrigger.ID, workflow.ID)

	//log.Printf("%#v", user)
	resp.WriteHeader(200)
	resp.Write([]byte(`{"success": true}`))
}

// Lists the users current subscriptions
func getOutlookSubscriptions(outlookClient *http.Client) (SubscriptionsWrapper, error) {
	fullUrl := fmt.Sprintf("https://graph.microsoft.com/v1.0/subscriptions")
	req, err := http.NewRequest(
		"GET",
		fullUrl,
		nil,
	)
	req.Header.Add("Content-Type", "application/json")
	res, err := outlookClient.Do(req)
	if err != nil {
		log.Printf("suberror Client: %s", err)
		return SubscriptionsWrapper{}, err
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Printf("Suberror Body: %s", err)
		return SubscriptionsWrapper{}, err
	}

	newSubs := SubscriptionsWrapper{}
	err = json.Unmarshal(body, &newSubs)
	if err != nil {
		return SubscriptionsWrapper{}, err
	}

	return newSubs, nil
}

type SubscriptionsWrapper struct {
	OdataContext string         `json:"@odata.context"`
	Value        []Subscription `json:"value"`
}

type Subscription struct {
	ChangeType         string `json:"changeType"`
	NotificationURL    string `json:"notificationUrl"`
	Resource           string `json:"resource"`
	ExpirationDateTime string `json:"expirationDateTime"`
	ClientState        string `json:"clientState"`
	Id                 string `json:"id"`
}

func makeOutlookSubscription(client *http.Client, folderIds []string, notificationURL string) (string, error) {
	fullUrl := "https://graph.microsoft.com/v1.0/subscriptions"

	// FIXME - this expires rofl
	t := time.Now().Local().Add(time.Minute * time.Duration(4300))
	timeFormat := fmt.Sprintf("%d-%02d-%02dT%02d:%02d:%02d.0000000Z", t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second())
	log.Println(timeFormat)

	resource := fmt.Sprintf("me/mailfolders('%s')/messages", strings.Join(folderIds, "','"))
	log.Println(resource)
	sub := Subscription{
		ChangeType:         "created",
		NotificationURL:    notificationURL,
		ExpirationDateTime: timeFormat,
		ClientState:        "This is a test",
		Resource:           resource,
	}

	data, err := json.Marshal(sub)
	if err != nil {
		log.Printf("Marshal: %s", err)
		return "", err
	}

	req, err := http.NewRequest(
		"POST",
		fullUrl,
		bytes.NewBuffer(data),
	)
	req.Header.Add("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		log.Printf("Client: %s", err)
		return "", err
	}

	log.Printf("Status: %d", res.StatusCode)
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Printf("Body: %s", err)
		return "", err
	}

	if res.StatusCode != 200 && res.StatusCode != 201 {
		return "", errors.New(fmt.Sprintf("Subscription failed: %s", string(body)))
	}

	// Use data from body here to create thingy
	newSub := Subscription{}
	err = json.Unmarshal(body, &newSub)
	if err != nil {
		return "", err
	}

	return newSub.Id, nil
}

func getOpenapi(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	// Just here to verify that the user is logged in
	_, err := handleApiAuthentication(resp, request)
	if err != nil {
		log.Printf("Api authentication failed in validate swagger: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	location := strings.Split(request.URL.String(), "/")
	var id string
	if location[1] == "api" {
		if len(location) <= 4 {
			resp.WriteHeader(401)
			resp.Write([]byte(`{"success": false}`))
			return
		}

		id = location[4]
	}

	if len(id) != 32 {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	// FIXME - FIX AUTH WITH APP
	//_, err = getApp(ctx, id)
	//if err == nil {
	//	log.Println("You're supposed to be able to continue now.")
	//}

	parsedApi, err := getOpenApiDatastore(id)
	if err != nil {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	parsedApi.Success = true
	data, err := json.Marshal(parsedApi)
	if err != nil {
		resp.WriteHeader(422)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Failed marshalling parsed swagger: %s"}`, err)))
		return
	}

	resp.WriteHeader(200)
	resp.Write(data)
}

func echoOpenapiData(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	// Just here to verify that the user is logged in
	_, err := handleApiAuthentication(resp, request)
	if err != nil {
		log.Printf("Api authentication failed in validate swagger: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		log.Printf("Bodyreader err: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Failed reading body"}`))
		return
	}

	newbody := string(body)
	newbody = strings.TrimSpace(newbody)
	if strings.HasPrefix(newbody, "\"") {
		newbody = newbody[1:len(newbody)]
	}

	if strings.HasSuffix(newbody, "\"") {
		newbody = newbody[0 : len(newbody)-1]
	}

	req, err := http.NewRequest("GET", newbody, nil)
	if err != nil {
		log.Printf("Requestbuilder err: %s", err)
		resp.WriteHeader(500)
		resp.Write([]byte(`{"success": false, "reason": "Failed building request"}`))
		return
	}

	httpClient := &http.Client{}
	newresp, err := httpClient.Do(req)
	if err != nil {
		resp.WriteHeader(500)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Failed making request for data"`)))
		return
	}
	defer newresp.Body.Close()

	urlbody, err := ioutil.ReadAll(newresp.Body)
	if err != nil {
		resp.WriteHeader(500)
		resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Can't get data from selected uri"`)))
		return
	}

	resp.WriteHeader(200)
	resp.Write(urlbody)
}

func validateSwagger(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	// Just here to verify that the user is logged in
	_, err := handleApiAuthentication(resp, request)
	if err != nil {
		log.Printf("Api authentication failed in validate swagger: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Failed reading body"}`))
		return
	}

	type versionCheck struct {
		Swagger        string `datastore:"swagger" json:"swagger" yaml:"swagger"`
		SwaggerVersion string `datastore:"swaggerVersion" json:"swaggerVersion" yaml:"swaggerVersion"`
		OpenAPI        string `datastore:"openapi" json:"openapi" yaml:"openapi"`
	}

	//body = []byte(`swagger: "2.0"`)
	//body = []byte(`swagger: '1.0'`)
	//newbody := string(body)
	//newbody = strings.TrimSpace(newbody)
	//body = []byte(newbody)
	//log.Println(string(body))
	//tmpbody, err := yaml.YAMLToJSON(body)
	//log.Println(err)
	//log.Println(string(tmpbody))

	// This has to be done in a weird way because Datastore doesn't
	// support map[string]interface and similar (openapi3.Swagger)
	var version versionCheck

	isJson := false
	err = json.Unmarshal(body, &version)
	if err != nil {
		log.Printf("Json err: %s", err)
		err = yaml.Unmarshal(body, &version)
		if err != nil {
			log.Printf("Yaml error: %s", err)
			//resp.WriteHeader(422)
			//resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Failed reading openapi to json and yaml: %s"}`, err)))
			//return
		} else {
			log.Printf("Successfully parsed YAML!")
		}
	} else {
		isJson = true
		log.Printf("Successfully parsed JSON!")
	}

	if len(version.SwaggerVersion) > 0 && len(version.Swagger) == 0 {
		version.Swagger = version.SwaggerVersion
	}

	if strings.HasPrefix(version.Swagger, "3.") || strings.HasPrefix(version.OpenAPI, "3.") {
		log.Println("Handling v3 API")
		swagger, err := openapi3.NewSwaggerLoader().LoadSwaggerFromData(body)
		if err != nil {
			resp.WriteHeader(401)
			resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "%s"}`, err)))
			return
		}

		hasher := md5.New()
		hasher.Write(body)
		idstring := hex.EncodeToString(hasher.Sum(nil))

		log.Printf("Swagger v3 validation success with ID %s!", idstring)
		log.Printf("Paths: %d", len(swagger.Paths))

		if !isJson {
			log.Printf("FIXME: NEED TO TRANSFORM FROM YAML TO JSON for %s", idstring)
		}

		parsed := model.ParsedOpenApi{
			ID:   idstring,
			Body: string(body),
		}

		err = setOpenApiDatastore(parsed)
		if err != nil {
			log.Printf("Failed uploading openapi to datastore: %s", err)
			resp.WriteHeader(422)
			resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Failed reading openapi2: %s"}`, err)))
			return
		}
		resp.WriteHeader(200)
		resp.Write([]byte(fmt.Sprintf(`{"success": true, "id": "%s"}`, idstring)))
		return
	} else { //strings.HasPrefix(version.Swagger, "2.") || strings.HasPrefix(version.OpenAPI, "2.") {
		// Convert
		log.Println("Handling v2 API")
		var swagger openapi2.Swagger
		//log.Println(string(body))
		err = json.Unmarshal(body, &swagger)
		if err != nil {
			log.Printf("Json error? %s", err)
			err = gyaml.Unmarshal(body, &swagger)
			if err != nil {
				log.Printf("Yaml error: %s", err)
			}

			resp.WriteHeader(422)
			resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Failed reading openapi2: %s"}`, err)))
			return
		}

		swaggerv3, err := openapi2conv.ToV3Swagger(&swagger)
		if err != nil {
			log.Printf("Failed converting from openapi2 to 3: %s", err)
			resp.WriteHeader(422)
			resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Failed converting from openapi2 to openapi3: %s"}`, err)))
			return
		}

		swaggerdata, err := json.Marshal(swaggerv3)
		if err != nil {
			log.Printf("Failed unmarshaling v3 data: %s", err)
			resp.WriteHeader(422)
			resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Failed marshalling swaggerv3 data: %s"}`, err)))
			return
		}

		hasher := md5.New()
		hasher.Write(swaggerdata)
		idstring := hex.EncodeToString(hasher.Sum(nil))
		if !isJson {
			log.Printf("FIXME: NEED TO TRANSFORM FROM YAML TO JSON for %s?", idstring)
		}
		log.Printf("Swagger v2 -> v3 validation success with ID %s!", idstring)

		parsed := model.ParsedOpenApi{
			ID:   idstring,
			Body: string(swaggerdata),
		}

		err = setOpenApiDatastore(parsed)
		if err != nil {
			log.Printf("Failed uploading openapi2 to datastore: %s", err)
			resp.WriteHeader(422)
			resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Failed reading openapi2: %s"}`, err)))
			return
		}

		resp.WriteHeader(200)
		resp.Write([]byte(fmt.Sprintf(`{"success": true, "id": "%s"}`, idstring)))
		return
	}
	/*
		else {
			log.Printf("Swagger / OpenAPI version %s is not supported or there is an error.", version.Swagger)
			resp.WriteHeader(422)
			resp.Write([]byte(fmt.Sprintf(`{"success": false, "reason": "Swagger version %s is not currently supported"}`, version.Swagger)))
			return
		}
	*/

	// save the openapi ID
	resp.WriteHeader(422)
	resp.Write([]byte(`{"success": false}`))
}

// Creates an app from the app builder
func verifySwagger(resp http.ResponseWriter, request *http.Request) {
	cors := handleCors(resp, request)
	if cors {
		return
	}

	user, err := handleApiAuthentication(resp, request)
	if err != nil {
		log.Printf("Api authentication failed in verify swagger: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false, "reason": "Failed reading body"}`))
		return
	}

	type Test struct {
		Editing bool   `datastore:"editing"`
		Id      string `datastore:"id"`
		Image   string `datastore:"image"`
	}

	var test Test
	err = json.Unmarshal(body, &test)
	if err != nil {
		log.Printf("Failed unmarshalling test: %s", err)
		resp.WriteHeader(401)
		resp.Write([]byte(`{"success": false}`))
		return
	}

	// Get an identifier
	hasher := md5.New()
	hasher.Write(body)
	newmd5 := hex.EncodeToString(hasher.Sum(nil))
	if test.Editing {
		// Quick verification test
		app, err := getApp(test.Id)
		if err != nil {
			log.Printf("Error getting app when editing: %s", app.Name)
			resp.WriteHeader(401)
			resp.Write([]byte(`{"success": false}`))
			return
		}

		// FIXME: Check whether it's in use.
		if user.Id != app.Owner && user.Role != "admin" {
			log.Printf("Wrong user (%s) for app %s when verifying swagger", user.Username, app.Name)
			resp.WriteHeader(401)
			resp.Write([]byte(`{"success": false}`))
			return
		}

		log.Printf("EDITING APP WITH ID %s", app.ID)
		newmd5 = app.ID
	}

	// Generate new app integration (bump version)
	// Test = client side with fetch?

	ctx := context.Background()
	//client, err := storage.NewClient(ctx)
	//if err != nil {
	//	log.Printf("Failed to create client (storage): %v", err)
	//	resp.WriteHeader(401)
	//	resp.Write([]byte(`{"success": false, "reason": "Failed creating client"}`))
	//	return
	//}

	swagger, err := openapi3.NewSwaggerLoader().LoadSwaggerFromData(body)
	if err != nil {
		log.Printf("Swagger validation error: %s", err)
		resp.WriteHeader(500)
		resp.Write([]byte(`{"success": false, "reason": "Failed verifying openapi"}`))
		return
	}

	if strings.Contains(swagger.Info.Title, " ") {
		strings.Replace(swagger.Info.Title, " ", "", -1)
	}

	basePath, err := buildStructure(swagger, newmd5)
	if err != nil {
		log.Printf("Failed to build base structure: %s", err)
		resp.WriteHeader(500)
		resp.Write([]byte(`{"success": false, "reason": "Failed building baseline structure"}`))
		return
	}

	log.Printf("Should generate yaml")
	api, pythonfunctions, err := generateYaml(swagger, newmd5)
	if err != nil {
		log.Printf("Failed building and generating yaml: %s", err)
		resp.WriteHeader(500)
		resp.Write([]byte(`{"success": false, "reason": "Failed building and parsing yaml"}`))
		return
	}

	api.Owner = user.Id
	if len(test.Image) > 0 {
		api.SmallImage = test.Image
		api.LargeImage = test.Image
	}

	err = dumpApi(basePath, api)
	if err != nil {
		log.Printf("Failed dumping yaml: %s", err)
		resp.WriteHeader(500)
		resp.Write([]byte(`{"success": false, "reason": "Failed dumping yaml"}`))
		return
	}

	identifier := fmt.Sprintf("%s-%s", swagger.Info.Title, newmd5)
	classname := strings.Replace(identifier, " ", "", -1)
	classname = strings.Replace(classname, "-", "", -1)
	parsedCode, err := dumpPython(basePath, classname, swagger.Info.Version, pythonfunctions)
	if err != nil {
		log.Printf("Failed dumping python: %s", err)
		resp.WriteHeader(500)
		resp.Write([]byte(`{"success": false, "reason": "Failed dumping appcode"}`))
		return
	}

	identifier = strings.Replace(identifier, " ", "-", -1)
	identifier = strings.Replace(identifier, "_", "-", -1)
	log.Printf("Successfully uploaded %s to bucket. Proceeding to cloud function", identifier)

	// Now that the baseline is setup, we need to make it into a cloud function
	// 1. Upload the API to datastore for use
	// 2. Get code from baseline/app_base.py & baseline/static_baseline.py
	// 3. Stitch code together from these two + our new app
	// 4. Zip the folder to cloud storage
	// 5. Upload as cloud function

	// 1. Upload the API to datastore
	err = deployAppToDatastore(ctx, api)
	if err != nil {
		log.Printf("Failed adding app to dbClient: %s", err)
		resp.WriteHeader(500)
		resp.Write([]byte(`{"success": false, "reason": "Failed adding app to dbClient"}`))
		return
	}

	// 2. Get all the required code
	appbase, staticBaseline, err := getAppbase()
	if err != nil {
		log.Printf("Failed getting appbase: %s", err)
		resp.WriteHeader(500)
		resp.Write([]byte(`{"success": false, "reason": "Failed getting appbase code"}`))
		return
	}

	// Have to do some quick checks of the python code (:
	_, parsedCode = formatAppfile(parsedCode)

	fixedAppbase := fixAppbase(appbase)
	runner := getRunner(classname)

	// 2. Put it together
	stitched := string(staticBaseline) + strings.Join(fixedAppbase, "\n") + parsedCode + string(runner)
	//log.Println(stitched)

	// 3. Zip and stream it directly in the directory
	_, err = streamZipdata(ctx, identifier, stitched, "requests\nurllib3")
	if err != nil {
		log.Printf("Zipfile error: %s", err)
		resp.WriteHeader(500)
		resp.Write([]byte(`{"success": false, "reason": "Failed to build zipfile"}`))
		return
	}

	log.Printf("Successfully uploaded ZIPFILE for %s", identifier)

	// 4. Upload as cloud function - this apikey is specifically for cloud functions rofl
	//environmentVariables := map[string]string{
	//	"FUNCTION_APIKEY": apikey,
	//}

	//fullLocation := fmt.Sprintf("gs://%s/%s", bucketName, applocation)
	//err = deployCloudFunctionPython(ctx, identifier, defaultLocation, fullLocation, environmentVariables)
	//if err != nil {
	//	log.Printf("Error uploading cloud function: %s", err)
	//	resp.WriteHeader(500)
	//	resp.Write([]byte(`{"success": false, "reason": "Failed to upload function"}`))
	//	return
	//}

	// 4. Build the image locally.
	// FIXME: Should be moved to a local docker registry
	dockerLocation := fmt.Sprintf("%s/Dockerfile", basePath)
	log.Printf("Dockerfile: %s", dockerLocation)

	versionName := fmt.Sprintf("%s_%s", strings.ReplaceAll(api.Name, " ", "-"), api.AppVersion)
	dockerTags := []string{
		fmt.Sprintf("%s:%s", baseDockerName, identifier),
		fmt.Sprintf("%s:%s", baseDockerName, versionName),
	}

	err = buildImage(dockerTags, dockerLocation)
	if err != nil {
		log.Printf("Docker build error: %s", err)
		resp.WriteHeader(500)
		resp.Write([]byte(fmt.Sprintf(`{"success": true, "reason": "Error in Docker build"}`)))
		return
	}

	found := false
	foundNumber := 0
	log.Printf("Checking for api with ID %s", newmd5)
	for appCounter, app := range user.PrivateApps {
		if app.ID == api.ID {
			found = true
			foundNumber = appCounter
			break
		} else if app.Name == api.Name && app.AppVersion == api.AppVersion {
			found = true
			foundNumber = appCounter
			break
		} else if app.PrivateID == test.Id && test.Editing {
			found = true
			foundNumber = appCounter
			break
		}
	}

	// Updating the user with the new app so that it can easily be retrieved
	if !found {
		user.PrivateApps = append(user.PrivateApps, api)
	} else {
		user.PrivateApps[foundNumber] = api
	}

	err = setUser(&user)
	if err != nil {
		log.Printf("Failed adding verification for user %s: %s", user.Username, err)
		resp.WriteHeader(500)
		resp.Write([]byte(fmt.Sprintf(`{"success": true, "reason": "Failed updating user"}`)))
		return
	}

	log.Println(len(user.PrivateApps))
	c, err := request.Cookie("session_token")
	if err == nil {
		log.Printf("Should've deleted cache for %s with token %s", user.Username, c.Value)
		//err = memcache.Delete(request.Context(), c.Value)
		//err = memcache.Delete(request.Context(), user.ApiKey)
	}

	parsed := model.ParsedOpenApi{
		ID:   api.ID,
		Body: string(body),
	}

	setOpenApiDatastore(parsed)
	err = increaseStatisticsField("total_apps_created", api.ID, 1)
	if err != nil {
		log.Printf("Failed to increase success execution stats: %s", err)
	}

	err = increaseStatisticsField("openapi_apps_created", api.ID, 1)
	if err != nil {
		log.Printf("Failed to increase success execution stats: %s", err)
	}

	resp.WriteHeader(200)
	resp.Write([]byte(`{"success": true}`))
}

func healthCheckHandler(resp http.ResponseWriter, request *http.Request) {
	fmt.Fprint(resp, "OK")
}

func init() {
	var err error

	log.Printf("Running INIT process")

	dbClient, err = storm.Open("backend.db", storm.Codec(gob.Codec))
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

	http.Handle("/", r)
}

// Had to move away from mux, which means Method is fucked up right now.
func main() {
	//init()
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "MISSING"
	}

	innerPort := os.Getenv("BACKEND_PORT")
	if innerPort == "" {
		log.Printf("Running on %s:5001", hostname)
		log.Fatal(http.ListenAndServe(":5001", nil))
	} else {
		log.Printf("Running on %s:%s", hostname, innerPort)
		log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", innerPort), nil))
	}
}
