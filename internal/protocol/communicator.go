package protocol

import (
	"context"
	"io"
	"log/slog"
	"sync"
)

// A Communicator handles communication with the Launcher and exposes a stream
// of requests for a client to handle.
type Communicator struct {
	lgr *slog.Logger
	enc *Encoder
	dec *Decoder
}

// NewCommunicator creates a new communication channel with the Launcher on the
// given io.Reader and io.Writer streams. The maximum message size is given by
// maxSize.
func NewCommunicator(lgr *slog.Logger, r io.Reader, w io.Writer, maxSize int) *Communicator {
	return &Communicator{
		lgr: lgr,
		enc: NewEncoder(w, maxSize),
		dec: NewDecoder(r, maxSize),
	}
}

// Serve begins serving requests using the given handler.
func (c *Communicator) Serve(ctx context.Context, handler func(Request, chan<- interface{})) error {
	var err error
	wg := sync.WaitGroup{}
	reqCh := make(chan Request, 100)
	respCh := make(chan interface{}, 100)
	errCh := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		for resp := range respCh {
			c.lgr.Debug("Dequeued response", "depth", len(respCh))
			if err := c.enc.Encode(resp); err != nil {
				errCh <- err
				break
			}
		}
		c.lgr.Debug("Stopping writer...")
		done <- struct{}{}
	}()
	// Note: this goroutine will leak if the io.Reader implementation hangs
	// indefinitely during Read() calls, as os.Stdin does. This is a general
	// limitation of Go; see e.g. https://github.com/golang/go/issues/58628.
	// (The way to handle this cleanly would be by using epoll, as some
	// libraries do.)
	go func() {
		for c.dec.More() {
			req := c.dec.Request()
			if req == nil {
				break
			}
			reqCh <- req
			c.lgr.Debug("Queued request", "id", req.ID(), "type",
				req.Type(), "bytes", c.dec.MsgSize(), "depth",
				len(reqCh))
		}
		if err := c.dec.Err(); err != nil {
			errCh <- err
		}
		c.lgr.Debug("Stopping reader...")
		close(reqCh)
	}()
poll:
	select {
	case <-ctx.Done():
		break
	case err = <-errCh:
		break
	case req, ok := <-reqCh:
		if !ok {
			c.lgr.Info("Request stream ended")
			break
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			handler(req, respCh)
		}()
		goto poll
	}
	c.lgr.Debug("Closing communicator...")
	wg.Wait()
	close(respCh)
	<-done
	if err != nil {
		return err
	}
	// Check for any errors encountered during shutdown.
	if len(errCh) != 0 {
		return <-errCh
	}
	return nil
}
