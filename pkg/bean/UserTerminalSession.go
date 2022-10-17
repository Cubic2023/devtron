package bean

type UserTerminalSessionRequest struct {
	Id        int
	UserId    int32
	ClusterId int
	NodeName  string
	BaseImage string
	ShellName string
}

type UserTerminalSessionConfig struct {
	MaxSessionPerUser               int `env:"MAX_SESSION_PER_USER" envDefault:"5"`
	TerminalPodStatusSyncTimeInSecs int `env:"TERMINAL_POD_STATUS_SYNC_In_SECS" envDefault:"5"`
}

type UserTerminalSessionResponse struct {
	UserTerminalSessionId int
	UserId                int32
	TerminalAccessId      int
	ShellName             string
	Status                TerminalPodStatus
}

const TerminalAccessPodNameTemplate = "terminal-access-" + TerminalAccessClusterIdTemplateVar + "-" + TerminalAccessUserIdTemplateVar + "-" + TerminalAccessRandomIdVar
const TerminalAccessClusterIdTemplateVar = "${cluster_id}"
const TerminalAccessUserIdTemplateVar = "${user_id}"
const TerminalAccessRandomIdVar = "${random_id}"
const TerminalAccessPodNameVar = "${pod_name}"
const TerminalAccessPodTemplateName = "terminal-access-pod-template"

type TerminalPodStatus string

const (
	TerminalPodStarting   TerminalPodStatus = "Starting"
	TerminalPodRunning    TerminalPodStatus = "Running"
	TerminalPodTerminated TerminalPodStatus = "Terminated"
	TerminalPodError      TerminalPodStatus = "Error"
)
