package server

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"shuffle/model"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/satori/go.uuid"
	"gopkg.in/yaml.v2"
)

func copyFile(fromfile, tofile string) error {
	from, err := os.Open(fromfile)
	if err != nil {
		return err
	}
	defer from.Close()

	to, err := os.OpenFile(tofile, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return err
	}
	defer to.Close()

	_, err = io.Copy(to, from)
	if err != nil {
		return err
	}

	return nil
}

func formatAppfile(filedata string) (string, string) {
	lines := strings.Split(filedata, "\n")

	newfile := []string{}
	classname := ""
	for _, line := range lines {
		if strings.Contains(line, "walkoff_app_sdk") {
			continue
		}

		// Remap logging. CBA this right now
		// This issue also persists in onprem apps because of await thingies.. :(
		// FIXME
		if strings.Contains(line, "console_logger") && strings.Contains(line, "await") {
			continue
			//line = strings.Replace(line, "console_logger", "logger", -1)
			//log.Println(line)
		}

		// Might not work with different import names
		// Could be fucked up with spaces everywhere? Idk
		if strings.Contains(line, "class") && strings.Contains(line, "(AppBase)") {
			items := strings.Split(line, " ")
			if len(items) > 0 && strings.Contains(items[1], "(AppBase)") {
				classname = strings.Split(items[1], "(")[0]
			} else {
				// This could break something..
				classname = "TMP"
			}
		}

		if strings.Contains(line, "if __name__ ==") {
			break
		}

		// asyncio.run(HelloWorld.run(), debug=True)

		newfile = append(newfile, line)
	}

	filedata = strings.Join(newfile, "\n")
	return classname, filedata
}

// Streams the data into a zip to be used for a cloud function
func streamZipdata(ctx context.Context, identifier, pythoncode, requirements string) (string, error) {
	filename := fmt.Sprintf("generated_cloudfunctions/%s.zip", identifier)

	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	zipFile, err := zipWriter.Create("main.py")
	if err != nil {
		log.Printf("Packing failed to create zip file from bucket: %v", err)
		return filename, err
	}

	// Have to use Fprintln otherwise it tries to parse all strings etc.
	if _, err := fmt.Fprintln(zipFile, pythoncode); err != nil {
		return filename, err
	}

	zipFile, err = zipWriter.Create("requirements.txt")
	if err != nil {
		log.Printf("Packing failed to create zip file from bucket: %v", err)
		return filename, err
	}
	if _, err := fmt.Fprintln(zipFile, requirements); err != nil {
		return filename, err
	}

	err = zipWriter.Close()
	if err != nil {
		log.Printf("Packing failed to close zip file writer from bucket: %v", err)
		return filename, err
	}

	return filename, nil
}

func getAppbase() ([]byte, []byte, error) {
	// 1. Have baseline in bucket/generated_apps/baseline
	// 2. Copy the baseline to a new folder with identifier name
	static := "../../functions/static_baseline.py"
	appbase := "../../functions/onprem/app_sdk/app_base.py"

	staticData, err := ioutil.ReadFile(static)
	if err != nil {
		return []byte{}, []byte{}, err
	}

	appbaseData, err := ioutil.ReadFile(appbase)
	if err != nil {
		return []byte{}, []byte{}, err
	}

	return appbaseData, staticData, nil
}

func fixAppbase(appbase []byte) []string {
	record := false
	validLines := []string{}
	for _, line := range strings.Split(string(appbase), "\n") {
		if strings.Contains(line, "#STOPCOPY") {
			//log.Println("Stopping copy")
			break
		}

		if record {
			validLines = append(validLines, line)
		}

		if strings.Contains(line, "#STARTCOPY") {
			//log.Println("Starting copy")
			record = true
		}
	}

	return validLines
}


// Builds the base structure for the app that we're making
// Returns error if anything goes wrong. This has to work if
// the python code is supposed to be generated
func buildStructure(swagger *openapi3.Swagger, curHash string) (string, error) {
	//log.Printf("%#v", swagger)

	// adding md5 based on input data to not overwrite earlier data.
	generatedPath := "generated"
	subpath := "../../app_gen/openapi/"
	identifier := fmt.Sprintf("%s-%s", swagger.Info.Title, curHash)
	appPath := fmt.Sprintf("%s/%s", generatedPath, identifier)

	os.MkdirAll(appPath, os.ModePerm)
	os.Mkdir(fmt.Sprintf("%s/src", appPath), os.ModePerm)

	err := copyFile(fmt.Sprintf("%sbaseline/Dockerfile", subpath), fmt.Sprintf("%s/%s", appPath, "Dockerfile"))
	if err != nil {
		log.Println("Failed to move Dockerfile")
		return appPath, err
	}

	err = copyFile(fmt.Sprintf("%sbaseline/requirements.txt", subpath), fmt.Sprintf("%s/%s", appPath, "requirements.txt"))
	if err != nil {
		log.Println("Failed to move requrements.txt")
		return appPath, err
	}

	return appPath, nil
}

func makePythoncode(swagger *openapi3.Swagger, name, url, method string, parameters, optionalQueries []string) (string, string) {
	method = strings.ToLower(method)
	queryString := ""
	queryData := ""

	// FIXME - this might break - need to check if ? or & should be set as query
	parameterData := ""
	if len(optionalQueries) > 0 {
		queryString += ", "
		for _, query := range optionalQueries {
			queryString += fmt.Sprintf("%s=\"\", ", query)
			queryData += fmt.Sprintf(`
        if %s:
            url += f"&%s={%s}"`, query, query, query)
		}
	}

	// How to add authentication?
	// I think it should be like:
	// async def(self, auth, baseurl, data):
	// api.Authentication.Parameters[0].Value = "BearerAuth"
	authenticationParameter := ""
	authenticationSetup := ""
	authenticationAddin := ""
	// Python configuration code that should work :)
	if swagger.Components.SecuritySchemes != nil {
		if swagger.Components.SecuritySchemes["BearerAuth"] != nil {
			authenticationParameter = ", apikey"
			authenticationSetup = "headers[\"Authorization\"] = f\"Bearer {apikey}\""
		} else if swagger.Components.SecuritySchemes["BasicAuth"] != nil {
			authenticationParameter = ", username, password"
			authenticationAddin = ", auth=(username, password)"
		} else if swagger.Components.SecuritySchemes["ApiKeyAuth"] != nil {
			authenticationParameter = ", apikey"
			if swagger.Components.SecuritySchemes["ApiKeyAuth"].Value.In == "header" {
				authenticationSetup = fmt.Sprintf("headers[\"%s\"] = apikey", swagger.Components.SecuritySchemes["ApiKeyAuth"].Value.Name)
			} else if swagger.Components.SecuritySchemes["ApiKeyAuth"].Value.In == "query" {
				authenticationSetup = fmt.Sprintf("url+=f\"?%s={apikey}\"", swagger.Components.SecuritySchemes["ApiKeyAuth"].Value.Name)
			}
		}
	}

	//baseUrl := fmt.Sprintf("%s%s", api.Link, actualPath)
	// This is a quickfix for onpremises stuff. Does work, but should really be
	// part of the authentication scheme from openapi3
	urlParameter := ""
	urlInline := ""
	//log.Printf("URL: %s", url)
	if !strings.HasPrefix(strings.ToLower(url), "http") {
		urlParameter = ", url"
		urlInline = "{url}"
	}

	if len(parameters) > 0 {
		parameterData = fmt.Sprintf(", %s", strings.Join(parameters, ", "))
	}

	// FIXME - add checks for query data etc

	functionname := strings.ToLower(fmt.Sprintf("%s_%s", method, name))
	if strings.Contains(strings.ToLower(name), strings.ToLower(method)) {
		functionname = strings.ToLower(name)
	}

	bodyParameter := ""
	bodyAddin := ""
	postParameters := []string{"post", "patch", "put"}
	for _, item := range postParameters {
		if method == item {
			bodyParameter = ", body=\"\""
			bodyAddin = ", json=body"
			break
		}
	}

	// Extra param for url if it's changeable
	// Extra param for authentication scheme(s)

	data := fmt.Sprintf(`    async def %s(self%s%s%s%s%s):
        headers={}
        url=f"%s%s"
        %s
        %s
        return requests.%s(url, headers=headers%s%s).text
	`, functionname, authenticationParameter, urlParameter, parameterData, queryString, bodyParameter, urlInline, url, authenticationSetup, queryData, method, authenticationAddin, bodyAddin)

	//log.Println(data)
	//log.Println(functionname)
	return functionname, data
}

func generateYaml(swagger *openapi3.Swagger, newmd5 string) (model.WorkflowApp, []string, error) {
	api := model.WorkflowApp{}
	//log.Printf("%#v", swagger.Info)

	if len(swagger.Info.Title) == 0 {
		return model.WorkflowApp{}, []string{}, errors.New("Swagger.Info.Title can't be empty.")
	}

	if len(swagger.Servers) == 0 {
		return model.WorkflowApp{}, []string{}, errors.New("Swagger.Servers can't be empty. Add 'servers':[{'url':'hostname.com'}'")
	}

	api.Name = swagger.Info.Title
	api.Description = swagger.Info.Description
	api.ID = uuid.NewV4().String()
	api.IsValid = true
	api.Link = swagger.Servers[0].URL // host doesnt exist lol
	if strings.HasSuffix(api.Link, "/") {
		api.Link = api.Link[:len(api.Link)-1]
	}

	api.AppVersion = "1.0.0"
	api.Environment = "Shuffle"
	api.SmallImage = ""
	api.LargeImage = ""
	api.Sharing = false
	api.Verified = false
	api.Tested = false
	api.PrivateID = newmd5
	api.Generated = true
	// Setting up security schemes
	extraParameters := []model.WorkflowAppActionParameter{}

	securitySchemes := swagger.Components.SecuritySchemes
	if securitySchemes != nil {
		log.Printf("%#v", securitySchemes)

		api.Authentication = model.Authentication{
			Required: true,
			Parameters: []model.AuthenticationParams{
				model.AuthenticationParams{
					Multiline: false,
					Required:  true,
				},
			},
		}

		// Used for python code generation lol
		// Not sure how this should work with oauth
		if securitySchemes["BearerAuth"] != nil {
			api.Authentication.Parameters[0].Value = "BearerAuth"
			api.Authentication.Parameters[0].Description = securitySchemes["BearerAuth"].Value.Description
			api.Authentication.Parameters[0].Name = securitySchemes["BearerAuth"].Value.Name
			api.Authentication.Parameters[0].In = securitySchemes["BearerAuth"].Value.In
			api.Authentication.Parameters[0].Scheme = securitySchemes["BearerAuth"].Value.Scheme
			log.Printf("HANDLE BEARER AUTH")
			extraParameters = append(extraParameters, model.WorkflowAppActionParameter{
				Name:        "apikey",
				Description: "The apikey to use",
				Multiline:   false,
				Required:    true,
				Schema: model.SchemaDefinition{
					Type: "string",
				},
			})
		} else if securitySchemes["ApiKeyAuth"] != nil {
			api.Authentication.Parameters[0].Value = "ApiKeyAuth"
			api.Authentication.Parameters[0].Description = securitySchemes["ApiKeyAuth"].Value.Description
			api.Authentication.Parameters[0].Name = securitySchemes["ApiKeyAuth"].Value.Name
			api.Authentication.Parameters[0].In = securitySchemes["ApiKeyAuth"].Value.In
			api.Authentication.Parameters[0].Scheme = securitySchemes["ApiKeyAuth"].Value.Scheme
			log.Printf("HANDLE APIKEY AUTH")
			extraParameters = append(extraParameters, model.WorkflowAppActionParameter{
				Name:        "apikey",
				Description: "The apikey to use",
				Multiline:   false,
				Required:    true,
				Schema: model.SchemaDefinition{
					Type: "string",
				},
			})
		} else if securitySchemes["BasicAuth"] != nil {
			api.Authentication.Parameters[0].Value = "BasicAuth"
			api.Authentication.Parameters[0].Description = securitySchemes["BasicAuth"].Value.Description
			api.Authentication.Parameters[0].Name = securitySchemes["BasicAuth"].Value.Name
			api.Authentication.Parameters[0].In = securitySchemes["BasicAuth"].Value.In
			api.Authentication.Parameters[0].Scheme = securitySchemes["BasicAuth"].Value.Scheme
			extraParameters = append(extraParameters, model.WorkflowAppActionParameter{
				Name:        "username",
				Description: "The username to use",
				Multiline:   false,
				Required:    true,
				Schema: model.SchemaDefinition{
					Type: "string",
				},
			})
			extraParameters = append(extraParameters, model.WorkflowAppActionParameter{
				Name:        "password",
				Description: "The password to use",
				Multiline:   false,
				Required:    true,
				Schema: model.SchemaDefinition{
					Type: "string",
				},
			})
		}
	}

	// Adds a link parameter if it's not already defined
	if len(api.Link) == 0 {
		extraParameters = append(extraParameters, model.WorkflowAppActionParameter{
			Name:        "url",
			Description: "The URL of the app",
			Multiline:   false,
			Required:    true,
			Schema: model.SchemaDefinition{
				Type: "string",
			},
		})
	}

	// This is the python code to be generated
	// Could just as well be go at this point lol
	pythonFunctions := []string{}

	for actualPath, path := range swagger.Paths {

		//log.Printf("%#v", path)
		//log.Printf("%#v", actualPath)

		// FIXME: Add everything from here:
		// https://godoc.org/github.com/getkin/kin-openapi/openapi3#PathItem
		firstQuery := true
		if path.Get != nil {
			action, curCode := handleGet(swagger, api, extraParameters, path, actualPath, firstQuery)
			api.Actions = append(api.Actions, action)
			pythonFunctions = append(pythonFunctions, curCode)
		}
		if path.Connect != nil {
			action, curCode := handleConnect(swagger, api, extraParameters, path, actualPath, firstQuery)
			api.Actions = append(api.Actions, action)
			pythonFunctions = append(pythonFunctions, curCode)
		}
		if path.Head != nil {
			action, curCode := handleHead(swagger, api, extraParameters, path, actualPath, firstQuery)
			api.Actions = append(api.Actions, action)
			pythonFunctions = append(pythonFunctions, curCode)
		}
		if path.Delete != nil {
			action, curCode := handleDelete(swagger, api, extraParameters, path, actualPath, firstQuery)
			api.Actions = append(api.Actions, action)
			pythonFunctions = append(pythonFunctions, curCode)
		}
		if path.Post != nil {
			action, curCode := handlePost(swagger, api, extraParameters, path, actualPath, firstQuery)
			api.Actions = append(api.Actions, action)
			pythonFunctions = append(pythonFunctions, curCode)
		}
		if path.Patch != nil {
			action, curCode := handlePatch(swagger, api, extraParameters, path, actualPath, firstQuery)
			api.Actions = append(api.Actions, action)
			pythonFunctions = append(pythonFunctions, curCode)
		}
		if path.Put != nil {
			action, curCode := handlePut(swagger, api, extraParameters, path, actualPath, firstQuery)
			api.Actions = append(api.Actions, action)
			pythonFunctions = append(pythonFunctions, curCode)
		}
	}

	return api, pythonFunctions, nil
}

// FIXME - have this give a real version?
func verifyApi(api model.WorkflowApp) model.WorkflowApp {
	if api.AppVersion == "" {
		api.AppVersion = "1.0.0"
	}

	return api
}

func getBasePython() string {
	baseString := `import requests
import asyncio
import json
import urllib3

from walkoff_app_sdk.app_base import AppBase

class %s(AppBase):
    """
    Autogenerated class by Shuffler
    """
    
    __version__ = "%s"
    app_name = "%s"
    
    def __init__(self, redis, logger, console_logger=None):
    	self.verify = False
    	urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)
    	super().__init__(redis, logger, console_logger)

%s

if __name__ == "__main__":
    asyncio.run(%s.run(), debug=True)
`
	return baseString
}


func dumpPython(basePath, name, version string, pythonFunctions []string) (string, error) {
	//log.Printf("%#v", api)
	//log.Printf(strings.Join(pythonFunctions, "\n"))

	parsedCode := fmt.Sprintf(getBasePython(), name, version, name, strings.Join(pythonFunctions, "\n"), name)

	err := ioutil.WriteFile(fmt.Sprintf("%s/src/app.py", basePath), []byte(parsedCode), os.ModePerm)
	if err != nil {
		return "", err
	}
	//fmt.Println(parsedCode)
	//log.Println(string(data))
	return parsedCode, nil
}


func dumpApi(basePath string, api model.WorkflowApp) error {
	//log.Printf("%#v", api)
	data, err := yaml.Marshal(api)
	if err != nil {
		log.Printf("Error with yaml marshal: %s", err)
		return err
	}

	err = ioutil.WriteFile(fmt.Sprintf("%s/api.yaml", basePath), []byte(data), os.ModePerm)
	if err != nil {
		return err
	}

	//log.Println(string(data))
	return nil
}

func getRunner(classname string) string {
	return fmt.Sprintf(`
# Run the actual thing after we've checked params
def run(request):
    action = request.get_json() 
    print(action)
    print(type(action))
    authorization_key = action.get("authorization")
    current_execution_id = action.get("execution_id")
	
    if action and "name" in action and "app_name" in action:
        asyncio.run(%s.run(action), debug=True)
        return f'Attempting to execute function {action["name"]} in app {action["app_name"]}' 
    else:
        return f'Invalid action'

	`, classname)
}

func deployAppToDatastore(ctx context.Context, workflowapp model.WorkflowApp) error {
	err := setWorkflowAppDatastore(workflowapp, workflowapp.ID)
	if err != nil {
		log.Printf("Failed setting workflowapp: %s", err)
		return err
	} else {
		log.Printf("Added %s:%s to the database", workflowapp.Name, workflowapp.AppVersion)
	}

	return nil
}

func fixFunctionName(functionName, actualPath string) string {
	if len(functionName) == 0 {
		functionName = actualPath
	}
	//log.Printf("Fixing function name for %s", functionName)
	functionName = strings.Replace(functionName, " ", "_", -1)
	functionName = strings.Replace(functionName, ".", "", -1)
	functionName = strings.Replace(functionName, ".", "", -1)
	functionName = strings.Replace(functionName, "/", "", -1)
	functionName = strings.Replace(functionName, "\\", "", -1)
	functionName = strings.ToLower(functionName)

	return functionName
}

func handleConnect(swagger *openapi3.Swagger, api model.WorkflowApp, extraParameters []model.WorkflowAppActionParameter, path *openapi3.PathItem, actualPath string, firstQuery bool) (model.WorkflowAppAction, string) {
	// What to do with this, hmm
	functionName := fixFunctionName(path.Connect.Summary, actualPath)

	action := model.WorkflowAppAction{
		Description: path.Connect.Description,
		Name:        fmt.Sprintf("%s %s", "Connect", path.Connect.Summary),
		Label:       fmt.Sprintf(path.Connect.Summary),
		NodeType:    "action",
		Environment: api.Environment,
		Parameters:  extraParameters,
	}

	action.Returns.Schema.Type = "string"
	baseUrl := fmt.Sprintf("%s%s", api.Link, actualPath)

	//log.Println(path.Parameters)

	// Parameters:  []WorkflowAppActionParameter{},
	// FIXME - add data for POST stuff
	firstQuery = true
	optionalQueries := []string{}
	parameters := []string{}
	optionalParameters := []model.WorkflowAppActionParameter{}
	if len(path.Connect.Parameters) > 0 {
		for _, param := range path.Connect.Parameters {
			curParam := model.WorkflowAppActionParameter{
				Name:        param.Value.Name,
				Description: param.Value.Description,
				Multiline:   false,
				Required:    param.Value.Required,
				Schema: model.SchemaDefinition{
					Type: param.Value.Schema.Value.Type,
				},
			}

			if param.Value.Required {
				action.Parameters = append(action.Parameters, curParam)
			} else {
				optionalParameters = append(optionalParameters, curParam)
			}

			if param.Value.In == "path" {
				//log.Printf("PATH!: %s", param.Value.Name)
				parameters = append(parameters, param.Value.Name)
				//baseUrl = fmt.Sprintf("%s%s", baseUrl)
			} else if param.Value.In == "query" {
				//log.Printf("QUERY!: %s", param.Value.Name)
				if !param.Value.Required {
					optionalQueries = append(optionalQueries, param.Value.Name)
					continue
				}

				parameters = append(parameters, param.Value.Name)

				if firstQuery {
					baseUrl = fmt.Sprintf("%s?%s={%s}", baseUrl, param.Value.Name, param.Value.Name)
					firstQuery = false
				} else {
					baseUrl = fmt.Sprintf("%s&%s={%s}", baseUrl, param.Value.Name, param.Value.Name)
					firstQuery = false
				}
			}

		}
	}

	// ensuring that they end up last in the specification
	// (order is ish important for optional params) - they need to be last.
	for _, optionalParam := range optionalParameters {
		action.Parameters = append(action.Parameters, optionalParam)
	}

	functionname, curCode := makePythoncode(swagger, functionName, baseUrl, "connect", parameters, optionalQueries)

	if len(functionname) > 0 {
		action.Name = functionname
	}

	return action, curCode
}

func handleGet(swagger *openapi3.Swagger, api model.WorkflowApp, extraParameters []model.WorkflowAppActionParameter, path *openapi3.PathItem, actualPath string, firstQuery bool) (model.WorkflowAppAction, string) {
	// What to do with this, hmm
	functionName := fixFunctionName(path.Get.Summary, actualPath)

	action := model.WorkflowAppAction{
		Description: path.Get.Description,
		Name:        fmt.Sprintf("%s %s", "Get", path.Get.Summary),
		Label:       fmt.Sprintf(path.Get.Summary),
		NodeType:    "action",
		Environment: api.Environment,
		Parameters:  extraParameters,
	}

	log.Printf("FUNCTION: %#v", action)

	action.Returns.Schema.Type = "string"
	baseUrl := fmt.Sprintf("%s%s", api.Link, actualPath)

	//log.Println(path.Parameters)

	// Parameters:  []WorkflowAppActionParameter{},
	// FIXME - add data for POST stuff
	firstQuery = true
	optionalQueries := []string{}

	// FIXME - remove this when authentication is properly introduced
	parameters := []string{}

	optionalParameters := []model.WorkflowAppActionParameter{}
	if len(path.Get.Parameters) > 0 {
		for _, param := range path.Get.Parameters {
			curParam := model.WorkflowAppActionParameter{
				Name:        param.Value.Name,
				Description: param.Value.Description,
				Multiline:   false,
				Required:    param.Value.Required,
				Schema: model.SchemaDefinition{
					Type: param.Value.Schema.Value.Type,
				},
			}

			if param.Value.Required {
				action.Parameters = append(action.Parameters, curParam)
			} else {
				optionalParameters = append(optionalParameters, curParam)
			}

			if param.Value.In == "path" {
				//log.Printf("PATH!: %s", param.Value.Name)
				parameters = append(parameters, param.Value.Name)
				//baseUrl = fmt.Sprintf("%s%s", baseUrl)
			} else if param.Value.In == "query" {
				//log.Printf("QUERY!: %s", param.Value.Name)
				if !param.Value.Required {
					optionalQueries = append(optionalQueries, param.Value.Name)
					continue
				}

				parameters = append(parameters, param.Value.Name)

				if firstQuery {
					baseUrl = fmt.Sprintf("%s?%s={%s}", baseUrl, param.Value.Name, param.Value.Name)
					firstQuery = false
				} else {
					baseUrl = fmt.Sprintf("%s&%s={%s}", baseUrl, param.Value.Name, param.Value.Name)
					firstQuery = false
				}
			}

		}
	}

	// ensuring that they end up last in the specification
	// (order is ish important for optional params) - they need to be last.
	for _, optionalParam := range optionalParameters {
		action.Parameters = append(action.Parameters, optionalParam)
	}

	functionname, curCode := makePythoncode(swagger, functionName, baseUrl, "get", parameters, optionalQueries)

	if len(functionname) > 0 {
		action.Name = functionname
	}

	return action, curCode
}

func handleHead(swagger *openapi3.Swagger, api model.WorkflowApp, extraParameters []model.WorkflowAppActionParameter, path *openapi3.PathItem, actualPath string, firstQuery bool) (model.WorkflowAppAction, string) {
	// What to do with this, hmm
	functionName := fixFunctionName(path.Head.Summary, actualPath)

	action := model.WorkflowAppAction{
		Description: path.Head.Description,
		Name:        fmt.Sprintf("%s %s", "Head", path.Head.Summary),
		Label:       fmt.Sprintf(path.Head.Summary),
		NodeType:    "action",
		Environment: api.Environment,
		Parameters:  extraParameters,
	}

	action.Returns.Schema.Type = "string"
	baseUrl := fmt.Sprintf("%s%s", api.Link, actualPath)

	//log.Println(path.Parameters)

	// Parameters:  []WorkflowAppActionParameter{},
	// FIXME - add data for POST stuff
	firstQuery = true
	optionalQueries := []string{}
	parameters := []string{}
	optionalParameters := []model.WorkflowAppActionParameter{}
	if len(path.Head.Parameters) > 0 {
		for _, param := range path.Head.Parameters {
			curParam := model.WorkflowAppActionParameter{
				Name:        param.Value.Name,
				Description: param.Value.Description,
				Multiline:   false,
				Required:    param.Value.Required,
				Schema: model.SchemaDefinition{
					Type: param.Value.Schema.Value.Type,
				},
			}

			if param.Value.Required {
				action.Parameters = append(action.Parameters, curParam)
			} else {
				optionalParameters = append(optionalParameters, curParam)
			}

			if param.Value.In == "path" {
				//log.Printf("PATH!: %s", param.Value.Name)
				parameters = append(parameters, param.Value.Name)
				//baseUrl = fmt.Sprintf("%s%s", baseUrl)
			} else if param.Value.In == "query" {
				//log.Printf("QUERY!: %s", param.Value.Name)
				if !param.Value.Required {
					optionalQueries = append(optionalQueries, param.Value.Name)
					continue
				}

				parameters = append(parameters, param.Value.Name)

				if firstQuery {
					baseUrl = fmt.Sprintf("%s?%s={%s}", baseUrl, param.Value.Name, param.Value.Name)
					firstQuery = false
				} else {
					baseUrl = fmt.Sprintf("%s&%s={%s}", baseUrl, param.Value.Name, param.Value.Name)
					firstQuery = false
				}
			}

		}
	}

	// ensuring that they end up last in the specification
	// (order is ish important for optional params) - they need to be last.
	for _, optionalParam := range optionalParameters {
		action.Parameters = append(action.Parameters, optionalParam)
	}

	functionname, curCode := makePythoncode(swagger, functionName, baseUrl, "head", parameters, optionalQueries)

	if len(functionname) > 0 {
		action.Name = functionname
	}

	return action, curCode
}

func handleDelete(swagger *openapi3.Swagger, api model.WorkflowApp, extraParameters []model.WorkflowAppActionParameter, path *openapi3.PathItem, actualPath string, firstQuery bool) (model.WorkflowAppAction, string) {
	// What to do with this, hmm
	functionName := fixFunctionName(path.Delete.Summary, actualPath)

	action := model.WorkflowAppAction{
		Description: path.Delete.Description,
		Name:        fmt.Sprintf("%s %s", "Delete", path.Delete.Summary),
		Label:       fmt.Sprintf(path.Delete.Summary),
		NodeType:    "action",
		Environment: api.Environment,
		Parameters:  extraParameters,
	}

	action.Returns.Schema.Type = "string"
	baseUrl := fmt.Sprintf("%s%s", api.Link, actualPath)

	//log.Println(path.Parameters)

	// Parameters:  []WorkflowAppActionParameter{},
	// FIXME - add data for POST stuff
	firstQuery = true
	optionalQueries := []string{}
	parameters := []string{}
	optionalParameters := []model.WorkflowAppActionParameter{}
	if len(path.Delete.Parameters) > 0 {
		for _, param := range path.Delete.Parameters {
			curParam := model.WorkflowAppActionParameter{
				Name:        param.Value.Name,
				Description: param.Value.Description,
				Multiline:   false,
				Required:    param.Value.Required,
				Schema: model.SchemaDefinition{
					Type: param.Value.Schema.Value.Type,
				},
			}

			if param.Value.Required {
				action.Parameters = append(action.Parameters, curParam)
			} else {
				optionalParameters = append(optionalParameters, curParam)
			}

			if param.Value.In == "path" {
				//log.Printf("PATH!: %s", param.Value.Name)
				parameters = append(parameters, param.Value.Name)
				//baseUrl = fmt.Sprintf("%s%s", baseUrl)
			} else if param.Value.In == "query" {
				//log.Printf("QUERY!: %s", param.Value.Name)
				if !param.Value.Required {
					optionalQueries = append(optionalQueries, param.Value.Name)
					continue
				}

				parameters = append(parameters, param.Value.Name)

				if firstQuery {
					baseUrl = fmt.Sprintf("%s?%s={%s}", baseUrl, param.Value.Name, param.Value.Name)
					firstQuery = false
				} else {
					baseUrl = fmt.Sprintf("%s&%s={%s}", baseUrl, param.Value.Name, param.Value.Name)
					firstQuery = false
				}
			}

		}
	}

	// ensuring that they end up last in the specification
	// (order is ish important for optional params) - they need to be last.
	for _, optionalParam := range optionalParameters {
		action.Parameters = append(action.Parameters, optionalParam)
	}

	functionname, curCode := makePythoncode(swagger, functionName, baseUrl, "delete", parameters, optionalQueries)

	if len(functionname) > 0 {
		action.Name = functionname
	}

	return action, curCode
}

func handlePost(swagger *openapi3.Swagger, api model.WorkflowApp, extraParameters []model.WorkflowAppActionParameter, path *openapi3.PathItem, actualPath string, firstQuery bool) (model.WorkflowAppAction, string) {
	// What to do with this, hmm
	log.Printf("PATH: %s", actualPath)
	functionName := fixFunctionName(path.Post.Summary, actualPath)

	action := model.WorkflowAppAction{
		Description: path.Post.Description,
		Name:        fmt.Sprintf("%s %s", "Post", path.Post.Summary),
		Label:       fmt.Sprintf(path.Post.Summary),
		NodeType:    "action",
		Environment: api.Environment,
		Parameters:  extraParameters,
	}

	action.Returns.Schema.Type = "string"
	baseUrl := fmt.Sprintf("%s%s", api.Link, actualPath)

	//log.Println(path.Parameters)

	// Parameters:  []WorkflowAppActionParameter{},
	// FIXME - add data for POST stuff
	firstQuery = true
	optionalQueries := []string{}
	parameters := []string{}
	optionalParameters := []model.WorkflowAppActionParameter{
		model.WorkflowAppActionParameter{
			Name:        "body",
			Description: "The body to use",
			Multiline:   true,
			Required:    false,
			Example:     `{"username": "test"}`,
			Schema: model.SchemaDefinition{
				Type: "string",
			},
		},
	}
	if len(path.Post.Parameters) > 0 {
		for _, param := range path.Post.Parameters {
			curParam := model.WorkflowAppActionParameter{
				Name:        param.Value.Name,
				Description: param.Value.Description,
				Multiline:   false,
				Required:    param.Value.Required,
				Schema: model.SchemaDefinition{
					Type: param.Value.Schema.Value.Type,
				},
			}

			if param.Value.Required {
				action.Parameters = append(action.Parameters, curParam)
			} else {
				optionalParameters = append(optionalParameters, curParam)
			}

			if param.Value.In == "path" {
				//log.Printf("PATH!: %s", param.Value.Name)
				parameters = append(parameters, param.Value.Name)
				//baseUrl = fmt.Sprintf("%s%s", baseUrl)
			} else if param.Value.In == "query" {
				//log.Printf("QUERY!: %s", param.Value.Name)
				if !param.Value.Required {
					optionalQueries = append(optionalQueries, param.Value.Name)
					continue
				}

				parameters = append(parameters, param.Value.Name)

				if firstQuery {
					baseUrl = fmt.Sprintf("%s?%s={%s}", baseUrl, param.Value.Name, param.Value.Name)
					firstQuery = false
				} else {
					baseUrl = fmt.Sprintf("%s&%s={%s}", baseUrl, param.Value.Name, param.Value.Name)
					firstQuery = false
				}
			}

		}
	}

	// ensuring that they end up last in the specification
	// (order is ish important for optional params) - they need to be last.
	for _, optionalParam := range optionalParameters {
		action.Parameters = append(action.Parameters, optionalParam)
	}

	functionname, curCode := makePythoncode(swagger, functionName, baseUrl, "post", parameters, optionalQueries)

	if len(functionname) > 0 {
		action.Name = functionname
	}

	return action, curCode
}

func handlePatch(swagger *openapi3.Swagger, api model.WorkflowApp, extraParameters []model.WorkflowAppActionParameter, path *openapi3.PathItem, actualPath string, firstQuery bool) (model.WorkflowAppAction, string) {
	// What to do with this, hmm
	functionName := fixFunctionName(path.Patch.Summary, actualPath)

	action := model.WorkflowAppAction{
		Description: path.Patch.Description,
		Name:        fmt.Sprintf("%s %s", "Patch", path.Patch.Summary),
		Label:       fmt.Sprintf(path.Patch.Summary),
		NodeType:    "action",
		Environment: api.Environment,
		Parameters:  extraParameters,
	}

	action.Returns.Schema.Type = "string"
	baseUrl := fmt.Sprintf("%s%s", api.Link, actualPath)

	//log.Println(path.Parameters)

	// Parameters:  []WorkflowAppActionParameter{},
	// FIXME - add data for POST stuff
	firstQuery = true
	optionalQueries := []string{}
	parameters := []string{}
	optionalParameters := []model.WorkflowAppActionParameter{
		model.WorkflowAppActionParameter{
			Name:        "body",
			Description: "The body to use",
			Multiline:   true,
			Required:    false,
			Example:     `{"username": "test"}`,
			Schema: model.SchemaDefinition{
				Type: "string",
			},
		},
	}
	if len(path.Patch.Parameters) > 0 {
		for _, param := range path.Patch.Parameters {
			curParam := model.WorkflowAppActionParameter{
				Name:        param.Value.Name,
				Description: param.Value.Description,
				Multiline:   false,
				Required:    param.Value.Required,
				Schema: model.SchemaDefinition{
					Type: param.Value.Schema.Value.Type,
				},
			}

			if param.Value.Required {
				action.Parameters = append(action.Parameters, curParam)
			} else {
				optionalParameters = append(optionalParameters, curParam)
			}

			if param.Value.In == "path" {
				//log.Printf("PATH!: %s", param.Value.Name)
				parameters = append(parameters, param.Value.Name)
				//baseUrl = fmt.Sprintf("%s%s", baseUrl)
			} else if param.Value.In == "query" {
				//log.Printf("QUERY!: %s", param.Value.Name)
				if !param.Value.Required {
					optionalQueries = append(optionalQueries, param.Value.Name)
					continue
				}

				parameters = append(parameters, param.Value.Name)

				if firstQuery {
					baseUrl = fmt.Sprintf("%s?%s={%s}", baseUrl, param.Value.Name, param.Value.Name)
					firstQuery = false
				} else {
					baseUrl = fmt.Sprintf("%s&%s={%s}", baseUrl, param.Value.Name, param.Value.Name)
					firstQuery = false
				}
			}

		}
	}

	// ensuring that they end up last in the specification
	// (order is ish important for optional params) - they need to be last.
	for _, optionalParam := range optionalParameters {
		action.Parameters = append(action.Parameters, optionalParam)
	}

	functionname, curCode := makePythoncode(swagger, functionName, baseUrl, "patch", parameters, optionalQueries)

	if len(functionname) > 0 {
		action.Name = functionname
	}

	return action, curCode
}

func handlePut(swagger *openapi3.Swagger, api model.WorkflowApp, extraParameters []model.WorkflowAppActionParameter, path *openapi3.PathItem, actualPath string, firstQuery bool) (model.WorkflowAppAction, string) {
	// What to do with this, hmm
	functionName := fixFunctionName(path.Put.Summary, actualPath)

	action := model.WorkflowAppAction{
		Description: path.Put.Description,
		Name:        fmt.Sprintf("%s %s", "Put", path.Put.Summary),
		Label:       fmt.Sprintf(path.Put.Summary),
		NodeType:    "action",
		Environment: api.Environment,
		Parameters:  extraParameters,
	}

	action.Returns.Schema.Type = "string"
	baseUrl := fmt.Sprintf("%s%s", api.Link, actualPath)

	//log.Println(path.Parameters)

	// Parameters:  []WorkflowAppActionParameter{},
	// FIXME - add data for POST stuff
	firstQuery = true
	optionalQueries := []string{}
	parameters := []string{}
	optionalParameters := []model.WorkflowAppActionParameter{
		model.WorkflowAppActionParameter{
			Name:        "body",
			Description: "The body to use",
			Multiline:   true,
			Required:    false,
			Example:     `{"username": "test"}`,
			Schema: model.SchemaDefinition{
				Type: "string",
			},
		},
	}
	if len(path.Put.Parameters) > 0 {
		for _, param := range path.Put.Parameters {
			curParam := model.WorkflowAppActionParameter{
				Name:        param.Value.Name,
				Description: param.Value.Description,
				Multiline:   false,
				Required:    param.Value.Required,
				Schema: model.SchemaDefinition{
					Type: param.Value.Schema.Value.Type,
				},
			}

			if param.Value.Required {
				action.Parameters = append(action.Parameters, curParam)
			} else {
				optionalParameters = append(optionalParameters, curParam)
			}

			if param.Value.In == "path" {
				//log.Printf("PATH!: %s", param.Value.Name)
				parameters = append(parameters, param.Value.Name)
				//baseUrl = fmt.Sprintf("%s%s", baseUrl)
			} else if param.Value.In == "query" {
				//log.Printf("QUERY!: %s", param.Value.Name)
				if !param.Value.Required {
					optionalQueries = append(optionalQueries, param.Value.Name)
					continue
				}

				parameters = append(parameters, param.Value.Name)

				if firstQuery {
					baseUrl = fmt.Sprintf("%s?%s={%s}", baseUrl, param.Value.Name, param.Value.Name)
					firstQuery = false
				} else {
					baseUrl = fmt.Sprintf("%s&%s={%s}", baseUrl, param.Value.Name, param.Value.Name)
					firstQuery = false
				}
			}

		}
	}

	// ensuring that they end up last in the specification
	// (order is ish important for optional params) - they need to be last.
	for _, optionalParam := range optionalParameters {
		action.Parameters = append(action.Parameters, optionalParam)
	}

	functionname, curCode := makePythoncode(swagger, functionName, baseUrl, "put", parameters, optionalQueries)

	if len(functionname) > 0 {
		action.Name = functionname
	}

	return action, curCode
}
