package creativestudio

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"
)

// Start is separate from New so the gateway executor can be installed after all routes exist.
func (s *Store) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started || s.closed {
		return
	}
	s.started = true
	s.wg.Add(1)
	go s.schedule()
}

func (s *Store) schedule() {
	defer s.wg.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		for s.dispatch() {
		}
		select {
		case <-s.ctx.Done():
			return
		case <-s.wake:
		case <-ticker.C:
		}
	}
}

// resolveInputs never reads a changing project graph; all input identities were frozen by Run.
func (s *Store) resolveInputs(key projectKey, doc *document, job Job) ([]AssetFile, bool, string, error) {
	files := make([]AssetFile, 0, len(job.Inputs))
	for _, input := range job.Inputs {
		assetID := input.AssetID
		if input.JobID != "" {
			var parent *Job
			for i := range doc.Jobs {
				if doc.Jobs[i].ID == input.JobID {
					parent = &doc.Jobs[i]
					break
				}
			}
			if parent == nil {
				return nil, false, input.JobID, fmt.Errorf("dependency task no longer exists")
			}
			if parent.Status == "failed" || parent.Status == "cancelled" || parent.Status == "interrupted" {
				return nil, false, parent.ID, fmt.Errorf("前一步未完成，请先重试前一步。 / An input task did not complete. Retry that task first")
			}
			if parent.Status != "succeeded" {
				return nil, false, "", nil
			}
			assetID = parent.AssetID
		}
		file, err := s.openAsset(key, doc, assetID)
		if err != nil {
			return nil, false, "", fmt.Errorf("input media unavailable: %w", err)
		}
		files = append(files, file)
	}
	return files, true, "", nil
}

func (s *Store) dispatch() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || len(s.active) >= s.opts.MaxWorkers {
		return false
	}
	userCounts := make(map[int64]int)
	for _, run := range s.active {
		userCounts[run.key.user]++
	}
	type candidate struct {
		key    projectKey
		job    Job
		inputs []AssetFile
	}
	candidates := make([]candidate, 0)
	for key, original := range s.documents {
		if userCounts[key.user] >= s.opts.PerUserWorkers {
			continue
		}
		doc := cloneDocument(original)
		changed := false
		for i := range doc.Jobs {
			job := &doc.Jobs[i]
			if job.Status != "queued" && job.Status != "waiting" {
				continue
			}
			_, ready, blockedBy, err := s.resolveInputs(key, doc, *job)
			if err != nil {
				job.Status, job.Error, job.BlockedBy = "failed", err.Error(), blockedBy
				job.UpdatedAt = now()
				changed = true
			} else if ready && job.Status == "waiting" {
				job.Status = "queued"
				job.UpdatedAt = now()
				changed = true
			}
		}
		if changed {
			if err := s.commit(key, doc); err != nil {
				continue
			}
		}
		for _, job := range doc.Jobs {
			if job.Status != "queued" {
				continue
			}
			inputs, ready, _, err := s.resolveInputs(key, doc, job)
			if err == nil && ready {
				candidates = append(candidates, candidate{key, job, inputs})
			}
		}
	}
	if len(candidates) == 0 {
		return false
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].job.CreatedAt == candidates[j].job.CreatedAt {
			return candidates[i].job.ID < candidates[j].job.ID
		}
		return candidates[i].job.CreatedAt < candidates[j].job.CreatedAt
	})
	selected := candidates[0]
	doc := cloneDocument(s.documents[selected.key])
	for i := range doc.Jobs {
		if doc.Jobs[i].ID == selected.job.ID {
			doc.Jobs[i].Status = "running"
			doc.Jobs[i].Error = ""
			doc.Jobs[i].UpdatedAt = now()
			selected.job = doc.Jobs[i]
			break
		}
	}
	if err := s.commit(selected.key, doc); err != nil {
		return false
	}
	ctx, cancel := context.WithCancel(s.ctx)
	s.active[selected.job.ID] = execution{key: selected.key, jobID: selected.job.ID, cancel: cancel}
	selected.job.UserID = selected.key.user
	selected.job.ProjectID = selected.key.project
	s.wg.Add(1)
	go s.execute(ctx, selected.key, selected.job, selected.inputs)
	return true
}

func (s *Store) execute(ctx context.Context, key projectKey, job Job, inputs []AssetFile) {
	defer s.wg.Done()
	var result Result
	var executionErr error
	func() {
		defer func() {
			if recover() != nil {
				executionErr = fmt.Errorf("executor failed unexpectedly; verify the upstream task before retrying")
			}
		}()
		result, executionErr = s.executor.Execute(ctx, job, inputs, func(remoteID string, terminal bool) error {
			return s.recordProgress(key, job.ID, remoteID, terminal)
		})
	}()
	s.mu.Lock()
	defer s.mu.Unlock()
	if run, exists := s.active[job.ID]; exists {
		run.cancel()
		delete(s.active, job.ID)
	}
	defer s.notify()
	// Shutdown leaves running tasks persisted. New() safely chooses polling or explicit retry.
	if s.closed {
		return
	}
	original, exists := s.documents[key]
	if !exists {
		return
	}
	doc := cloneDocument(original)
	var target *Job
	for i := range doc.Jobs {
		if doc.Jobs[i].ID == job.ID {
			target = &doc.Jobs[i]
			break
		}
	}
	if target == nil || target.Status != "running" {
		return
	}
	var savedAssetID string
	resultReceived := executionErr == nil
	if executionErr == nil {
		asset, err := s.putAsset(key, doc, result.Name, result.MIME, result.Data, job.NodeID, job.RunID)
		if err != nil {
			executionErr = err
		} else {
			expectedKind := job.Kind
			if expectedKind == "compose" {
				expectedKind = "video"
			}
			if asset.Kind != expectedKind {
				_ = os.Remove(s.assetPath(key, asset.ID))
				doc.Assets = doc.Assets[:len(doc.Assets)-1]
				executionErr = fmt.Errorf("upstream returned an unexpected media type")
			} else {
				savedAssetID = asset.ID
				target.AssetID = asset.ID
				target.Status = "succeeded"
				target.Error = ""
			}
		}
	}
	if executionErr != nil {
		if resultReceived && target.Kind == "image" {
			executionErr = fmt.Errorf("%w: 图片已返回但未能保存，请检查用量和服务器存储后决定是否重新生成 / The image was returned but could not be saved; check usage and storage before generating again: %v", ErrUncertain, executionErr)
		}
		target.Status = "failed"
		if errors.Is(executionErr, ErrUncertain) || (errors.Is(executionErr, context.DeadlineExceeded) && target.RemoteID == "" && target.Kind != "compose") {
			target.Status = "interrupted"
		}
		target.Error = executionErr.Error()
		if len(target.Error) > 2000 {
			target.Error = target.Error[:2000]
		}
	}
	target.UpdatedAt = now()
	if err := s.commit(key, doc); err != nil {
		if savedAssetID != "" {
			_ = os.Remove(s.assetPath(key, savedAssetID))
		}
		// Saving media can push the final manifest over quota. Persist a result
		// without the new asset when possible, so video retries retain their
		// remote ID and local exports do not look like uncertain paid requests.
		fallback := cloneDocument(original)
		for i := range fallback.Jobs {
			if fallback.Jobs[i].ID == job.ID {
				failed := &fallback.Jobs[i]
				failed.Status = "failed"
				if failed.Kind == "image" || (failed.Kind == "video" && failed.RemoteID == "") {
					failed.Status = "interrupted"
				}
				failed.Error = "无法保存任务结果，请检查服务器存储。 / Could not save the task result. Check server storage."
				failed.UpdatedAt = now()
			}
		}
		if err := s.commit(key, fallback); err != nil {
			// Disk may be unavailable even for the failure record. Prevent another
			// automatic POST in memory; persisted running state stays recoverable.
			s.documents[key] = fallback
		}
	}
}

func (s *Store) recordProgress(key projectKey, jobID, remoteID string, terminal bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		// Close still holds the process lock while joining executors. A video
		// may receive its remote ID just after cancellation; keep that ID so a
		// restart can poll it instead of losing track of a paid submission.
		running, exists := s.active[jobID]
		if !exists || running.key != key {
			return ErrClosed
		}
	}
	if len(remoteID) > 1000 {
		return ErrInvalid
	}
	original, exists := s.documents[key]
	if !exists {
		return ErrNotFound
	}
	doc := cloneDocument(original)
	for i := range doc.Jobs {
		job := &doc.Jobs[i]
		if job.ID != jobID {
			continue
		}
		if job.Status != "running" {
			return context.Canceled
		}
		if remoteID != "" {
			job.RemoteID = remoteID
		}
		job.UpstreamTerminal = terminal
		job.UpdatedAt = now()
		return s.commit(key, doc)
	}
	return ErrNotFound
}
