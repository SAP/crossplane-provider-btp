module github.com/sap/crossplane-provider-btp

go 1.25.0

toolchain go1.25.6

// Module conflict in Go dependencies. The package google.golang.org/genproto/googleapis/api/httpbody exists in two different modules:
// - google.golang.org/genproto (older version)
// - google.golang.org/genproto/googleapis/api (newer, split module)
replace google.golang.org/genproto => google.golang.org/genproto v0.0.0-20251202230838-ff82c1b0f217

require (
	dario.cat/mergo v1.0.2
	github.com/SAP/xp-clifford v0.0.0-20260202115841-f2ef82b7144e
	github.com/alecthomas/kingpin/v2 v2.4.0
	github.com/cloudfoundry/go-cfclient/v3 v3.0.0-alpha.18
	github.com/crossplane-contrib/xp-testing v1.9.0
	github.com/crossplane/crossplane-runtime v1.20.0
	github.com/crossplane/crossplane-tools v0.0.0-20251017183449-dd4517244339
	github.com/crossplane/upjet v1.11.0
	github.com/go-logr/logr v1.4.3
	github.com/go-openapi/runtime v0.29.2
	github.com/golang-jwt/jwt/v4 v4.5.2
	github.com/google/go-cmp v0.7.0
	github.com/google/uuid v1.6.0
	github.com/int128/kubelogin v1.31.0
	github.com/json-iterator/go v1.1.12
	github.com/mitchellh/reflectwalk v1.0.2
	github.com/muvaf/typewriter v0.0.0-20240614220100-70f9d4a54ea0
	github.com/pkg/errors v0.9.1
	github.com/samber/lo v1.52.0
	github.com/stretchr/testify v1.11.1
	github.com/vladimirvivien/gexe v0.5.0
	golang.org/x/oauth2 v0.35.0
	golang.org/x/sync v0.19.0
	gopkg.in/validator.v2 v2.0.1
	gopkg.in/yaml.v3 v3.0.1
	k8s.io/api v0.35.1
	k8s.io/apimachinery v0.35.1
	k8s.io/client-go v0.35.1
	k8s.io/klog/v2 v2.130.1
	k8s.io/utils v0.0.0-20260210185600-b8788abfbbc2
	sigs.k8s.io/controller-runtime v0.20.0
	sigs.k8s.io/controller-tools v0.16.5
	sigs.k8s.io/e2e-framework v0.6.0
	sigs.k8s.io/yaml v1.6.0
	software.sslmate.com/src/go-pkcs12 v0.7.0
)

require (
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/agext/levenshtein v1.2.3 // indirect
	github.com/alecthomas/chroma/v2 v2.14.0 // indirect
	github.com/alecthomas/units v0.0.0-20240927000941-0f3dac36c52b // indirect
	github.com/antchfx/htmlquery v1.2.4 // indirect
	github.com/antchfx/xpath v1.2.0 // indirect
	github.com/apparentlymart/go-textseg/v15 v15.0.0 // indirect
	github.com/atotto/clipboard v0.1.4 // indirect
	github.com/aymanbagabas/go-osc52/v2 v2.0.1 // indirect
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/catppuccin/go v0.3.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/charmbracelet/bubbles v0.21.1-0.20250623103423-23b8fd6302d7 // indirect
	github.com/charmbracelet/bubbletea v1.3.6 // indirect
	github.com/charmbracelet/colorprofile v0.2.3-0.20250311203215-f60798e515dc // indirect
	github.com/charmbracelet/glamour v0.10.0 // indirect
	github.com/charmbracelet/huh v0.8.0 // indirect
	github.com/charmbracelet/lipgloss v1.1.1-0.20250404203927-76690c660834 // indirect
	github.com/charmbracelet/log v0.4.2 // indirect
	github.com/charmbracelet/x/ansi v0.9.3 // indirect
	github.com/charmbracelet/x/cellbuf v0.0.13 // indirect
	github.com/charmbracelet/x/exp/slice v0.0.0-20250327172914-2fdc97757edf // indirect
	github.com/charmbracelet/x/exp/strings v0.0.0-20240722160745-212f7b056ed0 // indirect
	github.com/charmbracelet/x/term v0.2.1 // indirect
	github.com/codegangsta/inject v0.0.0-20150114235600-33e0aa1cb7c0 // indirect
	github.com/containerd/log v0.1.0 // indirect
	github.com/containerd/stargz-snapshotter/estargz v0.15.1 // indirect
	github.com/coreos/go-oidc/v3 v3.11.0 // indirect
	github.com/dave/jennifer v1.7.1 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/distribution/reference v0.5.0 // indirect
	github.com/dlclark/regexp2 v1.11.0 // indirect
	github.com/docker/docker v25.0.6+incompatible // indirect
	github.com/docker/go-connections v0.5.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/emicklei/go-restful/v3 v3.12.2 // indirect
	github.com/erikgeiser/coninput v0.0.0-20211004153227-1c3628e74d0f // indirect
	github.com/evanphx/json-patch v5.9.11+incompatible // indirect
	github.com/evanphx/json-patch/v5 v5.9.0 // indirect
	github.com/fatih/camelcase v1.0.0 // indirect
	github.com/fatih/color v1.18.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/fxamacker/cbor/v2 v2.9.0 // indirect
	github.com/go-jose/go-jose/v4 v4.1.3 // indirect
	github.com/go-logfmt/logfmt v0.6.0 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-logr/zapr v1.3.0 // indirect
	github.com/go-martini/martini v0.0.0-20170121215854-22fa46961aab // indirect
	github.com/go-openapi/analysis v0.24.1 // indirect
	github.com/go-openapi/errors v0.22.4 // indirect
	github.com/go-openapi/jsonpointer v0.22.1 // indirect
	github.com/go-openapi/jsonreference v0.21.3 // indirect
	github.com/go-openapi/loads v0.23.2 // indirect
	github.com/go-openapi/spec v0.22.1 // indirect
	github.com/go-openapi/strfmt v0.25.0 // indirect
	github.com/go-openapi/swag v0.23.0 // indirect
	github.com/go-openapi/swag/conv v0.25.1 // indirect
	github.com/go-openapi/swag/fileutils v0.25.1 // indirect
	github.com/go-openapi/swag/jsonname v0.25.1 // indirect
	github.com/go-openapi/swag/jsonutils v0.25.1 // indirect
	github.com/go-openapi/swag/loading v0.25.1 // indirect
	github.com/go-openapi/swag/mangling v0.25.1 // indirect
	github.com/go-openapi/swag/stringutils v0.25.1 // indirect
	github.com/go-openapi/swag/typeutils v0.25.1 // indirect
	github.com/go-openapi/swag/yamlutils v0.25.1 // indirect
	github.com/go-openapi/validate v0.25.1 // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/gobuffalo/flect v1.0.3 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/mock v1.7.0-rc.1 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/btree v1.1.3 // indirect
	github.com/google/gnostic-models v0.7.0 // indirect
	github.com/google/go-containerregistry v0.20.2 // indirect
	github.com/google/wire v0.6.0 // indirect
	github.com/gorilla/css v1.0.1 // indirect
	github.com/gorilla/websocket v1.5.4-0.20250319132907-e064f32e3674 // indirect
	github.com/hashicorp/go-cty v1.5.0 // indirect
	github.com/hashicorp/go-hclog v1.6.3 // indirect
	github.com/hashicorp/go-plugin v1.6.3 // indirect
	github.com/hashicorp/go-uuid v1.0.3 // indirect
	github.com/hashicorp/go-version v1.7.0 // indirect
	github.com/hashicorp/hcl/v2 v2.23.0 // indirect
	github.com/hashicorp/logutils v1.0.0 // indirect
	github.com/hashicorp/terraform-json v0.25.0 // indirect
	github.com/hashicorp/terraform-plugin-framework v1.15.0 // indirect
	github.com/hashicorp/terraform-plugin-go v0.28.0 // indirect
	github.com/hashicorp/terraform-plugin-log v0.9.0 // indirect
	github.com/hashicorp/terraform-plugin-sdk/v2 v2.37.0 // indirect
	github.com/hashicorp/terraform-registry-address v0.2.5 // indirect
	github.com/hashicorp/terraform-svchost v0.1.1 // indirect
	github.com/hashicorp/yamux v0.1.1 // indirect
	github.com/iancoleman/strcase v0.3.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/int128/listener v1.1.0 // indirect
	github.com/int128/oauth2cli v1.14.0 // indirect
	github.com/int128/oauth2dev v1.0.1 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/lucasb-eyer/go-colorful v1.2.0 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/martini-contrib/render v0.0.0-20150707142108-ec18f8345a11 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-localereader v0.0.1 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/microcosm-cc/bluemonday v1.0.27 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/go-ps v1.0.0 // indirect
	github.com/mitchellh/go-testing-interface v1.14.1 // indirect
	github.com/mitchellh/go-wordwrap v1.0.1 // indirect
	github.com/mitchellh/hashstructure/v2 v2.0.2 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/moby/spdystream v0.5.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/muesli/ansi v0.0.0-20230316100256-276c6243b2f6 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/muesli/reflow v0.3.0 // indirect
	github.com/muesli/termenv v0.16.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mxk/go-flowrate v0.0.0-20140419014527-cca7078d478f // indirect
	github.com/oklog/run v1.0.0 // indirect
	github.com/oklog/ulid v1.3.1 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.0-rc5 // indirect
	github.com/oxtoacart/bpool v0.0.0-20190530202638-03653db5a59c // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_golang v1.22.0 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.62.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/sagikazarmark/locafero v0.11.0 // indirect
	github.com/sourcegraph/conc v0.3.1-0.20240121214520-5f936abd7ae8 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/cobra v1.10.2 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/spf13/viper v1.21.0 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/tmccombs/hcl2json v0.3.3 // indirect
	github.com/vbatts/tar-split v0.11.5 // indirect
	github.com/vmihailenco/msgpack v4.0.4+incompatible // indirect
	github.com/vmihailenco/msgpack/v5 v5.4.1 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/xhit/go-str2duration/v2 v2.1.0 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	github.com/yuin/goldmark v1.7.8 // indirect
	github.com/yuin/goldmark-emoji v1.0.5 // indirect
	github.com/zclconf/go-cty v1.16.2 // indirect
	github.com/zclconf/go-cty-yaml v1.0.3 // indirect
	go.mongodb.org/mongo-driver v1.17.6 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.61.0 // indirect
	go.opentelemetry.io/otel v1.39.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.39.0 // indirect
	go.opentelemetry.io/otel/metric v1.39.0 // indirect
	go.opentelemetry.io/otel/sdk v1.39.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.39.0 // indirect
	go.opentelemetry.io/otel/trace v1.39.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	go.yaml.in/yaml/v2 v2.4.3 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/crypto v0.47.0 // indirect
	golang.org/x/exp v0.0.0-20241009180824-f66d83c29e7c // indirect
	golang.org/x/mod v0.32.0 // indirect
	golang.org/x/net v0.49.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
	golang.org/x/term v0.39.0 // indirect
	golang.org/x/text v0.33.0 // indirect
	golang.org/x/time v0.13.0 // indirect
	golang.org/x/tools v0.41.0 // indirect
	gomodules.xyz/jsonpatch/v2 v2.4.0 // indirect
	google.golang.org/appengine v1.6.8 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251202230838-ff82c1b0f217 // indirect
	google.golang.org/grpc v1.77.0 // indirect
	google.golang.org/protobuf v1.36.10 // indirect
	gopkg.in/evanphx/json-patch.v4 v4.13.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	k8s.io/apiextensions-apiserver v0.34.1 // indirect
	k8s.io/component-base v0.34.1 // indirect
	k8s.io/kube-openapi v0.0.0-20250910181357-589584f1c912 // indirect
	sigs.k8s.io/json v0.0.0-20250730193827-2d320260d730 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/structured-merge-diff/v6 v6.3.0 // indirect
)
