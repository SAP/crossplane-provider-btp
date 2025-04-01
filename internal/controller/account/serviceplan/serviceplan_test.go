package serviceplan

import (
	"context"
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	"github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	test2 "github.com/sap/crossplane-provider-btp/internal/tracking/test"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
)

var (
	apiError          = errors.New("apiError")
	deletionTimestamp = meta.Now()
)

func TestObserve(t *testing.T) {
	type args struct {
		client servicemanager.PlanIdResolver
		cr     *v1alpha1.ServicePlan
	}

	type want struct {
		o   managed.ExternalObservation
		err error
		cr  *v1alpha1.ServicePlan
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"Lookup Error": {
			reason: "API error should be returned to reconciler",
			args: args{
				client: &servicemanager.PlanIdResolverFake{
					PlanLookupMockFn: func() (string, error) {
						return "", apiError
					},
				},
				cr: servicePlan(WithUserData("offeringName", "planName")),
			},
			want: want{
				cr:  servicePlan(WithUserData("offeringName", "planName")),
				err: apiError,
			},
		},
		"SuccessfulObserve": {
			reason: "Lookup of Serviceplan should succeed",
			args: args{
				client: &servicemanager.PlanIdResolverFake{
					PlanLookupMockFn: func() (string, error) {
						return "123", nil
					},
				},
				cr: servicePlan(WithUserData("offeringName", "planName")),
			},
			want: want{
				cr: servicePlan(WithUserData("offeringName", "planName"), WithObservedId("123"), WithConditions(xpv1.Available())),
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				err: nil,
			},
		},
		"AcknowledgeDeletion": {
			reason: "CRs marked for deletion should be acknowledged as not found to unblock deletion",
			args: args{
				client: &servicemanager.PlanIdResolverFake{
					PlanLookupMockFn: func() (string, error) {
						// in this case we do not expect the client to be called, so we return an error here as a soft check for this
						return "", errors.New("unexpected api call")
					},
				},
				cr: servicePlan(WithUserData("offeringName", "planName"), WithObservedId("123"), WithConditions(xpv1.Deleting()), WithDeletionTimestamp()),
			},
			want: want{
				cr: servicePlan(WithUserData("offeringName", "planName"), WithObservedId("123"), WithConditions(xpv1.Deleting()), WithDeletionTimestamp()),
				o: managed.ExternalObservation{
					ResourceExists: false,
				},
				err: nil,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{client: tc.args.client}

			got, err := e.Observe(context.Background(), tc.args.cr)

			internal.VerifyTestError(t, tc.want.err, err)

			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want, +got:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr); diff != "" {
				t.Errorf("\ne.Observe(): expected cr after operation -want, +got:\n%s\n", diff)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	type args struct {
		cr *v1alpha1.ServicePlan
	}

	type want struct {
		err error
		cr  *v1alpha1.ServicePlan
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"SuccessfulDelete": {
			reason: "Should just gracefully return",
			args: args{
				cr: servicePlan(WithUserData("offeringName", "planName"), WithObservedId("123"), WithDeletionTimestamp()),
			},
			want: want{
				cr:  servicePlan(WithUserData("offeringName", "planName"), WithObservedId("123"), WithConditions(xpv1.Deleting()), WithDeletionTimestamp()),
				err: nil,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{}

			err := e.Delete(context.Background(), tc.args.cr)

			internal.VerifyTestError(t, tc.want.err, err)

			if diff := cmp.Diff(tc.want.cr, tc.args.cr); diff != "" {
				t.Errorf("\ne.Observe(): expected cr after operation -want, +got:\n%s\n", diff)
			}
		})
	}
}

func TestConnect(t *testing.T) {
	type want struct {
		err error
		cr  *v1alpha1.ServicePlan
	}
	type args struct {
		cr               *v1alpha1.ServicePlan
		kube             test.MockClient
		planIdResolverFn func(ctx context.Context, secretData map[string][]byte) (servicemanager.PlanIdResolver, error)
	}
	tests := []struct {
		name string
		args args

		want want
	}{
		{
			name: "NoServiceManagerSecretSpec",
			args: args{
				cr: servicePlan(),
			},
			want: want{
				err: errors.New(errExtractSecretKey),
				cr:  servicePlan(),
			},
		},
		{
			name: "ServiceManagerSecretNotFound",
			args: args{
				kube: test.MockClient{
					MockGet: test.NewMockGetFn(errors.New("GetSecretError")),
				},
				cr: servicePlan(WithServiceManagerSecret("someSecret", "someNamespace")),
			},
			want: want{
				err: errors.Wrap(errors.New("GetSecretError"), errGetCreds),
				cr:  servicePlan(WithServiceManagerSecret("someSecret", "someNamespace")),
			},
		},
		{
			name: "PlanIdResolverInitError",
			args: args{
				kube: test.MockClient{
					MockGet:          test.NewMockGetFn(nil),
					MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
				},
				cr: servicePlan(WithServiceManagerSecret("someSecret", "someNamespace")),
				planIdResolverFn: func(ctx context.Context, secretData map[string][]byte) (servicemanager.PlanIdResolver, error) {
					return nil, errors.New("ResolverInitError")
				},
			},
			want: want{
				err: errors.New("ResolverInitError"),
				cr:  servicePlan(WithServiceManagerSecret("someSecret", "someNamespace")),
			},
		},
		{
			name: "SuccessfulConnect",
			args: args{
				kube: test.MockClient{
					MockGet:          test.NewMockGetFn(nil),
					MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
				},
				cr: servicePlan(WithServiceManagerSecret("someSecret", "someNamespace")),
				planIdResolverFn: func(ctx context.Context, secretData map[string][]byte) (servicemanager.PlanIdResolver, error) {
					return &servicemanager.PlanIdResolverFake{}, nil
				},
			},
			want: want{
				err: nil,
				cr:  servicePlan(WithServiceManagerSecret("someSecret", "someNamespace")),
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uua := &connector{
				kube:                &tc.args.kube,
				usage:               test2.NoOpReferenceResolverTracker{},
				resourcetracker:     test2.NoOpReferenceResolverTracker{},
				newPlanIdResolverFn: tc.args.planIdResolverFn,
			}
			_, err := uua.Connect(context.TODO(), tc.args.cr)
			if diff := cmp.Diff(err, tc.want.err, test.EquateErrors()); diff != "" {
				t.Errorf("\ne.Observe(): -want error, +got error:\n%s\n", diff)
			}
			if diff := cmp.Diff(tc.args.cr, tc.want.cr); diff != "" {
				t.Errorf("\ne.Observe(): expected cr after operation -want, +got:\n%s\n", diff)
			}
		})
	}
}

type ServiceplanModifier func(spm *v1alpha1.ServicePlan)

func servicePlan(modifiers ...ServiceplanModifier) *v1alpha1.ServicePlan {
	sp := &v1alpha1.ServicePlan{}
	for _, m := range modifiers {
		m(sp)
	}
	return sp
}

func WithUserData(offeringName, planName string) ServiceplanModifier {
	return func(spm *v1alpha1.ServicePlan) {
		spm.Spec.ForProvider = v1alpha1.ServicePlanParameters{
			OfferingName: offeringName,
			PlanName:     planName,
		}
	}
}

func WithServiceManagerSecret(secret, namespace string) ServiceplanModifier {
	return func(spm *v1alpha1.ServicePlan) {
		spm.Spec.ForProvider = v1alpha1.ServicePlanParameters{
			ServiceManagerSecret:          secret,
			ServiceManagerSecretNamespace: namespace,
		}
	}
}

func WithObservedId(id string) ServiceplanModifier {
	return func(spm *v1alpha1.ServicePlan) {
		spm.Status.AtProvider = v1alpha1.ServicePlanObservation{
			ServicePlanId: id,
		}
	}
}

func WithConditions(conds ...xpv1.Condition) ServiceplanModifier {
	return func(spm *v1alpha1.ServicePlan) {
		spm.Status.SetConditions(conds...)
	}
}

func WithDeletionTimestamp() ServiceplanModifier {
	return func(spm *v1alpha1.ServicePlan) {
		spm.SetDeletionTimestamp(&deletionTimestamp)
	}
}
