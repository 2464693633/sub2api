package creativestudio

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var safeID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,100}$`)

type requestRecord struct {
	RunID       string `json:"run_id"`
	Fingerprint string `json:"fingerprint"`
}
type document struct {
	Snapshot
	Requests map[string]requestRecord `json:"requests"`
}
type projectKey struct {
	user    int64
	project string
}
type execution struct {
	key    projectKey
	jobID  string
	cancel context.CancelFunc
}

type Store struct {
	mu            sync.Mutex
	root          string
	executor      Executor
	opts          Options
	lock          *os.File
	documents     map[projectKey]*document
	documentBytes map[projectKey]int64
	active        map[string]execution
	wake          chan struct{}
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	started       bool
	closed        bool
}

func New(root string, executor Executor, options Options) (*Store, error) {
	if executor == nil || strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("%w: executor and storage directory are required", ErrInvalid)
	}
	if options.MaxWorkers <= 0 {
		options.MaxWorkers = 2
	}
	if options.PerUserWorkers <= 0 {
		options.PerUserWorkers = 1
	}
	if options.MaxUserBytes <= 0 {
		options.MaxUserBytes = 1 << 30
	}
	if options.MaxAssetBytes <= 0 {
		options.MaxAssetBytes = 128 << 20
	}
	if options.MaxNodes <= 0 {
		options.MaxNodes = 100
	}
	if options.MaxActiveJobs <= 0 {
		options.MaxActiveJobs = 50
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(filepath.Join(root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lock.Close()
		return nil, fmt.Errorf("%w: storage directory already in use", ErrBusy)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Store{root: root, executor: executor, opts: options, lock: lock, documents: make(map[projectKey]*document), documentBytes: make(map[projectKey]int64), active: make(map[string]execution), wake: make(chan struct{}, 1), ctx: ctx, cancel: cancel}
	if err = s.load(); err != nil {
		cancel()
		_ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
		_ = lock.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	users, err := os.ReadDir(s.root)
	if err != nil {
		return err
	}
	for _, userDir := range users {
		if !userDir.IsDir() {
			continue
		}
		user, err := strconv.ParseInt(userDir.Name(), 10, 64)
		if err != nil || user <= 0 {
			continue
		}
		projects, err := os.ReadDir(filepath.Join(s.root, userDir.Name()))
		if err != nil {
			return err
		}
		for _, projectDir := range projects {
			if !projectDir.IsDir() || !safeID.MatchString(projectDir.Name()) {
				continue
			}
			key := projectKey{user, projectDir.Name()}
			data, err := os.ReadFile(filepath.Join(s.projectDir(key), "project.json"))
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return err
			}
			var doc document
			if err := json.Unmarshal(data, &doc); err != nil {
				return fmt.Errorf("read creative project %s: %w", projectDir.Name(), err)
			}
			if doc.Project.ID != key.project {
				return fmt.Errorf("%w: invalid project identity", ErrInvalid)
			}
			if doc.Requests == nil {
				doc.Requests = make(map[string]requestRecord)
			}
			s.documentBytes[key] = int64(len(data))
			changed := false
			for i := range doc.Jobs {
				job := &doc.Jobs[i]
				if job.Status == "running" {
					if job.Kind == "video" && job.RemoteID != "" && !job.UpstreamTerminal {
						job.Status = "queued"
						job.Error = ""
					} else {
						job.Status = "interrupted"
						job.Error = "服务重启时任务尚未确认完成，请检查上游结果后重试。 / The service restarted before this task was confirmed. Check the upstream result before retrying."
					}
					job.UpdatedAt = now()
					changed = true
				}
			}
			if changed {
				if err := s.commitJobState(key, &doc); err != nil {
					return err
				}
			}
			s.documents[key] = &doc
		}
	}
	return nil
}

func now() int64 { return time.Now().UnixMilli() }
func uid() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func (s *Store) projectDir(key projectKey) string {
	return filepath.Join(s.root, strconv.FormatInt(key.user, 10), key.project)
}
func (s *Store) assetPath(key projectKey, id string) string {
	return filepath.Join(s.projectDir(key), "assets", id)
}

func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".pending-")
	if err != nil {
		return err
	}
	defer func() { _ = f.Close(); _ = os.Remove(f.Name()) }()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	if dir, openErr := os.Open(filepath.Dir(path)); openErr == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}
func (s *Store) persist(key projectKey, doc *document) error {
	data, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	// Count project/task metadata as well as media, so repeated failed requests
	// cannot bypass a user's storage quota by growing JSON files indefinitely.
	used := int64(len(data))
	for _, asset := range doc.Assets {
		used += asset.Size
	}
	for otherKey, other := range s.documents {
		if otherKey.user != key.user || otherKey == key {
			continue
		}
		used += s.documentBytes[otherKey]
		for _, asset := range other.Assets {
			used += asset.Size
		}
	}
	if used > s.opts.MaxUserBytes {
		return ErrQuota
	}
	return s.writeDocument(key, data)
}
func (s *Store) writeDocument(key projectKey, data []byte) error {
	if err := atomicWrite(filepath.Join(s.projectDir(key), "project.json"), data); err != nil {
		return err
	}
	s.documentBytes[key] = int64(len(data))
	return nil
}
func cloneDocument(doc *document) *document {
	data, _ := json.Marshal(doc)
	var copied document
	_ = json.Unmarshal(data, &copied)
	return &copied
}
func (s *Store) lookup(userID int64, projectID string) (projectKey, *document, error) {
	key := projectKey{userID, projectID}
	if s.closed {
		return key, nil, ErrClosed
	}
	if userID <= 0 || !safeID.MatchString(projectID) {
		return key, nil, ErrNotFound
	}
	doc, ok := s.documents[key]
	if !ok {
		return key, nil, ErrNotFound
	}
	return key, doc, nil
}
func (s *Store) commit(key projectKey, doc *document) error {
	if err := s.persist(key, doc); err != nil {
		return err
	}
	s.documents[key] = doc
	return nil
}

// commitJobState persists only bounded state changes to already admitted jobs.
// A full media quota must not discard paid task IDs or prevent recovery and
// cancellation. New jobs, assets and project edits still go through commit.
func (s *Store) commitJobState(key projectKey, doc *document) error {
	data, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	if err := s.writeDocument(key, data); err != nil {
		return err
	}
	s.documents[key] = doc
	return nil
}

func (s *Store) notify() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Store) List(userID int64) ([]Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrClosed
	}
	if userID <= 0 {
		return nil, ErrNotFound
	}
	projects := make([]Project, 0)
	for key, doc := range s.documents {
		if key.user == userID {
			projects = append(projects, cloneDocument(doc).Project)
		}
	}
	sort.Slice(projects, func(i, j int) bool { return projects[i].UpdatedAt > projects[j].UpdatedAt })
	return projects, nil
}
func (s *Store) Create(userID int64, name string) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Snapshot{}, ErrClosed
	}
	if userID <= 0 || len(name) > 240 {
		return Snapshot{}, ErrInvalid
	}
	count := 0
	for key := range s.documents {
		if key.user == userID {
			count++
		}
	}
	if count >= 200 {
		return Snapshot{}, fmt.Errorf("%w: maximum 200 projects", ErrQuota)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Untitled project"
	}
	stamp := now()
	project := Project{ID: uid(), Name: name, Revision: 1, Nodes: []Node{}, Edges: []Edge{}, Viewport: Viewport{Zoom: 1}, CreatedAt: stamp, UpdatedAt: stamp}
	doc := &document{Snapshot: Snapshot{Project: project, Assets: []Asset{}, Jobs: []Job{}}, Requests: make(map[string]requestRecord)}
	if err := s.commit(projectKey{userID, project.ID}, doc); err != nil {
		return Snapshot{}, err
	}
	return cloneDocument(doc).Snapshot, nil
}
func (s *Store) Get(userID int64, projectID string) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, doc, err := s.lookup(userID, projectID)
	if err != nil {
		return Snapshot{}, err
	}
	return cloneDocument(doc).Snapshot, nil
}
func (s *Store) Save(userID int64, projectID string, project Project) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, original, err := s.lookup(userID, projectID)
	if err != nil {
		return Snapshot{}, err
	}
	if project.Revision != original.Project.Revision {
		return Snapshot{}, ErrConflict
	}
	if project.ID != projectID {
		return Snapshot{}, ErrInvalid
	}
	if err := s.validateProject(&project, original.Assets); err != nil {
		return Snapshot{}, err
	}
	doc := cloneDocument(original)
	project.CreatedAt = original.Project.CreatedAt
	project.UpdatedAt = now()
	project.Revision++
	doc.Project = project
	// Save callers may keep editing their object; storage owns its immutable copy.
	doc = cloneDocument(doc)
	if err := s.commit(key, doc); err != nil {
		return Snapshot{}, err
	}
	return cloneDocument(doc).Snapshot, nil
}
func (s *Store) Delete(userID int64, projectID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, doc, err := s.lookup(userID, projectID)
	if err != nil {
		return err
	}
	for _, job := range doc.Jobs {
		if activeStatus(job.Status) {
			return ErrBusy
		}
	}
	for _, running := range s.active {
		if running.key == key {
			return ErrBusy
		}
	}
	// Rename first, so a partially removed tree is never loaded as a project.
	tombstone := filepath.Join(s.root, ".deleted-"+uid())
	if err := os.Rename(s.projectDir(key), tombstone); err != nil {
		return err
	}
	delete(s.documents, key)
	delete(s.documentBytes, key)
	return os.RemoveAll(tombstone)
}
func (s *Store) PutAsset(userID int64, projectID, name, contentType string, data []byte) (Asset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, original, err := s.lookup(userID, projectID)
	if err != nil {
		return Asset{}, err
	}
	doc := cloneDocument(original)
	asset, err := s.putAsset(key, doc, name, contentType, data, "", "")
	if err != nil {
		return Asset{}, err
	}
	if err := s.commit(key, doc); err != nil {
		_ = os.Remove(s.assetPath(key, asset.ID))
		return Asset{}, err
	}
	return asset, nil
}
func (s *Store) putAsset(key projectKey, doc *document, name, contentType string, data []byte, nodeID, runID string) (Asset, error) {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return Asset{}, fmt.Errorf("%w: invalid media type", ErrInvalid)
	}
	var kind string
	switch mediaType {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/avif":
		kind = "image"
	case "video/mp4", "video/webm", "video/quicktime":
		kind = "video"
	case "audio/mpeg", "audio/mp3", "audio/wav", "audio/x-wav", "audio/ogg", "audio/webm", "audio/mp4", "audio/aac", "audio/flac":
		kind = "audio"
	default:
		return Asset{}, fmt.Errorf("%w: unsupported media type", ErrInvalid)
	}
	if len(data) == 0 {
		return Asset{}, fmt.Errorf("%w: empty media file", ErrInvalid)
	}
	limit := s.opts.MaxAssetBytes
	if kind == "image" && limit > 32<<20 {
		limit = 32 << 20
	}
	if int64(len(data)) > limit {
		return Asset{}, fmt.Errorf("%w: media file too large", ErrQuota)
	}
	// Do not store upstream error pages or active content as media.
	sniffed := http.DetectContentType(data)
	if strings.HasPrefix(sniffed, "text/") || strings.Contains(sniffed, "xml") {
		return Asset{}, fmt.Errorf("%w: file is not media", ErrInvalid)
	}
	var used int64
	for otherKey, other := range s.documents {
		if otherKey.user == key.user {
			for _, asset := range other.Assets {
				used += asset.Size
			}
		}
	}
	if used+int64(len(data)) > s.opts.MaxUserBytes || len(doc.Assets) >= 10000 {
		return Asset{}, ErrQuota
	}
	if name == "" {
		name = kind
	}
	if len(name) > 240 {
		name = name[:240]
	}
	asset := Asset{ID: uid(), Kind: kind, Name: filepath.Base(name), MIME: mediaType, Size: int64(len(data)), CreatedAt: now(), NodeID: nodeID, RunID: runID}
	if err := atomicWrite(s.assetPath(key, asset.ID), data); err != nil {
		return Asset{}, err
	}
	doc.Assets = append(doc.Assets, asset)
	return asset, nil
}
func (s *Store) OpenAsset(userID int64, projectID, assetID string) (AssetFile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, doc, err := s.lookup(userID, projectID)
	if err != nil {
		return AssetFile{}, err
	}
	return s.openAsset(key, doc, assetID)
}
func (s *Store) openAsset(key projectKey, doc *document, assetID string) (AssetFile, error) {
	if !safeID.MatchString(assetID) {
		return AssetFile{}, ErrNotFound
	}
	for _, asset := range doc.Assets {
		if asset.ID == assetID {
			path := s.assetPath(key, assetID)
			info, err := os.Lstat(path)
			if err != nil {
				if os.IsNotExist(err) {
					return AssetFile{}, ErrNotFound
				}
				return AssetFile{}, err
			}
			if !info.Mode().IsRegular() || info.Mode()&fs.ModeSymlink != 0 {
				return AssetFile{}, ErrInvalid
			}
			return AssetFile{Asset: asset, Path: path}, nil
		}
	}
	return AssetFile{}, ErrNotFound
}

func activeStatus(status string) bool {
	return status == "queued" || status == "waiting" || status == "running"
}

func (s *Store) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.cancel()
	for _, run := range s.active {
		run.cancel()
	}
	s.mu.Unlock()
	s.wg.Wait()
	if err := syscall.Flock(int(s.lock.Fd()), syscall.LOCK_UN); err != nil {
		_ = s.lock.Close()
		return err
	}
	return s.lock.Close()
}
