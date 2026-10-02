package creativestudio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type executorFunc func(context.Context, Job, []AssetFile, func(string, bool) error) (Result, error)

func (f executorFunc) Execute(ctx context.Context, job Job, files []AssetFile, progress func(string, bool) error) (Result, error) {
	return f(ctx, job, files, progress)
}
func pngData(t *testing.T) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := png.Encode(&out, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
func testStore(t *testing.T, executor Executor, options Options) *Store {
	t.Helper()
	s, err := New(t.TempDir(), executor, options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}
func card(id, kind string) Node {
	return Node{ID: id, Type: kind, Data: NodeData{Kind: kind, Title: id, Prompt: "Prompt " + id, Params: json.RawMessage(`{"model":"example","group_id":1}`)}}
}
func saveGraph(t *testing.T, s *Store, user int64, nodes []Node, edges []Edge) Snapshot {
	t.Helper()
	snapshot, err := s.Create(user, "Project")
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Project.Nodes = nodes
	snapshot.Project.Edges = edges
	snapshot, err = s.Save(user, snapshot.Project.ID, snapshot.Project)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
func waitSnapshot(t *testing.T, s *Store, user int64, project string, matches func(Snapshot) bool) Snapshot {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		snapshot, err := s.Get(user, project)
		if err != nil {
			t.Fatal(err)
		}
		if matches(snapshot) {
			return snapshot
		}
		time.Sleep(5 * time.Millisecond)
	}
	snapshot, _ := s.Get(user, project)
	t.Fatalf("task state did not settle: %+v", snapshot.Jobs)
	return Snapshot{}
}
func noActive(snapshot Snapshot) bool {
	for _, job := range snapshot.Jobs {
		if activeStatus(job.Status) {
			return false
		}
	}
	return true
}
func unusedExecutor() Executor {
	return executorFunc(func(context.Context, Job, []AssetFile, func(string, bool) error) (Result, error) {
		return Result{}, errors.New("not expected")
	})
}

func TestOwnerIsolationAndRevisionCAS(t *testing.T) {
	s := testStore(t, unusedExecutor(), Options{})
	first := saveGraph(t, s, 7, []Node{card("image", "image")}, nil)
	projectID := first.Project.ID
	asset, err := s.PutAsset(7, projectID, "test.png", "image/png", pngData(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get(8, projectID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user get: %v", err)
	}
	if _, err = s.OpenAsset(8, projectID, asset.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user asset: %v", err)
	}
	if _, err = s.OpenAsset(7, projectID, "../../project.json"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("path traversal: %v", err)
	}
	if projects, err := s.List(8); err != nil || len(projects) != 0 {
		t.Fatalf("unexpected projects: %+v %v", projects, err)
	}
	updated := first.Project
	updated.Name = "Changed"
	if _, err = s.Save(7, projectID, updated); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Save(7, projectID, first.Project); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale revision accepted: %v", err)
	}
	first.Project.Nodes[0].Data.Prompt = "changed outside storage"
	current, _ := s.Get(7, projectID)
	if current.Project.Nodes[0].Data.Prompt != "Prompt image" {
		t.Fatal("returned snapshot mutated stored state")
	}
	other := saveGraph(t, s, 7, []Node{card("other", "image")}, nil)
	other.Project.Nodes[0].Data.AssetID = asset.ID
	if _, err := s.Save(7, other.Project.ID, other.Project); !errors.Is(err, ErrInvalid) {
		t.Fatalf("cross-project asset accepted: %v", err)
	}
}

func TestGraphRejectsCyclesAndUnsupportedConnections(t *testing.T) {
	s := testStore(t, unusedExecutor(), Options{})
	initial := saveGraph(t, s, 1, []Node{card("a", "image"), card("b", "image"), card("v", "video"), card("t", "text")}, nil)
	for name, edges := range map[string][]Edge{
		"cycle":        {{ID: "ab", Source: "a", Target: "b"}, {ID: "ba", Source: "b", Target: "a"}},
		"self":         {{ID: "aa", Source: "a", Target: "a"}},
		"video source": {{ID: "va", Source: "v", Target: "a"}},
		"text target":  {{ID: "at", Source: "a", Target: "t"}},
		"duplicate":    {{ID: "ab", Source: "a", Target: "b"}, {ID: "ab2", Source: "a", Target: "b"}},
	} {
		t.Run(name, func(t *testing.T) {
			project := initial.Project
			project.Edges = edges
			if _, err := s.Save(1, project.ID, project); !errors.Is(err, ErrInvalid) {
				t.Fatalf("invalid graph accepted: %v", err)
			}
		})
	}
}

func TestRunIdempotencyAndFrozenInputs(t *testing.T) {
	data := pngData(t)
	seen := make(chan Job, 1)
	s := testStore(t, executorFunc(func(_ context.Context, job Job, _ []AssetFile, _ func(string, bool) error) (Result, error) {
		seen <- job
		return Result{Data: data, MIME: "image/png"}, nil
	}), Options{})
	initial := saveGraph(t, s, 1, []Node{card("image", "image")}, nil)
	id := initial.Project.ID
	first, err := s.Run(1, id, "request-1", []string{"image"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Run(1, id, "request-1", []string{"image"})
	if err != nil || len(second.Jobs) != 1 || second.Jobs[0].ID != first.Jobs[0].ID {
		t.Fatalf("duplicate request created another task: %+v %v", second.Jobs, err)
	}
	initial.Project.Nodes[0].Data.Prompt = "New prompt"
	if _, err := s.Save(1, id, initial.Project); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Run(1, id, "request-1", []string{"image"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("request reused with different graph: %v", err)
	}
	s.Start()
	done := waitSnapshot(t, s, 1, id, noActive)
	if done.Jobs[0].Status != "succeeded" {
		t.Fatalf("failed job: %+v", done.Jobs)
	}
	job := <-seen
	if job.Prompt != "Prompt image" || job.UserID != 1 || job.ProjectID != id {
		t.Fatalf("mutable or missing executor context: %+v", job)
	}
	if done.Project.Revision != initial.Project.Revision+1 {
		t.Fatal("background completion changed graph revision")
	}
}

func TestRunAtRevisionReplaysLostResponseAfterLaterEdits(t *testing.T) {
	s := testStore(t, unusedExecutor(), Options{})
	initial := saveGraph(t, s, 1, []Node{card("image", "image"), card("other", "image")}, nil)
	id, revision := initial.Project.ID, initial.Project.Revision
	first, err := s.RunAtRevision(1, id, "request", []string{"image"}, revision)
	if err != nil {
		t.Fatal(err)
	}
	initial.Project.Name = "Another tab edited the project"
	if _, err := s.Save(1, id, initial.Project); err != nil {
		t.Fatal(err)
	}
	replay, err := s.RunAtRevision(1, id, "request", []string{"image"}, revision)
	if err != nil || len(replay.Jobs) != 1 || replay.Jobs[0].ID != first.Jobs[0].ID {
		t.Fatalf("lost response could not be replayed: %+v %v", replay.Jobs, err)
	}
	if _, err := s.RunAtRevision(1, id, "request", []string{"other"}, revision); !errors.Is(err, ErrConflict) {
		t.Fatalf("same ID accepted different selection: %v", err)
	}
	if _, err := s.RunAtRevision(1, id, "new-request", []string{"other"}, revision); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale unseen graph executed: %v", err)
	}
}

func TestDependentTasksResumeAfterParentRetry(t *testing.T) {
	data := pngData(t)
	var attempts atomic.Int32
	s := testStore(t, executorFunc(func(_ context.Context, job Job, inputs []AssetFile, _ func(string, bool) error) (Result, error) {
		if job.NodeID == "overview" && attempts.Add(1) == 1 {
			return Result{}, errors.New("temporary image failure")
		}
		if job.NodeID == "detail" {
			if len(inputs) != 1 || inputs[0].NodeID != "overview" {
				return Result{}, fmt.Errorf("wrong input: %+v", inputs)
			}
			if _, err := os.ReadFile(inputs[0].Path); err != nil {
				return Result{}, err
			}
		}
		return Result{Data: data, MIME: "image/png"}, nil
	}), Options{})
	initial := saveGraph(t, s, 1, []Node{card("overview", "image"), card("detail", "image")}, []Edge{{ID: "ref", Source: "overview", Target: "detail"}})
	s.Start()
	if _, err := s.Run(1, initial.Project.ID, "req", []string{"detail"}); err != nil {
		t.Fatal(err)
	}
	failed := waitSnapshot(t, s, 1, initial.Project.ID, noActive)
	if len(failed.Jobs) != 2 || failed.Jobs[0].Status != "failed" || failed.Jobs[1].BlockedBy != failed.Jobs[0].ID {
		t.Fatalf("incorrect dependency failure: %+v", failed.Jobs)
	}
	if _, err := s.Retry(1, initial.Project.ID, failed.Jobs[0].ID); err != nil {
		t.Fatal(err)
	}
	done := waitSnapshot(t, s, 1, initial.Project.ID, noActive)
	for _, job := range done.Jobs {
		if job.Status != "succeeded" {
			t.Fatalf("dependency did not resume: %+v", done.Jobs)
		}
	}
	if attempts.Load() != 2 {
		t.Fatalf("unexpected overview attempts: %d", attempts.Load())
	}
}

func TestRetryDoesNotRegenerateSuccessfulSibling(t *testing.T) {
	data := pngData(t)
	var mu sync.Mutex
	counts := make(map[string]int)
	s := testStore(t, executorFunc(func(_ context.Context, job Job, _ []AssetFile, _ func(string, bool) error) (Result, error) {
		mu.Lock()
		counts[job.NodeID]++
		count := counts[job.NodeID]
		mu.Unlock()
		if job.NodeID == "b" && count == 1 {
			return Result{}, errors.New("retry this branch")
		}
		return Result{Data: data, MIME: "image/png"}, nil
	}), Options{})
	initial := saveGraph(t, s, 1, []Node{card("a", "image"), card("b", "image"), card("c", "image")}, []Edge{{ID: "ab", Source: "a", Target: "b"}, {ID: "ac", Source: "a", Target: "c"}})
	s.Start()
	if _, err := s.Run(1, initial.Project.ID, "req", []string{"a", "b", "c"}); err != nil {
		t.Fatal(err)
	}
	first := waitSnapshot(t, s, 1, initial.Project.ID, noActive)
	var failedID string
	for _, job := range first.Jobs {
		if job.Status == "failed" {
			failedID = job.ID
		}
	}
	if _, err := s.Retry(1, initial.Project.ID, failedID); err != nil {
		t.Fatal(err)
	}
	done := waitSnapshot(t, s, 1, initial.Project.ID, noActive)
	for _, job := range done.Jobs {
		if job.Status != "succeeded" {
			t.Fatalf("unexpected state: %+v", done.Jobs)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if counts["a"] != 1 || counts["b"] != 2 || counts["c"] != 1 {
		t.Fatalf("unrelated task regenerated: %+v", counts)
	}
}

func TestVideoRetryDistinguishesTerminalFailure(t *testing.T) {
	for _, terminal := range []bool{false, true} {
		t.Run(fmt.Sprint(terminal), func(t *testing.T) {
			var attempts atomic.Int32
			seen := make(chan string, 1)
			s := testStore(t, executorFunc(func(_ context.Context, job Job, _ []AssetFile, progress func(string, bool) error) (Result, error) {
				if attempts.Add(1) == 1 {
					if err := progress("remote-123", terminal); err != nil {
						return Result{}, err
					}
					return Result{}, errors.New("upstream or polling failure")
				}
				seen <- job.RemoteID
				return Result{}, errors.New("done observing retry")
			}), Options{})
			project := saveGraph(t, s, 1, []Node{card("v", "video")}, nil)
			s.Start()
			if _, err := s.Run(1, project.Project.ID, "req", []string{"v"}); err != nil {
				t.Fatal(err)
			}
			first := waitSnapshot(t, s, 1, project.Project.ID, noActive)
			if _, err := s.Retry(1, project.Project.ID, first.Jobs[0].ID); err != nil {
				t.Fatal(err)
			}
			waitSnapshot(t, s, 1, project.Project.ID, noActive)
			remote := <-seen
			if (terminal && remote != "") || (!terminal && remote != "remote-123") {
				t.Fatalf("wrong remote task reuse: terminal=%v remote=%s", terminal, remote)
			}
		})
	}
}

func TestRestartPollsKnownVideoAndDoesNotResubmitUnknownImage(t *testing.T) {
	for _, kind := range []string{"image", "video"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			started := make(chan struct{})
			first, err := New(root, executorFunc(func(ctx context.Context, _ Job, _ []AssetFile, progress func(string, bool) error) (Result, error) {
				if kind == "video" {
					if err := progress("remote-resume", false); err != nil {
						return Result{}, err
					}
				}
				close(started)
				<-ctx.Done()
				return Result{}, ctx.Err()
			}), Options{})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = first.Close() })
			project := saveGraph(t, first, 1, []Node{card("n", kind)}, nil)
			first.Start()
			if _, err = first.Run(1, project.Project.ID, "req", []string{"n"}); err != nil {
				t.Fatal(err)
			}
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("executor did not start")
			}
			if err = first.Close(); err != nil {
				t.Fatal(err)
			}
			seen := make(chan Job, 1)
			restarted, err := New(root, executorFunc(func(_ context.Context, job Job, _ []AssetFile, _ func(string, bool) error) (Result, error) {
				seen <- job
				return Result{}, errors.New("observed resumed task")
			}), Options{})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = restarted.Close() })
			snapshot, _ := restarted.Get(1, project.Project.ID)
			if kind == "image" {
				if snapshot.Jobs[0].Status != "interrupted" {
					t.Fatalf("unknown image would be resent: %+v", snapshot.Jobs)
				}
			} else {
				if snapshot.Jobs[0].Status != "queued" {
					t.Fatalf("known video not resumed: %+v", snapshot.Jobs)
				}
				restarted.Start()
				waitSnapshot(t, restarted, 1, project.Project.ID, noActive)
				if job := <-seen; job.RemoteID != "remote-resume" {
					t.Fatalf("lost remote ID: %+v", job)
				}
			}
		})
	}
}

func TestQueueConcurrencyAndCancel(t *testing.T) {
	started := make(chan Job, 10)
	s := testStore(t, executorFunc(func(ctx context.Context, job Job, _ []AssetFile, _ func(string, bool) error) (Result, error) {
		started <- job
		<-ctx.Done()
		return Result{}, ctx.Err()
	}), Options{MaxWorkers: 2, PerUserWorkers: 1})
	first := saveGraph(t, s, 1, []Node{card("a", "image"), card("b", "image")}, nil)
	second := saveGraph(t, s, 2, []Node{card("c", "image")}, nil)
	if _, err := s.Run(1, first.Project.ID, "r1", []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Run(2, second.Project.ID, "r2", []string{"c"}); err != nil {
		t.Fatal(err)
	}
	s.Start()
	jobs := make([]Job, 0, 2)
	for len(jobs) < 2 {
		select {
		case job := <-started:
			jobs = append(jobs, job)
		case <-time.After(3 * time.Second):
			t.Fatal("workers did not start")
		}
	}
	if jobs[0].UserID == jobs[1].UserID {
		t.Fatal("per-user concurrency was exceeded")
	}
	select {
	case <-started:
		t.Fatal("global concurrency was exceeded")
	default:
	}
	var target Job
	for _, job := range jobs {
		if job.UserID == 1 {
			target = job
		}
	}
	if _, err := s.Cancel(1, first.Project.ID, target.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case next := <-started:
		if next.UserID != 1 || next.ID == target.ID {
			t.Fatal("wrong queued task dispatched")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancel did not free worker")
	}
}

func TestExclusiveStorageLockAndQuota(t *testing.T) {
	root := t.TempDir()
	s, err := New(root, unusedExecutor(), Options{MaxUserBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if duplicate, err := New(root, unusedExecutor(), Options{}); !errors.Is(err, ErrBusy) {
		if duplicate != nil {
			_ = duplicate.Close()
		}
		t.Fatalf("shared storage was not locked: %v", err)
	}
	project, err := s.Create(1, "Quota")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.PutAsset(1, project.Project.ID, "test.png", "image/png", append(pngData(t), make([]byte, 2048)...)); !errors.Is(err, ErrQuota) {
		t.Fatalf("quota bypassed: %v", err)
	}
	if _, err := s.PutAsset(1, project.Project.ID, "test.png", "image/png", []byte("<html>error</html>")); !errors.Is(err, ErrInvalid) {
		t.Fatalf("active content accepted: %v", err)
	}
}

func TestExistingReferenceVersionIsPinnedBeforeGraphEdits(t *testing.T) {
	data := pngData(t)
	seen := make(chan []AssetFile, 1)
	s := testStore(t, executorFunc(func(_ context.Context, _ Job, files []AssetFile, _ func(string, bool) error) (Result, error) {
		seen <- files
		return Result{Data: data, MIME: "image/png"}, nil
	}), Options{})
	initial := saveGraph(t, s, 1, []Node{card("overview", "image"), card("detail", "image")}, []Edge{{ID: "ref", Source: "overview", Target: "detail"}})
	id := initial.Project.ID
	oldVersion, err := s.PutAsset(1, id, "old.png", "image/png", data)
	if err != nil {
		t.Fatal(err)
	}
	newVersion, err := s.PutAsset(1, id, "new.png", "image/png", data)
	if err != nil {
		t.Fatal(err)
	}
	initial.Project.Nodes[0].Data.AssetID = oldVersion.ID
	initial, err = s.Save(1, id, initial.Project)
	if err != nil {
		t.Fatal(err)
	}
	run, err := s.Run(1, id, "detail-run", []string{"detail"})
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Jobs) != 1 {
		t.Fatal("existing reference caused an unnecessary paid generation")
	}
	initial.Project.Nodes[0].Data.AssetID = newVersion.ID
	if _, err := s.Save(1, id, initial.Project); err != nil {
		t.Fatal(err)
	}
	s.Start()
	finished := waitSnapshot(t, s, 1, id, noActive)
	if finished.Jobs[0].Status != "succeeded" {
		t.Fatalf("detail failed: %+v", finished.Jobs)
	}
	files := <-seen
	if len(files) != 1 || files[0].ID != oldVersion.ID {
		t.Fatalf("reference silently changed after submission: %+v", files)
	}
}

func TestUnsupportedVideoReferencesAreRejectedBeforeAnyTaskIsQueued(t *testing.T) {
	s := testStore(t, unusedExecutor(), Options{})
	initial := saveGraph(t, s, 1, []Node{card("a", "image"), card("b", "image"), card("v", "video")}, []Edge{{ID: "av", Source: "a", Target: "v"}, {ID: "bv", Source: "b", Target: "v"}})
	if _, err := s.Run(1, initial.Project.ID, "unsupported", []string{"v"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unsupported reference count accepted: %v", err)
	}
	unchanged, _ := s.Get(1, initial.Project.ID)
	if len(unchanged.Jobs) != 0 {
		t.Fatal("invalid workflow submitted paid ancestor tasks")
	}
}

func TestCompositionIdempotencyAndProjectIsolation(t *testing.T) {
	s := testStore(t, unusedExecutor(), Options{})
	first, err := s.Create(1, "First")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Create(1, "Second")
	if err != nil {
		t.Fatal(err)
	}
	asset, err := s.PutAsset(1, first.Project.ID, "image.png", "image/png", pngData(t))
	if err != nil {
		t.Fatal(err)
	}
	params := json.RawMessage(`{"clips":[{"asset_id":"` + asset.ID + `","duration":1}],"size":"720x720"}`)
	if _, err := s.Enqueue(1, second.Project.ID, "export", "export", "compose", "", params, []string{asset.ID}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("composition accepted another project's media: %v", err)
	}
	run, err := s.Enqueue(1, first.Project.ID, "export", "export", "compose", "", params, []string{asset.ID})
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := s.Enqueue(1, first.Project.ID, "export", "export", "compose", "", params, []string{asset.ID})
	if err != nil || len(repeated.Jobs) != 1 || repeated.Jobs[0].ID != run.Jobs[0].ID {
		t.Fatalf("duplicate export was enqueued: %+v %v", repeated.Jobs, err)
	}
	changed := json.RawMessage(`{"clips":[{"asset_id":"` + asset.ID + `","duration":2}],"size":"720x720"}`)
	if _, err := s.Enqueue(1, first.Project.ID, "export", "export", "compose", "", changed, []string{asset.ID}); !errors.Is(err, ErrConflict) {
		t.Fatalf("export request ID reused with different timeline: %v", err)
	}
}

func TestUnconfirmedSubmissionIsInterrupted(t *testing.T) {
	s := testStore(t, executorFunc(func(context.Context, Job, []AssetFile, func(string, bool) error) (Result, error) {
		return Result{}, fmt.Errorf("%w: no task ID received", ErrUncertain)
	}), Options{})
	project := saveGraph(t, s, 1, []Node{card("v", "video")}, nil)
	s.Start()
	if _, err := s.Run(1, project.Project.ID, "request", []string{"v"}); err != nil {
		t.Fatal(err)
	}
	finished := waitSnapshot(t, s, 1, project.Project.ID, noActive)
	if finished.Jobs[0].Status != "interrupted" {
		t.Fatalf("unknown submission was shown as safely retryable: %+v", finished.Jobs)
	}
}

func TestNewChildNeverImplicitlyResubmitsUnresolvedParent(t *testing.T) {
	for _, status := range []string{"failed", "cancelled", "interrupted"} {
		t.Run(status, func(t *testing.T) {
			s := testStore(t, unusedExecutor(), Options{})
			project := saveGraph(t, s, 1, []Node{card("parent", "image"), card("child", "image")}, []Edge{{ID: "ref", Source: "parent", Target: "child"}})
			id := project.Project.ID
			initial, err := s.Run(1, id, "parent-run", []string{"parent"})
			if err != nil {
				t.Fatal(err)
			}
			// Exercise the planner against each durable unresolved state without
			// sending any provider request in this safety regression.
			s.mu.Lock()
			key := projectKey{1, id}
			doc := cloneDocument(s.documents[key])
			doc.Jobs[0].Status = status
			err = s.commit(key, doc)
			s.mu.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.Run(1, id, "child-run", []string{"child"}); !errors.Is(err, ErrInvalid) {
				t.Fatalf("unresolved input was silently regenerated: %v", err)
			}
			after, _ := s.Get(1, id)
			if len(after.Jobs) != 1 || after.Jobs[0].ID != initial.Jobs[0].ID {
				t.Fatal("rejected child submission still appended paid tasks")
			}
		})
	}
}

func TestNewChildUsesCompletedParentBeforeGraphResultIsSaved(t *testing.T) {
	data := pngData(t)
	var parentCalls atomic.Int32
	inputs := make(chan []AssetFile, 1)
	s := testStore(t, executorFunc(func(_ context.Context, job Job, files []AssetFile, _ func(string, bool) error) (Result, error) {
		if job.NodeID == "parent" {
			parentCalls.Add(1)
		} else {
			inputs <- files
		}
		return Result{Data: data, MIME: "image/png"}, nil
	}), Options{})
	project := saveGraph(t, s, 1, []Node{card("parent", "image"), card("child", "image")}, []Edge{{ID: "ref", Source: "parent", Target: "child"}})
	id := project.Project.ID
	s.Start()
	if _, err := s.Run(1, id, "parent-run", []string{"parent"}); err != nil {
		t.Fatal(err)
	}
	completed := waitSnapshot(t, s, 1, id, noActive)
	if completed.Project.Nodes[0].Data.AssetID != "" {
		t.Fatal("test requires graph without a consumed task result")
	}
	if _, err := s.Run(1, id, "child-run", []string{"child"}); err != nil {
		t.Fatal(err)
	}
	after := waitSnapshot(t, s, 1, id, noActive)
	if len(after.Jobs) != 2 || parentCalls.Load() != 1 {
		t.Fatalf("completed parent generated twice: jobs=%d calls=%d", len(after.Jobs), parentCalls.Load())
	}
	files := <-inputs
	if len(files) != 1 || files[0].ID != completed.Jobs[0].AssetID {
		t.Fatalf("child did not use the durable parent version: %+v", files)
	}
}
