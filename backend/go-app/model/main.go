package model

type ExecutionInfo struct {
	ID                      uint64 `storm:"id,increment"`
	TotalApiUsage           int64  `json:"total_api_usage"`
	TotalWorkflowExecutions int64  `json:"total_workflow_executions"`
	TotalAppExecutions      int64  `json:"total_app_executions"`
	TotalCloudExecutions    int64  `json:"total_cloud_executions"`
	TotalOnpremExecutions   int64  `json:"total_onprem_executions"`
	DailyApiUsage           int64  `json:"daily_api_usage"`
	DailyWorkflowExecutions int64  `json:"daily_workflow_executions"`
	DailyAppExecutions      int64  `json:"daily_app_executions"`
	DailyCloudExecutions    int64  `json:"daily_cloud_executions"`
	DailyOnpremExecutions   int64  `json:"daily_onprem_executions"`
}

type StatisticsData struct {
	Timestamp int64  `json:"timestamp"`
	Id        string `json:"id" storm:"id"`
	Amount    int64  `json:"amount"`
}

type StatisticsItem struct {
	ID        uint64           `storm:"id,increment"`
	Total     int64            `json:"total"`
	Fieldname string           `json:"field_name"`
	Data      []StatisticsData `json:"data"`
}

type ParsedOpenApi struct {
	Body    string `json:"body"`
	ID      string `json:"id" storm:"id"`
	Success bool   `json:"success,omitempty"`
}

// Limits set for a user so that they can't do a shitload
type UserLimits struct {
	DailyApiUsage           int64 `json:"daily_api_usage"`
	DailyWorkflowExecutions int64 `json:"daily_workflow_executions"`
	DailyCloudExecutions    int64 `json:"daily_cloud_executions"`
	DailyTriggers           int64 `json:"daily_triggers"`
	DailyMailUsage          int64 `json:"daily_mail_usage"`
	MaxTriggers             int64 `json:"max_triggers"`
	MaxWorkflows            int64 `json:"max_workflows"`
}

// Saves some data, not sure what to have here lol
type UserAuth struct {
	Description string          `json:"description" yaml:"description"`
	Name        string          `json:"name" yaml:"name"`
	Workflows   []string        `json:"workflows"`
	Username    string          `json:"username"`
	Fields      []UserAuthField `json:"fields"`
}

type UserAuthField struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Not environment, but execution environment
type Environment struct {
	Id         uint64 `json:"id" storm:"id,increment"`
	Name       string
	Type       string
	Registered bool
}

type User struct {
	Id                string `storm:"id" json:"id"`
	Username          string
	Password          string `json:"-"`
	Session           string `json:"-"`
	Verified          bool
	PrivateApps       []WorkflowApp
	Role              string
	VerificationToken string `json:"-"`
	ApiKey            string
	ResetReference    string
	Executions        ExecutionInfo `json:"executions"`
	Limits            UserLimits    `json:"limits"`
	Authentication    []UserAuth    `json:"authentication"`
	ResetTimeout      int64
	Orgs              string `json:"orgs"`
	CreationTime      int64  `json:"creation_time"`
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
		Name        string `json:"name"`
		Value       string `json:"value"`
		Description string `json:"description"`
		Required    string `json:"required"`
		Type        string `json:"type"`
		Schema      struct {
			Type string `json:"type"`
		} `json:"schema"`
	} `json:"src"`
	Dst struct {
		Name        string `json:"name"`
		Value       string `json:"value"`
		Type        string `json:"type"`
		Description string `json:"description"`
		Required    string `json:"required"`
		Schema      struct {
			Type string `json:"type"`
		} `json:"schema"`
	} `json:"dst"`
}

type Appconfig struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type ScheduleApp struct {
	Foldername  string      `json:"foldername"`
	Name        string      `json:"name"`
	Id          string      `json:"id"`
	Description string      `json:"description"`
	Action      string      `json:"action"`
	Config      []Appconfig `json:"config,omitempty"`
}

type AppInfo struct {
	SourceApp      ScheduleApp `json:"sourceapp,omitempty"`
	DestinationApp ScheduleApp `json:"destinationapp,omitempty"`
}

// Used for the api integrator
//Username string `datastore:"Username,noindex"`
type ScheduleOld struct {
	Id                   string       `json:"id" storm:"id"`
	Seconds              int          `json:"seconds"`
	WorkflowId           string       `json:"workflow_id"`
	Argument             string       `json:"argument"`
	AppInfo              AppInfo      `json:"appinfo"`
	Finished             bool         `json:"finished"`
	BaseAppLocation      string       `json:"base_app_location"`
	Translator           []Translator `json:"translator,omitempty"`
	Org                  string       `json:"org"`
	CreatedBy            string       `json:"createdby"`
	Availability         string       `json:"availability"`
	CreationTime         int64        `json:"creationtime"`
	LastModificationtime int64        `json:"lastmodificationtime"`
	LastRuntime          int64        `json:"lastruntime"`
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
	Url         string `json:"url"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Actions to be done by webhooks etc
// Field is the actual field to use from json
type HookAction struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Id    string `json:"id"`
	Field string `json:"field"`
}

type Hook struct {
	Id        string       `json:"id" storm:"id"`
	Info      Info         `json:"info"`
	Actions   []HookAction `json:"actions"`
	Type      string       `json:"type"`
	Owner     string       `json:"owner"`
	Status    string       `json:"status"`
	Workflows []string     `json:"workflows"`
	Running   bool         `json:"running"`
}

type Org struct {
	Name  string `json:"name"`
	Org   string `json:"org"`
	Users []User `json:"users"`
	Id    string `json:"id"`
}

type WorkflowApp struct {
	Name        string `json:"name" yaml:"name"`
	IsValid     bool   `json:"is_valid" yaml:"is_valid"`
	ID          string `json:"id" yaml:"id,omitempty" storm:"id"`
	Link        string `json:"link" yaml:"link"`
	AppVersion  string `json:"app_version" yaml:"app_version"`
	Generated   bool   `json:"generated" yaml:"generated"`
	Downloaded  bool   `json:"downloaded" yaml:"downloaded"`
	Sharing     bool   `json:"sharing" yaml:"sharing"`
	Verified    bool   `json:"verified" yaml:"verified"`
	Tested      bool   `json:"tested" yaml:"tested"`
	Owner       string `json:"owner" yaml:"owner"`
	PrivateID   string `json:"private_id" yaml:"private_id"`
	Description string `json:"description" yaml:"description"`
	Environment string `json:"environment" yaml:"environment"`
	SmallImage  string `json:"small_image" yaml:"small_image"`
	LargeImage  string `json:"large_image" yaml:"large_image"`
	ContactInfo struct {
		Name string `json:"name" yaml:"name"`
		Url  string `json:"url" yaml:"url"`
	} `json:"contact_info" yaml:"contact_info"`
	Actions        []WorkflowAppAction `json:"actions" yaml:"actions"`
	Authentication Authentication      `json:"authentication" yaml:"authentication"`
}

type WorkflowAppActionParameter struct {
	Description string           `json:"description" yaml:"description"`
	ID          string           `json:"id" storm:"id" yaml:"id,omitempty"`
	Name        string           `json:"name" yaml:"name"`
	Example     string           `json:"example" yaml:"example"`
	Value       string           `json:"value" yaml:"value,omitempty"`
	Multiline   bool             `json:"multiline" yaml:"multiline"`
	ActionField string           `json:"action_field" yaml:"actionfield,omitempty"`
	Variant     string           `json:"variant" yaml:"variant,omitempty"`
	Required    bool             `json:"required" yaml:"required"`
	Schema      SchemaDefinition `json:"schema" yaml:"schema"`
}

type SchemaDefinition struct {
	Type string `json:"type"`
}

type WorkflowAppAction struct {
	Description    string                       `json:"description"`
	ID             string                       `json:"id" storm:"id" yaml:"id,omitempty"`
	Name           string                       `json:"name"`
	Label          string                       `json:"label"`
	NodeType       string                       `json:"node_type"`
	Environment    string                       `json:"environment"`
	Sharing        bool                         `json:"sharing"`
	PrivateID      string                       `json:"private_id"`
	AppID          string                       `json:"app_id"`
	Authentication []AuthenticationStore        `json:"authentication" yaml:"authentication,omitempty"`
	Tested         bool                         `json:"tested" yaml:"tested"`
	Parameters     []WorkflowAppActionParameter `json:"parameters"`
	Returns        struct {
		Description string           `json:"description" yaml:"description,omitempty"`
		ID          string           `json:"id" yaml:"id,omitempty"`
		Schema      SchemaDefinition `json:"schema" yaml:"schema"`
	} `json:"returns"`
}

// FIXME: Generate a callback authentication ID?
type WorkflowExecution struct {
	Type              string         `json:"type"`
	Status            string         `json:"status"`
	Start             string         `json:"start"`
	ExecutionArgument string         `json:"execution_argument"`
	ExecutionId       string         `json:"execution_id" storm:"id"`
	WorkflowId        string         `json:"workflow_id"`
	LastNode          string         `json:"last_node"`
	Authorization     string         `json:"authorization"`
	Result            string         `json:"result"`
	StartedAt         int64          `json:"started_at"`
	CompletedAt       int64          `json:"completed_at"`
	ProjectId         string         `json:"project_id"`
	Locations         []string       `json:"locations"`
	Workflow          Workflow       `json:"workflow"`
	Results           []ActionResult `json:"results"`
}

// Added environment for location to execute
type Action struct {
	AppName     string                       `json:"app_name"`
	AppVersion  string                       `json:"app_version"`
	AppID       string                       `json:"app_id"`
	Errors      []string                     `json:"errors"`
	ID          string                       `json:"id" storm:"id"`
	IsValid     bool                         `json:"is_valid"`
	IsStartNode bool                         `json:"isStartNode"`
	Sharing     bool                         `json:"sharing"`
	PrivateID   string                       `json:"private_id"`
	Label       string                       `json:"label"`
	SmallImage  string                       `json:"small_image" yaml:"small_image"`
	LargeImage  string                       `json:"large_image" yaml:"large_image"`
	Environment string                       `json:"environment"`
	Name        string                       `json:"name"`
	Parameters  []WorkflowAppActionParameter `json:"parameters"`
	Position    struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	} `json:"position"`
	Priority int `json:"priority"`
}

// Added environment for location to execute
type Trigger struct {
	AppName         string                       `json:"app_name"`
	Description     string                       `json:"description"`
	LongDescription string                       `json:"long_description"`
	Status          string                       `json:"status"`
	AppVersion      string                       `json:"app_version"`
	Errors          []string                     `json:"errors"`
	ID              string                       `json:"id" storm:"id"`
	IsValid         bool                         `json:"is_valid"`
	IsStartNode     bool                         `json:"isStartNode"`
	Label           string                       `json:"label"`
	SmallImage      string                       `json:"small_image" yaml:"small_image"`
	LargeImage      string                       `json:"large_image" yaml:"large_image"`
	Environment     string                       `json:"environment"`
	TriggerType     string                       `json:"trigger_type"`
	Name            string                       `json:"name"`
	Parameters      []WorkflowAppActionParameter `json:"parameters"`
	Position        struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	} `json:"position"`
	Priority int `json:"priority"`
}

type Branch struct {
	DestinationID string      `json:"destination_id"`
	ID            string      `json:"id" storm:"id"`
	SourceID      string      `json:"source_id"`
	Label         string      `json:"label"`
	HasError      bool        `json:"has_errors"`
	Conditions    []Condition `json:"conditions"`
}

// Same format for a lot of stuff
type Condition struct {
	Condition   WorkflowAppActionParameter `json:"condition"`
	Source      WorkflowAppActionParameter `json:"source"`
	Destination WorkflowAppActionParameter `json:"destination"`
}

type Schedule struct {
	Id                string `json:"id" storm:"id"`
	Name              string `json:"name"`
	Frequency         string `json:"frequency"`
	ExecutionArgument string `json:"execution_argument"`
}

type Workflow struct {
	Actions           []Action   `json:"actions"`
	Branches          []Branch   `json:"branches"`
	Triggers          []Trigger  `json:"triggers"`
	Schedules         []Schedule `json:"schedules"`
	Errors            []string   `json:"errors,omitempty"`
	Tags              []string   `json:"tags,omitempty"`
	ID                string     `json:"id" storm:"id"`
	IsValid           bool       `json:"is_valid"`
	Name              string     `json:"name"`
	Description       string     `json:"description"`
	Start             string     `json:"start"`
	Owner             string     `json:"owner"`
	Sharing           string     `json:"sharing"`
	Org               []Org      `json:"org,omitempty"`
	ExecutingOrg      Org        `json:"execution_org,omitempty"`
	WorkflowVariables []struct {
		Description string `json:"description"`
		ID          string `json:"id"`
		Name        string `json:"name"`
		Value       string `json:"value"`
	} `json:"workflow_variables"`
}

type ActionResult struct {
	Action        Action `json:"action"`
	ExecutionId   string `json:"execution_id"`
	Authorization string `json:"authorization"`
	Result        string `json:"result"`
	StartedAt     int64  `json:"started_at"`
	CompletedAt   int64  `json:"completed_at"`
	Status        string `json:"status"`
}

type Authentication struct {
	Required   bool                   `json:"required" yaml:"required" `
	Parameters []AuthenticationParams `json:"parameters" yaml:"parameters"`
}

type AuthenticationParams struct {
	Description string `json:"description" yaml:"description"`
	ID          string `json:"id" yaml:"id"`
	Name        string `json:"name" yaml:"name"`
	Example     string `json:"example" yaml:"example"`
	Value       string `json:"value,omitempty" yaml:"value"`
	Multiline   bool   `json:"multiline" yaml:"multiline"`
	Required    bool   `json:"required" yaml:"required"`
	In          string `json:"in" yaml:"in"`
	Scheme      string `json:"scheme" yaml:"scheme"`
}

type AuthenticationStore struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type ExecutionRequestWrapper struct {
	ID   string             `storm:"id"`
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
	Name        string `json:"name" yaml:"name"`
	Foldername  string `json:"foldername" yaml:"foldername"`
	Id          string `json:"id" yaml:"id"`
	Description string `json:"description" yaml:"description"`
	AppVersion  string `json:"app_version" yaml:"app_version"`
	ContactInfo struct {
		Name string `json:"name" yaml:"name"`
		Url  string `json:"url" yaml:"url"`
	} `json:"contact_info" yaml:"contact_info"`
	Types []string `json:"types" yaml:"types"`
	Input []struct {
		Name            string `json:"name" yaml:"name"`
		Description     string `json:"description" yaml:"description"`
		InputParameters []struct {
			Name        string `json:"name" yaml:"name"`
			Description string `json:"description" yaml:"description"`
			Required    string `json:"required" yaml:"required"`
			Schema      struct {
				Type string `json:"type" yaml:"type"`
			} `json:"schema" yaml:"schema"`
		} `json:"inputparameters" yaml:"inputparameters"`
		OutputParameters []struct {
			Name        string `json:"name" yaml:"name"`
			Description string `json:"description" yaml:"description"`
			Required    string `json:"required" yaml:"required"`
			Schema      struct {
				Type string `json:"type" yaml:"type"`
			} `json:"schema" yaml:"schema"`
		} `json:"outputparameters" yaml:"outputparameters"`
		Config []struct {
			Name        string `json:"name" yaml:"name"`
			Description string `json:"description" yaml:"description"`
			Required    string `json:"required" yaml:"required"`
			Schema      struct {
				Type string `json:"type" yaml:"type"`
			} `json:"schema" yaml:"schema"`
		} `json:"config" yaml:"config"`
	} `json:"input" yaml:"input"`
	Output []struct {
		Name        string `json:"name" yaml:"name"`
		Description string `json:"description" yaml:"description"`
		Config      []struct {
			Name        string `json:"name" yaml:"name"`
			Description string `json:"description" yaml:"description"`
			Required    string `json:"required" yaml:"required"`
			Schema      struct {
				Type string `json:"type" yaml:"type"`
			} `json:"schema" yaml:"schema"`
		} `json:"config" yaml:"config"`
		InputParameters []struct {
			Name        string `json:"name" yaml:"name"`
			Description string `json:"description" yaml:"description"`
			Required    string `json:"required" yaml:"required"`
			Schema      struct {
				Type string `json:"type" yaml:"type"`
			} `json:"schema" yaml:"schema"`
		} `json:"inputparameters" yaml:"inputparameters"`
		OutputParameters []struct {
			Name        string `json:"name" yaml:"name"`
			Description string `json:"description" yaml:"description"`
			Required    string `json:"required" yaml:"required"`
			Schema      struct {
				Type string `json:"type" yaml:"type"`
			} `json:"schema" yaml:"schema"`
		} `json:"outputparameters" yaml:"outputparameters"`
	} `json:"output" yaml:"output"`
}
