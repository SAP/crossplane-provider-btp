package cli

import (
	"context"
	"os/signal"
	"syscall"
)

var (
	rootCtx context.Context    // A context.Context to be used to detect ctrl-c interrupts. Its Done channel closes on SIGINT or SIGTERM.
	cancel  context.CancelFunc // A function to close the Done channel of Ctx.
)

func init() {
	rootCtx, cancel = signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
}
