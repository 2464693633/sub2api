package creativestudio

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

func (s *Store) validateProject(project *Project, assets []Asset) error {
	if strings.TrimSpace(project.Name) == "" || len(project.Name) > 240 || len(project.Nodes) > s.opts.MaxNodes || len(project.Edges) > s.opts.MaxNodes*4 {
		return ErrInvalid
	}
	if math.IsNaN(project.Viewport.Zoom) || math.IsInf(project.Viewport.Zoom, 0) || project.Viewport.Zoom < .05 || project.Viewport.Zoom > 10 {
		return fmt.Errorf("%w: invalid viewport", ErrInvalid)
	}
	finite := func(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
	if !finite(project.Viewport.X) || !finite(project.Viewport.Y) {
		return fmt.Errorf("%w: invalid viewport", ErrInvalid)
	}
	if len(project.Timeline) > 128<<10 || (len(project.Timeline) > 0 && !json.Valid(project.Timeline)) {
		return ErrInvalid
	}
	assetKinds := make(map[string]string, len(assets))
	for _, asset := range assets {
		assetKinds[asset.ID] = asset.Kind
	}
	nodes := make(map[string]Node, len(project.Nodes))
	for i := range project.Nodes {
		node := &project.Nodes[i]
		if !finite(node.Position.X) || !finite(node.Position.Y) {
			return fmt.Errorf("%w: invalid card position", ErrInvalid)
		}
		if !safeID.MatchString(node.ID) || len(node.Data.Title) > 240 || len(node.Data.Prompt) > 12000 || len(node.Data.Params) > 16384 {
			return fmt.Errorf("%w: invalid card", ErrInvalid)
		}
		if _, exists := nodes[node.ID]; exists {
			return fmt.Errorf("%w: duplicate card", ErrInvalid)
		}
		if node.Type != "text" && node.Type != "image" && node.Type != "video" {
			return fmt.Errorf("%w: unsupported card type", ErrInvalid)
		}
		if node.Data.AssetID != "" && (assetKinds[node.Data.AssetID] != node.Type || node.Type == "text") {
			return fmt.Errorf("%w: asset does not belong to this project or card type", ErrInvalid)
		}
		if len(node.Data.Params) > 0 && !json.Valid(node.Data.Params) {
			return fmt.Errorf("%w: invalid generation parameters", ErrInvalid)
		}
		node.Data.Kind = node.Type
		nodes[node.ID] = *node
	}
	indegree := make(map[string]int, len(nodes))
	outgoing := make(map[string][]string)
	edges := make(map[string]bool)
	edgeIDs := make(map[string]bool)
	for _, edge := range project.Edges {
		source, sourceExists := nodes[edge.Source]
		target, targetExists := nodes[edge.Target]
		pair := edge.Source + ":" + edge.Target
		if edge.ID == "" || len(edge.ID) > 240 || edgeIDs[edge.ID] || len(edge.Label) > 240 || edges[pair] || !sourceExists || !targetExists || edge.Source == edge.Target || source.Type == "video" || target.Type == "text" {
			return fmt.Errorf("%w: invalid dependency connection", ErrInvalid)
		}
		edges[pair] = true
		edgeIDs[edge.ID] = true
		indegree[edge.Target]++
		outgoing[edge.Source] = append(outgoing[edge.Source], edge.Target)
	}
	ready := make([]string, 0)
	for id := range nodes {
		if indegree[id] == 0 {
			ready = append(ready, id)
		}
	}
	visited := 0
	for len(ready) > 0 {
		id := ready[len(ready)-1]
		ready = ready[:len(ready)-1]
		visited++
		for _, child := range outgoing[id] {
			indegree[child]--
			if indegree[child] == 0 {
				ready = append(ready, child)
			}
		}
	}
	if visited != len(nodes) {
		return fmt.Errorf("%w: dependency cycle", ErrInvalid)
	}
	if project.Nodes == nil {
		project.Nodes = []Node{}
	}
	if project.Edges == nil {
		project.Edges = []Edge{}
	}
	return nil
}

func fingerprint(v any) string {
	data, _ := json.Marshal(v)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
func validRequestID(id string) bool { return len(id) > 0 && len(id) <= 128 }
func checkParams(raw json.RawMessage) error {
	var params struct {
		Model   string `json:"model"`
		GroupID int64  `json:"group_id"`
	}
	if json.Unmarshal(raw, &params) != nil || strings.TrimSpace(params.Model) == "" || params.GroupID <= 0 {
		return fmt.Errorf("%w: choose a model and group before running", ErrInvalid)
	}
	return nil
}
func (s *Store) checkJobLimit(doc *document, count int) error {
	active := 0
	for _, job := range doc.Jobs {
		if activeStatus(job.Status) {
			active++
		}
	}
	if active+count > s.opts.MaxActiveJobs || len(doc.Jobs)+count > 5000 {
		return fmt.Errorf("%w: too many tasks in this project", ErrQuota)
	}
	return nil
}

func (s *Store) Run(userID int64, projectID, requestID string, nodeIDs []string) (Snapshot, error) {
	return s.run(userID, projectID, requestID, nodeIDs, 0)
}

// RunAtRevision executes the graph the caller actually saved. Replaying a known
// request is allowed after subsequent edits, but a new request needs an exact revision.
func (s *Store) RunAtRevision(userID int64, projectID, requestID string, nodeIDs []string, revision int64) (Snapshot, error) {
	if revision <= 0 {
		return Snapshot{}, ErrInvalid
	}
	return s.run(userID, projectID, requestID, nodeIDs, revision)
}

func (s *Store) run(userID int64, projectID, requestID string, nodeIDs []string, revision int64) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, original, err := s.lookup(userID, projectID)
	if err != nil {
		return Snapshot{}, err
	}
	if !validRequestID(requestID) || len(nodeIDs) == 0 || len(nodeIDs) > s.opts.MaxNodes {
		return Snapshot{}, ErrInvalid
	}
	ids := append([]string(nil), nodeIDs...)
	sort.Strings(ids)
	if revision == 0 {
		revision = original.Project.Revision
	}
	requestFingerprint := fingerprint(struct {
		Kind     string
		Nodes    []string
		Revision int64
	}{"run", ids, revision})
	if previous, exists := original.Requests[requestID]; exists {
		if previous.Fingerprint != requestFingerprint {
			return Snapshot{}, ErrConflict
		}
		return cloneDocument(original).Snapshot, nil
	}
	if revision != original.Project.Revision {
		return Snapshot{}, ErrConflict
	}
	doc := cloneDocument(original)
	if err := s.validateProject(&doc.Project, doc.Assets); err != nil {
		return Snapshot{}, err
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
			return Snapshot{}, fmt.Errorf("%w: select an image or video card", ErrInvalid)
		}
		selected[id] = true
	}
	runID := uid()
	planned := make(map[string]*Job)
	order := make([]string, 0)
	var plan func(string) (*Job, error)
	plan = func(id string) (*Job, error) {
		if job := planned[id]; job != nil {
			return job, nil
		}
		node := nodes[id]
		for _, existing := range doc.Jobs {
			if existing.NodeID == id && activeStatus(existing.Status) {
				return nil, fmt.Errorf("%w: card already has an active task", ErrBusy)
			}
		}
		if err := checkParams(node.Data.Params); err != nil {
			return nil, err
		}
		referenceCount := 0
		for _, sourceID := range incoming[id] {
			if nodes[sourceID].Type == "image" {
				referenceCount++
			}
		}
		if (node.Type == "video" && referenceCount > 1) || (node.Type == "image" && referenceCount > 8) {
			return nil, fmt.Errorf("%w: video accepts one reference image; images accept up to eight", ErrInvalid)
		}
		stamp := now()
		job := &Job{ID: uid(), RunID: runID, NodeID: id, Kind: node.Type, Status: "queued", Params: append(json.RawMessage(nil), node.Data.Params...), Inputs: []Input{}, CreatedAt: stamp, UpdatedAt: stamp}
		planned[id] = job
		prompts := make([]string, 0)
		for _, sourceID := range incoming[id] {
			source := nodes[sourceID]
			if source.Type == "text" {
				if prompt := strings.TrimSpace(source.Data.Prompt); prompt != "" {
					prompts = append(prompts, prompt)
				}
				continue
			}
			assetID := source.Data.AssetID
			if !selected[sourceID] && assetID == "" {
				// A closed browser may not yet have copied a completed job's asset
				// into its graph. Reuse that durable result instead of charging for
				// another implicit generation. Failed/unknown prior submissions
				// require an explicit retry or a selected reference version.
				var latest *Job
				for index := len(doc.Jobs) - 1; index >= 0; index-- {
					candidate := &doc.Jobs[index]
					if candidate.NodeID == sourceID && candidate.Kind == source.Type {
						latest = candidate
						break
					}
				}
				if latest != nil {
					switch latest.Status {
					case "succeeded":
						assetID = latest.AssetID
						if assetID == "" {
							return nil, fmt.Errorf("%w: previous input result is unavailable", ErrInvalid)
						}
					case "failed", "cancelled", "interrupted":
						return nil, fmt.Errorf("%w: 前置卡片 %q 有未完成的历史任务，请先重试或选择已生成的版本 / Resolve the input card's previous task before running this branch", ErrInvalid, source.Data.Title)
					default:
						return nil, fmt.Errorf("%w: an input card already has an active task", ErrBusy)
					}
				}
			}
			if !selected[sourceID] && assetID != "" {
				file, err := s.openAsset(key, doc, assetID)
				if err != nil {
					return nil, err
				}
				if file.Kind != "image" {
					return nil, fmt.Errorf("%w: generation input must be an image", ErrInvalid)
				}
				job.Inputs = append(job.Inputs, Input{AssetID: assetID})
			} else {
				parent, err := plan(sourceID)
				if err != nil {
					return nil, err
				}
				job.Inputs = append(job.Inputs, Input{JobID: parent.ID})
				job.Status = "waiting"
			}
		}
		if own := strings.TrimSpace(node.Data.Prompt); own != "" {
			prompts = append(prompts, own)
		}
		job.Prompt = strings.Join(prompts, "\n\n")
		if len(job.Prompt) == 0 || len(job.Prompt) > 24000 {
			return nil, fmt.Errorf("%w: a nonempty prompt of at most 24000 bytes is required", ErrInvalid)
		}
		order = append(order, id)
		return job, nil
	}
	for _, id := range nodeIDs {
		if _, err := plan(id); err != nil {
			return Snapshot{}, err
		}
	}
	if err := s.checkJobLimit(doc, len(planned)); err != nil {
		return Snapshot{}, err
	}
	for _, id := range order {
		doc.Jobs = append(doc.Jobs, *planned[id])
	}
	doc.Requests[requestID] = requestRecord{RunID: runID, Fingerprint: requestFingerprint}
	if err := s.commit(key, doc); err != nil {
		return Snapshot{}, err
	}
	s.notify()
	return cloneDocument(doc).Snapshot, nil
}

// Enqueue creates a standalone task such as a timeline export, with immutable asset inputs.
func (s *Store) Enqueue(userID int64, projectID, requestID, nodeID, kind, prompt string, params json.RawMessage, assetIDs []string) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, original, err := s.lookup(userID, projectID)
	if err != nil {
		return Snapshot{}, err
	}
	if !validRequestID(requestID) || !safeID.MatchString(nodeID) || kind != "compose" || len(params) > 128<<10 || !json.Valid(params) || len(assetIDs) == 0 || len(assetIDs) > 200 {
		return Snapshot{}, ErrInvalid
	}
	fp := fingerprint(struct {
		NodeID, Kind, Prompt string
		Params               json.RawMessage
		Assets               []string
	}{nodeID, kind, prompt, params, assetIDs})
	if previous, exists := original.Requests[requestID]; exists {
		if previous.Fingerprint != fp {
			return Snapshot{}, ErrConflict
		}
		return cloneDocument(original).Snapshot, nil
	}
	doc := cloneDocument(original)
	if err := s.checkJobLimit(doc, 1); err != nil {
		return Snapshot{}, err
	}
	stamp := now()
	job := Job{ID: uid(), RunID: uid(), NodeID: nodeID, Kind: kind, Status: "queued", Prompt: prompt, Params: append(json.RawMessage(nil), params...), Inputs: []Input{}, CreatedAt: stamp, UpdatedAt: stamp}
	seen := make(map[string]bool)
	for _, id := range assetIDs {
		if _, err := s.openAsset(key, doc, id); err != nil {
			return Snapshot{}, err
		}
		if !seen[id] {
			job.Inputs = append(job.Inputs, Input{AssetID: id})
			seen[id] = true
		}
	}
	doc.Jobs = append(doc.Jobs, job)
	doc.Requests[requestID] = requestRecord{RunID: job.RunID, Fingerprint: fp}
	if err := s.commit(key, doc); err != nil {
		return Snapshot{}, err
	}
	s.notify()
	return cloneDocument(doc).Snapshot, nil
}

func (s *Store) Retry(userID int64, projectID, jobID string) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, original, err := s.lookup(userID, projectID)
	if err != nil {
		return Snapshot{}, err
	}
	doc := cloneDocument(original)
	var target *Job
	for i := range doc.Jobs {
		if doc.Jobs[i].ID == jobID {
			target = &doc.Jobs[i]
			break
		}
	}
	if target == nil {
		return Snapshot{}, ErrNotFound
	}
	if target.Status != "failed" && target.Status != "cancelled" && target.Status != "interrupted" {
		return Snapshot{}, ErrBusy
	}
	if _, running := s.active[jobID]; running {
		return Snapshot{}, ErrBusy
	}
	for _, job := range doc.Jobs {
		if job.ID != target.ID && job.NodeID == target.NodeID && activeStatus(job.Status) {
			return Snapshot{}, ErrBusy
		}
	}
	if err := s.checkJobLimit(doc, 1); err != nil {
		return Snapshot{}, err
	}
	if target.UpstreamTerminal {
		target.RemoteID = ""
		target.UpstreamTerminal = false
	}
	target.Status, target.Error, target.BlockedBy = "queued", "", ""
	target.UpdatedAt = now()
	// Only resume tasks blocked by this branch. Independently cancelled tasks stay cancelled.
	resumed := map[string]bool{jobID: true}
	for changed := true; changed; {
		changed = false
		for i := range doc.Jobs {
			job := &doc.Jobs[i]
			if job.Status == "failed" && resumed[job.BlockedBy] {
				job.Status, job.Error, job.BlockedBy = "waiting", "", ""
				job.UpdatedAt = now()
				resumed[job.ID] = true
				changed = true
			}
		}
	}
	if err := s.checkJobLimit(doc, 0); err != nil {
		return Snapshot{}, err
	}
	if err := s.commit(key, doc); err != nil {
		return Snapshot{}, err
	}
	s.notify()
	return cloneDocument(doc).Snapshot, nil
}

func (s *Store) Cancel(userID int64, projectID, jobID string) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, original, err := s.lookup(userID, projectID)
	if err != nil {
		return Snapshot{}, err
	}
	doc := cloneDocument(original)
	found := false
	for i := range doc.Jobs {
		job := &doc.Jobs[i]
		if job.ID != jobID {
			continue
		}
		found = true
		if job.Status == "cancelled" {
			return cloneDocument(doc).Snapshot, nil
		}
		if !activeStatus(job.Status) {
			return Snapshot{}, ErrInvalid
		}
		job.Status = "cancelled"
		job.Error = ""
		job.UpdatedAt = now()
		break
	}
	if !found {
		return Snapshot{}, ErrNotFound
	}
	if err := s.commit(key, doc); err != nil {
		return Snapshot{}, err
	}
	if running, exists := s.active[jobID]; exists {
		running.cancel()
	}
	s.notify()
	return cloneDocument(doc).Snapshot, nil
}
