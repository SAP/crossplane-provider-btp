package destination

import (
	"errors"
	"strings"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
)

func TestSubaccountFromJWT(t *testing.T) {
	cases := map[string]struct{ token, want string }{
		"claim present":        {jwtWith(`{"ext_attr":{"subaccountid":"sub-1"},"zid":"zone"}`), "sub-1"},
		"claim absent, no zid": {jwtWith(`{"zid":"zone"}`), ""},
		"padded payload":       {strings.Replace(jwtWith(`{"ext_attr":{"subaccountid":"s"}}`), ".sig", "", 1) + "==.sig", "s"},
		"two parts":            {"a.b", ""},
		"bad base64":           {"a.!!!.c", ""},
		"bad json":             {"a." + "bm90LWpzb24" + ".c", ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := subaccountFromJWT(tc.token); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCheckBindingSubaccount(t *testing.T) {
	cases := map[string]struct {
		token, subaccountID, extName string
		wantErr                      bool
		wantReason                   event.Reason
	}{
		"match":                           {"sub-a", "sub-a", "sub-a/x", false, ""},
		"match ignores case and space":    {"SUB-A", " sub-a ", "sub-a/x", false, ""},
		"subaccountId unset":              {"sub-a", "", "", false, ""},
		"binding of other subaccount":     {"sub-b", "sub-a", "sub-a/x", true, ""},
		"claim missing":                   {"", "sub-a", "sub-a/x", false, ReasonBindingSubaccountUnverified},
		"claim missing, mismatch unknown": {"", "sub-a", "sub-b/x", false, ReasonBindingSubaccountUnverified},
		"prefix differs from token":       {"sub-a", "sub-a", "typo/x", false, ReasonExternalNameSubaccountMismatch},
		"external name without prefix":    {"sub-a", "sub-a", "x", false, ""},
		"external name empty":             {"sub-a", "sub-a", "", false, ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			events, err := CheckBindingSubaccount(tc.token, tc.subaccountID, tc.extName)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr && !strings.Contains(err.Error(), tc.token) {
				t.Errorf("error %q does not name the binding's subaccount", err)
			}
			var got event.Reason
			if len(events) == 1 {
				got = events[0].Reason
			} else if len(events) > 1 {
				t.Fatalf("events = %v, want at most one", events)
			}
			if got != tc.wantReason {
				t.Errorf("reason = %q, want %q", got, tc.wantReason)
			}
		})
	}
}

type fakeTokenSubaccounter struct {
	sub   string
	err   error
	calls int
}

func (f *fakeTokenSubaccounter) TokenSubaccount() (string, error) { f.calls++; return f.sub, f.err }

type recorderFake struct{ events []event.Event }

func (r *recorderFake) Event(_ runtime.Object, e event.Event)    { r.events = append(r.events, e) }
func (r *recorderFake) WithAnnotations(...string) event.Recorder { return r }

func crWith(extName string, deleted bool) *v1alpha1.SubaccountDestination {
	cr := &v1alpha1.SubaccountDestination{ObjectMeta: metav1.ObjectMeta{Name: "d"}}
	if extName != "" {
		meta.SetExternalName(cr, extName)
	}
	if deleted {
		now := metav1.Now()
		cr.SetDeletionTimestamp(&now)
	}
	return cr
}

func TestVerifyBinding(t *testing.T) {
	sub := "sub-a"

	t.Run("deleting a never-created CR fetches no token", func(t *testing.T) {
		ts := &fakeTokenSubaccounter{err: errors.New("token endpoint down")}
		if err := VerifyBinding(ts, crWith("", true), &sub, nil); err != nil || ts.calls != 0 {
			t.Fatalf("err = %v, calls = %d", err, ts.calls)
		}
	})
	t.Run("token fetch failure is an error", func(t *testing.T) {
		ts := &fakeTokenSubaccounter{err: errors.New("boom")}
		if err := VerifyBinding(ts, crWith("sub-a/d", false), &sub, nil); err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("warning is recorded on the CR", func(t *testing.T) {
		rec := &recorderFake{}
		ts := &fakeTokenSubaccounter{sub: ""}
		if err := VerifyBinding(ts, crWith("sub-a/d", false), &sub, rec); err != nil {
			t.Fatal(err)
		}
		if len(rec.events) != 1 || rec.events[0].Type != event.TypeWarning {
			t.Fatalf("events = %+v", rec.events)
		}
	})
	t.Run("mismatch blocks deletion of an existing resource", func(t *testing.T) {
		ts := &fakeTokenSubaccounter{sub: "sub-b"}
		if err := VerifyBinding(ts, crWith("sub-a/d", true), &sub, nil); err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("mismatch blocks the first create", func(t *testing.T) {
		ts := &fakeTokenSubaccounter{sub: "sub-b"}
		if err := VerifyBinding(ts, crWith("", false), &sub, nil); err == nil {
			t.Fatal("want error")
		}
	})
}
