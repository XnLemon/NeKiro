package kubernetes

import (
	"net/netip"
	"sort"
	"strconv"

	"github.com/NeKiro-project/NeKiro/registry"
)

type sliceRecord struct {
	uid       string
	endpoints []endpointRecord
}

type endpointRecord struct {
	id          string
	endpoints   []registry.NetworkEndpoint
	ready       bool
	serving     bool
	terminating bool
	zone        *string
	metadata    map[string]string
}

type topologyState struct {
	servicePresent          bool
	serviceResourceVersion  string
	endpointResourceVersion string
	slices                  map[string]sliceRecord
}

func validateService(service wireService, binding Binding, requireResourceVersion bool) error {
	metadata := service.Metadata
	if metadata.Name != binding.serviceName || metadata.Namespace != binding.namespace ||
		metadata.UID != binding.serviceUID || !validOpaqueIdentifier(metadata.UID) ||
		(requireResourceVersion && !validResourceVersion(metadata.ResourceVersion)) ||
		!hasRequiredLabels(metadata.Labels, binding.ServiceLabels()) {
		return invalidInput()
	}
	return nil
}

func validateEndpointSliceIdentity(slice wireEndpointSlice, binding Binding, requireResourceVersion bool) error {
	metadata := slice.Metadata
	if !validOpaqueIdentifier(metadata.Name) || metadata.Namespace != binding.namespace ||
		!validOpaqueIdentifier(metadata.UID) ||
		(requireResourceVersion && !validResourceVersion(metadata.ResourceVersion)) ||
		!hasRequiredLabels(metadata.Labels, binding.EndpointSliceLabelsForObject()) {
		return invalidInput()
	}
	if slice.AddressType != nil && *slice.AddressType != string(binding.addressType) {
		return invalidInput()
	}
	foundServiceOwner := false
	for _, owner := range metadata.OwnerReferences {
		if owner.Kind != "Service" {
			continue
		}
		if owner.APIVersion != "v1" || owner.Name != binding.serviceName || owner.UID != binding.serviceUID {
			return invalidInput()
		}
		foundServiceOwner = true
	}
	if !foundServiceOwner {
		return invalidInput()
	}
	return nil
}

func hasRequiredLabels(actual, required map[string]string) bool {
	for key, expected := range required {
		if actual[key] != expected {
			return false
		}
	}
	return true
}

func normalizeEndpointSlice(slice wireEndpointSlice, binding Binding, requireResourceVersion bool) (sliceRecord, error) {
	if err := validateEndpointSliceIdentity(slice, binding, requireResourceVersion); err != nil {
		return sliceRecord{}, err
	}
	if slice.AddressType == nil || *slice.AddressType != string(binding.addressType) {
		return sliceRecord{}, invalidInput()
	}
	port, err := configuredPort(slice.Ports, binding)
	if err != nil {
		return sliceRecord{}, err
	}
	record := sliceRecord{
		uid:       slice.Metadata.UID,
		endpoints: make([]endpointRecord, 0, len(slice.Endpoints)),
	}
	for _, endpoint := range slice.Endpoints {
		normalized, err := normalizeEndpoint(endpoint, binding, port)
		if err != nil {
			return sliceRecord{}, err
		}
		record.endpoints = append(record.endpoints, normalized)
	}
	return record, nil
}

func configuredPort(ports []wireEndpointPort, binding Binding) (int, error) {
	matchCount := 0
	port := 0
	for _, candidate := range ports {
		if candidate.Name == nil || candidate.Protocol == nil || *candidate.Name != binding.portName ||
			*candidate.Protocol != string(binding.protocol) {
			continue
		}
		matchCount++
		if candidate.Port == nil || *candidate.Port < 1 || *candidate.Port > 65535 {
			return 0, invalidInput()
		}
		port = *candidate.Port
	}
	if matchCount != 1 {
		return 0, invalidInput()
	}
	return port, nil
}

func normalizeEndpoint(endpoint wireEndpoint, binding Binding, port int) (endpointRecord, error) {
	if endpoint.TargetRef == nil || !validOpaqueIdentifier(endpoint.TargetRef.UID) || len(endpoint.Addresses) == 0 {
		return endpointRecord{}, invalidInput()
	}
	if endpoint.Zone != nil && !validOpaqueIdentifier(*endpoint.Zone) {
		return endpointRecord{}, invalidInput()
	}
	networkEndpoints := make([]registry.NetworkEndpoint, 0, len(endpoint.Addresses))
	for _, address := range endpoint.Addresses {
		if !validCanonicalAddress(address, binding.addressType) {
			return endpointRecord{}, invalidInput()
		}
		networkEndpoint, err := registry.NewNetworkEndpoint(registry.NetworkEndpointInput{
			AddressType: binding.addressType,
			Address:     address,
			PortName:    binding.portName,
			Port:        port,
			Protocol:    binding.protocol,
		})
		if err != nil {
			return endpointRecord{}, err
		}
		networkEndpoints = append(networkEndpoints, networkEndpoint)
	}
	ready := endpoint.Conditions.Ready == nil || *endpoint.Conditions.Ready
	serving := endpoint.Conditions.Serving == nil || *endpoint.Conditions.Serving
	terminating := endpoint.Conditions.Terminating != nil && *endpoint.Conditions.Terminating
	uid := endpoint.TargetRef.UID
	return endpointRecord{
		id:          uid,
		endpoints:   networkEndpoints,
		ready:       ready,
		serving:     serving,
		terminating: terminating,
		zone:        copyStringPointer(endpoint.Zone),
		metadata: map[string]string{
			MetadataTargetRefUID: uid,
			MetadataAddressType:  string(binding.addressType),
		},
	}, nil
}

func validCanonicalAddress(raw string, addressType registry.AddressType) bool {
	address, err := netip.ParseAddr(raw)
	if err != nil || address.String() != raw {
		return false
	}
	switch addressType {
	case registry.AddressTypeIPv4:
		return address.Is4()
	case registry.AddressTypeIPv6:
		return address.Is6() && !address.Is4In6()
	default:
		return false
	}
}

func (s topologyState) snapshot(target registry.ReleaseTarget, localOrder uint64, binding Binding) (registry.InstanceSnapshot, error) {
	instances, err := s.instances(binding)
	if err != nil {
		return registry.InstanceSnapshot{}, err
	}
	state := registry.SnapshotStateMissing
	if s.servicePresent {
		state = registry.SnapshotStateEmpty
		if len(instances) > 0 {
			state = registry.SnapshotStatePopulated
		}
	}
	revision, err := registry.NewRevision(registry.RevisionInput{
		SourceTokens: []string{s.serviceResourceVersion, s.endpointResourceVersion},
		LocalOrder:   localOrder,
	})
	if err != nil {
		return registry.InstanceSnapshot{}, err
	}
	return registry.NewInstanceSnapshot(registry.InstanceSnapshotInput{
		Target:    target,
		Revision:  revision,
		State:     state,
		Instances: instances,
	})
}

func (s topologyState) instances(binding Binding) ([]registry.Instance, error) {
	byID := make(map[string]aggregateInstance)
	tupleOwners := make(map[string]string)
	for _, slice := range s.slices {
		for _, endpoint := range slice.endpoints {
			aggregate, exists := byID[endpoint.id]
			if !exists {
				aggregate = aggregateInstance{
					id:          endpoint.id,
					ready:       endpoint.ready,
					serving:     endpoint.serving,
					terminating: endpoint.terminating,
					zone:        copyStringPointer(endpoint.zone),
					metadata:    cloneStringMap(endpoint.metadata),
					endpoints:   make(map[string]registry.NetworkEndpoint),
				}
			} else if aggregate.ready != endpoint.ready || aggregate.serving != endpoint.serving ||
				aggregate.terminating != endpoint.terminating || !equalStringPointer(aggregate.zone, endpoint.zone) ||
				!equalStringMap(aggregate.metadata, endpoint.metadata) {
				return nil, invalidInput()
			}
			for _, networkEndpoint := range endpoint.endpoints {
				key := networkEndpointKey(networkEndpoint)
				if owner, claimed := tupleOwners[key]; claimed && owner != endpoint.id {
					return nil, invalidInput()
				}
				tupleOwners[key] = endpoint.id
				aggregate.endpoints[key] = networkEndpoint
			}
			byID[endpoint.id] = aggregate
		}
	}
	if !s.servicePresent {
		return nil, nil
	}
	instances := make([]registry.Instance, 0, len(byID))
	for _, aggregate := range byID {
		endpoints := make([]registry.NetworkEndpoint, 0, len(aggregate.endpoints))
		for _, endpoint := range aggregate.endpoints {
			endpoints = append(endpoints, endpoint)
		}
		instance, err := registry.NewInstance(registry.InstanceInput{
			ID:          aggregate.id,
			Endpoints:   endpoints,
			Ready:       aggregate.ready,
			Serving:     aggregate.serving,
			Terminating: aggregate.terminating,
			Zone:        copyStringPointer(aggregate.zone),
			Metadata:    cloneStringMap(aggregate.metadata),
		})
		if err != nil {
			return nil, err
		}
		instances = append(instances, instance)
	}
	// registry.NewInstanceSnapshot sorts instances, but sorting here also keeps
	// all provider-internal comparisons deterministic before construction.
	sort.Slice(instances, func(left, right int) bool { return instances[left].ID() < instances[right].ID() })
	return instances, nil
}

type aggregateInstance struct {
	id          string
	ready       bool
	serving     bool
	terminating bool
	zone        *string
	metadata    map[string]string
	endpoints   map[string]registry.NetworkEndpoint
}

func networkEndpointKey(endpoint registry.NetworkEndpoint) string {
	return string(endpoint.AddressType()) + "\x00" + endpoint.Address() + "\x00" + endpoint.PortName() + "\x00" +
		strconv.Itoa(endpoint.Port()) + "\x00" + string(endpoint.Protocol())
}

func copyStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func equalStringPointer(left, right *string) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

func equalStringMap(left, right map[string]string) bool {
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
