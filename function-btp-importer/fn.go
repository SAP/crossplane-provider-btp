package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/crossplane/crossplane-runtime/v2/pkg/fieldpath"
	"github.com/crossplane/function-sdk-go/errors"
	"github.com/crossplane/function-sdk-go/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/response"

	"github.com/sap/crossplane-provider-btp/function-btp-importer/input/v1beta1"
	"github.com/sap/crossplane-provider-btp/function-btp-importer/internal/btp"
)

const (
	// cisSecretKey is the key used in rsp.Requirements.Resources and
	// request.GetRequiredResources to identify the CIS provider secret.
	cisSecretKey = "cis-secret"

	// kindSecret is the core/v1 kind of the CIS provider secret this function
	// requires.
	kindSecret = "Secret"

	// conditionImportsVerified is the claim-visible condition reporting the
	// identity-verification outcome. Check-framed positive polarity: True
	// means no name match was refused this pass.
	conditionImportsVerified = "ImportsVerified"

	reasonIdentityMismatch  = "IdentityMismatch"
	reasonLookupErrors      = "LookupErrors"
	reasonResourcesImported = "ResourcesImported"
	reasonNothingMatched    = "NothingMatched"
	reasonNothingToImport   = "NothingToImport"
)

// Function implements the BTP importer Crossplane composition function.
// It automatically sets crossplane.io/external-name on desired composed
// resources by querying the BTP API, enabling adoption of existing resources.
type Function struct {
	fnv1.UnimplementedFunctionRunnerServiceServer

	log logging.Logger
}

// RunFunction runs the Function.
func (f *Function) RunFunction(ctx context.Context, req *fnv1.RunFunctionRequest) (*fnv1.RunFunctionResponse, error) {
	rsp := response.To(req, response.DefaultTTL)

	oxr, err := request.GetObservedCompositeResource(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrap(err, "cannot get observed composite resource"))
		return rsp, nil
	}

	f.log.Debug("Running function",
		"tag", req.GetMeta().GetTag(),
		"apiVersion", oxr.Resource.GetObjectKind().GroupVersionKind().GroupVersion().String(),
		"kind", oxr.Resource.GetObjectKind().GroupVersionKind().Kind,
		"name", oxr.Resource.GetName(),
	)

	in := &v1beta1.Input{}
	if err := request.GetInput(req, in); err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot get Function input from %T", req))
		return rsp, nil
	}

	// Resolve the CIS secret name and declare the requirement unconditionally
	// on every pass — mirrors the pattern used by function-environment-configs
	// and function-extra-resources, and keeps the response stable across
	// iterations for crossplane render stabilisation detection.
	secretName, secretNamespace, ok := resolveCISSecret(req, rsp, in, oxr)
	if !ok {
		return rsp, nil
	}
	rsp.Requirements = &fnv1.Requirements{
		Resources: map[string]*fnv1.ResourceSelector{
			cisSecretKey: {
				ApiVersion: "v1",
				Kind:       kindSecret,
				Namespace:  &secretNamespace,
				Match:      &fnv1.ResourceSelector_MatchName{MatchName: secretName},
			},
		},
	}

	// Pass 1: RequiredResources is nil — the secret hasn't been resolved yet.
	if req.GetRequiredResources() == nil {
		f.log.Debug("Pass 1: CIS secret requirement declared", "secretName", secretName, "secretNamespace", secretNamespace)
		return rsp, nil
	}

	// Pass 2: secret is present — extract credentials and resolve external names.
	required, err := request.GetRequiredResources(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrap(err, "cannot get required resources"))
		return rsp, nil
	}

	items := required[cisSecretKey]
	if len(items) == 0 {
		response.Fatal(rsp, errors.Errorf("CIS provider secret not found in cluster (key: %q)", cisSecretKey))
		return rsp, nil
	}

	// MatchName selector guarantees at most one result.
	secretRequired := items[0]

	credentialKey := in.SecretRef.Key
	rawCreds, err := fieldpath.Pave(secretRequired.Resource.Object).GetString("data." + credentialKey)
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot read data.%s from CIS secret", credentialKey))
		return rsp, nil
	}

	creds, err := btp.ParseCISCredentials(rawCreds)
	if err != nil {
		response.Fatal(rsp, errors.Wrap(err, "cannot parse CIS credentials"))
		return rsp, nil
	}

	f.log.Info("Pass 2: CIS credentials resolved", "subaccountID", creds.UAA.SubaccountID)

	desired, err := request.GetDesiredComposedResources(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrap(err, "cannot get desired composed resources"))
		return rsp, nil
	}

	observed, err := request.GetObservedComposedResources(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrap(err, "cannot get observed composed resources"))
		return rsp, nil
	}

	r, err := newResolver(ctx, resolveConfig{
		Creds:    creds,
		Observed: observed,
		Desired:  desired,
		Input:    in,
		Log:      f.log,
		Warn:     func(err error) { response.Warning(rsp, err) },
	})
	if err != nil {
		response.Fatal(rsp, err)
		return rsp, nil
	}
	defer func() {
		if err := r.Close(); err != nil {
			f.log.Info("SM admin binding cleanup failed; existing binding will be reused (not deleted) on future reconciles",
				"error", err)
		}
	}()

	summary := r.resolveAll(ctx, desired)

	f.log.Debug("Pass 2 complete",
		"total", summary.total,
		"imported", summary.imported,
		"propagated", summary.propagated,
		"skipped", summary.skipped,
		"noMatches", summary.noMatches,
		"warnings", summary.warnings,
		"identityMismatch", summary.identityMismatch,
	)

	setImportsVerifiedCondition(rsp, summary)

	if err := response.SetDesiredComposedResources(rsp, desired); err != nil {
		response.Fatal(rsp, errors.Wrap(err, "cannot set desired composed resources"))
		return rsp, nil
	}

	return rsp, nil
}

// setImportsVerifiedCondition surfaces the identity-verification outcome on
// the XR and the claim. The condition is level-based — recomputed from this
// pass alone — so a block clears on the reconcile after the user fixes the
// claim, and a past block never lingers. It complements the provider's 409
// create-loop error by explaining why an existing resource was not adopted.
func setImportsVerifiedCondition(rsp *fnv1.RunFunctionResponse, summary resolveSummary) {
	switch {
	case summary.identityMismatch > 0:
		response.ConditionFalse(rsp, conditionImportsVerified, reasonIdentityMismatch).
			WithMessage(strings.Join(summary.blocked, "; ")).
			TargetCompositeAndClaim()
	case summary.warnings > 0:
		response.ConditionUnknown(rsp, conditionImportsVerified, reasonLookupErrors).
			WithMessage(fmt.Sprintf("%d lookup(s) failed; identity verification incomplete this pass", summary.warnings)).
			TargetCompositeAndClaim()
	case summary.imported > 0:
		response.ConditionTrue(rsp, conditionImportsVerified, reasonResourcesImported).
			WithMessage(fmt.Sprintf("%d existing BTP resource(s) adopted", summary.imported)).
			TargetCompositeAndClaim()
	case summary.noMatches > 0:
		response.ConditionTrue(rsp, conditionImportsVerified, reasonNothingMatched).
			WithMessage("no existing BTP resources matched; the provider will create them").
			TargetCompositeAndClaim()
	default:
		response.ConditionTrue(rsp, conditionImportsVerified, reasonNothingToImport).
			TargetCompositeAndClaim()
	}
}

// resolveCISSecret resolves the CIS provider secret name and namespace from the
// input configuration. Returns (name, namespace, true) on success, or
// ("", "", false) after writing a Fatal to rsp on failure.
func resolveCISSecret(req *fnv1.RunFunctionRequest, rsp *fnv1.RunFunctionResponse, in *v1beta1.Input, oxr *resource.Composite) (string, string, bool) {
	name, err := resolveValue(in.SecretRef.Name, oxr, req)
	if err != nil {
		response.Fatal(rsp, errors.Wrap(err, "cannot resolve CIS secret name"))
		return "", "", false
	}

	namespace, err := resolveValue(in.SecretRef.Namespace, oxr, req)
	if err != nil {
		response.Fatal(rsp, errors.Wrap(err, "cannot resolve CIS secret namespace"))
		return "", "", false
	}

	return name, namespace, true
}

// resolveValue resolves a ValueSource to a concrete string using the observed
// XR and pipeline context. Exactly one source must be set — anything else is
// ambiguous and refused, matching the schema's CEL rule for pipelines that
// validate and guarding the ones that don't.
func resolveValue(src v1beta1.ValueSource, oxr *resource.Composite, req *fnv1.RunFunctionRequest) (string, error) {
	set := 0
	for _, p := range []bool{src.Value != nil, src.FromFieldPath != nil, src.FromContextKey != nil} {
		if p {
			set++
		}
	}
	if set > 1 {
		return "", errors.New("exactly one of value, fromFieldPath, or fromContextKey must be set; found multiple")
	}

	if src.Value != nil {
		return *src.Value, nil
	}
	if src.FromFieldPath != nil {
		val, err := fieldpath.Pave(oxr.Resource.Object).GetString(*src.FromFieldPath)
		if err != nil {
			return "", errors.Wrapf(err, "cannot resolve field path %q", *src.FromFieldPath)
		}
		return val, nil
	}
	if src.FromContextKey != nil {
		val, ok := request.GetContextKey(req, *src.FromContextKey)
		if !ok {
			return "", errors.Errorf("context key %q not found", *src.FromContextKey)
		}
		s := val.GetStringValue()
		if s == "" {
			return "", errors.Errorf("context key %q resolved to empty string", *src.FromContextKey)
		}
		return s, nil
	}
	return "", errors.New("one of value, fromFieldPath, or fromContextKey must be set")
}
