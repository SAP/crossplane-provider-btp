//go:build upgrade

package upgrade

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/images"
	"github.com/crossplane-contrib/xp-testing/pkg/setup"
	"github.com/crossplane-contrib/xp-testing/pkg/upgrade"
	"github.com/sap/crossplane-provider-btp/test"
	"k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
	"sigs.k8s.io/e2e-framework/support/kind"
)

// CustomUpgradeTestBuilder provides an API for creating custom upgrade tests.
// It allows developers to easily configure upgrade test scenarios with custom versions,
// resource directories, and test phases while minimizing boilerplate code.
//
// Example usage:
//
//	test := NewCustomUpgradeTest("my-custom-test").
//		FromVersion("v1.0.0").
//		ToVersion("v1.1.0").
//		WithResourceDirectories([]string{"./testdata/customCRs"}).
//		WithCustomPreUpgradeAssessment("Verify custom field", assessFunc).
//		Build()
type CustomUpgradeTestBuilder struct {
	testName string

	// Version configuration
	fromTag string
	toTag   string

	// Resource configuration
	resourceDirectories []string

	// Timeout configuration
	verifyTimeout time.Duration
	waitForPause  time.Duration

	// Custom test phases
	preUpgradeAssessments  []phaseFunc
	postUpgradeAssessments []phaseFunc

	// Disable default phases
	skipDefaultResourceVerification bool

	// Configuration
	kindClusterName string
	testenv         env.Environment
}

// phaseFunc represents a test phase function that can be added to the test feature.
type phaseFunc struct {
	description string
	fn          func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context
}

// NewCustomUpgradeTest creates a new CustomUpgradeTestBuilder with the given test name.
// The builder will use baseline defaults from environment variables unless overridden.
//
// Example:
//
//	builder := NewCustomUpgradeTest("test-external-name-migration")
func NewCustomUpgradeTest(testName string) *CustomUpgradeTestBuilder {
	testenv := env.New()

	return &CustomUpgradeTestBuilder{
		testName:               testName,
		resourceDirectories:    []string{},
		preUpgradeAssessments:  []phaseFunc{},
		postUpgradeAssessments: []phaseFunc{},
		testenv:                testenv,
		waitForPause:           globalWaitForPause,
		verifyTimeout:          globalVerifyTimeout,
	}
}

// FromVersion sets the source version for the upgrade test.
// Can be set to "local" to use the locally built provider.
func (b *CustomUpgradeTestBuilder) FromVersion(version string) *CustomUpgradeTestBuilder {
	b.fromTag = version
	return b
}

// ToVersion sets the target version for the upgrade test.
// Can be set to "local" to use the locally built provider.
func (b *CustomUpgradeTestBuilder) ToVersion(version string) *CustomUpgradeTestBuilder {
	b.toTag = version
	return b
}

// WithResourceDirectories sets the directories containing test resources to be used in the upgrade test.
// If not set, the baseline resource directories will be used.
//
// Example:
//
//	builder.WithResourceDirectories([]string{
//	    "./testdata/customCRs/subaccount",
//	    "./testdata/customCRs/directory",
//	})
func (b *CustomUpgradeTestBuilder) WithResourceDirectories(dirs []string) *CustomUpgradeTestBuilder {
	b.resourceDirectories = dirs
	return b
}

// WithVerifyTimeout sets the timeout duration for resource verification.
// If not set, the value from UPGRADE_TEST_VERIFY_TIMEOUT or default (30 minutes) will be used.
func (b *CustomUpgradeTestBuilder) WithVerifyTimeout(timeout time.Duration) *CustomUpgradeTestBuilder {
	b.verifyTimeout = timeout
	return b
}

// WithWaitForPause sets the duration to wait for resources to pause during upgrade.
// If not set, the value from UPGRADE_TEST_WAIT_FOR_PAUSE or default (1 minute) will be used.
func (b *CustomUpgradeTestBuilder) WithWaitForPause(duration time.Duration) *CustomUpgradeTestBuilder {
	b.waitForPause = duration
	return b
}

// WithCustomPreUpgradeAssessment adds a custom assessment phase that runs before the upgrade.
// This can be used to verify specific conditions or resource states before upgrading.
//
// Example:
//
//	builder.WithCustomPostUpgradeAssessment("Verify external names", assertFunc)
func (b *CustomUpgradeTestBuilder) WithCustomPreUpgradeAssessment(
	description string,
	fn func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context,
) *CustomUpgradeTestBuilder {
	b.preUpgradeAssessments = append(b.preUpgradeAssessments, phaseFunc{description: description, fn: fn})
	return b
}

// WithCustomPostUpgradeAssessment adds a custom assessment phase that runs after the upgrade.
// This can be used to verify migration outcomes or new behavior in the upgraded version.
//
// Example:
//
//	builder.WithCustomPostUpgradeAssessment("Verify migrated external names", assertFunc)
func (b *CustomUpgradeTestBuilder) WithCustomPostUpgradeAssessment(
	description string,
	fn func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context,
) *CustomUpgradeTestBuilder {
	b.postUpgradeAssessments = append(b.postUpgradeAssessments, phaseFunc{description: description, fn: fn})
	return b
}

// SkipDefaultResourceVerification disables the default resource verification phases.
// This means that no checks are being carried out by default before and after upgrading the provider.
// Custom verification phases can be added using WithCustomPreUpgradeAssessment.
//
// The function that would otherwise be used is upgrade.VerifyResources(upgradeTest.ResourceDirectories, verifyTimeout).
func (b *CustomUpgradeTestBuilder) SkipDefaultResourceVerification() *CustomUpgradeTestBuilder {
	b.skipDefaultResourceVerification = true
	return b
}

// Feature constructs the upgrade test feature from the builder configuration.
// It resolves all configuration values (using defaults where not explicitly set),
// builds the test phases in the correct order, and returns a features.Feature ready for execution.
//
// The test phases are executed in this order:
//  1. Provider installation
//  2. Resource import
//  3. Pre-upgrade verification (unless skipped)
//  4. Provider upgrade
//  5. Post-upgrade verification (unless skipped)
//  6. Custom post-upgrade assessments
//  7. Resource cleanup
//  8. Provider cleanup
//
// Example:
//
//	feature := builder.Feature()
//	testenv.Test(t, feature)
func (b *CustomUpgradeTestBuilder) Feature() features.Feature {
	if b.fromTag == "" || b.toTag == "" {
		panic("Both fromTag and toTag must be specified before building an upgrade test feature")
	}

	fromProviderPackage, toProviderPackage, fromControllerPackage, toControllerPackage := loadPackages(b.fromTag, b.toTag)

	upgradeTest := upgrade.UpgradeTest{
		ProviderName:        providerName,
		ClusterName:         b.kindClusterName,
		FromProviderPackage: fromProviderPackage,
		ToProviderPackage:   toProviderPackage,
		ResourceDirectories: b.resourceDirectories,
	}

	featureName := fmt.Sprintf("%s: Upgrade %s from %s to %s", b.testName, providerName, b.fromTag, b.toTag)
	feature := features.New(featureName)

	b.setupClusterWithCrossplane(
		fromProviderPackage,
		fromControllerPackage,
		feature,
	)

	feature.
		WithSetup(
			"Install provider with version "+b.fromTag,
			upgrade.ApplyProvider(upgradeTest.ClusterName, upgradeTest.FromProviderInstallOptions()),
		).
		WithSetup(
			"Import resources from directories",
			upgrade.ImportResources(upgradeTest.ResourceDirectories),
		)

	if !b.skipDefaultResourceVerification {
		feature = feature.Assess(
			"Verify resources before upgrade",
			upgrade.VerifyResources(upgradeTest.ResourceDirectories, b.verifyTimeout),
		)
	}

	// Add custom pre-upgrade assessments
	for _, phase := range b.preUpgradeAssessments {
		feature = feature.Assess(phase.description, phase.fn)
	}

	feature = feature.Assess(
		"Upgrade provider to version "+b.toTag,
		upgrade.UpgradeProvider(upgrade.UpgradeProviderOptions{
			ClusterName: upgradeTest.ClusterName,
			ProviderOptions: test.InstallProviderOptionsWithController(
				upgradeTest.ToProviderInstallOptions(),
				toControllerPackage,
			),
			ResourceDirectories: upgradeTest.ResourceDirectories,
			WaitForPause:        b.waitForPause,
		}),
	)

	if !b.skipDefaultResourceVerification {
		feature = feature.Assess(
			"Verify resources after upgrade",
			upgrade.VerifyResources(upgradeTest.ResourceDirectories, b.verifyTimeout),
		)
	}

	// Add custom post-upgrade assessments
	for _, phase := range b.postUpgradeAssessments {
		feature = feature.Assess(phase.description, phase.fn)
	}

	feature = feature.WithTeardown(
		"Delete resources",
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			err := test.DeleteResourcesFromDirsGracefully(
				ctx,
				cfg,
				b.resourceDirectories,
				wait.WithTimeout(b.verifyTimeout),
			)
			if err != nil {
				t.Logf("failed to clean up resources: %v", err)
			}
			return ctx
		},
	)

	feature = feature.WithTeardown(
		"Delete provider",
		upgrade.DeleteProvider(upgradeTest.ProviderName),
	)

	return feature.Feature()
}

func (b *CustomUpgradeTestBuilder) setupClusterWithCrossplane(
	fromProviderPackage, fromControllerPackage string,
	featureBuilder *features.FeatureBuilder,
) {
	namespace := envconf.RandomName(namespacePrefix, 16)

	deploymentRuntimeConfig := test.DeploymentRuntimeConfig(providerName)

	cfg := setup.ClusterSetup{
		ProviderName: providerName,
		Images: images.ProviderImages{
			Package:         fromProviderPackage,
			ControllerImage: &fromControllerPackage,
		},
		CrossplaneSetup: setup.CrossplaneSetup{
			Version:  crossplaneVersion,
			Registry: setup.DockerRegistry,
		},
		DeploymentRuntimeConfig: &deploymentRuntimeConfig,
	}

	cfg.PostCreate(func(clusterName string) env.Func {
		b.kindClusterName = clusterName
		return func(ctx context.Context, config *envconf.Config) (context.Context, error) {
			klog.V(4).Infof("Upgrade cluster %s has been created", clusterName)
			return ctx, nil
		}
	})

	// Problem:
	//   - testenv.Setup() steps are only being executed in testenv.Run()
	//   - testenv.Run() takes a *testing.M as a parameter (cannot be called here)
	//   - xp-testing uses testenv.Setup() to create the cluster in setup.ClusterSetup.Configure()
	//
	// TLDR: This still uses testenv.Setup, which will not run
	_ = cfg.Configure(b.testenv, &kind.Cluster{})

	featureBuilder.WithSetup(
		"Apply provider config and credentials",
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			ctx, err := test.ApplySecretInCrossplaneNamespace(cisSecretName, bindingSecretData)(ctx, cfg)
			if err != nil {
				t.Fatalf("Failed to apply CIS secret: %v", err)
			}

			ctx, err = test.ApplySecretInCrossplaneNamespace(serviceUserSecretName, userSecretData)(ctx, cfg)
			if err != nil {
				t.Fatalf("Failed to apply Service User secret: %v", err)
			}

			ctx, err = test.CreateProviderConfigFn(
				namespace,
				globalAccount,
				cliServerUrl,
				cisSecretName,
				serviceUserSecretName,
			)(ctx, cfg)

			return ctx
		},
	)
}

func loadPackages(fromTag, toTag string) (string, string, string, string) {
	return test.LoadUpgradePackages(
		fromTag, toTag,
		fromProviderRepository, toProviderRepository, fromControllerRepository, toControllerRepository,
		uutImagesEnvVar, localTagName,
	)
}
