package worker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/markbates/safe"
	"github.com/sirupsen/logrus"
)

var _ Worker = &Simple{}

// NewSimple creates a basic implementation of the Worker interface
// that is backed using just the standard library and goroutines.
func NewSimple() *Simple {
	// TODO(sio4): #road-to-v1 - how to check if the worker is ready to work
	// when worker should be initialized? how to check if worker is ready?
	// and purpose of the context
	return NewSimpleWithContext(context.Background())
}

// NewSimpleWithContext creates a basic implementation of the Worker interface
// that is backed using just the standard library and goroutines.
func NewSimpleWithContext(ctx context.Context) *Simple {
	ctx, cancel := context.WithCancel(ctx)

	l := logrus.New()
	l.Level = logrus.InfoLevel
	l.Formatter = &logrus.TextFormatter{}

	return &Simple{
		Logger:   l,
		ctx:      ctx,
		cancel:   cancel,
		handlers: map[string]Handler{},
		moot:     &sync.Mutex{},
	}
}

// Simple is a basic implementation of the Worker interface
// that is backed using just the standard library and goroutines.
type Simple struct {
	Logger   SimpleLogger
	ctx      context.Context
	cancel   context.CancelFunc
	handlers map[string]Handler
	moot     *sync.Mutex
	wg       sync.WaitGroup
}

// Register Handler with the worker
func (w *Simple) Register(name string, h Handler) error {
	if name == "" || h == nil {
		return fmt.Errorf("name or handler cannot be empty/nil")
	}

	w.moot.Lock()
	defer w.moot.Unlock()
	if _, ok := w.handlers[name]; ok {
		return fmt.Errorf("handler already mapped for name %s", name)
	}
	w.handlers[name] = h
	return nil
}

// Start the worker
func (w *Simple) Start(ctx context.Context) error {
	// TODO(sio4): #road-to-v1 - define the purpose of Start clearly
	w.Logger.Info("starting Simple background worker")

	w.ctx, w.cancel = context.WithCancel(ctx)
	return nil
}

// Stop the worker
func (w *Simple) Stop() error {
	// prevent job submission when stopping
	w.moot.Lock()
	defer w.moot.Unlock()

	w.Logger.Info("stopping Simple background worker")

	w.cancel()

	w.wg.Wait()
	w.Logger.Info("all background jobs stopped completely")
	return nil
}

// Perform a job as soon as possibly using a goroutine.
func (w *Simple) Perform(job Job) error {
	// Perform should not allow a job submission if the worker is not running
	if err := w.ctx.Err(); err != nil {
		return fmt.Errorf("worker is not ready to perform a job: %v", err)
	}

	w.Logger.Debugf("performing job %s", job)

	if job.Handler == "" {
		err := fmt.Errorf("no handler name given: %s", job)
		w.Logger.Error(err)
		return err
	}

	w.moot.Lock()
	defer w.moot.Unlock()
	if h, ok := w.handlers[job.Handler]; ok {
		// TODO(sio4): #road-to-v1 - consider timeout and/or cancellation
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			err := safe.RunE(func() error {
				return h(job.Args)
			})

			if err != nil {
				w.Logger.Error(err)
			}
			w.Logger.Debugf("completed job %s", job)
		}()
		return nil
	}

	err := fmt.Errorf("no handler mapped for name %s", job.Handler)
	w.Logger.Error(err)
	return err
}

// PerformAt performs a job at a particular time using a goroutine.
func (w *Simple) PerformAt(job Job, t time.Time) error {
	return w.PerformIn(job, time.Until(t))
}

// PerformIn performs a job after waiting for a specified amount
// using a goroutine.
func (w *Simple) PerformIn(job Job, d time.Duration) error {
	// Perform should not allow a job submission if the worker is not running
	if err := w.ctx.Err(); err != nil {
		return fmt.Errorf("worker is not ready to perform a job: %v", err)
	}

	w.wg.Add(1) // waiting job also should be counted
	go func() {
		defer w.wg.Done()

		select {
		case <-time.After(d):
			w.Perform(job)
		case <-w.ctx.Done():
			// TODO(sio4): #road-to-v1 - it should be guaranteed to be performed
			w.cancel()
		}
	}()
	return nil
}

// SimpleLogger is used by the Simple worker to write logs
type SimpleLogger interface {
	Debugf(string, ...interface{})
	Infof(string, ...interface{})
	Errorf(string, ...interface{})
	Debug(...interface{})
	Info(...interface{})
	Error(...interface{})
}
