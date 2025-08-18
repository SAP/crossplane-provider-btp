package certbasedoidclogin

import (
	"context"
	"testing"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	corev1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/google/go-cmp/cmp"

	"github.com/sap/crossplane-provider-btp/apis/oidc/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	"github.com/sap/crossplane-provider-btp/internal/clients/oidc"
	ctrloidc "github.com/sap/crossplane-provider-btp/internal/controller/oidc"
	"github.com/sap/crossplane-provider-btp/internal/testutils"

	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/test"
)

func TestConnect(t *testing.T) {
	type args struct {
		mg      resource.Managed
		secrets []corev1.Secret
	}
	type want struct {
		err                    error
		calledNewServiceFnCert []byte
		calledNewServiceFnPW   string
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"Missing secret": {
			reason: "We should return cert secret resolval error",
			args: args{
				mg:      defaultResource("unittest-login"),
				secrets: []corev1.Secret{fakePWSecret},
			},
			want: want{err: certError()},
		},
		"Missing Password": {
			reason: "We should return password secret resolval error",
			args: args{
				mg:      defaultResource("unittest-login"),
				secrets: []corev1.Secret{fakeCertSecret},
			},
			want: want{err: pwError()},
		},
		"No problems": {
			reason: "Service should be created just fine",
			args: args{
				mg:      defaultResource("unittest-login"),
				secrets: []corev1.Secret{fakeCertSecret, fakePWSecret},
			},
			want: want{calledNewServiceFnCert: fakeCertSecret.Data["cert"], calledNewServiceFnPW: string(fakePWSecret.Data["password"])},
		},
	}

	for _, tc := range cases {
		t.Run(tc.reason, func(t *testing.T) {
			var recordedUserCertCall []byte
			var recordedPWCall string

			mockNewServiceFn := newServiceFnWithRecorder(&recordedUserCertCall, &recordedPWCall)
			c := connector{ctrloidc.MockCertLookup(tc.args.secrets, nil), ctrloidc.MockTracker(), mockNewServiceFn}
			got, err := c.Connect(context.Background(), tc.args.mg)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Connect(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if tc.want.err == nil && got == nil {
				t.Errorf("\n%s\n.Connect(...):no service has been created", tc.reason)
			}
			if diff := cmp.Diff(tc.want.calledNewServiceFnCert, recordedUserCertCall); diff != "" {
				t.Errorf("\n%s\n.Connect(...): -want called newservicefn with cert, +got: \n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.calledNewServiceFnPW, recordedPWCall); diff != "" {
				t.Errorf("\n%s\n.Connect(...): -want called newservicefn with pw, +got: \n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestObserve(t *testing.T) {
	type fields struct {
		loginClient oidc.LoginPerformer
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
		"Needs Creation": {
			reason: "It should be detected that there is no saved token secret and return needed creation",
			args: args{
				mg: defaultResource("unittest-login"),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:    false,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				err: nil,
				mg:  defaultResource("unittest-login"),
			},
		},
		"Requires Update": {
			reason: "We expect token secret to be present, but expired",
			fields: fields{loginClient: mockCertLoginService(false, true, false)},
			args: args{
				mg:      defaultResource("unittest-login"),
				secrets: []corev1.Secret{expiredTokenSecret},
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  false,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				err: nil,
				mg: cr(
					defaultResource("unittest-login"),
					conditions(
						xpv1.Available(),
						v1alpha1.IntrospectOk(),
					),
					jwtStatus(v1alpha1.JwtStatus{
						IssuedAt:  testingTime(time.Hour * -1),
						ExpiresAt: testingTime(time.Minute - 1),
						RotationNotBefore: testingTimeWith(
							time.Now().Add(time.Minute-1),
							((time.Minute*20)+(time.Second*20))*-1),
						RotationStrategy: internal.Ptr(v1alpha1.RotationStrategyDynamic),
						RotationDuration: testingDuration((time.Minute * 20) + (time.Second * 20)),
					})),
			},
		},
		"Token in refresh range": {
			reason: "We expect token secret to be present, but about to expire",
			fields: fields{loginClient: mockCertLoginService(false, true, false)},
			args: args{
				mg:      defaultResource("unittest-login"),
				secrets: []corev1.Secret{aboutToExpireTokenSecret},
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  false,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				err: nil,
				mg: cr(
					defaultResource("unittest-login"),
					conditions(
						xpv1.Available(),
						v1alpha1.IntrospectOk(),
					),
					jwtStatus(v1alpha1.JwtStatus{
						IssuedAt:  testingTime(time.Hour * 3 * -1),
						ExpiresAt: testingTime(time.Hour),
						RotationNotBefore: testingTimeWith(
							testutils.Now().Add(time.Hour),
							(time.Minute*80)*-1),
						RotationStrategy: internal.Ptr(v1alpha1.RotationStrategyDynamic),
						RotationDuration: testingDuration(time.Minute * 80),
					}),
				),
			},
		},
		"Resource is Up to date": {
			reason: "We expect token secret to be present and not expired",
			fields: fields{loginClient: mockCertLoginService(false, false, false)},
			args: args{
				mg:      defaultResource("unittest-login"),
				secrets: []corev1.Secret{validTokenSecret},
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				err: nil,
				mg: cr(
					defaultResource("unittest-login"),
					conditions(
						xpv1.Available(),
						v1alpha1.IntrospectOk(),
					),
					jwtStatus(v1alpha1.JwtStatus{
						IssuedAt:  testingTime(time.Hour * -1),
						ExpiresAt: testingTime(time.Hour * 3),
						RotationNotBefore: testingTimeWith(
							testutils.Now().Add(time.Hour*3),
							(time.Minute*80)*-1),
						RotationStrategy: internal.Ptr(v1alpha1.RotationStrategyDynamic),
						RotationDuration: testingDuration(time.Minute * 80),
					}),
				),
			},
		},
		"Resource is Up to date, but token cannot be introspected": {
			reason: "We expect token secret to be present and not expired, but cannot be introspected",
			fields: fields{loginClient: mockCertLoginService(false, false, false)},
			args: args{
				mg:      defaultResource("unittest-login"),
				secrets: []corev1.Secret{validTokenSecretWoIssuedAt},
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				err: nil,
				mg: cr(
					defaultResource("unittest-login"),
					conditions(
						xpv1.Available(),
						v1alpha1.IntrospectError(errors.Wrap(errors.New("could not extract 'iat' from tokens"), errCouldNotJudgeToken).Error()),
					)),
			},
		},
	}

	for _, tc := range cases {

		t.Run(tc.reason, func(t *testing.T) {
			e := external{service: tc.fields.loginClient, kube: ctrloidc.MockCertLookup(tc.args.secrets, nil)}
			got, err := e.Observe(context.Background(), tc.args.mg)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
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

func testingDuration(d time.Duration) *meta_v1.Duration {
	return internal.Ptr(meta_v1.Duration{Duration: d})
}

func TestCreate(t *testing.T) {
	type fields struct {
		loginClient oidc.LoginPerformer
	}
	type args struct {
		mg resource.Managed
	}
	type want struct {
		o   managed.ExternalCreation
		err error
		mg  resource.Managed
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"Login Failure": {
			reason: "Should return error if token dance can't be performed",
			fields: fields{loginClient: mockCertLoginService(false, false, false)},
			args: args{
				mg: defaultResource("unittest-login"),
			},
			want: want{
				o:   managed.ExternalCreation{},
				err: errMockedLogin,
				mg: cr(
					defaultResource("unittest-login"),
					conditions(
						xpv1.Creating(),
					)),
			},
		},
		"Login Successful": {
			reason: "Should return newly created tokenSet",
			fields: fields{loginClient: mockCertLoginService(true, false, false)},
			args: args{
				mg: defaultResource("unittest-login"),
			},
			want: want{
				o: managed.ExternalCreation{ConnectionDetails: managed.ConnectionDetails{
					v1alpha1.ConDetailsIDToken: []byte(validToken.IDToken),
					v1alpha1.ConDetailsRefresh: []byte(validToken.RefreshToken),
				}},
				mg: cr(
					defaultResource("unittest-login"),
					conditions(
						xpv1.Creating(),
					)),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.reason, func(t *testing.T) {
			e := external{service: tc.fields.loginClient}
			got, err := e.Create(context.Background(), tc.args.mg)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Create(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.Create(...): -want, +got:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.mg, tc.args.mg); diff != "" {
				t.Errorf("\n%s\ne.Create(...): -want cr, +got cr:\n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	type fields struct {
		loginClient oidc.LoginPerformer
	}
	type args struct {
		mg      resource.Managed
		secrets []corev1.Secret
	}
	type want struct {
		o   managed.ExternalUpdate
		err error
		mg  resource.Managed
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"Missing token secret": {
			reason: "Should fail the whole update if there is no saved token secret to work with",
			fields: fields{loginClient: mockCertLoginService(false, false, false)},
			args: args{
				mg: defaultResource("unittest-login"),
			},
			want: want{
				o:   managed.ExternalUpdate{},
				err: tokenSecretError(),
				mg:  defaultResource("unittest-login"),
			},
		},
		"Refresh and or recreate token failure": {
			reason: "If first refreshing token then recreating fails, return err from update",
			fields: fields{
				loginClient: mockCertLoginService(false, false, false)},
			args: args{
				mg:      defaultResource("unittest-login"),
				secrets: []corev1.Secret{validTokenSecret},
			},
			want: want{
				o:   managed.ExternalUpdate{},
				err: errMockedLogin,
				mg:  defaultResource("unittest-login"),
			},
		},
		"Refresh failed, but recreate successful": {
			reason: "We should try to recreate token if refresh failed and return it",
			fields: fields{
				loginClient: mockCertLoginService(true, false, false)},
			args: args{
				mg:      defaultResource("unittest-login"),
				secrets: []corev1.Secret{validTokenSecret},
			},
			want: want{
				o: managed.ExternalUpdate{ConnectionDetails: managed.ConnectionDetails{
					v1alpha1.ConDetailsIDToken: []byte(validToken.IDToken),
					v1alpha1.ConDetailsRefresh: []byte(validToken.RefreshToken),
				}},
				mg: defaultResource("unittest-login"),
			},
		},
		"Refresh successful, but recreate successful": {
			reason: "We should return token if refreshing suceeded",
			fields: fields{
				loginClient: mockCertLoginService(false, false, true)},
			args: args{
				mg:      defaultResource("unittest-login"),
				secrets: []corev1.Secret{validTokenSecret},
			},
			want: want{
				o: managed.ExternalUpdate{ConnectionDetails: managed.ConnectionDetails{
					v1alpha1.ConDetailsIDToken: []byte(validToken.IDToken),
					v1alpha1.ConDetailsRefresh: []byte(validToken.RefreshToken),
				}},
				mg: defaultResource("unittest-login"),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.reason, func(t *testing.T) {
			e := external{service: tc.fields.loginClient, kube: ctrloidc.MockCertLookup(tc.args.secrets, nil)}
			got, err := e.Update(context.Background(), tc.args.mg)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Update(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.Update(...): -want, +got:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.mg, tc.args.mg); diff != "" {
				t.Errorf("\n%s\ne.Update(...): -want cr, +got cr:\n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	type fields struct {
		loginClient oidc.LoginPerformer
	}
	type args struct {
		mg      resource.Managed
		secrets []corev1.Secret
	}
	type want struct {
		err           error
		mg            resource.Managed
		cleanedSecret corev1.Secret
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"Delete Token Secret on delete": {
			reason: "We should cleanup the connection details secret before deleting the CR",
			fields: fields{loginClient: mockCertLoginService(false, false, false)},
			args: args{
				mg:      defaultResource("unittest-login"),
				secrets: []corev1.Secret{validTokenSecret},
			},
			want: want{
				mg: cr(
					defaultResource("unittest-login"),
					conditions(
						xpv1.Deleting(),
					)),
				cleanedSecret: validTokenSecret,
			},
		},
		"Succeed without existing token secret as well": {
			reason: "If no token secret exist, we should still gracefully succeed",
			fields: fields{loginClient: mockCertLoginService(false, false, false)},
			args: args{
				mg: defaultResource("unittest-login"),
			},
			want: want{
				mg: cr(
					defaultResource("unittest-login"),
					conditions(
						xpv1.Deleting(),
					)),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.reason, func(t *testing.T) {
			calledDeleteOnName := ""
			e := external{service: tc.fields.loginClient, kube: ctrloidc.MockCertLookup(tc.args.secrets, &calledDeleteOnName)}
			_, err := e.Delete(context.Background(), tc.args.mg)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Delete(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.mg, tc.args.mg); diff != "" {
				t.Errorf("\n%s\ne.Delete(...): -want cr, +got cr:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.cleanedSecret.Name, calledDeleteOnName); diff != "" {
				t.Errorf("\n%s\ne.Delete(...): -want call delete on, +got call delete on:\n%s\n", tc.reason, diff)
			}
		})
	}
}

func testingTime(adjustment time.Duration) *meta_v1.Time {
	t := testutils.Now().Add(adjustment)
	return internal.Ptr(meta_v1.Time{Time: time.Unix(t.Unix(), 0)})
}
func testingTimeWith(base time.Time, adjustment time.Duration) *meta_v1.Time {
	t := base.Add(adjustment)
	return internal.Ptr(meta_v1.Time{Time: time.Unix(t.Unix(), 0)})
}
