package controlplane

type Claim struct {
	Name       string  `json:"name"`
	Host       string  `json:"host"`
	ProjectDir string  `json:"project_dir"`
	SessionID  string  `json:"session_id"`
	Routes     []Route `json:"routes"`
}

type ClaimReceipt struct {
	Token      string `json:"token"`
	Generation int64  `json:"generation"`
}

type Publication struct {
	Name       string  `json:"name"`
	Host       string  `json:"host"`
	ProjectDir string  `json:"project_dir"`
	SessionID  string  `json:"session_id"`
	Routes     []Route `json:"routes"`
	ClaimToken string  `json:"claim_token"`
	Generation int64   `json:"generation"`
}

type Release struct {
	Name       string `json:"name"`
	Host       string `json:"host"`
	ProjectDir string `json:"project_dir"`
	SessionID  string `json:"session_id"`
	ClaimToken string `json:"claim_token"`
	Generation int64  `json:"generation"`
}

type Route struct {
	Path    string `json:"path"`
	Process string `json:"process"`
	Port    int    `json:"port"`
}

type ErrorResponse struct {
	Layer   string `json:"layer"`
	Code    string `json:"code"`
	Owner   string `json:"owner,omitempty"`
	Message string `json:"message,omitempty"`
}
