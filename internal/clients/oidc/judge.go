package oidc

import (
	"encoding/json"
	"errors"
	"time"

	jwt "github.com/golang-jwt/jwt/v4"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sap/crossplane-provider-btp/apis/oidc/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
)

type JwtJudge struct {
	idToken string
	claims  jwt.MapClaims
}

func NewJwtJudge(idToken string) (*JwtJudge, error) {
	claims, err := parseClaims(idToken)
	if err != nil {
		return nil, err
	}
	return &JwtJudge{idToken: idToken, claims: claims}, nil
}

func (judge *JwtJudge) EstimateRotationDuration() (*time.Duration, error) {

	exp, ok := judge.extractExpiryInSeconds()
	if !ok {
		return nil, errors.New(errCouldNotExtractExpiry)
	}
	iat, ok := judge.extractIssuedAtInSeconds()

	if !ok {
		return nil, errors.New(errCouldNotExtractIssuedAt)
	}
	expiryAt := time.Unix(exp, 0)
	issuedAt := time.Unix(iat, 0)
	jwtDuration := expiryAt.Sub(issuedAt)

	rotationDuration := time.Duration(jwtDuration / 3)
	if rotationDuration < 0 {
		return nil, errors.New(errDurationNegative)
	}
	return &rotationDuration, nil
}

// IsInRenewPeriod returns if a jwt should be rotated
func (judge *JwtJudge) IsInRenewPeriod(duration time.Duration) bool {
	if judge.claims.Valid() != nil {
		return true
	}

	now := time.Unix(time.Now().Unix(), 0)
	expiry, ok := judge.extractExpiryInSeconds()
	if !ok {
		return false
	}
	renewStart := judge.renewStarttime(duration, expiry)

	return now.After(renewStart)
}

func (judge *JwtJudge) renewStarttime(duration time.Duration, expiry int64) time.Time {
	expiryTime := time.Unix(expiry, 0)
	renewStart := expiryTime.Add(duration * -1)
	return renewStart
}

// extractExpiryInSeconds extract `exp` from jwt. By RFC(https://www.rfc-editor.org/rfc/rfc7519#section-2) this is "Seconds Since the Epoch"
func (judge *JwtJudge) extractExpiryInSeconds() (int64, bool) {
	exp, ok := judge.claims["exp"]
	if !ok {
		return 0, false
	}
	switch expType := exp.(type) {
	case float64:
		return int64(expType), true
	case json.Number:
		v, _ := expType.Int64()
		return v, true
	}
	return 0, false
}

// extractExpiryInSeconds extract `iat` from jwt. By RFC(https://www.rfc-editor.org/rfc/rfc7519#section-2) this is "Seconds Since the Epoch"
func (judge *JwtJudge) extractIssuedAtInSeconds() (int64, bool) {
	exp, ok := judge.claims["iat"]
	if !ok {
		return 0, false
	}
	switch expType := exp.(type) {
	case float64:
		return int64(expType), true
	case json.Number:
		v, _ := expType.Int64()
		return v, true
	}
	return 0, false
}

func (judge *JwtJudge) Status() v1alpha1.JwtStatus {
	return v1alpha1.JwtStatus{
		Issuer:            judge.issuer(),
		IssuedAt:          judge.iatTime(),
		ExpiresAt:         judge.expTime(),
		RotationNotBefore: judge.rotationNotBefore(),
		RotationDuration:  judge.rotationDuration(),
		RotationStrategy:  internal.Ptr(v1alpha1.RotationStrategyDynamic),
	}
}

func (judge *JwtJudge) issuer() *string {

	issuer, ok := judge.claims["iss"]
	if !ok {
		return nil
	}
	switch expType := issuer.(type) {
	case string:
		return &expType
	}
	return nil
}
func (judge *JwtJudge) iatTime() *metav1.Time {
	seconds, ok := judge.extractIssuedAtInSeconds()
	if !ok {
		return nil
	}
	t := metav1.Time{Time: time.Unix(seconds, 0)}

	return &t
}
func (judge *JwtJudge) expTime() *metav1.Time {
	seconds, ok := judge.extractExpiryInSeconds()
	return toMetav1TimePtr(ok, seconds)
}

func (judge *JwtJudge) rotationDuration() *metav1.Duration {
	duration, err := judge.EstimateRotationDuration()
	if err != nil {
		return nil
	}
	metaDuration := metav1.Duration{Duration: *duration}
	return &metaDuration
}

func (judge *JwtJudge) rotationNotBefore() *metav1.Time {
	duration, err := judge.EstimateRotationDuration()
	if err != nil {
		return nil
	}
	expiry, ok := judge.extractExpiryInSeconds()
	if !ok {
		return nil
	}
	starttime := judge.renewStarttime(*duration, expiry)
	metaTime := metav1.Time{Time: starttime}
	return &metaTime
}

func toMetav1TimePtr(ok bool, seconds int64) *metav1.Time {
	if !ok {
		return nil
	}
	t := metav1.Time{Time: time.Unix(seconds, 0)}

	return &t
}
