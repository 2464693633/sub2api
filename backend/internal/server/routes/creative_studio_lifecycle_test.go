package routes

import (
	"errors"
	"sync"
	"testing"
	"time"
)

type creativeCloseFunc func() error

func (f creativeCloseFunc) Close() error { return f() }

func TestCreativeStudioShutdownWaitsAndClosesBeforeResources(t *testing.T) {
	registry := &creativeStudioWorkerRegistry{}
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	var once sync.Once
	unlock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unlock)
	closed := 0
	expected := errors.New("close error must reach application cleanup")
	registry.add(creativeCloseFunc(func() error { closed++; close(entered); <-release; return expected }))
	registry.add(creativeCloseFunc(func() error { closed++; return nil }))
	go func() { finished <- registry.close() }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("worker cleanup did not start")
	}
	select {
	case <-finished:
		t.Fatal("cleanup returned before a worker finished")
	default:
	}
	unlock()
	select {
	case err := <-finished:
		if !errors.Is(err, expected) {
			t.Fatalf("cleanup error lost: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cleanup failed to finish")
	}
	if closed != 2 {
		t.Fatalf("not all workers closed: %d", closed)
	}
	if err := registry.close(); err != nil || closed != 2 {
		t.Fatal("repeated cleanup closed a worker twice")
	}
}
