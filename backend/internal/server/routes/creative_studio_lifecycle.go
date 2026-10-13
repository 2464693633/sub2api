package routes

import (
	"errors"
	"sync"
)

type creativeStudioCloser interface{ Close() error }

type creativeStudioWorkerRegistry struct {
	mu      sync.Mutex
	workers []creativeStudioCloser
}

func (r *creativeStudioWorkerRegistry) add(worker creativeStudioCloser) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.workers = append(r.workers, worker)
}

func (r *creativeStudioWorkerRegistry) close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result error
	for _, worker := range r.workers {
		result = errors.Join(result, worker.Close())
	}
	r.workers = nil
	return result
}

var creativeStudioWorkers creativeStudioWorkerRegistry

// CloseCreativeStudioWorkers must run before gateway workers, Redis and the
// database are shut down. Store.Close cancels requests and joins its workers;
// using application cleanup instead of a separate signal goroutine ensures
// the process waits for them and remote-task state remains durable.
func CloseCreativeStudioWorkers() error { return creativeStudioWorkers.close() }
