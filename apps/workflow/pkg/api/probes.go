// Package api defines the JSON contracts shared by the Workflow service and client.
// The authenticated project contracts will be introduced later in S01a.
package api

const (
	ServiceName  = "label-enrollment-workflow"
	StatusOK     = "ok"
	StatusReady  = "ready"
	CodeNotReady = "NOT_READY"
)

type ProbeResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

type ErrorResponse struct {
	Error  ErrorDetail      `json:"error"`
	Checks *ReadinessChecks `json:"checks,omitempty"`
}

type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
