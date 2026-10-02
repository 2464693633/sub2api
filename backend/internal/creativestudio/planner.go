package creativestudio

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type PreflightInput struct {
	NodeID  string `json:"node_id"`
	Kind    string `json:"kind"`
	AssetID string `json:"asset_id,omitempty"`
}
type PreflightNode struct {
	NodeID string           `json:"node_id"`
	Title  string           `json:"title"`
	Kind   string           `json:"kind"`
	Prompt string           `json:"prompt"`
	Params json.RawMessage  `json:"params"`
	Inputs []PreflightInput `json:"inputs"`
}
type PreflightError struct {
	NodeID  string `json:"node_id,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}
type PreflightCounts struct {
	Image int `json:"image"`
	Video int `json:"video"`
	Total int `json:"total"`
}
type Preflight struct {
	Revision int64            `json:"revision"`
	Valid    bool             `json:"valid"`
	Counts   PreflightCounts  `json:"counts"`
	Nodes    []PreflightNode  `json:"nodes"`
	Errors   []PreflightError `json:"errors"`
}

func validOptionalParams(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}
	var value map[string]json.RawMessage
	return len(raw) <= 16384 && json.Unmarshal(raw, &value) == nil && value != nil
}

func effectiveParams(project Project, node Node) json.RawMessage {
	if node.Data.ParamsMode == "project" {
		if project.Settings == nil {
			return nil
		}
		if node.Type == "image" {
			return project.Settings.ImageParams
		}
		if node.Type == "video" {
			return project.Settings.VideoParams
		}
	}
	return node.Data.Params
}

// Preflight uses exactly the execution planner without creating jobs, request IDs,
// files or upstream calls. A stale graph revision remains an HTTP conflict.
func (s *Store) Preflight(userID int64, projectID string, nodeIDs []string, revision int64) (Preflight, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, original, err := s.lookup(userID, projectID)
	if err != nil {
		return Preflight{}, err
	}
	if revision <= 0 {
		return Preflight{}, ErrInvalid
	}
	if revision != original.Project.Revision {
		return Preflight{}, ErrConflict
	}
	report, _, err := s.plan(key, cloneDocument(original), nodeIDs)
	if err != nil && len(report.Errors) == 0 {
		return Preflight{}, err
	}
	return report, nil
}

func (s *Store) plan(key projectKey, doc *document, nodeIDs []string) (Preflight, []Job, error) {
	report := Preflight{Revision: doc.Project.Revision, Valid: true, Nodes: []PreflightNode{}, Errors: []PreflightError{}}
	var firstErr error
	addError := func(id string, err error) {
		if firstErr == nil {
			firstErr = err
		}
		report.Valid = false
		code := "invalid"
		if errors.Is(err, ErrBusy) {
			code = "busy"
		}
		if errors.Is(err, ErrQuota) {
			code = "quota"
		}
		if errors.Is(err, ErrNotFound) {
			code = "missing_asset"
		}
		for _, previous := range report.Errors {
			if previous.NodeID == id && previous.Message == err.Error() {
				return
			}
		}
		report.Errors = append(report.Errors, PreflightError{NodeID: id, Code: code, Message: err.Error()})
	}
	if len(nodeIDs) == 0 || len(nodeIDs) > s.opts.MaxNodes {
		addError("", fmt.Errorf("%w: select at least one image or video card", ErrInvalid))
		return report, nil, firstErr
	}
	if err := s.validateProject(&doc.Project, doc.Assets); err != nil {
		addError("", err)
		return report, nil, firstErr
	}
	nodes := make(map[string]Node)
	incoming := make(map[string][]string)
	selected := make(map[string]bool)
	for _, node := range doc.Project.Nodes {
		nodes[node.ID] = node
	}
	for _, edge := range doc.Project.Edges {
		incoming[edge.Target] = append(incoming[edge.Target], edge.Source)
	}
	for _, id := range nodeIDs {
		node, exists := nodes[id]
		if !exists || node.Type == "text" {
			addError(id, fmt.Errorf("%w: select an image or video card", ErrInvalid))
			continue
		}
		selected[id] = true
	}
	planned := make(map[string]*Job)
	order := make([]string, 0)
	var plan func(string) *Job
	plan = func(id string) *Job {
		if job := planned[id]; job != nil {
			return job
		}
		node := nodes[id]
		params := effectiveParams(doc.Project, node)
		if len(params) == 0 {
			params = json.RawMessage(`{}`)
		}
		job := &Job{ID: uid(), NodeID: id, Kind: node.Type, Status: "queued", Params: append(json.RawMessage(nil), params...), Inputs: []Input{}, CreatedAt: now(), UpdatedAt: now()}
		planned[id] = job
		preview := PreflightNode{NodeID: id, Title: node.Data.Title, Kind: node.Type, Params: append(json.RawMessage(nil), params...), Inputs: []PreflightInput{}}
		for _, existing := range doc.Jobs {
			if existing.NodeID == id && activeStatus(existing.Status) {
				addError(id, fmt.Errorf("%w: card already has an active task", ErrBusy))
				break
			}
		}
		references := 0
		for _, sourceID := range incoming[id] {
			if nodes[sourceID].Type == "image" {
				references++
			}
		}
		if err := ValidateGenerationParams(node.Type, params, references); err != nil {
			addError(id, err)
		}
		prompts := make([]string, 0)
		if settings := doc.Project.Settings; settings != nil {
			if subject := strings.TrimSpace(settings.Subject); subject != "" {
				prompts = append(prompts, "主体 / Subject: "+subject)
			}
			if style := strings.TrimSpace(settings.Style); style != "" {
				prompts = append(prompts, "风格 / Style: "+style)
			}
		}
		for _, sourceID := range incoming[id] {
			source := nodes[sourceID]
			if source.Type == "text" {
				if prompt := strings.TrimSpace(source.Data.Prompt); prompt != "" {
					prompts = append(prompts, prompt)
				}
				preview.Inputs = append(preview.Inputs, PreflightInput{NodeID: sourceID, Kind: "text"})
				continue
			}
			assetID := source.Data.AssetID
			if !selected[sourceID] && assetID == "" {
				var latest *Job
				for i := len(doc.Jobs) - 1; i >= 0; i-- {
					candidate := &doc.Jobs[i]
					if candidate.NodeID == sourceID && candidate.Kind == source.Type {
						latest = candidate
						break
					}
				}
				if latest != nil {
					if latest.Status == "succeeded" && latest.AssetID != "" {
						assetID = latest.AssetID
					} else {
						problem := fmt.Errorf("%w: 前置卡片 %q 有未完成的历史任务，请先重试或选择已生成的版本 / Resolve the input card's previous task before running this branch", ErrInvalid, source.Data.Title)
						if activeStatus(latest.Status) {
							problem = fmt.Errorf("%w: an input card already has an active task", ErrBusy)
						}
						addError(id, problem)
						preview.Inputs = append(preview.Inputs, PreflightInput{NodeID: sourceID, Kind: "job"})
						continue
					}
				}
			}
			if !selected[sourceID] && assetID != "" {
				file, err := s.openAsset(key, doc, assetID)
				if err != nil {
					addError(id, err)
				} else if file.Kind != "image" {
					addError(id, fmt.Errorf("%w: generation input must be an image", ErrInvalid))
				}
				job.Inputs = append(job.Inputs, Input{AssetID: assetID})
				preview.Inputs = append(preview.Inputs, PreflightInput{NodeID: sourceID, Kind: "asset", AssetID: assetID})
			} else {
				parent := plan(sourceID)
				job.Inputs = append(job.Inputs, Input{JobID: parent.ID})
				job.Status = "waiting"
				preview.Inputs = append(preview.Inputs, PreflightInput{NodeID: sourceID, Kind: "job"})
			}
		}
		if own := strings.TrimSpace(node.Data.Prompt); own != "" {
			prompts = append(prompts, own)
		}
		job.Prompt = strings.Join(prompts, "\n\n")
		preview.Prompt = job.Prompt
		if len(job.Prompt) == 0 || len(job.Prompt) > 24000 {
			addError(id, fmt.Errorf("%w: a nonempty prompt of at most 24000 bytes is required", ErrInvalid))
		}
		order = append(order, id)
		report.Nodes = append(report.Nodes, preview)
		if node.Type == "image" {
			report.Counts.Image++
		} else {
			report.Counts.Video++
		}
		report.Counts.Total++
		return job
	}
	for _, id := range nodeIDs {
		if selected[id] {
			plan(id)
		}
	}
	if err := s.checkJobLimit(doc, len(planned)); err != nil {
		addError("", err)
	}
	jobs := make([]Job, 0, len(order))
	for _, id := range order {
		jobs = append(jobs, *planned[id])
	}
	return report, jobs, firstErr
}
