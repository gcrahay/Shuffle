package model

type ExecutionInfo struct {
	ID uint64 `storm:"id,increment"`
	TotalApiUsage           int64 `json:"total_api_usage" datastore:"total_api_usage"`
	TotalWorkflowExecutions int64 `json:"total_workflow_executions" datastore:"total_workflow_executions"`
	TotalAppExecutions      int64 `json:"total_app_executions" datastore:"total_app_executions"`
	TotalCloudExecutions    int64 `json:"total_cloud_executions" datastore:"total_cloud_executions"`
	TotalOnpremExecutions   int64 `json:"total_onprem_executions" datastore:"total_onprem_executions"`
	DailyApiUsage           int64 `json:"daily_api_usage" datastore:"daily_api_usage"`
	DailyWorkflowExecutions int64 `json:"daily_workflow_executions" datastore:"daily_workflow_executions"`
	DailyAppExecutions      int64 `json:"daily_app_executions" datastore:"daily_app_executions"`
	DailyCloudExecutions    int64 `json:"daily_cloud_executions" datastore:"daily_cloud_executions"`
	DailyOnpremExecutions   int64 `json:"daily_onprem_executions" datastore:"daily_onprem_executions"`
}

type StatisticsData struct {
	Timestamp int64  `json:"timestamp" datastore:"timestamp"`
	Id        string `json:"id" datastore:"id" storm:"id"`
	Amount    int64  `json:"amount" datastore:"amount"`
}

type StatisticsItem struct {
	ID uint64 `storm:"id,increment"`
	Total     int64            `json:"total" datastore:"total"`
	Fieldname string           `json:"field_name" datastore:"field_name"`
	Data      []StatisticsData `json:"data" datastore:"data"`
}

type ParsedOpenApi struct {
	Body    string `datastore:"body,noindex" json:"body"`
	ID      string `datastore:"id" json:"id"`
	Success bool   `datastore:"success,omitempty" json:"success,omitempty"`
}

// Limits set for a user so that they can't do a shitload
type UserLimits struct {
	DailyApiUsage           int64 `json:"daily_api_usage" datastore:"daily_api_usage"`
	DailyWorkflowExecutions int64 `json:"daily_workflow_executions" datastore:"daily_workflow_executions"`
	DailyCloudExecutions    int64 `json:"daily_cloud_executions" datastore:"daily_cloud_executions"`
	DailyTriggers           int64 `json:"daily_triggers" datastore:"daily_triggers"`
	DailyMailUsage          int64 `json:"daily_mail_usage" datastore:"daily_mail_usage"`
	MaxTriggers             int64 `json:"max_triggers" datastore:"max_triggers"`
	MaxWorkflows            int64 `json:"max_workflows" datastore:"max_workflows"`
}

// Saves some data, not sure what to have here lol
type UserAuth struct {
	Description string          `json:"description" datastore:"description" yaml:"description"`
	Name        string          `json:"name" datastore:"name" yaml:"name"`
	Workflows   []string        `json:"workflows" datastore:"workflows"`
	Username    string          `json:"username" datastore:"username"`
	Fields      []UserAuthField `json:"fields" datastore:"fields"`
}

type UserAuthField struct {
	Key   string `json:"key" datastore:"key"`
	Value string `json:"value" datastore:"value"`
}

// Not environment, but execution environment
type Environment struct {
	Id uint64 `json:"id" storm:"id,increment"`
	Name       string `datastore:"name"`
	Type       string `datastore:"type"`
	Registered bool   `datastore:"registered"`
}

type User struct {
	Id                string        `storm:"id" json:"id"`
	Username          string        `datastore:"Username"`
	Password          string        `json:"-" datastore:"password,noindex"`
	Session           string        `json:"-" dataore:"session,noindex"`
	Verified          bool          `datastore:"verified,noindex"`
	PrivateApps       []WorkflowApp `datastore:"privateapps"`
	Role              string        `datastore:"role"`
	VerificationToken string        `json:"-" datastore:"verification_token"`
	ApiKey            string        `datastore:"apikey"`
	ResetReference    string        `datastore:"reset_reference"`
	Executions        ExecutionInfo `datastore:"executions" json:"executions"`
	Limits            UserLimits    `datastore:"limits" json:"limits"`
	Authentication    []UserAuth    `datastore:"authentication,noindex" json:"authentication"`
	ResetTimeout      int64         `datastore:"reset_timeout,noindex"`
	Orgs              string        `datastore:"orgs" json:"orgs"`
	CreationTime      int64         `datastore:"creation_time" json:"creation_time"`
}

type Contact struct {
	Firstname   string `json:"firstname"`
	Lastname    string `json:"lastname"`
	Title       string `json:"title"`
	Companyname string `json:"companyname"`
	Phone       string `json:"phone"`
	Email       string `json:"email"`
	Message     string `json:"message"`
}

type Translator struct {
	Src struct {
		Name        string `json:"name" datastore:"name"`
		Value       string `json:"value" datastore:"value"`
		Description string `json:"description" datastore:"description"`
		Required    string `json:"required" datastore:"required"`
		Type        string `json:"type" datastore:"type"`
		Schema      struct {
			Type string `json:"type" datastore:"type"`
		} `json:"schema" datastore:"schema"`
	} `json:"src" datastore:"src"`
	Dst struct {
		Name        string `json:"name" datastore:"name"`
		Value       string `json:"value" datastore:"value"`
		Type        string `json:"type" datastore:"type"`
		Description string `json:"description" datastore:"description"`
		Required    string `json:"required" datastore:"required"`
		Schema      struct {
			Type string `json:"type" datastore:"type"`
		} `json:"schema" datastore:"schema"`
	} `json:"dst" datastore:"dst"`
}

type Appconfig struct {
	Key   string `json:"key" datastore:"key"`
	Value string `json:"value" datastore:"value"`
}

type ScheduleApp struct {
	Foldername  string      `json:"foldername" datastore:"foldername,noindex"`
	Name        string      `json:"name" datastore:"name,noindex"`
	Id          string      `json:"id" datastore:"id,noindex"`
	Description string      `json:"description" datastore:"description,noindex"`
	Action      string      `json:"action" datastore:"action,noindex"`
	Config      []Appconfig `json:"config,omitempty" datastore:"config,noindex"`
}

type AppInfo struct {
	SourceApp      ScheduleApp `json:"sourceapp,omitempty" datastore:"sourceapp,noindex"`
	DestinationApp ScheduleApp `json:"destinationapp,omitempty" datastore:"destinationapp,noindex"`
}

// Used for the api integrator
//Username string `datastore:"Username,noindex"`
type ScheduleOld struct {
	Id                   string       `json:"id" datastore:"id"`
	Seconds              int          `json:"seconds" datastore:"seconds"`
	WorkflowId           string       `json:"workflow_id" datastore:"workflow_id"`
	Argument             string       `json:"argument" datastore:"argument"`
	AppInfo              AppInfo      `json:"appinfo" datastore:"appinfo,noindex"`
	Finished             bool         `json:"finished" finished:"id"`
	BaseAppLocation      string       `json:"base_app_location" datastore:"baseapplocation,noindex"`
	Translator           []Translator `json:"translator,omitempty" datastore:"translator"`
	Org                  string       `json:"org" datastore:"org"`
	CreatedBy            string       `json:"createdby" datastore:"createdby"`
	Availability         string       `json:"availability" datastore:"availability"`
	CreationTime         int64        `json:"creationtime" datastore:"creationtime,noindex"`
	LastModificationtime int64        `json:"lastmodificationtime" datastore:"lastmodificationtime,noindex"`
	LastRuntime          int64        `json:"lastruntime" datastore:"lastruntime,noindex"`
}

// Returned from /GET /schedules
type Schedules struct {
	Schedules []ScheduleOld `json:"schedules"`
	Success   bool          `json:"success"`
}

type ScheduleApps struct {
	Apps    []ApiYaml `json:"apps"`
	Success bool      `json:"success"`
}

type Hooks struct {
	Hooks   []Hook `json:"hooks"`
	Success bool   `json:"-"`
}

type Info struct {
	Url         string `json:"url" datastore:"url"`
	Name        string `json:"name" datastore:"name"`
	Description string `json:"description" datastore:"description"`
}

// Actions to be done by webhooks etc
// Field is the actual field to use from json
type HookAction struct {
	Type  string `json:"type" datastore:"type"`
	Name  string `json:"name" datastore:"name"`
	Id    string `json:"id" datastore:"id"`
	Field string `json:"field" datastore:"field"`
}

type Hook struct {
	Id        string       `json:"id" datastore:"id"`
	Info      Info         `json:"info" datastore:"info"`
	Actions   []HookAction `json:"actions" datastore:"actions"`
	Type      string       `json:"type" datastore:"type"`
	Owner     string       `json:"owner" datastore:"owner"`
	Status    string       `json:"status" datastore:"status"`
	Workflows []string     `json:"workflows" datastore:"workflows"`
	Running   bool         `json:"running" datastore:"running"`
}

type Org struct {
	Name  string `json:"name"`
	Org   string `json:"org"`
	Users []User `json:"users"`
	Id    string `json:"id"`
}

type WorkflowApp struct {
	Name        string `json:"name" yaml:"name" required:true datastore:"name"`
	IsValid     bool   `json:"is_valid" yaml:"is_valid" required:true datastore:"is_valid"`
	ID          string `json:"id" yaml:"id,omitempty" required:false datastore:"id"`
	Link        string `json:"link" yaml:"link" required:false datastore:"link,noindex"`
	AppVersion  string `json:"app_version" yaml:"app_version" required:true datastore:"app_version"`
	Generated   bool   `json:"generated" yaml:"generated" required:false datastore:"generated"`
	Downloaded  bool   `json:"downloaded" yaml:"downloaded" required:false datastore:"downloaded"`
	Sharing     bool   `json:"sharing" yaml:"sharing" required:false datastore:"sharing"`
	Verified    bool   `json:"verified" yaml:"verified" required:false datastore:"verified"`
	Tested      bool   `json:"tested" yaml:"tested" required:false datastore:"tested"`
	Owner       string `json:"owner" datastore:"owner" yaml:"owner"`
	PrivateID   string `json:"private_id" yaml:"private_id" required:false datastore:"private_id"`
	Description string `json:"description" datastore:"description" required:false yaml:"description"`
	Environment string `json:"environment" datastore:"environment" required:true yaml:"environment"`
	SmallImage  string `json:"small_image" datastore:"small_image,noindex" required:false yaml:"small_image"`
	LargeImage  string `json:"large_image" datastore:"large_image,noindex" yaml:"large_image" required:false`
	ContactInfo struct {
		Name string `json:"name" datastore:"name" yaml:"name"`
		Url  string `json:"url" datastore:"url" yaml:"url"`
	} `json:"contact_info" datastore:"contact_info" yaml:"contact_info" required:false`
	Actions        []WorkflowAppAction `json:"actions" yaml:"actions" required:true datastore:"actions"`
	Authentication Authentication      `json:"authentication" yaml:"authentication" required:false datastore:"authentication"`
}

type WorkflowAppActionParameter struct {
	Description string           `json:"description" datastore:"description" yaml:"description"`
	ID          string           `json:"id" datastore:"id" yaml:"id,omitempty"`
	Name        string           `json:"name" datastore:"name" yaml:"name"`
	Example     string           `json:"example" datastore:"example" yaml:"example"`
	Value       string           `json:"value" datastore:"value" yaml:"value,omitempty"`
	Multiline   bool             `json:"multiline" datastore:"multiline" yaml:"multiline"`
	ActionField string           `json:"action_field" datastore:"action_field" yaml:"actionfield,omitempty"`
	Variant     string           `json:"variant" datastore:"variant" yaml:"variant,omitempty"`
	Required    bool             `json:"required" datastore:"required" yaml:"required"`
	Schema      SchemaDefinition `json:"schema" datastore:"schema" yaml:"schema"`
}

type SchemaDefinition struct {
	Type string `json:"type" datastore:"type"`
}

type WorkflowAppAction struct {
	Description    string                       `json:"description" datastore:"description"`
	ID             string                       `json:"id" datastore:"id" yaml:"id,omitempty"`
	Name           string                       `json:"name" datastore:"name"`
	Label          string                       `json:"label" datastore:"label"`
	NodeType       string                       `json:"node_type" datastore:"node_type"`
	Environment    string                       `json:"environment" datastore:"environment"`
	Sharing        bool                         `json:"sharing" datastore:"sharing"`
	PrivateID      string                       `json:"private_id" datastore:"private_id"`
	AppID          string                       `json:"app_id" datastore:"app_id"`
	Authentication []AuthenticationStore        `json:"authentication" datastore:"authentication" yaml:"authentication,omitempty"`
	Tested         bool                         `json:"tested" datastore:"tested" yaml:"tested"`
	Parameters     []WorkflowAppActionParameter `json:"parameters" datastore: "parameters"`
	Returns        struct {
		Description string           `json:"description" datastore:"returns" yaml:"description,omitempty"`
		ID          string           `json:"id" datastore:"id" yaml:"id,omitempty"`
		Schema      SchemaDefinition `json:"schema" datastore:"schema" yaml:"schema"`
	} `json:"returns" datastore:"returns"`
}

// FIXME: Generate a callback authentication ID?
type WorkflowExecution struct {
	Type              string         `json:"type" datastore:"type"`
	Status            string         `json:"status" datastore:"status"`
	Start             string         `json:"start" datastore:"start"`
	ExecutionArgument string         `json:"execution_argument" datastore:"execution_argument"`
	ExecutionId       string         `json:"execution_id" datastore:"execution_id" storm:"id"`
	WorkflowId        string         `json:"workflow_id" datastore:"workflow_id"`
	LastNode          string         `json:"last_node" datastore:"last_node"`
	Authorization     string         `json:"authorization" datastore:"authorization"`
	Result            string         `json:"result" datastore:"result,noindex"`
	StartedAt         int64          `json:"started_at" datastore:"started_at"`
	CompletedAt       int64          `json:"completed_at" datastore:"completed_at"`
	ProjectId         string         `json:"project_id" datastore:"project_id"`
	Locations         []string       `json:"locations" datastore:"locations"`
	Workflow          Workflow       `json:"workflow" datastore:"workflow,noindex"`
	Results           []ActionResult `json:"results" datastore:"results,noindex"`
}

// Added environment for location to execute
type Action struct {
	AppName     string                       `json:"app_name" datastore:"app_name"`
	AppVersion  string                       `json:"app_version" datastore:"app_version"`
	AppID       string                       `json:"app_id" datastore:"app_id"`
	Errors      []string                     `json:"errors" datastore:"errors"`
	ID          string                       `json:"id" datastore:"id"`
	IsValid     bool                         `json:"is_valid" datastore:"is_valid"`
	IsStartNode bool                         `json:"isStartNode" datastore:"isStartNode"`
	Sharing     bool                         `json:"sharing" datastore:"sharing"`
	PrivateID   string                       `json:"private_id" datastore:"private_id"`
	Label       string                       `json:"label" datastore:"label"`
	SmallImage  string                       `json:"small_image" datastore:"small_image,noindex" required:false yaml:"small_image"`
	LargeImage  string                       `json:"large_image" datastore:"large_image,noindex" yaml:"large_image" required:false`
	Environment string                       `json:"environment" datastore:"environment"`
	Name        string                       `json:"name" datastore:"name"`
	Parameters  []WorkflowAppActionParameter `json:"parameters" datastore: "parameters,noindex"`
	Position    struct {
		X float64 `json:"x" datastore:"x"`
		Y float64 `json:"y" datastore:"y"`
	} `json:"position"`
	Priority int `json:"priority" datastore:"priority"`
}

// Added environment for location to execute
type Trigger struct {
	AppName         string                       `json:"app_name" datastore:"app_name"`
	Description     string                       `json:"description" datastore:"description"`
	LongDescription string                       `json:"long_description" datastore:"long_description"`
	Status          string                       `json:"status" datastore:"status"`
	AppVersion      string                       `json:"app_version" datastore:"app_version"`
	Errors          []string                     `json:"errors" datastore:"errors"`
	ID              string                       `json:"id" datastore:"id"`
	IsValid         bool                         `json:"is_valid" datastore:"is_valid"`
	IsStartNode     bool                         `json:"isStartNode" datastore:"isStartNode"`
	Label           string                       `json:"label" datastore:"label"`
	SmallImage      string                       `json:"small_image" datastore:"small_image,noindex" required:false yaml:"small_image"`
	LargeImage      string                       `json:"large_image" datastore:"large_image,noindex" yaml:"large_image" required:false`
	Environment     string                       `json:"environment" datastore:"environment"`
	TriggerType     string                       `json:"trigger_type" datastore:"trigger_type"`
	Name            string                       `json:"name" datastore:"name"`
	Parameters      []WorkflowAppActionParameter `json:"parameters" datastore: "parameters,noindex"`
	Position        struct {
		X float64 `json:"x" datastore:"x"`
		Y float64 `json:"y" datastore:"y"`
	} `json:"position"`
	Priority int `json:"priority" datastore:"priority"`
}

type Branch struct {
	DestinationID string      `json:"destination_id" datastore:"destination_id"`
	ID            string      `json:"id" datastore:"id"`
	SourceID      string      `json:"source_id" datastore:"source_id"`
	Label         string      `json:"label" datastore:"label"`
	HasError      bool        `json:"has_errors" datastore: "has_errors"`
	Conditions    []Condition `json:"conditions" datastore: "conditions"`
}

// Same format for a lot of stuff
type Condition struct {
	Condition   WorkflowAppActionParameter `json:"condition" datastore:"condition"`
	Source      WorkflowAppActionParameter `json:"source" datastore:"source"`
	Destination WorkflowAppActionParameter `json:"destination" datastore:"destination"`
}

type Schedule struct {
	Id                string `json:"id" storm:"id"`
	Name              string `json:"name" datastore:"name"`
	Frequency         string `json:"frequency" datastore:"frequency"`
	ExecutionArgument string `json:"execution_argument" datastore:"execution_argument"`
}

type Workflow struct {
	Actions           []Action   `json:"actions" datastore:"actions,noindex"`
	Branches          []Branch   `json:"branches" datastore:"branches,noindex"`
	Triggers          []Trigger  `json:"triggers" datastore:"triggers,noindex"`
	Schedules         []Schedule `json:"schedules" datastore:"schedules,noindex"`
	Errors            []string   `json:"errors,omitempty" datastore:"errors"`
	Tags              []string   `json:"tags,omitempty" datastore:"tags"`
	ID                string     `json:"id" datastore:"id"`
	IsValid           bool       `json:"is_valid" datastore:"is_valid"`
	Name              string     `json:"name" datastore:"name"`
	Description       string     `json:"description" datastore:"description"`
	Start             string     `json:"start" datastore:"start"`
	Owner             string     `json:"owner" datastore:"owner"`
	Sharing           string     `json:"sharing" datastore:"sharing"`
	Org               []Org      `json:"org,omitempty" datastore:"org"`
	ExecutingOrg      Org        `json:"execution_org,omitempty" datastore:"execution_org"`
	WorkflowVariables []struct {
		Description string `json:"description" datastore:"description"`
		ID          string `json:"id" datastore:"id"`
		Name        string `json:"name" datastore:"name"`
		Value       string `json:"value" datastore:"value"`
	} `json:"workflow_variables" datastore:"workflow_variables"`
}

type ActionResult struct {
	Action        Action `json:"action" datastore:"action"`
	ExecutionId   string `json:"execution_id" datastore:"execution_id"`
	Authorization string `json:"authorization" datastore:"authorization"`
	Result        string `json:"result" datastore:"result,noindex"`
	StartedAt     int64  `json:"started_at" datastore:"started_at"`
	CompletedAt   int64  `json:"completed_at" datastore:"completed_at"`
	Status        string `json:"status" datastore:"status"`
}

type Authentication struct {
	Required   bool                   `json:"required" datastore:"required" yaml:"required" `
	Parameters []AuthenticationParams `json:"parameters" datastore:"parameters" yaml:"parameters"`
}

type AuthenticationParams struct {
	Description string `json:"description" datastore:"description" yaml:"description"`
	ID          string `json:"id" datastore:"id" yaml:"id"`
	Name        string `json:"name" datastore:"name" yaml:"name"`
	Example     string `json:"example" datastore:"example" yaml:"example"`
	Value       string `json:"value,omitempty" datastore:"value" yaml:"value"`
	Multiline   bool   `json:"multiline" datastore:"multiline" yaml:"multiline"`
	Required    bool   `json:"required" datastore:"required" yaml:"required"`
	In          string `json:"in" datastore:"in" yaml:"in"`
	Scheme      string `json:"scheme" datastore:"scheme" yaml:"scheme"`
}

type AuthenticationStore struct {
	Key   string `json:"key" datastore:"key"`
	Value string `json:"value" datastore:"value"`
}

type ExecutionRequestWrapper struct {
	ID string `storm:"id"`
	Data []ExecutionRequest `json:"data"`
}

type ExecutionRequest struct {
	ExecutionId       string   `json:"execution_id"`
	ExecutionArgument string   `json:"execution_argument"`
	WorkflowId        string   `json:"workflow_id"`
	Authorization     string   `json:"authorization"`
	Environments      []string `json:"environments"`
	Start             string   `json:"start"`
}

// The yaml that is uploaded
type ApiYaml struct {
	Name        string `json:"name" yaml:"name" required:"true datastore:"name"`
	Foldername  string `json:"foldername" yaml:"foldername" required:"true datastore:"foldername"`
	Id          string `json:"id" yaml:"id",required:"true, datastore:"id"`
	Description string `json:"description" datastore:"description" yaml:"description"`
	AppVersion  string `json:"app_version" yaml:"app_version",datastore:"app_version"`
	ContactInfo struct {
		Name string `json:"name" datastore:"name" yaml:"name"`
		Url  string `json:"url" datastore:"url" yaml:"url"`
	} `json:"contact_info" datastore:"contact_info" yaml:"contact_info"`
	Types []string `json:"types" datastore:"types" yaml:"types"`
	Input []struct {
		Name            string `json:"name" datastore:"name" yaml:"name"`
		Description     string `json:"description" datastore:"description" yaml:"description"`
		InputParameters []struct {
			Name        string `json:"name" datastore:"name" yaml:"name"`
			Description string `json:"description" datastore:"description" yaml:"description"`
			Required    string `json:"required" datastore:"required" yaml:"required"`
			Schema      struct {
				Type string `json:"type" datastore:"type" yaml:"type"`
			} `json:"schema" datastore:"schema" yaml:"schema"`
		} `json:"inputparameters" datastore:"inputparameters" yaml:"inputparameters"`
		OutputParameters []struct {
			Name        string `json:"name" datastore:"name" yaml:"name"`
			Description string `json:"description" datastore:"description" yaml:"description"`
			Required    string `json:"required" datastore:"required" yaml:"required"`
			Schema      struct {
				Type string `json:"type" datastore:"type" yaml:"type"`
			} `json:"schema" datastore:"schema" yaml:"schema"`
		} `json:"outputparameters" datastore:"outputparameters" yaml:"outputparameters"`
		Config []struct {
			Name        string `json:"name" datastore:"name" yaml:"name"`
			Description string `json:"description" datastore:"description" yaml:"description"`
			Required    string `json:"required" datastore:"required" yaml:"required"`
			Schema      struct {
				Type string `json:"type" datastore:"type" yaml:"type"`
			} `json:"schema" datastore:"schema" yaml:"schema"`
		} `json:"config" datastore:"config" yaml:"config"`
	} `json:"input" datastore:"input" yaml:"input"`
	Output []struct {
		Name        string `json:"name" datastore:"name" yaml:"name"`
		Description string `json:"description" datastore:"description" yaml:"description"`
		Config      []struct {
			Name        string `json:"name" datastore:"name" yaml:"name"`
			Description string `json:"description" datastore:"description" yaml:"description"`
			Required    string `json:"required" datastore:"required" yaml:"required"`
			Schema      struct {
				Type string `json:"type" datastore:"type" yaml:"type"`
			} `json:"schema" datastore:"schema" yaml:"schema"`
		} `json:"config" datastore:"config" yaml:"config"`
		InputParameters []struct {
			Name        string `json:"name" datastore:"name" yaml:"name"`
			Description string `json:"description" datastore:"description" yaml:"description"`
			Required    string `json:"required" datastore:"required" yaml:"required"`
			Schema      struct {
				Type string `json:"type" datastore:"type" yaml:"type"`
			} `json:"schema" datastore:"schema" yaml:"schema"`
		} `json:"inputparameters" datastore:"inputparameters" yaml:"inputparameters"`
		OutputParameters []struct {
			Name        string `json:"name" datastore:"name" yaml:"name"`
			Description string `json:"description" datastore:"description" yaml:"description"`
			Required    string `json:"required" datastore:"required" yaml:"required"`
			Schema      struct {
				Type string `json:"type" datastore:"type" yaml:"type"`
			} `json:"schema" datastore:"schema" yaml:"schema"`
		} `json:"outputparameters" datastore:"outputparameters" yaml:"outputparameters"`
	} `json:"output" datastore:"output" yaml:"output"`
}
