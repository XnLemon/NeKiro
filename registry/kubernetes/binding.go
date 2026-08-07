// Package kubernetes provides the read/watch-only Kubernetes EndpointSlice
// Instance Directory provider. It reports topology for an exact, already
// authorized registry.ReleaseTarget; it does not own Catalog facts.
package kubernetes

import (
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/NeKiro-project/NeKiro/registry"
)

const (
	// BindingVersionV1 is the only supported Kubernetes binding version.
	BindingVersionV1 = "v1"

	// Reserved Kubernetes labels are generated from the exact target and may
	// not be overridden by caller-supplied selector/owner labels.
	LabelBindingVersion = "registry.nekiro.dev/binding-version"
	LabelReleaseTarget  = "registry.nekiro.dev/release-target"

	// Kubernetes selector labels used to identify the Service and its slices.
	LabelServiceName = "kubernetes.io/service-name"
	LabelManagedBy   = "endpointslice.kubernetes.io/managed-by"

	// Metadata keys exposed on normalized instances. They are intentionally
	// namespaced and form the complete provider metadata allowlist.
	MetadataTargetRefUID = "kubernetes.target_ref_uid"
	MetadataAddressType  = "kubernetes.address_type"
)

// ResourceBounds contains all required positive provider limits. No value is
// inferred when a field is zero.
type ResourceBounds struct {
	ListResponseBytes  int
	WatchEnvelopeBytes int
	EndpointSliceCount int
	EndpointCount      int
	PendingChanges     int
}

func (b ResourceBounds) Validate() error {
	if b.ListResponseBytes <= 0 || b.WatchEnvelopeBytes <= 0 ||
		b.EndpointSliceCount <= 0 || b.EndpointCount <= 0 || b.PendingChanges <= 0 {
		return invalidInput()
	}
	return nil
}

// BindingInput is the explicit immutable Binding v1 configuration supplied to
// a Kubernetes Directory. Every field is required; there are no defaults.
type BindingInput struct {
	Version string

	Target registry.ReleaseTarget

	APIOrigin   string
	Namespace   string
	ServiceName string
	ServiceUID  string

	EndpointSliceManagedBy string
	ServiceOwnerLabels     map[string]string
	EndpointSliceLabels    map[string]string

	AddressType registry.AddressType
	PortName    string
	Protocol    registry.TransportProtocol

	Bounds ResourceBounds
}

// Binding is an immutable Kubernetes Binding v1. It is provider-local
// configuration and is not a Catalog binding store.
type Binding struct {
	version string
	target  registry.ReleaseTarget

	apiOrigin   string
	namespace   string
	serviceName string
	serviceUID  string

	endpointSliceManagedBy string
	serviceOwnerLabels     map[string]string
	endpointSliceLabels    map[string]string

	addressType registry.AddressType
	portName    string
	protocol    registry.TransportProtocol

	bounds    ResourceBounds
	targetKey string
}

// NewBinding validates and copies one explicit Binding v1.
func NewBinding(input BindingInput) (Binding, error) {
	if input.Version != BindingVersionV1 {
		return Binding{}, invalidInput()
	}
	if err := input.Target.Validate(); err != nil {
		return Binding{}, err
	}
	if !validAPIOrigin(input.APIOrigin) || !validDNS1123Label(input.Namespace) ||
		!validDNS1035Label(input.ServiceName) || !validOpaqueIdentifier(input.ServiceUID) ||
		!validLabelValue(input.EndpointSliceManagedBy) || !validPortName(input.PortName) ||
		!validAddressType(input.AddressType) || input.Protocol != registry.TransportProtocolTCP {
		return Binding{}, invalidInput()
	}
	if err := input.Bounds.Validate(); err != nil {
		return Binding{}, err
	}
	serviceOwnerLabels, err := cloneRequiredLabels(input.ServiceOwnerLabels)
	if err != nil {
		return Binding{}, err
	}
	endpointSliceLabels, err := cloneRequiredLabels(input.EndpointSliceLabels)
	if err != nil {
		return Binding{}, err
	}
	for key := range serviceOwnerLabels {
		if isReservedLabel(key) {
			return Binding{}, invalidInput()
		}
	}
	for key := range endpointSliceLabels {
		if isReservedLabel(key) {
			return Binding{}, invalidInput()
		}
	}
	for _, value := range serviceOwnerLabels {
		if value == input.Target.ReleaseID() {
			return Binding{}, invalidInput()
		}
	}
	for _, value := range endpointSliceLabels {
		if value == input.Target.ReleaseID() {
			return Binding{}, invalidInput()
		}
	}
	targetKey, err := TargetKey(input.Target)
	if err != nil {
		return Binding{}, err
	}
	return Binding{
		version:                input.Version,
		target:                 input.Target,
		apiOrigin:              input.APIOrigin,
		namespace:              input.Namespace,
		serviceName:            input.ServiceName,
		serviceUID:             input.ServiceUID,
		endpointSliceManagedBy: input.EndpointSliceManagedBy,
		serviceOwnerLabels:     serviceOwnerLabels,
		endpointSliceLabels:    endpointSliceLabels,
		addressType:            input.AddressType,
		portName:               input.PortName,
		protocol:               input.Protocol,
		bounds:                 input.Bounds,
		targetKey:              targetKey,
	}, nil
}

// NewBindingV1 is an explicit spelling for callers constructing only v1.
func NewBindingV1(input BindingInput) (Binding, error) { return NewBinding(input) }

// Validate verifies a Binding value, including its derived target key and
// immutable map invariants.
func (b Binding) Validate() error {
	if b.version != BindingVersionV1 {
		return invalidInput()
	}
	if err := b.target.Validate(); err != nil {
		return err
	}
	if !validAPIOrigin(b.apiOrigin) || !validDNS1123Label(b.namespace) ||
		!validDNS1035Label(b.serviceName) || !validOpaqueIdentifier(b.serviceUID) ||
		!validLabelValue(b.endpointSliceManagedBy) || !validPortName(b.portName) ||
		!validAddressType(b.addressType) || b.protocol != registry.TransportProtocolTCP {
		return invalidInput()
	}
	if err := b.bounds.Validate(); err != nil {
		return err
	}
	if err := validateRequiredLabels(b.serviceOwnerLabels); err != nil {
		return err
	}
	if err := validateRequiredLabels(b.endpointSliceLabels); err != nil {
		return err
	}
	for key := range b.serviceOwnerLabels {
		if isReservedLabel(key) {
			return invalidInput()
		}
	}
	for key := range b.endpointSliceLabels {
		if isReservedLabel(key) {
			return invalidInput()
		}
	}
	for _, value := range b.serviceOwnerLabels {
		if value == b.target.ReleaseID() {
			return invalidInput()
		}
	}
	for _, value := range b.endpointSliceLabels {
		if value == b.target.ReleaseID() {
			return invalidInput()
		}
	}
	targetKey, err := TargetKey(b.target)
	if err != nil || targetKey != b.targetKey {
		return invalidInput()
	}
	return nil
}

func (b Binding) Version() string                { return b.version }
func (b Binding) Target() registry.ReleaseTarget { return b.target }
func (b Binding) APIOrigin() string              { return b.apiOrigin }
func (b Binding) Namespace() string              { return b.namespace }
func (b Binding) ServiceName() string            { return b.serviceName }
func (b Binding) ServiceUID() string             { return b.serviceUID }
func (b Binding) EndpointSliceManagedBy() string { return b.endpointSliceManagedBy }
func (b Binding) AddressType() registry.AddressType {
	return b.addressType
}
func (b Binding) PortName() string                     { return b.portName }
func (b Binding) Protocol() registry.TransportProtocol { return b.protocol }
func (b Binding) Bounds() ResourceBounds               { return b.bounds }
func (b Binding) TargetKey() string                    { return b.targetKey }

func (b Binding) ServiceOwnerLabels() map[string]string {
	return cloneStringMap(b.serviceOwnerLabels)
}

func (b Binding) EndpointSliceLabels() map[string]string {
	return cloneStringMap(b.endpointSliceLabels)
}

// ServiceLabels returns the exact labels required on the bound Service,
// including the two generated target markers and configured owner markers.
func (b Binding) ServiceLabels() map[string]string {
	labels := map[string]string{
		LabelBindingVersion: BindingVersionV1,
		LabelReleaseTarget:  b.targetKey,
	}
	for key, value := range b.serviceOwnerLabels {
		labels[key] = value
	}
	return labels
}

// EndpointSliceSelectorLabels returns the exact non-reserved selector labels
// used for EndpointSlice List/Watch requests.
func (b Binding) EndpointSliceSelectorLabels() map[string]string {
	labels := map[string]string{
		LabelServiceName: b.serviceName,
		LabelManagedBy:   b.endpointSliceManagedBy,
	}
	for key, value := range b.endpointSliceLabels {
		labels[key] = value
	}
	return labels
}

// EndpointSliceLabelsForObject returns the exact labels required on a selected
// EndpointSlice, including generated markers and selector labels.
func (b Binding) EndpointSliceLabelsForObject() map[string]string {
	labels := b.EndpointSliceSelectorLabels()
	labels[LabelBindingVersion] = BindingVersionV1
	labels[LabelReleaseTarget] = b.targetKey
	return labels
}

// TargetKey returns the lower-case, unpadded base32 SHA-256 marker for an
// exact ReleaseTarget v1.
func TargetKey(target registry.ReleaseTarget) (string, error) {
	if err := target.Validate(); err != nil {
		return "", err
	}
	hash := sha256.Sum256(targetKeyRecord(target))
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(hash[:])
	return strings.ToLower(encoded), nil
}

// TargetKeyV1 is an explicit versioned alias for TargetKey.
func TargetKeyV1(target registry.ReleaseTarget) (string, error) { return TargetKey(target) }

func targetKeyRecord(target registry.ReleaseTarget) []byte {
	fields := []string{
		target.AgentID(), target.AgentCardVersion(), target.ReleaseID(),
		target.CardDigest(), target.CanonicalEndpoint(), target.Audience(),
	}
	var record []byte
	for _, field := range fields {
		var length [4]byte
		binary.BigEndian.PutUint32(length[:], uint32(len(field)))
		record = append(record, length[:]...)
		record = append(record, []byte(field)...)
	}
	return record
}

func cloneRequiredLabels(labels map[string]string) (map[string]string, error) {
	if err := validateRequiredLabels(labels); err != nil {
		return nil, err
	}
	return cloneStringMap(labels), nil
}

func validateRequiredLabels(labels map[string]string) error {
	if len(labels) == 0 {
		return invalidInput()
	}
	for key, value := range labels {
		if !validLabelKey(key) || !validLabelValue(value) {
			return invalidInput()
		}
	}
	return nil
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	copyValues := make(map[string]string, len(values))
	for key, value := range values {
		copyValues[key] = value
	}
	return copyValues
}

func isReservedLabel(key string) bool {
	switch key {
	case LabelBindingVersion, LabelReleaseTarget, LabelServiceName, LabelManagedBy:
		return true
	default:
		return false
	}
}

func validAddressType(value registry.AddressType) bool {
	return value == registry.AddressTypeIPv4 || value == registry.AddressTypeIPv6
}

func validExactText(value string) bool {
	if value == "" || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func validOpaqueIdentifier(value string) bool {
	if !validExactText(value) || len(value) > 253 {
		return false
	}
	for _, character := range value {
		if unicode.IsSpace(character) {
			return false
		}
	}
	return true
}

func validAPIOrigin(raw string) bool {
	if !validExactText(raw) || len(raw) > 2048 {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Opaque != "" || parsed.Host == "" || parsed.User != nil ||
		parsed.Path != "" || parsed.RawPath != "" || parsed.RawQuery != "" ||
		parsed.ForceQuery || parsed.Fragment != "" || parsed.RawFragment != "" {
		return false
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return false
	}
	hostname := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if hostname == "" || strings.Contains(hostname, "%") {
		return false
	}
	ipAddress, isIP := netip.Addr{}, false
	if parsedAddress, err := netip.ParseAddr(hostname); err == nil {
		ipAddress, isIP = parsedAddress, true
		if ipAddress.String() != hostname {
			return false
		}
	} else if !validDNSHost(hostname) {
		return false
	}
	portText := parsed.Port()
	if portText == "" && strings.HasSuffix(parsed.Host, ":") {
		return false
	}
	port := ""
	if portText != "" {
		portNumber, err := strconv.Atoi(portText)
		if err != nil || portNumber < 1 || portNumber > 65535 {
			return false
		}
		port = strconv.Itoa(portNumber)
	}
	// net/url's Hostname strips brackets. Rebuild the canonical host/port
	// without resolving DNS or consulting any external provider.
	hostPort := hostname
	if isIP && ipAddress.Is6() {
		hostPort = "[" + hostname + "]"
	}
	if port != "" {
		if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
			port = ""
		}
	}
	if port != "" {
		hostPort += ":" + port
	}
	return scheme+"://"+hostPort == raw
}

func validDNS1035Label(value string) bool {
	if len(value) == 0 || len(value) > 63 || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	if value[len(value)-1] == '-' {
		return false
	}
	for index := 1; index < len(value); index++ {
		char := value[index]
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
			return false
		}
	}
	return true
}

func validDNS1123Label(value string) bool {
	return !strings.Contains(value, ".") && validDNSHost(value)
}

func validPortName(value string) bool {
	if len(value) == 0 || len(value) > 15 || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	if value[len(value)-1] == '-' {
		return false
	}
	for index := 1; index < len(value); index++ {
		char := value[index]
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
			return false
		}
	}
	return true
}

func validLabelKey(value string) bool {
	if !validExactText(value) || len(value) > 253+1+63 {
		return false
	}
	parts := strings.Split(value, "/")
	if len(parts) > 2 || len(parts) == 0 {
		return false
	}
	name := parts[len(parts)-1]
	if !validLabelName(name) {
		return false
	}
	if len(parts) == 2 {
		return validDNSHost(parts[0])
	}
	return true
}

func validLabelName(value string) bool {
	if len(value) == 0 || len(value) > 63 ||
		!isASCIIAlphaNumeric(value[0]) || !isASCIIAlphaNumeric(value[len(value)-1]) {
		return false
	}
	for index := 1; index < len(value)-1; index++ {
		char := value[index]
		if !isASCIIAlphaNumeric(char) && char != '-' && char != '_' && char != '.' {
			return false
		}
	}
	return true
}

func validLabelValue(value string) bool {
	if len(value) == 0 || len(value) > 63 ||
		!isASCIIAlphaNumeric(value[0]) || !isASCIIAlphaNumeric(value[len(value)-1]) {
		return false
	}
	for index := 1; index < len(value)-1; index++ {
		char := value[index]
		if !isASCIIAlphaNumeric(char) && char != '-' && char != '_' && char != '.' {
			return false
		}
	}
	return true
}

func validDNSHost(value string) bool {
	if len(value) == 0 || len(value) > 253 {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if len(label) == 0 || len(label) > 63 || !isLowerAlphaNumeric(label[0]) || !isLowerAlphaNumeric(label[len(label)-1]) {
			return false
		}
		for index := 1; index < len(label)-1; index++ {
			char := label[index]
			if !isLowerAlphaNumeric(char) && char != '-' {
				return false
			}
		}
	}
	return true
}

func isLowerAlphaNumeric(value byte) bool {
	return (value >= 'a' && value <= 'z') || (value >= '0' && value <= '9')
}

func isASCIIAlphaNumeric(value byte) bool {
	return (value >= 'a' && value <= 'z') || (value >= 'A' && value <= 'Z') || (value >= '0' && value <= '9')
}

func sortedLabelKeys(labels map[string]string) []string {
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func invalidInput() error {
	return registry.NewOutcomeError(registry.OutcomeInvalid, registry.CauseInvalidInput)
}
