package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

const externalNameAnnotation = "crossplane.io/external-name"

var (
	subdomainPattern         = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{1,61}[a-z0-9])$`)
	sensitiveValueAssignment = regexp.MustCompile(`(?i)((?:password|token|authorization|secret|client_secret)\s*[=:]\s*)(?:"[^"]*"|'[^']*'|[^\s,}]+)`)
	sensitiveCommandArgument = regexp.MustCompile(`(?i)(--(?:password|token|authorization|secret|client-secret)\s+)(\S+)`)
)

type technicalUser struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
	IDP      string `json:"idp"`
}

func main() {
	if len(os.Args) < 2 {
		fatalf("usage: %s <write-export-config|login|source-subdomain|derive-subdomain|transform> [flags]", os.Args[0])
	}

	var err error
	switch os.Args[1] {
	case "write-export-config":
		err = writeExportConfig(os.Args[2:])
	case "login":
		err = login(os.Args[2:])
	case "source-subdomain":
		err = printSourceSubdomain(os.Args[2:])
	case "derive-subdomain":
		err = printDerivedSubdomain(os.Args[2:])
	case "transform":
		err = transformWithProgress(os.Args[2:], os.Stdout)
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fatalf("%v", err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}

func writeExportConfig(args []string) error {
	flags := flag.NewFlagSet("write-export-config", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	output := flags.String("output", "", "configuration file")
	rawOutput := flags.String("raw-output", "", "raw exporter output")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *output == "" || *rawOutput == "" {
		return errors.New("--output and --raw-output are required")
	}

	config := struct {
		All               bool   `yaml:"all"`
		ResolveReferences bool   `yaml:"resolve-references"`
		Output            string `yaml:"output"`
	}{
		All:               true,
		ResolveReferences: true,
		Output:            *rawOutput,
	}
	content, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal exporter configuration: %w", err)
	}
	if err := os.WriteFile(*output, content, 0o600); err != nil {
		return fmt.Errorf("write exporter configuration: %w", err)
	}
	return nil
}

func login(args []string) error {
	flags := flag.NewFlagSet("login", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	exporter := flags.String("exporter", "", "exporter binary")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *exporter == "" {
		return errors.New("--exporter is required")
	}

	var user technicalUser
	if err := json.Unmarshal([]byte(os.Getenv("BTP_TECHNICAL_USER")), &user); err != nil {
		return fmt.Errorf("BTP_TECHNICAL_USER must contain valid JSON: %w", err)
	}
	if strings.TrimSpace(user.Email) == "" || user.Password == "" {
		return errors.New("BTP_TECHNICAL_USER must contain non-empty email and password fields")
	}
	globalAccount := strings.TrimSpace(os.Getenv("GLOBAL_ACCOUNT"))
	serverURL := strings.TrimSpace(os.Getenv("CLI_SERVER_URL"))
	if globalAccount == "" || serverURL == "" {
		return errors.New("GLOBAL_ACCOUNT and CLI_SERVER_URL must be non-empty")
	}
	// IDP_URL configures the provider later in the demo. The BTP CLI expects
	// its origin key instead, which is optionally part of BTP_TECHNICAL_USER.
	idp := strings.TrimSpace(user.IDP)

	env := replaceEnvironment(os.Environ(), map[string]string{
		// BTP CLI authentication uses the technical user's email. The username
		// (P-user ID) in this JSON is for the provider configuration instead.
		"BTP_EXPORT_USER_NAME":          user.Email,
		"BTP_EXPORT_PASSWORD":           user.Password,
		"BTP_EXPORT_GLOBAL_ACCOUNT":     globalAccount,
		"BTP_EXPORT_BTP_CLI_SERVER_URL": serverURL,
		"BTP_EXPORT_IDP":                idp,
	}, "BTP_TECHNICAL_USER")
	cmd := exec.Command(*exporter, "login")
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := redactLoginOutput(string(output), user, globalAccount, serverURL)
		if message == "" {
			return fmt.Errorf("exporter login failed: %w", err)
		}
		return fmt.Errorf("exporter login failed: %w: %s", err, message)
	}
	return nil
}

func redactLoginOutput(output string, user technicalUser, globalAccount, serverURL string) string {
	output = strings.TrimSpace(output)
	for _, value := range []string{user.Email, user.Username, user.Password, globalAccount, serverURL, os.Getenv("BTP_TECHNICAL_USER")} {
		if value != "" {
			output = strings.ReplaceAll(output, value, "<redacted>")
		}
	}
	output = sensitiveValueAssignment.ReplaceAllString(output, "$1<redacted>")
	output = sensitiveCommandArgument.ReplaceAllString(output, "$1<redacted>")
	const maxDiagnosticLength = 2048
	if len(output) > maxDiagnosticLength {
		output = output[:maxDiagnosticLength] + "…"
	}
	return output
}

func replaceEnvironment(base []string, replacements map[string]string, remove ...string) []string {
	removed := make(map[string]bool, len(remove))
	for _, key := range remove {
		removed[key] = true
	}
	for key := range replacements {
		removed[key] = true
	}

	env := make([]string, 0, len(base)+len(replacements))
	for _, entry := range base {
		key, _, found := strings.Cut(entry, "=")
		if !found || removed[key] {
			continue
		}
		env = append(env, entry)
	}
	for key, value := range replacements {
		env = append(env, key+"="+value)
	}
	return env
}

func printSourceSubdomain(args []string) error {
	flags := flag.NewFlagSet("source-subdomain", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	input := flags.String("input", "", "raw export")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *input == "" {
		return errors.New("--input is required")
	}
	documents, err := readDocuments(*input)
	if err != nil {
		return err
	}
	subaccount, err := exactlyOneSubaccount(documents)
	if err != nil {
		return err
	}
	forProvider, ok := nestedMapping(subaccount, "spec", "forProvider")
	if !ok {
		return errors.New("Subaccount is missing spec.forProvider")
	}
	subdomain, ok := stringValue(forProvider, "subdomain")
	if !ok || subdomain == "" {
		return errors.New("Subaccount is missing spec.forProvider.subdomain")
	}
	fmt.Println(subdomain)
	return nil
}

func printDerivedSubdomain(args []string) error {
	flags := flag.NewFlagSet("derive-subdomain", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	source := flags.String("source-subdomain", "", "source subdomain")
	buildID := flags.String("build-id", "", "build identifier")
	if err := flags.Parse(args); err != nil {
		return err
	}
	target, err := deriveSubdomain(*source, *buildID)
	if err != nil {
		return err
	}
	fmt.Println(target)
	return nil
}

func deriveSubdomain(source, buildID string) (string, error) {
	source = sanitizeSubdomainPart(source)
	suffix := sanitizeSubdomainPart(buildID)
	if source == "" || suffix == "" {
		return "", errors.New("source subdomain and BUILD_ID must contain at least one letter or digit")
	}
	if len(suffix) > 61 {
		suffix = suffix[len(suffix)-61:]
	}
	maxSourceLength := 63 - len(suffix) - 1
	if maxSourceLength < 1 {
		return "", errors.New("BUILD_ID is too long to derive a target subdomain")
	}
	if len(source) > maxSourceLength {
		source = strings.TrimRight(source[:maxSourceLength], "-")
	}
	if source == "" {
		return "", errors.New("source subdomain cannot be shortened safely")
	}
	target := source + "-" + suffix
	if !validSubdomain(target) {
		return "", fmt.Errorf("cannot derive a valid target subdomain from source subdomain and BUILD_ID")
	}
	return target, nil
}

func sanitizeSubdomainPart(value string) string {
	var builder strings.Builder
	lastHyphen := false
	for _, char := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			builder.WriteRune(char)
			lastHyphen = false
		} else if !lastHyphen {
			builder.WriteByte('-')
			lastHyphen = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

func validSubdomain(value string) bool {
	return subdomainPattern.MatchString(value)
}

func transform(args []string) error {
	return transformWithProgress(args, io.Discard)
}

func transformWithProgress(args []string, progress io.Writer) error {
	flags := flag.NewFlagSet("transform", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	input := flags.String("input", "", "raw export")
	output := flags.String("output", "", "clone-ready export")
	targetSubdomain := flags.String("target-subdomain", "", "target subdomain")
	technicalUserEmail := flags.String("technical-user-email", "", "subaccount administrator")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *input == "" || *output == "" || *targetSubdomain == "" || *technicalUserEmail == "" {
		return errors.New("--input, --output, --target-subdomain, and --technical-user-email are required")
	}
	if !validSubdomain(*targetSubdomain) {
		return fmt.Errorf("invalid target subdomain %q; expected 3-63 lowercase letters, digits, or hyphens, beginning and ending with a letter or digit", *targetSubdomain)
	}

	documents, err := readDocuments(*input)
	if err != nil {
		return err
	}
	subaccount, err := exactlyOneSubaccount(documents)
	if err != nil {
		return err
	}

	reportTransformation(progress, "setting the clone subdomain so BTP creates a distinct subaccount")
	reportTransformation(progress, "ensuring the technical user is a subaccount administrator so the provider can manage the clone")
	if err := updateSubaccount(subaccount, *targetSubdomain, *technicalUserEmail); err != nil {
		return err
	}

	reportTransformation(progress, "removing adoption external names so Crossplane creates cloned resources instead of adopting sources")
	for _, document := range documents {
		removeExternalName(document.Content[0])
	}

	reportTransformation(progress, "removing stale generated-reference IDs so references resolve to cloned resources")
	for _, document := range documents {
		resource := document.Content[0]
		if isManagedBTPResource(resource) {
			removeStaleIDsWithReferences(resource)
		}
	}

	reportTransformation(progress, "normalizing clone-specific BTP values: using the target subdomain for Cloud Foundry organization names, making CIS Central entitlements enable-only, and converting service-binding names to valid Terraform identifiers")
	reportTransformation(progress, "setting full management policies so Crossplane provisions and manages the clone")
	for _, document := range documents {
		resource := document.Content[0]
		if !isManagedBTPResource(resource) {
			continue
		}
		normalizeCloneResource(resource, *targetSubdomain)
		setManagementPolicies(resource)
	}
	if err := validateClone(documents, *targetSubdomain, *technicalUserEmail); err != nil {
		return err
	}
	if err := writeDocuments(*output, documents); err != nil {
		return err
	}
	return nil
}

func reportTransformation(progress io.Writer, message string) {
	if progress != nil {
		fmt.Fprintf(progress, "Transformation: %s.\n", message)
	}
}

func readDocuments(path string) ([]*yaml.Node, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	var documents []*yaml.Node
	for {
		var document yaml.Node
		err := decoder.Decode(&document)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
			return nil, fmt.Errorf("%s contains a document that is not a Kubernetes resource mapping", path)
		}
		documents = append(documents, &document)
	}
	if len(documents) == 0 {
		return nil, fmt.Errorf("%s contains no resource documents", path)
	}
	return documents, nil
}

func writeDocuments(path string, documents []*yaml.Node) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	for _, document := range documents {
		// Start every document explicitly, including the first one, so the
		// clone-ready artifact is unambiguously a multi-document YAML stream.
		if _, err := file.WriteString("---\n"); err != nil {
			_ = file.Close()
			return fmt.Errorf("write %s: %w", path, err)
		}
		encoder := yaml.NewEncoder(file)
		encoder.SetIndent(2)
		if err := encoder.Encode(document); err != nil {
			_ = file.Close()
			return fmt.Errorf("write %s: %w", path, err)
		}
		if err := encoder.Close(); err != nil {
			_ = file.Close()
			return fmt.Errorf("finish %s: %w", path, err)
		}
		// Match exporter output by explicitly ending every YAML document.
		if _, err := file.WriteString("...\n"); err != nil {
			_ = file.Close()
			return fmt.Errorf("finish %s: %w", path, err)
		}
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	return nil
}

func exactlyOneSubaccount(documents []*yaml.Node) (*yaml.Node, error) {
	var subaccount *yaml.Node
	for _, document := range documents {
		resource := document.Content[0]
		kind, _ := stringValue(resource, "kind")
		if kind != "Subaccount" {
			continue
		}
		if subaccount != nil {
			return nil, errors.New("raw export must contain exactly one Subaccount; found multiple")
		}
		subaccount = resource
	}
	if subaccount == nil {
		return nil, errors.New("raw export must contain exactly one Subaccount; found none")
	}
	return subaccount, nil
}

func isManagedBTPResource(resource *yaml.Node) bool {
	apiVersion, hasAPIVersion := stringValue(resource, "apiVersion")
	_, hasKind := stringValue(resource, "kind")
	return hasAPIVersion && hasKind && strings.Contains(apiVersion, "btp.sap.crossplane.io/")
}

func updateSubaccount(subaccount *yaml.Node, targetSubdomain, technicalUserEmail string) error {
	forProvider := ensureNestedMapping(subaccount, "spec", "forProvider")
	// BTP requires display names to be unique within their parent. The target
	// subdomain is already unique for each clone and is valid as a display name.
	setString(forProvider, "displayName", targetSubdomain)
	setString(forProvider, "subdomain", targetSubdomain)
	admins := ensureSequence(forProvider, "subaccountAdmins")
	for _, admin := range admins.Content {
		if admin.Value == technicalUserEmail {
			return nil
		}
	}
	admins.Content = append(admins.Content, stringNode(technicalUserEmail))
	return nil
}

func removeExternalName(resource *yaml.Node) {
	metadata, ok := mappingValue(resource, "metadata")
	if !ok || metadata.Kind != yaml.MappingNode {
		return
	}
	annotations, ok := mappingValue(metadata, "annotations")
	if !ok || annotations.Kind != yaml.MappingNode {
		return
	}
	removeMappingKey(annotations, externalNameAnnotation)
	if len(annotations.Content) == 0 {
		removeMappingKey(metadata, "annotations")
	}
}

func removeStaleIDsWithReferences(resource *yaml.Node) {
	forProvider, ok := nestedMapping(resource, "spec", "forProvider")
	if !ok {
		return
	}
	for index := 0; index < len(forProvider.Content); index += 2 {
		key := forProvider.Content[index].Value
		if strings.HasSuffix(key, "Ref") {
			removeMappingKey(forProvider, staleIDForReference(key))
		}
	}
}

func staleIDForReference(referenceKey string) string {
	// ServiceBinding uses serviceInstanceId with serviceInstanceRef, unlike the
	// usual <field>Ref naming convention. Retaining the source ID makes a clone
	// bind to an instance that does not exist in the target subaccount.
	if referenceKey == "serviceInstanceRef" {
		return "serviceInstanceId"
	}
	return strings.TrimSuffix(referenceKey, "Ref")
}

func normalizeCloneResource(resource *yaml.Node, targetSubdomain string) {
	kind, _ := stringValue(resource, "kind")
	forProvider, ok := nestedMapping(resource, "spec", "forProvider")
	if !ok {
		return
	}

	switch kind {
	case "CloudFoundryEnvironment":
		// Cloud Foundry org names must be unique within a landscape. The source
		// org name can therefore not be reused by a clone; the target subdomain
		// is globally unique and uses a safe subset of org-name characters.
		setString(forProvider, "orgName", targetSubdomain)
	case "Entitlement":
		// CIS central is a multitenant plan and BTP rejects numeric quotas for
		// it. A clone must grant it as an enable-only entitlement.
		serviceName, _ := stringValue(forProvider, "serviceName")
		planName, _ := stringValue(forProvider, "servicePlanName")
		if serviceName == "cis" && planName == "central" {
			removeMappingKey(forProvider, "amount")
			setBool(forProvider, "enable", true)
		}
	case "ServiceBinding":
		// The provider uses this value as a Terraform resource identifier, for
		// which Unicode punctuation (for example an em dash) is invalid.
		if name, ok := stringValue(forProvider, "name"); ok {
			setString(forProvider, "name", normalizeTerraformIdentifier(name))
		}
	}
}

func normalizeTerraformIdentifier(value string) string {
	var builder strings.Builder
	previousSeparator := false
	for _, char := range value {
		isLetter := char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z'
		isDigit := char >= '0' && char <= '9'
		if isLetter || isDigit || char == '_' || char == '-' {
			builder.WriteRune(char)
			previousSeparator = false
			continue
		}
		if !previousSeparator {
			builder.WriteByte('-')
			previousSeparator = true
		}
	}
	result := strings.Trim(builder.String(), "-")
	if result == "" {
		return "binding"
	}
	if first := result[0]; !(first >= 'a' && first <= 'z' || first >= 'A' && first <= 'Z' || first == '_') {
		return "binding-" + result
	}
	return result
}

func setManagementPolicies(resource *yaml.Node) {
	spec := ensureMapping(resource, "spec")
	policies := ensureSequence(spec, "managementPolicies")
	policies.Content = []*yaml.Node{stringNode("*")}
}

func validateClone(documents []*yaml.Node, targetSubdomain, technicalUserEmail string) error {
	subaccount, err := exactlyOneSubaccount(documents)
	if err != nil {
		return err
	}
	forProvider, ok := nestedMapping(subaccount, "spec", "forProvider")
	if !ok {
		return errors.New("clone-ready Subaccount is missing spec.forProvider")
	}
	if subdomain, ok := stringValue(forProvider, "subdomain"); !ok || subdomain != targetSubdomain {
		return errors.New("clone-ready Subaccount does not contain the target subdomain")
	}
	if displayName, ok := stringValue(forProvider, "displayName"); !ok || displayName != targetSubdomain {
		return errors.New("clone-ready Subaccount does not contain a unique target display name")
	}
	admins, ok := mappingValue(forProvider, "subaccountAdmins")
	if !ok || admins.Kind != yaml.SequenceNode || !sequenceContains(admins, technicalUserEmail) {
		return errors.New("clone-ready Subaccount does not include TECHNICAL_USER_EMAIL in subaccountAdmins")
	}

	for _, document := range documents {
		resource := document.Content[0]
		if hasExternalName(resource) {
			return errors.New("clone-ready manifest still contains a crossplane.io/external-name annotation")
		}
		if !isManagedBTPResource(resource) {
			continue
		}
		policies, ok := nestedValue(resource, "spec", "managementPolicies")
		if !ok || policies.Kind != yaml.SequenceNode || len(policies.Content) != 1 || policies.Content[0].Value != "*" {
			return errors.New("clone-ready manifest contains a managed resource without managementPolicies: [\"*\"]")
		}
		if hasStaleIDWithReference(resource) {
			return errors.New("clone-ready manifest contains a stale ID alongside a generated reference")
		}
		kind, _ := stringValue(resource, "kind")
		if kind == "CloudFoundryEnvironment" {
			forProvider, ok := nestedMapping(resource, "spec", "forProvider")
			if !ok {
				return errors.New("clone-ready CloudFoundryEnvironment is missing spec.forProvider")
			}
			if orgName, ok := stringValue(forProvider, "orgName"); !ok || orgName != targetSubdomain {
				return errors.New("clone-ready CloudFoundryEnvironment does not contain the target-specific org name")
			}
		}
	}
	return nil
}

func hasExternalName(resource *yaml.Node) bool {
	metadata, ok := mappingValue(resource, "metadata")
	if !ok {
		return false
	}
	annotations, ok := mappingValue(metadata, "annotations")
	if !ok {
		return false
	}
	_, ok = mappingValue(annotations, externalNameAnnotation)
	return ok
}

func hasStaleIDWithReference(resource *yaml.Node) bool {
	forProvider, ok := nestedMapping(resource, "spec", "forProvider")
	if !ok {
		return false
	}
	for index := 0; index < len(forProvider.Content); index += 2 {
		key := forProvider.Content[index].Value
		if strings.HasSuffix(key, "Ref") {
			if _, staleIDPresent := mappingValue(forProvider, staleIDForReference(key)); staleIDPresent {
				return true
			}
		}
	}
	return false
}

func ensureNestedMapping(node *yaml.Node, keys ...string) *yaml.Node {
	current := node
	for _, key := range keys {
		current = ensureMapping(current, key)
	}
	return current
}

func nestedMapping(node *yaml.Node, keys ...string) (*yaml.Node, bool) {
	current := node
	for _, key := range keys {
		var ok bool
		current, ok = mappingValue(current, key)
		if !ok || current.Kind != yaml.MappingNode {
			return nil, false
		}
	}
	return current, true
}

func nestedValue(node *yaml.Node, keys ...string) (*yaml.Node, bool) {
	current := node
	for _, key := range keys {
		var ok bool
		current, ok = mappingValue(current, key)
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func mappingValue(mapping *yaml.Node, key string) (*yaml.Node, bool) {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil, false
	}
	for index := 0; index < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			return mapping.Content[index+1], true
		}
	}
	return nil, false
}

func ensureMapping(mapping *yaml.Node, key string) *yaml.Node {
	if value, ok := mappingValue(mapping, key); ok && value.Kind == yaml.MappingNode {
		return value
	}
	value := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	setValue(mapping, key, value)
	return value
}

func ensureSequence(mapping *yaml.Node, key string) *yaml.Node {
	if value, ok := mappingValue(mapping, key); ok && value.Kind == yaml.SequenceNode {
		return value
	}
	value := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	setValue(mapping, key, value)
	return value
}

func setString(mapping *yaml.Node, key, value string) {
	setValue(mapping, key, stringNode(value))
}

func setBool(mapping *yaml.Node, key string, value bool) {
	setValue(mapping, key, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: fmt.Sprintf("%t", value)})
}

func setValue(mapping *yaml.Node, key string, value *yaml.Node) {
	for index := 0; index < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			mapping.Content[index+1] = value
			return
		}
	}
	mapping.Content = append(mapping.Content, stringNode(key), value)
}

func removeMappingKey(mapping *yaml.Node, key string) {
	for index := 0; index < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			mapping.Content = append(mapping.Content[:index], mapping.Content[index+2:]...)
			return
		}
	}
}

func stringValue(mapping *yaml.Node, key string) (string, bool) {
	value, ok := mappingValue(mapping, key)
	if !ok || value.Kind != yaml.ScalarNode {
		return "", false
	}
	return value.Value, true
}

func stringNode(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

func sequenceContains(sequence *yaml.Node, value string) bool {
	for _, item := range sequence.Content {
		if item.Kind == yaml.ScalarNode && item.Value == value {
			return true
		}
	}
	return false
}
