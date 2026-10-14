package loxone

import (
	"sync"

	"github.com/rs/zerolog/log"
)

// worker runs queued jobs one after another on its own goroutine so a slow or
// unreachable Miniserver never blocks the event loop. Order within one worker
// is preserved; jobs are dropped when the queue is full.
type worker struct {
	name string
	jobs chan func()
	stop chan struct{}
	once sync.Once
}

func newWorker(name string, size int) *worker {
	w := &worker{name: name, jobs: make(chan func(), size), stop: make(chan struct{})}
	go w.run()
	return w
}

func (w *worker) run() {
	for {
		select {
		case <-w.stop:
			return
		case job := <-w.jobs:
			job()
		}
	}
}

// submit queues a job without blocking. It returns false when the queue is full.
func (w *worker) submit(job func()) bool {
	select {
	case w.jobs <- job:
		return true
	default:
		log.Warn().Str("worker", w.name).Msg("Feedback queue full, dropping message")
		return false
	}
}

// close stops the worker; queued jobs are discarded.
func (w *worker) close() {
	w.once.Do(func() { close(w.stop) })
}
