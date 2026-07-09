package protocol

import "encoding/json"

type MsgType string

const (
	MsgAgentAuth    MsgType = "agent_auth"
	MsgOperatorAuth MsgType = "operator_auth"
	MsgAuthOK       MsgType = "auth_ok"
	MsgAuthFail     MsgType = "auth_fail"

	MsgAgentList   MsgType = "agent_list"
	MsgSelectAgent MsgType = "select_agent"

	MsgShellStart  MsgType = "shell_start"
	MsgShellInput  MsgType = "shell_input"
	MsgShellOutput MsgType = "shell_output"
	MsgShellResize MsgType = "shell_resize"

	MsgScreenshotReq  MsgType = "screenshot_req"
	MsgScreenshotData MsgType = "screenshot_data"

	MsgCameraStart MsgType = "camera_start"
	MsgCameraStop  MsgType = "camera_stop"
	MsgCameraFrame MsgType = "camera_frame"

	MsgPing  MsgType = "ping"
	MsgPong  MsgType = "pong"
	MsgError MsgType = "error"
)

type Message struct {
	Type MsgType         `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

type AgentInfo struct {
	ID       string `json:"id"`
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
}

type AgentAuthMsg struct {
	Token    string `json:"token"`
	AgentID  string `json:"agent_id"`
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
}

type OperatorAuthMsg struct {
	Password string `json:"password"`
}

type SelectAgentMsg struct {
	AgentID string `json:"agent_id"`
}

type ShellInputMsg struct {
	Data string `json:"data"`
}

type ShellOutputMsg struct {
	Data string `json:"data"`
}

type ShellResizeMsg struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

type ScreenshotDataMsg struct {
	Image string `json:"image"` // base64 PNG
}

type CameraFrameMsg struct {
	Image string `json:"image"` // base64 JPEG
}

type ErrorMsg struct {
	Message string `json:"message"`
}
