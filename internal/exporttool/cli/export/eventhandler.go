package export

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/sap/crossplane-provider-btp/internal/exporttool/erratt"
	"github.com/sap/crossplane-provider-btp/internal/exporttool/yaml"

	"github.com/charmbracelet/log"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
)

func printErrors(ctx context.Context, wg *sync.WaitGroup, errChan <-chan error) {
	defer wg.Done()
	errlog := slog.New(log.NewWithOptions(os.Stdout, log.Options{}))
	for {
		select {
		case err, ok := <-errChan:
			if !ok {
				// error channel is closed
				return
			}
			erratt.SlogWarnWith(err, errlog)
		case <-ctx.Done():
			// execution is cancelled
			return
		}
	}
}

func openOutput() (*os.File, erratt.Error) {
	var fileOutput *os.File
	if o := OutputParam.Value(); o != "" {
		var err error
		fileOutput, err = os.Create(filepath.Clean(o))
		if err != nil {
			return nil, erratt.Errorf("Cannot create output file: %w", err).With("output", o)
		}

		slog.Info("Writing output to file", "output", o)
	}
	return fileOutput, nil
}

func resourceLoop(ctx context.Context, fileOutput *os.File, resourceChan <-chan resource.Object, errChan chan<- error) {
	for {
		select {
		case res, ok := <-resourceChan:
			if !ok {
				// resource channel is closed
				return
			}
			if fileOutput != nil {
				// output to file
				y, err := yaml.Marshal(res)
				if err != nil {
					erratt.Slog(erratt.Errorf("cannot YAML-marshal resource: %w", err).With("resource", res))
				} else {
					if _, err := fmt.Fprint(fileOutput, y); err != nil {
						erratt.Slog(erratt.Errorf("cannot write YAML to output: %w", err).With("output", fileOutput.Name()))
					}
				}
			} else {
				// output to console
				slog.Debug("marshalling resource", "res", res)
				y, err := yaml.MarshalPretty(res)
				if err != nil {
					erratt.Slog(erratt.Errorf("cannot YAML-marshal resource: %w", err).With("resource", res))
				} else {
					fmt.Print(y)
				}
			}
		case <-ctx.Done():
			// execution is cancelled
			return
		}
	}
}

func handleResources(ctx context.Context, wg *sync.WaitGroup, resourceChan <-chan resource.Object, errChan chan<- error) {
	defer wg.Done()
	fileOutput, err := openOutput()
	if err != nil {
		errChan <- err
	}
	defer func() {
		if fileOutput != nil {
			err := fileOutput.Close()
			if err != nil {
				errChan <- erratt.Errorf("Cannot close output file: %w", err).With("output", fileOutput.Name())
			}
		}
	}()
	resourceLoop(ctx, fileOutput, resourceChan, errChan)
}

type handler[T any] struct {
	ctx    context.Context
	closed bool
	ch     chan T
}

func newHandler[T any](ctx context.Context) *handler[T] {
	return &handler[T]{
		ctx:    ctx,
		closed: false,
		ch:     make(chan T),
	}
}

func (h *handler[T]) Event(event T) {
	if !h.closed {
		select {
		case h.ch <- event:
		case <-h.ctx.Done():
			h.Stop()
		}
	}
}

func (h *handler[T]) Stop() {
	if !h.closed {
		h.closed = true
		close(h.ch)
	}
}

type EventHandler interface {
	Warn(error)
	Resource(resource.Object)
	Stop()
}

type eventHandler struct {
	errorHandler    *handler[error]
	resourceHandler *handler[resource.Object]
}

var _ EventHandler = eventHandler{}

func newEventHandler(ctx context.Context) eventHandler {
	return eventHandler{
		errorHandler:    newHandler[error](ctx),
		resourceHandler: newHandler[resource.Object](ctx),
	}
}

func (eh eventHandler) Warn(err error) {
	eh.errorHandler.Event(err)
}

func (eh eventHandler) Resource(res resource.Object) {
	eh.resourceHandler.Event(res)
}

func (eh eventHandler) Stop() {
	eh.errorHandler.Stop()
	eh.resourceHandler.Stop()
}
