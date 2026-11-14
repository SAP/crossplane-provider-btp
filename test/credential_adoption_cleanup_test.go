package test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

const credentialCleanupTranscriptEnv = "BTP_CREDENTIAL_CLEANUP_TRANSCRIPT"

func TestCredentialAdoptionCleanup(t *testing.T) {
	t.Run("detaches from canceled context and continues after errors", func(t *testing.T) {
		parent, cancel := context.WithCancel(context.Background())
		cancel()
		var ran []string
		got := runCredentialAdoptionCleanup(parent, []CredentialAdoptionCleanupAction{
			{Name: "first", Timeout: time.Second, Run: func(ctx context.Context) error {
				ran = append(ran, "first")
				if ctx.Err() != nil {
					t.Fatal("cleanup inherited cancellation")
				}
				return errors.New("injected failure")
			}},
			{Name: "second", Timeout: time.Second, Run: func(context.Context) error {
				ran = append(ran, "second")
				return nil
			}},
		}, 20*time.Second)
		if got == nil || !strings.Contains(got.Error(), "first: injected failure") {
			t.Fatalf("cleanup error = %v, want first action failure", got)
		}
		if strings.Join(ran, ",") != "first,second" {
			t.Fatalf("actions ran in order %v", ran)
		}
	})

	t.Run("bounds individual actions and reports each failure", func(t *testing.T) {
		started := time.Now()
		got := runCredentialAdoptionCleanup(context.Background(), []CredentialAdoptionCleanupAction{
			{Name: "bounded", Timeout: 20 * time.Millisecond, Run: func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			}},
			{Name: "after-timeout", Timeout: time.Second, Run: func(context.Context) error { return errors.New("second failure") }},
		}, 20*time.Second)
		if time.Since(started) > time.Second {
			t.Fatal("cleanup exceeded total budget")
		}
		if got == nil || !strings.Contains(got.Error(), "bounded: context deadline exceeded") || !strings.Contains(got.Error(), "after-timeout: second failure") {
			t.Fatalf("aggregated cleanup error = %v", got)
		}
	})
}

func TestCredentialAdoptionCleanupFailureProcess(t *testing.T) {
	transcript := os.Getenv(credentialCleanupTranscriptEnv)
	if transcript == "" {
		t.Skip("subprocess-only framework control-flow test")
	}
	failFast := os.Getenv(credentialCleanupTranscriptEnv+"_FAIL_FAST") == "1"
	fatalInAssessment := os.Getenv(credentialCleanupTranscriptEnv+"_ASSESSMENT") == "1"
	assessmentError := os.Getenv(credentialCleanupTranscriptEnv+"_ASSESSMENT_ERROR") == "1"
	normalCompletion := os.Getenv(credentialCleanupTranscriptEnv+"_NORMAL") == "1"
	cfg := envconf.New()
	if failFast {
		cfg.WithFailFast()
	}
	feature := features.New("cleanup failure process").Setup(func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
		RegisterCredentialAdoptionCleanup(t, ctx,
			CredentialAdoptionCleanupAction{Name: "first", Timeout: time.Second, Run: appendCleanupTranscript(transcript, "first")},
			CredentialAdoptionCleanupAction{Name: "second", Timeout: time.Second, Run: appendCleanupTranscript(transcript, "second")},
		)
		if !fatalInAssessment && !assessmentError && !normalCompletion {
			t.Fatal("injected setup fatal")
		}
		return ctx
	})
	if fatalInAssessment || assessmentError || normalCompletion {
		feature.Assess("injected assessment outcome", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			if fatalInAssessment {
				t.Fatal("injected assessment fatal")
			}
			if assessmentError {
				t.Error("injected assessment error")
			}
			return ctx
		})
	}
	env.NewWithConfig(cfg).Test(t, feature.Feature())
}

func TestCredentialAdoptionCleanupFrameworkSubprocess(t *testing.T) {
	for _, scenario := range []struct {
		name            string
		failFast        bool
		assessment      bool
		assessmentError bool
		normal          bool
	}{
		{name: "fatal setup"},
		{name: "fatal assessment", assessment: true},
		{name: "fail-fast assessment", failFast: true, assessment: true},
		{name: "assessment error return", assessmentError: true},
		{name: "normal completion", normal: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			transcript := filepath.Join(t.TempDir(), "cleanup.log")
			cmd := exec.Command(os.Args[0], "-test.run=^TestCredentialAdoptionCleanupFailureProcess$")
			cmd.Env = append(os.Environ(), credentialCleanupTranscriptEnv+"="+transcript)
			if scenario.failFast {
				cmd.Env = append(cmd.Env, credentialCleanupTranscriptEnv+"_FAIL_FAST=1")
			}
			if scenario.assessment {
				cmd.Env = append(cmd.Env, credentialCleanupTranscriptEnv+"_ASSESSMENT=1")
			}
			if scenario.assessmentError {
				cmd.Env = append(cmd.Env, credentialCleanupTranscriptEnv+"_ASSESSMENT_ERROR=1")
			}
			if scenario.normal {
				cmd.Env = append(cmd.Env, credentialCleanupTranscriptEnv+"_NORMAL=1")
			}
			err := cmd.Run()
			if scenario.normal && err != nil {
				t.Fatalf("normal child test failed: %v", err)
			}
			if !scenario.normal && err == nil {
				t.Fatal("expected injected child test failure")
			}
			got, err := os.ReadFile(transcript)
			if err != nil {
				t.Fatalf("cleanup transcript missing: %v", err)
			}
			if string(got) != "first\nsecond\n" {
				t.Fatalf("cleanup transcript = %q, want ordered single execution", got)
			}
		})
	}
}

func appendCleanupTranscript(path, entry string) func(context.Context) error {
	return func(context.Context) error {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		_, writeErr := file.WriteString(entry + "\n")
		closeErr := file.Close()
		return errors.Join(writeErr, closeErr)
	}
}
