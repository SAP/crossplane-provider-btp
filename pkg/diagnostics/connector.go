package diagnostics

import (
	"context"
	"strings"

	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
)

type Connector struct {
	Inner managed.ExternalConnector
	Kind  string
	Log   logging.Logger
}

func (c *Connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	ctx = Begin(ctx, c.Kind+"/"+strings.TrimPrefix(mg.GetName(), "TF-"), c.Log)
	stageCtx, finish := Stage(ctx, "connect")
	e, err := c.Inner.Connect(stageCtx, mg)
	finish(err)
	if err != nil || e == nil {
		return e, err
	}
	return &external{ExternalClient: e, trace: ctx}, nil
}

type external struct {
	managed.ExternalClient
	trace context.Context
}

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	ctx = Copy(ctx, e.trace)
	name := StageName(ctx)
	if name == "" {
		name = "observe"
	}
	ctx, finish := Stage(ctx, name)
	o, err := e.ExternalClient.Observe(ctx, mg)
	finish(err)
	return o, err
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	ctx, finish := Stage(Copy(ctx, e.trace), "create")
	o, err := e.ExternalClient.Create(ctx, mg)
	finish(err)
	return o, err
}

func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	ctx, finish := Stage(Copy(ctx, e.trace), "update")
	o, err := e.ExternalClient.Update(ctx, mg)
	finish(err)
	return o, err
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	ctx, finish := Stage(Copy(ctx, e.trace), "delete")
	o, err := e.ExternalClient.Delete(ctx, mg)
	finish(err)
	return o, err
}

func (e *external) Disconnect(ctx context.Context) error {
	return e.ExternalClient.Disconnect(Copy(ctx, e.trace))
}
