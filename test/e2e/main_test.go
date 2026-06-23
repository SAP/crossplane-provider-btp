//go:build e2e || e2e_long

package e2e

import (
	"context"
	"os"
	"testing"

	"github.com/crossplane-contrib/xp-testing/pkg/envvar"
	"github.com/crossplane-contrib/xp-testing/pkg/vendored"
	"github.com/crossplane-contrib/xp-testing/pkg/xpenvfuncs"
	testutil "github.com/sap/crossplane-provider-btp/test"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/third_party/kind"
)

var (
	UUT_IMAGES_KEY = "UUT_IMAGES"

	CIS_SECRET_NAME              = "cis-provider-secret"
	SERVICE_USER_SECRET_NAME     = "sa-provider-secret"
	TECHNICAL_USER_EMAIL_ENV_KEY = "TECHNICAL_USER_EMAIL"
	SECONDARY_DIRC_ADMIN_ENV_KEY = "SECOND_DIRECTORY_ADMIN_EMAIL"

	UUT_BUILD_ID_KEY = "BUILD_ID"
)

const (
	// crossplaneChartPathEnv names the env var that points at a local
	// Crossplane chart tarball (e.g. /path/to/crossplane-2.1.3.tgz). When
	// set, the test installs from the local file (avoiding the per-test
	// `helm repo add charts.crossplane.io/stable` network call which has
	// been throwing intermittent 403 Forbidden errors; see
	// docs/development/flakiness-analysis.md, Pattern A). When unset, the
	// test falls back to xp-testing's bundled helm-repo install
	// (xpenvfuncs.InstallCrossplane).
	//
	// The fallback is required because this PR's workflow uses
	// `pull_request_target`, which means GitHub runs the workflow file
	// from the BASE branch (main), not from the PR head — so the
	// chart-tarball export step doesn't run on this PR's own CI. Once the
	// PR merges and main's `Resolve Crossplane chart path` step in
	// `e2e_test.yaml` exports the env var, every matrix leg automatically
	// switches to the chart-tarball path. The fallback also lets local
	// developers run e2e tests without pre-staging the chart.
	crossplaneChartPathEnv = "CROSSPLANE_CHART_PATH"
	// crossplaneVersion is the Crossplane chart version. Must match the
	// version that was pulled into the tarball at CROSSPLANE_CHART_PATH.
	crossplaneVersion = "2.1.3"

	// reuseClusterEnv mirrors xp-testing's E2E_REUSE_CLUSTER semantics: when
	// set, the cluster, Crossplane, and provider are reused across runs and
	// the cluster is not destroyed at teardown.
	reuseClusterEnv = "E2E_REUSE_CLUSTER"
	// clusterNameEnv mirrors xp-testing's E2E_CLUSTER_NAME: when set, it
	// overrides the generated cluster name.
	clusterNameEnv = "E2E_CLUSTER_NAME"
	// defaultClusterPrefix matches xp-testing's defaultPrefix used for the
	// reused-cluster name.
	defaultClusterPrefix = "e2e"
)

var (
	testenv  env.Environment
	BUILD_ID string
)

func TestMain(m *testing.M) {
	var verbosity = 4
	testutil.SetupLogging(verbosity)

	namespace := envconf.RandomName("test-ns", 16)

	SetupClusterWithCrossplane(namespace)

	os.Exit(testenv.Run(m))
}

func SetupClusterWithCrossplane(namespace string) {
	// e.g. pr-16-3... defaults to empty string if not set
	BUILD_ID = envvar.Get(UUT_BUILD_ID_KEY)

	uutImages := envvar.GetOrPanic(UUT_IMAGES_KEY)
	uutConfig, uutController := testutil.GetImagesFromJsonOrPanic(uutImages)

	testenv = env.New()

	bindingSecretData := testutil.GetBindingSecretOrPanic()
	userSecretData := testutil.GetUserSecretOrPanic()
	globalAccount := envvar.GetOrPanic("GLOBAL_ACCOUNT")
	cliServerUrl := envvar.GetOrPanic("CLI_SERVER_URL")

	// Setup uses pre-defined funcs to create kind cluster
	// and create a namespace for the environment

	deploymentRuntimeConfig := vendored.DeploymentRuntimeConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "btp-provider-runtime-config",
		},
		Spec: vendored.DeploymentRuntimeConfigSpec{
			DeploymentTemplate: &vendored.DeploymentTemplate{
				Spec: &appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "package-runtime",
									Args: []string{"--debug", "--sync=10s"},
								},
							},
						},
					},
				},
			},
		},
	}

	// Replicate the slice of setup.ClusterSetup.Configure that we need, swapping
	// xp-testing's bundled InstallCrossplane (which goes through
	// `helm repo add https://charts.crossplane.io/stable`) for an install from
	// a chart reference passed via CROSSPLANE_CHART_PATH when available, and
	// falling back to the bundled helm-repo install when it's not.
	//
	// xp-testing v1.9.2's setup.ClusterSetup.Configure pinned to the helm-repo
	// install path with no opt-out. PR crossplane-contrib/xp-testing#134 adds a
	// ChartRef field, but it sits on xp-testing main which has bumped k8s deps
	// to v0.36 incompatibly with this repo's crossplane-runtime/v2 v2.2.0-rc.0.
	// Replicating inline keeps the dep tree intact.
	clusterName := resolveClusterName()
	reuseCluster := envvar.CheckEnvVarExists(reuseClusterEnv)
	// Resolve the install func to use on a fresh cluster. On reuse runs the
	// cluster already has Crossplane wired up, so we skip both paths via the
	// xpenvfuncs.Conditional below.
	var installCrossplaneFunc env.Func
	if !reuseCluster {
		if chartRef := os.Getenv(crossplaneChartPathEnv); chartRef != "" {
			installCrossplaneFunc = testutil.InstallCrossplaneFromChart(clusterName, chartRef, crossplaneVersion)
		} else {
			// Fallback: xp-testing's bundled helm-repo install. Used (a) on
			// this PR's own CI before the chart-tarball workflow lands on
			// main (pull_request_target runs base-branch workflows, so the
			// PR's chart-export step doesn't run), and (b) for local dev
			// when the chart isn't pre-staged.
			installCrossplaneFunc = xpenvfuncs.InstallCrossplane(
				clusterName,
				xpenvfuncs.Version(crossplaneVersion),
			)
		}
	}

	testenv.Setup(
		xpenvfuncs.ValidateTestSetup(xpenvfuncs.ValidateTestSetupOptions{
			CrossplaneVersion: crossplaneVersion,
		}),
		envfuncs.CreateCluster(&kind.Cluster{}, clusterName),
		// Conditional matches Configure's behavior: when reusing an existing
		// cluster we skip the install steps. We always install on a fresh
		// cluster (firstSetup=true) and skip when reusing. We do not have a
		// cheap way to detect "cluster exists" without the gexe shell out
		// xp-testing uses, so we approximate: reuseCluster off => install,
		// reuseCluster on => skip and trust the previous run wired things up.
		xpenvfuncs.Conditional(
			xpenvfuncs.Compose(
				installCrossplaneFunc,
				xpenvfuncs.InstallCrossplaneProvider(
					clusterName, xpenvfuncs.InstallCrossplaneProviderOptions{
						Name:                    "btp-account",
						Package:                 uutConfig,
						ControllerImage:         &uutController,
						DeploymentRuntimeConfig: &deploymentRuntimeConfig,
					}),
			), !reuseCluster),
		xpenvfuncs.ApplyProviderConfigFromDir("./provider"),
		xpenvfuncs.LoadSchemas(),
		xpenvfuncs.AwaitCRDsEstablished,
		envfuncs.CreateNamespace(namespace),
		func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
			cfg.WithNamespace(namespace)
			return ctx, nil
		},
		testutil.ApplySecretInCrossplaneNamespace(CIS_SECRET_NAME, bindingSecretData),
		testutil.ApplySecretInCrossplaneNamespace(SERVICE_USER_SECRET_NAME, userSecretData),
		testutil.CreateProviderConfigEnvFn(namespace, globalAccount, cliServerUrl, CIS_SECRET_NAME, SERVICE_USER_SECRET_NAME),
	)

	testenv.Finish(
		xpenvfuncs.DumpLogs(clusterName, "post-tests"),
		xpenvfuncs.Conditional(envfuncs.DestroyCluster(clusterName), !reuseCluster),
	)
}

// resolveClusterName mirrors xp-testing's setup.clusterName logic so that the
// E2E_CLUSTER_NAME / E2E_REUSE_CLUSTER environment variables continue to work.
func resolveClusterName() string {
	if envvar.CheckEnvVarExists(clusterNameEnv) {
		return os.Getenv(clusterNameEnv)
	}
	if envvar.CheckEnvVarExists(reuseClusterEnv) {
		return defaultClusterPrefix
	}
	return envconf.RandomName(defaultClusterPrefix, 10)
}
