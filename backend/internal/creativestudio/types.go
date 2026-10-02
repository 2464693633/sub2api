// Package creativestudio persists creative projects, media and dependent jobs.
// Its executor uses the normal gateway so authorization and billing remain there.
package creativestudio

import (
	"context"
	"encoding/json"
	"errors"
)

var (
	ErrNotFound  = errors.New("creative studio resource not found")
	ErrConflict  = errors.New("creative studio revision conflict")
	ErrInvalid   = errors.New("invalid creative studio request")
	ErrQuota     = errors.New("creative studio quota exceeded")
	ErrBusy      = errors.New("creative studio has active jobs")
	ErrClosed    = errors.New("creative studio is closed")
	ErrUncertain = errors.New("upstream submission result is unknown")
)

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}
type Viewport struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Zoom float64 `json:"zoom"`
}
type NodeData struct {
	Kind     string          `json:"kind"`
	Title    string          `json:"title"`
	Prompt   string          `json:"prompt"`
	AssetID  string          `json:"assetId,omitempty"`
	ResultID string          `json:"resultId,omitempty"`
	Params   json.RawMessage `json:"params,omitempty"`
}
type Node struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Position Point    `json:"position"`
	Data     NodeData `json:"data"`
}
type Edge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Label  string `json:"label,omitempty"`
	Type   string `json:"type,omitempty"`
}
type Project struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Revision  int64           `json:"revision"`
	Nodes     []Node          `json:"nodes"`
	Edges     []Edge          `json:"edges"`
	Viewport  Viewport        `json:"viewport"`
	Timeline  json.RawMessage `json:"timeline,omitempty"`
	CreatedAt int64           `json:"created_at"`
	UpdatedAt int64           `json:"updated_at"`
}
type Asset struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	MIME      string `json:"mime"`
	Size      int64  `json:"size"`
	CreatedAt int64  `json:"created_at"`
	NodeID    string `json:"node_id,omitempty"`
	RunID     string `json:"run_id,omitempty"`
}
type Input struct {
	AssetID string `json:"asset_id,omitempty"`
	JobID   string `json:"job_id,omitempty"`
}
type Job struct {
	ID               string          `json:"id"`
	RunID            string          `json:"run_id"`
	NodeID           string          `json:"node_id"`
	Kind             string          `json:"kind"`
	Status           string          `json:"status"`
	Prompt           string          `json:"prompt"`
	Params           json.RawMessage `json:"params"`
	Inputs           []Input         `json:"inputs,omitempty"`
	RemoteID         string          `json:"remote_id,omitempty"`
	UpstreamTerminal bool            `json:"upstream_terminal,omitempty"`
	AssetID          string          `json:"asset_id,omitempty"`
	Error            string          `json:"error,omitempty"`
	BlockedBy        string          `json:"blocked_by,omitempty"`
	CreatedAt        int64           `json:"created_at"`
	UpdatedAt        int64           `json:"updated_at"`
	UserID           int64           `json:"-"`
	ProjectID        string          `json:"-"`
}
type Snapshot struct {
	Project Project `json:"project"`
	Assets  []Asset `json:"assets"`
	Jobs    []Job   `json:"jobs"`
}
type AssetFile struct {
	Asset
	Path string
}
type Result struct {
	Data []byte
	MIME string
	Name string
}
type Executor interface {
	Execute(context.Context, Job, []AssetFile, func(remoteID string, terminal bool) error) (Result, error)
}
type Options struct {
	MaxWorkers     int
	PerUserWorkers int
	MaxUserBytes   int64
	MaxAssetBytes  int64
	MaxNodes       int
	MaxActiveJobs  int
}
