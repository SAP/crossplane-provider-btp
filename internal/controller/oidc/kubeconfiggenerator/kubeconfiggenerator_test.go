package kubeconfiggenerator

import (
	"context"
	"errors"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/google/go-cmp/cmp/cmpopts"
	corev1 "k8s.io/api/core/v1"

	"github.com/sap/crossplane-provider-btp/apis/oidc/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/clients/oidc"
	ctrloidc "github.com/sap/crossplane-provider-btp/internal/controller/oidc"

	"github.com/google/go-cmp/cmp"

	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/test"
)

func TestConnect(t *testing.T) {
	type args struct {
		mg *v1alpha1.KubeConfigGenerator
	}
	type want struct {
		err error
	}
	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"create connector successfully": {
			reason: "Connector should be created just fine",
			args:   args{mg: crResource("unittests-kubeconfig-generator", withStatusData(MockedKubeConfigHash, MockedTokenHash, 2, ""))},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var recordedKubeConfig []byte
			var recordedToken []byte
			c := connector{kube: &test.MockClient{}, usage: ctrloidc.MockTracker(), newServiceFn: newServiceFnWithRecorder(&recordedKubeConfig, &recordedToken)}
			got, err := c.Connect(context.Background(), tc.args.mg)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Connect(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if tc.want.err == nil && got == nil {
				t.Errorf("\n%s\ne.Connect(...):no service has been created", tc.reason)
			}
			if diff := cmp.Diff(tc.args.mg.Status.AtProvider.KubeConfigHash, recordedKubeConfig); diff != "" {
				t.Errorf("\n%s\n.Connect(...): -want called newservicefn with cert, +got: \n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.args.mg.Status.AtProvider.TokenHash, recordedToken); diff != "" {
				t.Errorf("\n%s\n.Connect(...): -want called newservicefn with pw, +got: \n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestObserve(t *testing.T) {
	type fields struct {
		service oidc.KubeConfigClient
	}

	type args struct {
		mg      resource.Managed
		secrets []corev1.Secret
	}

	type want struct {
		o   managed.ExternalObservation
		err error
		mg  resource.Managed
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"Failure on missing OIDC Secret": {
			reason: "Observe should fail if OIDC secret isn't available",
			args: args{
				mg:      crResource("unittests-kubeconfig-generator"),
				secrets: []corev1.Secret{fakeKubeConfigSecret},
			},
			want: want{
				o:   managed.ExternalObservation{},
				mg:  crResource("unittests-kubeconfig-generator"),
				err: oidcError(),
			},
		},
		"Failure on missing Kubeconfig Secret": {
			reason: "Observe should fail Kubeconfig secret isn't available",
			args: args{
				mg:      crResource("unittests-kubeconfig-generator"),
				secrets: []corev1.Secret{fakeOIDCSecret},
			},
			want: want{
				o:   managed.ExternalObservation{},
				mg:  crResource("unittests-kubeconfig-generator"),
				err: kubeConfigError(),
			},
		},
		"Needs creation": {
			reason: "We should return creation needed if no connectiondetails have been published yet",
			args: args{
				mg:      crResource("unittests-kubeconfig-generator"),
				secrets: []corev1.Secret{fakeOIDCSecret, fakeKubeConfigSecret},
			},
			want: want{
				o:  managed.ExternalObservation{ResourceExists: false, ConnectionDetails: managed.ConnectionDetails{}},
				mg: crResource("unittests-kubeconfig-generator"),
			},
		},
		"Needs Update Credentials Changed": {
			reason: "We should return not up to date if hashes don't match",
			fields: fields{service: clientMock(false, false, "")},
			args: args{
				mg:      crResource("unittests-kubeconfig-generator"),
				secrets: []corev1.Secret{fakeKubeConfigSecret, fakeOIDCSecret, fakeKubeConfigTokenSecret},
			},
			want: want{
				o:  managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false, ConnectionDetails: managed.ConnectionDetails{}},
				mg: crResource("unittests-kubeconfig-generator", withStatus(xpv1.Available())),
			},
		},
		"Needs Update CR Changed": {
			reason: "We should return not up to date if changes to the CR have been applied",
			fields: fields{service: clientMock(true, false, "")},
			args: args{
				mg:      crResource("unittests-kubeconfig-generator", withStatusData(nil, nil, 3, "")),
				secrets: []corev1.Secret{fakeKubeConfigSecret, fakeOIDCSecret, fakeKubeConfigTokenSecret},
			},
			want: want{
				o:  managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false, ConnectionDetails: managed.ConnectionDetails{}},
				mg: crResource("unittests-kubeconfig-generator", withStatus(xpv1.Available()), withStatusData(nil, nil, 3, "")),
			},
		},
		"Is in sync": {
			reason: "We should return Up to date if hashes do match",
			fields: fields{service: clientMock(true, false, "")},
			args: args{
				mg:      crResource("unittests-kubeconfig-generator", withStatusData(nil, nil, 2, "")),
				secrets: []corev1.Secret{fakeOIDCSecret, fakeKubeConfigSecret, fakeKubeConfigTokenSecret},
			},
			want: want{
				o:  managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true, ConnectionDetails: managed.ConnectionDetails{}},
				mg: crResource("unittests-kubeconfig-generator", withStatus(xpv1.Available()), withStatusData(nil, nil, 2, "")),
			},
		},
		"Deletion with no secret, no error": {
			reason: "Deletion should be possible without secret ",
			args: args{
				mg:      crResource("unittests-kubeconfig-generator", withDeletion()),
				secrets: []corev1.Secret{},
			},
			want: want{
				o:  managed.ExternalObservation{ResourceExists: true},
				mg: crResource("unittests-kubeconfig-generator", withDeletion()),
			},
		},
		"Deletion status set, pretend we dont exist": {
			reason: "Deletion should be possible without secret ",
			args: args{
				mg:      crResource("unittests-kubeconfig-generator", withDeletion(), withStatus(xpv1.Deleting())),
				secrets: []corev1.Secret{},
			},
			want: want{
				o:  managed.ExternalObservation{ResourceExists: false},
				mg: crResource("unittests-kubeconfig-generator", withDeletion(), withStatus(xpv1.Deleting())),
			},
		},
		"Don't set Spec.WriteConnectionSecretToReference, raises an error": {
			reason: "Deletion should be possible without secret ",
			args: args{
				mg:      crResource("unittests-kubeconfig-generator", removeConnectionReference()),
				secrets: []corev1.Secret{},
			},
			want: want{
				o:   managed.ExternalObservation{ResourceExists: false},
				mg:  crResource("unittests-kubeconfig-generator", removeConnectionReference()),
				err: errors.New(errNoConnectionSecret),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{service: tc.fields.service, kube: ctrloidc.MockCertLookup(tc.args.secrets, nil)}
			got, err := e.Observe(context.Background(), tc.args.mg)
			if diff := compareErrorMessages(err, tc.want.err); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want, +got:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.mg, tc.args.mg); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want cr, +got cr:\n%s\n", tc.reason, diff)
			}
		})
	}
}

func compareErrorMessages(is error, target error) string {
	if is == nil && target == nil {
		return ""
	}
	return cmp.Diff(is.Error(), target.Error())
}
func removeConnectionReference() func(generator *v1alpha1.KubeConfigGenerator) {
	return func(cr *v1alpha1.KubeConfigGenerator) {
		cr.Spec.WriteConnectionSecretToReference = nil
	}
}

func TestCreate(t *testing.T) {
	type fields struct {
		service oidc.KubeConfigClient
	}

	type args struct {
		mg      *v1alpha1.KubeConfigGenerator
		secrets []corev1.Secret
	}

	type want struct {
		o   managed.ExternalCreation
		err error
		mg  *v1alpha1.KubeConfigGenerator
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"Failure on missing OIDC Secret": {
			reason: "Create should fail if OIDC secret isn't available",
			args: args{
				mg:      crResource("unittests-kubeconfig-generator"),
				secrets: []corev1.Secret{fakeKubeConfigSecret},
			},
			want: want{
				o:   managed.ExternalCreation{},
				mg:  crResource("unittests-kubeconfig-generator"),
				err: oidcError(),
			},
		},
		"Failure on missing Kubeconfig Secret": {
			reason: "Create should fail Kubeconfig secret isn't available",
			args: args{
				mg:      crResource("unittests-kubeconfig-generator"),
				secrets: []corev1.Secret{fakeOIDCSecret},
			},
			want: want{
				o:   managed.ExternalCreation{},
				mg:  crResource("unittests-kubeconfig-generator"),
				err: kubeConfigError(),
			},
		},
		"Failure on Generate": {
			reason: "Create should fail when in case of corrupted template",
			fields: fields{service: clientMock(false, false, "")},
			args: args{
				mg:      crResource("unittests-kubeconfig-generator"),
				secrets: []corev1.Secret{fakeOIDCSecret, fakeKubeConfigSecret},
			},
			want: want{
				o:   managed.ExternalCreation{},
				mg:  crResource("unittests-kubeconfig-generator", withStatus(xpv1.Creating())),
				err: kubeConfigGenerationError(),
			},
		},
		"Successful CREATE": {
			reason: "Create succeed and return generated kubeconfig if configured properly",
			fields: fields{service: clientMock(false, true, "someServerUrl")},
			args: args{
				mg:      crResource("unittests-kubeconfig-generator"),
				secrets: []corev1.Secret{fakeOIDCSecret, fakeKubeConfigSecret, fakeKubeConfigTokenSecret},
			},
			want: want{
				o:  managed.ExternalCreation{ConnectionDetails: managed.ConnectionDetails{v1alpha1.KubeConfigSecreKey: []byte("new_generated_kubeconfig")}},
				mg: crResource("unittests-kubeconfig-generator", withStatus(xpv1.Creating()), withStatusData(MockedKubeConfigHash, MockedTokenHash, 2, "someServerUrl")),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{service: tc.fields.service, kube: ctrloidc.MockCertLookup(tc.args.secrets, nil)}
			got, err := e.Create(context.Background(), tc.args.mg)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Create(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.Create(...): -want, +got:\n%s\n", tc.reason, diff)
			}
			// compare spec and conditions manually, to avoid conflict with timestamp in at provider
			if diff := cmp.Diff(tc.want.mg, tc.args.mg, cmpopts.IgnoreFields(v1alpha1.KubeConfigGeneratorObservation{}, "LastUpdatedAt")); diff != "" {
				t.Errorf("\n%s\ne.Create(...): -want cr, +got cr:\n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	type fields struct {
		service oidc.KubeConfigClient
	}

	type args struct {
		mg      *v1alpha1.KubeConfigGenerator
		secrets []corev1.Secret
	}

	type want struct {
		o   managed.ExternalUpdate
		err error
		mg  *v1alpha1.KubeConfigGenerator
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"Failure on missing OIDC Secret": {
			reason: "Update should fail if OIDC secret isn't available",
			args: args{
				mg:      crResource("unittests-kubeconfig-generator"),
				secrets: []corev1.Secret{fakeKubeConfigSecret},
			},
			want: want{
				o:   managed.ExternalUpdate{},
				mg:  crResource("unittests-kubeconfig-generator"),
				err: oidcError(),
			},
		},
		"Failure on missing Kubeconfig Secret": {
			reason: "Update should fail Kubeconfig secret isn't available",
			args: args{
				mg:      crResource("unittests-kubeconfig-generator"),
				secrets: []corev1.Secret{fakeOIDCSecret},
			},
			want: want{
				o:   managed.ExternalUpdate{},
				mg:  crResource("unittests-kubeconfig-generator"),
				err: kubeConfigError(),
			},
		},
		"Failure on Generate": {
			reason: "Update should fail when in case of corrupted template",
			fields: fields{service: clientMock(false, false, "")},
			args: args{
				mg:      crResource("unittests-kubeconfig-generator"),
				secrets: []corev1.Secret{fakeOIDCSecret, fakeKubeConfigSecret},
			},
			want: want{
				o:   managed.ExternalUpdate{},
				mg:  crResource("unittests-kubeconfig-generator"),
				err: kubeConfigGenerationError(),
			},
		},
		"Successful Update": {
			reason: "Update succeed and return generated kubeconfig if configured properly",
			fields: fields{service: clientMock(false, true, "someServerUrl")},
			args: args{
				mg:      crResource("unittests-kubeconfig-generator"),
				secrets: []corev1.Secret{fakeOIDCSecret, fakeKubeConfigSecret, fakeKubeConfigTokenSecret},
			},
			want: want{
				o:  managed.ExternalUpdate{ConnectionDetails: managed.ConnectionDetails{v1alpha1.KubeConfigSecreKey: []byte("new_generated_kubeconfig")}},
				mg: crResource("unittests-kubeconfig-generator"),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{service: tc.fields.service, kube: ctrloidc.MockCertLookup(tc.args.secrets, nil)}
			got, err := e.Update(context.Background(), tc.args.mg)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Update(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.Update(...): -want, +got:\n%s\n", tc.reason, diff)
			}
			// compare spec and conditions manually, to avoid conflict with timestamp in at provider
			if diff := cmp.Diff(tc.want.mg.Spec, tc.args.mg.Spec); diff != "" {
				t.Errorf("\n%s\ne.Update(...): -want cr spec, +got cr spec:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.mg.Status.Conditions, tc.args.mg.Status.Conditions); diff != "" {
				t.Errorf("\n%s\ne.Update(...): -want cr status, +got cr status:\n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	type args struct {
		mg      *v1alpha1.KubeConfigGenerator
		secrets []corev1.Secret
	}

	type want struct {
		mg  *v1alpha1.KubeConfigGenerator
		err error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"Cleanup created KubeConfig Secret": {
			reason: "Along with deleting CR, connection details secret should be cleaned up as well",
			args: args{
				mg:      crResource("unittests-kubeconfig-generator"),
				secrets: []corev1.Secret{fakeKubeConfigTokenSecret},
			},
			want: want{
				mg: crResource("unittests-kubeconfig-generator", withStatus(xpv1.Deleting())),
			},
		},
		"Delete without cleanup": {
			reason: "If no connection details secret yet exists, should still delete gracefully",
			args: args{
				mg:      crResource("unittests-kubeconfig-generator"),
				secrets: []corev1.Secret{fakeKubeConfigTokenSecret},
			},
			want: want{
				mg: crResource("unittests-kubeconfig-generator", withStatus(xpv1.Deleting())),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{service: clientMock(false, false, ""), kube: ctrloidc.MockCertLookup(tc.args.secrets, nil)}
			_, err := e.Delete(context.Background(), tc.args.mg)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Update(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			// compare spec and conditions manually, to avoid conflict with timestamp in at provider
			if diff := cmp.Diff(tc.want.mg.Spec, tc.args.mg.Spec); diff != "" {
				t.Errorf("\n%s\ne.Update(...): -want cr spec, +got cr spec:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.mg.Status.Conditions, tc.args.mg.Status.Conditions); diff != "" {
				t.Errorf("\n%s\ne.Update(...): -want cr status, +got cr status:\n%s\n", tc.reason, diff)
			}
		})
	}
}
