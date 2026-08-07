package registry

import (
	"sort"
)

// AddressType identifies the address family represented by a NetworkEndpoint.
// Providers may define additional valid values only in a later contract.
type AddressType string

const (
	AddressTypeIPv4 AddressType = "IPv4"
	AddressTypeIPv6 AddressType = "IPv6"
)

// TransportProtocol identifies the transport protocol of a NetworkEndpoint.
type TransportProtocol string

const (
	TransportProtocolTCP TransportProtocol = "TCP"
)

// NetworkEndpointInput is the input form of one network endpoint tuple.
type NetworkEndpointInput struct {
	AddressType AddressType
	Address     string
	PortName    string
	Port        int
	Protocol    TransportProtocol
}

// NetworkEndpoint is an immutable address, port, and transport tuple.
type NetworkEndpoint struct {
	addressType AddressType
	address     string
	portName    string
	port        int
	protocol    TransportProtocol
}

// NewNetworkEndpoint validates and copies one endpoint tuple.
func NewNetworkEndpoint(input NetworkEndpointInput) (NetworkEndpoint, error) {
	endpoint := NetworkEndpoint{
		addressType: input.AddressType,
		address:     input.Address,
		portName:    input.PortName,
		port:        input.Port,
		protocol:    input.Protocol,
	}
	if err := endpoint.Validate(); err != nil {
		return NetworkEndpoint{}, err
	}
	return endpoint, nil
}

// Validate verifies that this endpoint is complete. Address canonicalization is
// provider-specific; this provider-neutral layer preserves the supplied exact
// canonical address after requiring a non-empty safe value.
func (e NetworkEndpoint) Validate() error {
	if !validExactText(string(e.addressType)) {
		return newInvalidError("address_type")
	}
	if !validExactText(e.address) {
		return newInvalidError("address")
	}
	if !validExactText(e.portName) {
		return newInvalidError("port_name")
	}
	if e.port < 1 || e.port > 65535 {
		return newInvalidError("port")
	}
	if !validExactText(string(e.protocol)) {
		return newInvalidError("protocol")
	}
	return nil
}

func (e NetworkEndpoint) AddressType() AddressType    { return e.addressType }
func (e NetworkEndpoint) Address() string             { return e.address }
func (e NetworkEndpoint) PortName() string            { return e.portName }
func (e NetworkEndpoint) Port() int                   { return e.port }
func (e NetworkEndpoint) Protocol() TransportProtocol { return e.protocol }

// Equal reports tuple equality.
func (e NetworkEndpoint) Equal(other NetworkEndpoint) bool {
	return e.addressType == other.addressType &&
		e.address == other.address &&
		e.portName == other.portName &&
		e.port == other.port &&
		e.protocol == other.protocol
}

func compareNetworkEndpoints(left, right NetworkEndpoint) int {
	if left.addressType != right.addressType {
		if left.addressType < right.addressType {
			return -1
		}
		return 1
	}
	if left.address != right.address {
		if left.address < right.address {
			return -1
		}
		return 1
	}
	if left.portName != right.portName {
		if left.portName < right.portName {
			return -1
		}
		return 1
	}
	if left.port != right.port {
		if left.port < right.port {
			return -1
		}
		return 1
	}
	if left.protocol != right.protocol {
		if left.protocol < right.protocol {
			return -1
		}
		return 1
	}
	return 0
}

// LifecycleState is the derived state of one provider-reported instance.
type LifecycleState string

const (
	LifecycleStateReady       LifecycleState = "ready"
	LifecycleStateUnavailable LifecycleState = "unavailable"
	LifecycleStateDraining    LifecycleState = "draining"

	// Aliases keep state names readable in provider code.
	InstanceStateReady       = LifecycleStateReady
	InstanceStateUnavailable = LifecycleStateUnavailable
	InstanceStateDraining    = LifecycleStateDraining
)

// DeriveLifecycleState applies the v1 lifecycle truth table.
func DeriveLifecycleState(ready, serving, terminating bool) LifecycleState {
	if terminating && serving {
		return LifecycleStateDraining
	}
	if !terminating && serving && ready {
		return LifecycleStateReady
	}
	return LifecycleStateUnavailable
}

// InstanceInput is the input form of one provider-reported instance. State is
// optional and, when supplied, must equal the derived state from the three
// explicit source condition booleans.
type InstanceInput struct {
	ID          string
	Endpoints   []NetworkEndpoint
	Ready       bool
	Serving     bool
	Terminating bool
	State       LifecycleState
	Zone        *string
	Weight      *int
	Metadata    map[string]string
}

// Instance is an immutable provider-reported runtime identity. It never
// selects an endpoint or makes a routing decision.
type Instance struct {
	id          string
	endpoints   []NetworkEndpoint
	ready       bool
	serving     bool
	terminating bool
	state       LifecycleState
	zone        *string
	weight      *int
	metadata    map[string]string
}

// NewInstance validates and copies one instance.
func NewInstance(input InstanceInput) (Instance, error) {
	endpoints, err := normalizeNetworkEndpoints(input.Endpoints)
	if err != nil {
		return Instance{}, err
	}
	instance := Instance{
		id:          input.ID,
		endpoints:   endpoints,
		ready:       input.Ready,
		serving:     input.Serving,
		terminating: input.Terminating,
		zone:        copyStringPointer(input.Zone),
		weight:      copyIntPointer(input.Weight),
		metadata:    cloneStringMap(input.Metadata),
	}
	instance.state = DeriveLifecycleState(instance.ready, instance.serving, instance.terminating)
	if input.State != "" && input.State != instance.state {
		return Instance{}, newInvalidError("lifecycle_state")
	}
	if err := instance.Validate(); err != nil {
		return Instance{}, err
	}
	return instance, nil
}

// Validate verifies the complete, immutable instance shape.
func (i Instance) Validate() error {
	if !validExactText(i.id) {
		return newInvalidError("instance_id")
	}
	if len(i.endpoints) == 0 {
		return newInvalidError("network_endpoints")
	}
	endpoints, err := normalizeNetworkEndpoints(i.endpoints)
	if err != nil {
		return err
	}
	if len(endpoints) != len(i.endpoints) {
		return newInvalidError("network_endpoints")
	}
	for index := range endpoints {
		if !endpoints[index].Equal(i.endpoints[index]) {
			return newInvalidError("network_endpoints_not_sorted")
		}
	}
	if i.state != DeriveLifecycleState(i.ready, i.serving, i.terminating) {
		return newInvalidError("lifecycle_state")
	}
	if i.zone != nil && !validExactText(*i.zone) {
		return newInvalidError("zone")
	}
	for key, value := range i.metadata {
		if !validExactText(key) || !validExactText(value) {
			return newInvalidError("metadata")
		}
	}
	return nil
}

func (i Instance) ID() string                { return i.id }
func (i Instance) Ready() bool               { return i.ready }
func (i Instance) Serving() bool             { return i.serving }
func (i Instance) Terminating() bool         { return i.terminating }
func (i Instance) State() LifecycleState     { return i.state }
func (i Instance) Lifecycle() LifecycleState { return i.state }

// Endpoints returns a sorted immutable-copy view of the network endpoint set.
func (i Instance) Endpoints() []NetworkEndpoint {
	return append([]NetworkEndpoint(nil), i.endpoints...)
}

// Zone returns the optional provider-reported zone without exposing an input
// pointer that could be mutated by a caller.
func (i Instance) Zone() (string, bool) {
	if i.zone == nil {
		return "", false
	}
	return *i.zone, true
}

// Weight returns the optional provider-reported weight. A present zero stays
// present; the directory never infers a weight.
func (i Instance) Weight() (int, bool) {
	if i.weight == nil {
		return 0, false
	}
	return *i.weight, true
}

// Metadata returns a copy of the allowlisted safe metadata selected by the
// provider.
func (i Instance) Metadata() map[string]string {
	return cloneStringMap(i.metadata)
}

// SafeMetadata is an explicit synonym for Metadata.
func (i Instance) SafeMetadata() map[string]string {
	return i.Metadata()
}

// Equal reports complete instance equality, including optional fields and safe
// metadata, but not pointer identity.
func (i Instance) Equal(other Instance) bool {
	if i.id != other.id ||
		i.ready != other.ready ||
		i.serving != other.serving ||
		i.terminating != other.terminating ||
		i.state != other.state ||
		!equalStringPointers(i.zone, other.zone) ||
		!equalIntPointers(i.weight, other.weight) ||
		!equalStringMaps(i.metadata, other.metadata) ||
		len(i.endpoints) != len(other.endpoints) {
		return false
	}
	for index := range i.endpoints {
		if !i.endpoints[index].Equal(other.endpoints[index]) {
			return false
		}
	}
	return true
}

func normalizeInstances(instances []Instance) ([]Instance, error) {
	copyInstances := append([]Instance(nil), instances...)
	for _, instance := range copyInstances {
		if err := instance.Validate(); err != nil {
			return nil, err
		}
	}
	sort.Slice(copyInstances, func(i, j int) bool {
		return copyInstances[i].id < copyInstances[j].id
	})
	for index := range copyInstances {
		if index > 0 && copyInstances[index-1].id == copyInstances[index].id {
			return nil, newInvalidError("duplicate_instance_id")
		}
	}
	return copyInstances, nil
}

func normalizeNetworkEndpoints(endpoints []NetworkEndpoint) ([]NetworkEndpoint, error) {
	copyEndpoints := append([]NetworkEndpoint(nil), endpoints...)
	for _, endpoint := range copyEndpoints {
		if err := endpoint.Validate(); err != nil {
			return nil, err
		}
	}
	sort.Slice(copyEndpoints, func(left, right int) bool {
		return compareNetworkEndpoints(copyEndpoints[left], copyEndpoints[right]) < 0
	})
	for index := 1; index < len(copyEndpoints); index++ {
		if compareNetworkEndpoints(copyEndpoints[index-1], copyEndpoints[index]) == 0 {
			return nil, newInvalidError("duplicate_network_endpoint")
		}
	}
	return copyEndpoints, nil
}

// SnapshotState identifies the complete topology state of a bound target.
type SnapshotState string

const (
	SnapshotStateMissing   SnapshotState = "missing"
	SnapshotStateEmpty     SnapshotState = "empty"
	SnapshotStatePopulated SnapshotState = "populated"

	// Short aliases are retained for ergonomic state comparisons.
	SnapshotMissing   = SnapshotStateMissing
	SnapshotEmpty     = SnapshotStateEmpty
	SnapshotPopulated = SnapshotStatePopulated
)

// RevisionInput is an opaque provider revision plus observation-local order.
// Source tokens are never compared by this package.
type RevisionInput struct {
	SourceTokens []string
	LocalOrder   uint64
}

// Revision is an immutable opaque provider revision scoped to one observation.
type Revision struct {
	sourceTokens []string
	localOrder   uint64
}

// NewRevision validates and copies an opaque revision.
func NewRevision(input RevisionInput) (Revision, error) {
	revision := Revision{
		sourceTokens: append([]string(nil), input.SourceTokens...),
		localOrder:   input.LocalOrder,
	}
	if err := revision.Validate(); err != nil {
		return Revision{}, err
	}
	return revision, nil
}

// Validate verifies a complete opaque revision. The source token values are
// preserved and never parsed or ordered.
func (r Revision) Validate() error {
	if len(r.sourceTokens) == 0 {
		return newInvalidError("revision_source_tokens")
	}
	for _, sourceToken := range r.sourceTokens {
		if !validExactText(sourceToken) {
			return newInvalidError("revision_source_token")
		}
	}
	return nil
}

// SourceTokens returns a copy of the opaque source token sequence.
func (r Revision) SourceTokens() []string {
	return append([]string(nil), r.sourceTokens...)
}

// LocalOrder returns the observation-local logical ordering number.
func (r Revision) LocalOrder() uint64 { return r.localOrder }

// Equal reports equal local order and byte-exact source token sequence.
func (r Revision) Equal(other Revision) bool {
	if r.localOrder != other.localOrder || len(r.sourceTokens) != len(other.sourceTokens) {
		return false
	}
	for index := range r.sourceTokens {
		if r.sourceTokens[index] != other.sourceTokens[index] {
			return false
		}
	}
	return true
}

// InstanceSnapshotInput is the input form of one complete topology snapshot.
type InstanceSnapshotInput struct {
	Target    ReleaseTarget
	Revision  Revision
	State     SnapshotState
	Instances []Instance
}

// SnapshotInput is a compatibility-friendly short name for
// InstanceSnapshotInput.
type SnapshotInput = InstanceSnapshotInput

// InstanceSnapshot is an immutable complete topology view for one target.
type InstanceSnapshot struct {
	target    ReleaseTarget
	revision  Revision
	state     SnapshotState
	instances []Instance
}

// NewInstanceSnapshot validates and copies one complete topology snapshot.
func NewInstanceSnapshot(input InstanceSnapshotInput) (InstanceSnapshot, error) {
	instances, err := normalizeInstances(input.Instances)
	if err != nil {
		return InstanceSnapshot{}, err
	}
	snapshot := InstanceSnapshot{
		target:    input.Target,
		revision:  Revision{sourceTokens: append([]string(nil), input.Revision.sourceTokens...), localOrder: input.Revision.localOrder},
		state:     input.State,
		instances: instances,
	}
	if err := snapshot.Validate(); err != nil {
		return InstanceSnapshot{}, err
	}
	return snapshot, nil
}

// NewSnapshot is a short alias for NewInstanceSnapshot.
func NewSnapshot(input SnapshotInput) (InstanceSnapshot, error) {
	return NewInstanceSnapshot(input)
}

// Validate verifies the complete snapshot and its state/instance relation.
func (s InstanceSnapshot) Validate() error {
	if err := s.target.Validate(); err != nil {
		return err
	}
	if err := s.revision.Validate(); err != nil {
		return err
	}
	instances, err := normalizeInstances(s.instances)
	if err != nil {
		return err
	}
	if len(instances) != len(s.instances) {
		return newInvalidError("instances")
	}
	for index := range instances {
		if !instances[index].Equal(s.instances[index]) {
			return newInvalidError("instances_not_sorted")
		}
	}
	switch s.state {
	case SnapshotStateMissing, SnapshotStateEmpty:
		if len(s.instances) != 0 {
			return newInvalidError("snapshot_state")
		}
	case SnapshotStatePopulated:
		if len(s.instances) == 0 {
			return newInvalidError("snapshot_state")
		}
	default:
		return newInvalidError("snapshot_state")
	}
	return nil
}

func validSnapshotState(state SnapshotState) bool {
	switch state {
	case SnapshotStateMissing, SnapshotStateEmpty, SnapshotStatePopulated:
		return true
	default:
		return false
	}
}

func (s InstanceSnapshot) Target() ReleaseTarget { return s.target }
func (s InstanceSnapshot) Revision() Revision    { return cloneRevision(s.revision) }
func (s InstanceSnapshot) State() SnapshotState  { return s.state }

// Instances returns a sorted copy of the immutable instance set.
func (s InstanceSnapshot) Instances() []Instance {
	return cloneInstances(s.instances)
}

// Equal reports complete snapshot equality.
func (s InstanceSnapshot) Equal(other InstanceSnapshot) bool {
	if !s.target.Equal(other.target) || !s.revision.Equal(other.revision) || s.state != other.state || len(s.instances) != len(other.instances) {
		return false
	}
	for index := range s.instances {
		if !s.instances[index].Equal(other.instances[index]) {
			return false
		}
	}
	return true
}

// InstanceChangeKind identifies a complete snapshot transition.
type InstanceChangeKind string

const (
	InstanceChangeInstancesChanged InstanceChangeKind = "instances_changed"
	// InstanceChangeStateChanged records a transition whose instance set is
	// unchanged but whose explicit snapshot state changed (for example, a
	// bound Service appearing after a missing snapshot).
	InstanceChangeStateChanged  InstanceChangeKind = "state_changed"
	InstanceChangeTargetDeleted InstanceChangeKind = "target_deleted"

	ChangeInstancesChanged = InstanceChangeInstancesChanged
	ChangeStateChanged     = InstanceChangeStateChanged
	ChangeTargetDeleted    = InstanceChangeTargetDeleted
)

// InstanceChangeInput is the input form of one logical topology change.
type InstanceChangeInput struct {
	Kind               InstanceChangeKind
	Revision           Revision
	Upserts            []Instance
	DeletedInstanceIDs []string
	// PreviousState is required only for InstanceChangeStateChanged. It is
	// deliberately a state value rather than a second snapshot so callers
	// cannot smuggle an unrelated topology or revision into the transition.
	PreviousState SnapshotState
	Snapshot      InstanceSnapshot
}

// InstanceChange is an immutable aggregate topology transition.
type InstanceChange struct {
	kind               InstanceChangeKind
	revision           Revision
	upserts            []Instance
	deletedInstanceIDs []string
	previousState      SnapshotState
	snapshot           InstanceSnapshot
}

// NewInstanceChange validates and copies one complete topology transition.
func NewInstanceChange(input InstanceChangeInput) (InstanceChange, error) {
	upserts, err := normalizeInstances(input.Upserts)
	if err != nil {
		return InstanceChange{}, err
	}
	deletedIDs, err := normalizeDeletedInstanceIDs(input.DeletedInstanceIDs)
	if err != nil {
		return InstanceChange{}, err
	}
	change := InstanceChange{
		kind:               input.Kind,
		revision:           cloneRevision(input.Revision),
		upserts:            upserts,
		deletedInstanceIDs: deletedIDs,
		previousState:      input.PreviousState,
		snapshot:           cloneSnapshot(input.Snapshot),
	}
	if err := change.Validate(); err != nil {
		return InstanceChange{}, err
	}
	return change, nil
}

// Validate verifies all transition invariants.
func (c InstanceChange) Validate() error {
	if err := c.revision.Validate(); err != nil {
		return err
	}
	if c.revision.localOrder == 0 {
		return newInvalidError("change_local_order")
	}
	if err := c.snapshot.Validate(); err != nil {
		return err
	}
	if !c.revision.Equal(c.snapshot.revision) {
		return newInvalidError("change_snapshot_revision")
	}
	upserts, err := normalizeInstances(c.upserts)
	if err != nil {
		return err
	}
	if !equalInstances(upserts, c.upserts) {
		return newInvalidError("upserts_not_sorted")
	}
	deletedIDs, err := normalizeDeletedInstanceIDs(c.deletedInstanceIDs)
	if err != nil {
		return err
	}
	if !equalStrings(deletedIDs, c.deletedInstanceIDs) {
		return newInvalidError("deleted_instance_ids_not_sorted")
	}

	snapshotInstances := make(map[string]Instance, len(c.snapshot.instances))
	for _, instance := range c.snapshot.instances {
		snapshotInstances[instance.id] = instance
	}
	for _, instance := range c.upserts {
		result, found := snapshotInstances[instance.id]
		if !found {
			return newInvalidError("upsert_missing_from_snapshot")
		}
		if !instance.Equal(result) {
			return newInvalidError("upsert_does_not_match_snapshot")
		}
	}
	for _, id := range c.deletedInstanceIDs {
		if _, found := snapshotInstances[id]; found {
			return newInvalidError("deleted_instance_present_in_snapshot")
		}
	}

	switch c.kind {
	case InstanceChangeInstancesChanged:
		if len(c.upserts) == 0 && len(c.deletedInstanceIDs) == 0 {
			return newInvalidError("empty_instance_change")
		}
		if c.previousState != "" {
			return newInvalidError("instances_changed_previous_state")
		}
	case InstanceChangeStateChanged:
		if !validSnapshotState(c.previousState) || c.previousState == c.snapshot.state ||
			c.previousState == SnapshotStatePopulated || c.snapshot.state == SnapshotStatePopulated {
			return newInvalidError("state_change_previous_state")
		}
		if len(c.upserts) != 0 || len(c.deletedInstanceIDs) != 0 {
			return newInvalidError("state_change_delta")
		}
	case InstanceChangeTargetDeleted:
		if c.previousState != "" || c.snapshot.state != SnapshotStateMissing || len(c.upserts) != 0 || len(c.deletedInstanceIDs) != 0 {
			return newInvalidError("target_deleted_change")
		}
	default:
		return newInvalidError("change_kind")
	}
	return nil
}

func (c InstanceChange) Kind() InstanceChangeKind   { return c.kind }
func (c InstanceChange) Revision() Revision         { return cloneRevision(c.revision) }
func (c InstanceChange) Snapshot() InstanceSnapshot { return cloneSnapshot(c.snapshot) }

// PreviousState returns the prior explicit snapshot state for a state-only
// transition. It is empty for instances_changed and target_deleted changes.
func (c InstanceChange) PreviousState() SnapshotState { return c.previousState }

// Upserts returns a sorted copy of upserted instances.
func (c InstanceChange) Upserts() []Instance {
	return cloneInstances(c.upserts)
}

// DeletedInstanceIDs returns a sorted copy of deleted instance IDs.
func (c InstanceChange) DeletedInstanceIDs() []string {
	return append([]string(nil), c.deletedInstanceIDs...)
}

// Equal reports complete change equality.
func (c InstanceChange) Equal(other InstanceChange) bool {
	return c.kind == other.kind &&
		c.revision.Equal(other.revision) &&
		equalInstances(c.upserts, other.upserts) &&
		equalStrings(c.deletedInstanceIDs, other.deletedInstanceIDs) &&
		c.previousState == other.previousState &&
		c.snapshot.Equal(other.snapshot)
}

func normalizeDeletedInstanceIDs(ids []string) ([]string, error) {
	copyIDs := append([]string(nil), ids...)
	for _, id := range copyIDs {
		if !validExactText(id) {
			return nil, newInvalidError("deleted_instance_id")
		}
	}
	sort.Strings(copyIDs)
	for index := 1; index < len(copyIDs); index++ {
		if copyIDs[index-1] == copyIDs[index] {
			return nil, newInvalidError("duplicate_deleted_instance_id")
		}
	}
	return copyIDs, nil
}

func cloneRevision(revision Revision) Revision {
	return Revision{sourceTokens: append([]string(nil), revision.sourceTokens...), localOrder: revision.localOrder}
}

func cloneSnapshot(snapshot InstanceSnapshot) InstanceSnapshot {
	return InstanceSnapshot{
		target:    snapshot.target,
		revision:  cloneRevision(snapshot.revision),
		state:     snapshot.state,
		instances: cloneInstances(snapshot.instances),
	}
}

func cloneChange(change InstanceChange) InstanceChange {
	return InstanceChange{
		kind:               change.kind,
		revision:           cloneRevision(change.revision),
		upserts:            cloneInstances(change.upserts),
		deletedInstanceIDs: append([]string(nil), change.deletedInstanceIDs...),
		previousState:      change.previousState,
		snapshot:           cloneSnapshot(change.snapshot),
	}
}

func cloneInstances(instances []Instance) []Instance {
	copyInstances := make([]Instance, len(instances))
	for index, instance := range instances {
		copyInstances[index] = Instance{
			id:          instance.id,
			endpoints:   append([]NetworkEndpoint(nil), instance.endpoints...),
			ready:       instance.ready,
			serving:     instance.serving,
			terminating: instance.terminating,
			state:       instance.state,
			zone:        copyStringPointer(instance.zone),
			weight:      copyIntPointer(instance.weight),
			metadata:    cloneStringMap(instance.metadata),
		}
	}
	return copyInstances
}

func equalInstances(left, right []Instance) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !left[index].Equal(right[index]) {
			return false
		}
	}
	return true
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func equalStringMaps(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

func copyStringPointer(input *string) *string {
	if input == nil {
		return nil
	}
	value := *input
	return &value
}

func copyIntPointer(input *int) *int {
	if input == nil {
		return nil
	}
	value := *input
	return &value
}

func equalStringPointers(left, right *string) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

func equalIntPointers(left, right *int) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}
