package kubernetes

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/NeKiro-project/NeKiro/registry"
)

type wireListMeta struct {
	ResourceVersion string `json:"resourceVersion"`
	Continue        string `json:"continue"`
}

type wireObjectMeta struct {
	Name            string            `json:"name"`
	Namespace       string            `json:"namespace"`
	UID             string            `json:"uid"`
	ResourceVersion string            `json:"resourceVersion"`
	Labels          map[string]string `json:"labels"`
	OwnerReferences []wireOwnerRef    `json:"ownerReferences"`
}

type wireOwnerRef struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	UID        string `json:"uid"`
}

type wireServiceList struct {
	Metadata wireListMeta  `json:"metadata"`
	Items    []wireService `json:"items"`
}

type wireService struct {
	Metadata wireObjectMeta `json:"metadata"`
}

type wireEndpointSliceList struct {
	Metadata wireListMeta        `json:"metadata"`
	Items    []wireEndpointSlice `json:"items"`
}

type wireEndpointSlice struct {
	Metadata    wireObjectMeta     `json:"metadata"`
	AddressType *string            `json:"addressType"`
	Ports       []wireEndpointPort `json:"ports"`
	Endpoints   []wireEndpoint     `json:"endpoints"`
}

type wireEndpointPort struct {
	Name     *string `json:"name"`
	Protocol *string `json:"protocol"`
	Port     *int    `json:"port"`
}

type wireEndpoint struct {
	Addresses  []string               `json:"addresses"`
	Conditions wireEndpointConditions `json:"conditions"`
	TargetRef  *wireObjectReference   `json:"targetRef"`
	Zone       *string                `json:"zone"`
}

type wireEndpointConditions struct {
	Ready       *bool `json:"ready"`
	Serving     *bool `json:"serving"`
	Terminating *bool `json:"terminating"`
}

type wireObjectReference struct {
	UID string `json:"uid"`
}

type wireWatchEvent struct {
	Type   string          `json:"type"`
	Object json.RawMessage `json:"object"`
}

type wireStatus struct {
	Code   int    `json:"code"`
	Reason string `json:"reason"`
}

func decodeServiceList(payload []byte, binding Binding) (servicePresent bool, resourceVersion string, err error) {
	var list wireServiceList
	if err := decodeOneJSON(payload, &list); err != nil {
		return false, "", invalidInput()
	}
	if !validResourceVersion(list.Metadata.ResourceVersion) || list.Metadata.Continue != "" || len(list.Items) > 1 {
		return false, "", invalidInput()
	}
	if len(list.Items) == 0 {
		return false, list.Metadata.ResourceVersion, nil
	}
	if err := validateService(list.Items[0], binding, false); err != nil {
		return false, "", err
	}
	return true, list.Metadata.ResourceVersion, nil
}

func decodeEndpointSliceList(payload []byte, binding Binding) (map[string]sliceRecord, string, error) {
	var list wireEndpointSliceList
	if err := decodeOneJSON(payload, &list); err != nil {
		return nil, "", invalidInput()
	}
	if !validResourceVersion(list.Metadata.ResourceVersion) || list.Metadata.Continue != "" || len(list.Items) > binding.bounds.EndpointSliceCount {
		return nil, "", invalidInput()
	}

	slices := make(map[string]sliceRecord, len(list.Items))
	endpointCount := 0
	for _, item := range list.Items {
		record, err := normalizeEndpointSlice(item, binding, false)
		if err != nil {
			return nil, "", err
		}
		endpointCount += len(item.Endpoints)
		if endpointCount > binding.bounds.EndpointCount {
			return nil, "", invalidInput()
		}
		if _, exists := slices[record.uid]; exists {
			return nil, "", invalidInput()
		}
		slices[record.uid] = record
	}
	return slices, list.Metadata.ResourceVersion, nil
}

func decodeWatchEvent(payload []byte) (wireWatchEvent, error) {
	var event wireWatchEvent
	if err := decodeOneJSON(payload, &event); err != nil {
		return wireWatchEvent{}, err
	}
	if len(event.Object) == 0 {
		return wireWatchEvent{}, io.ErrUnexpectedEOF
	}
	switch event.Type {
	case "ADDED", "MODIFIED", "DELETED", "ERROR":
		return event, nil
	default:
		return wireWatchEvent{}, io.ErrUnexpectedEOF
	}
}

func decodeOneJSON(payload []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return io.ErrUnexpectedEOF
		}
		return err
	}
	return nil
}

func decodeServiceObject(payload []byte, binding Binding, requireResourceVersion bool) (wireService, error) {
	var service wireService
	if err := decodeOneJSON(payload, &service); err != nil {
		return wireService{}, invalidInput()
	}
	if err := validateService(service, binding, requireResourceVersion); err != nil {
		return wireService{}, err
	}
	return service, nil
}

func decodeEndpointSliceObject(payload []byte, binding Binding, requireResourceVersion bool) (sliceRecord, error) {
	var slice wireEndpointSlice
	if err := decodeOneJSON(payload, &slice); err != nil {
		return sliceRecord{}, invalidInput()
	}
	return normalizeEndpointSlice(slice, binding, requireResourceVersion)
}

func decodeEndpointSliceDelete(payload []byte, binding Binding, requireResourceVersion bool) (string, string, error) {
	var slice wireEndpointSlice
	if err := decodeOneJSON(payload, &slice); err != nil {
		return "", "", invalidInput()
	}
	if err := validateEndpointSliceIdentity(slice, binding, requireResourceVersion); err != nil {
		return "", "", err
	}
	return slice.Metadata.UID, slice.Metadata.ResourceVersion, nil
}

func decodeWatchStatus(payload []byte) (wireStatus, error) {
	var status wireStatus
	if err := decodeOneJSON(payload, &status); err != nil {
		return wireStatus{}, err
	}
	return status, nil
}

func validResourceVersion(value string) bool {
	return validOpaqueIdentifier(value)
}

func outcomeFromWatchStatus(status wireStatus) error {
	if status.Code == 410 {
		return registry.NewOutcomeError(registry.OutcomeStale, registry.CauseResourceVersionExpired)
	}
	return registry.NewOutcomeError(registry.OutcomeWatchInterrupted, registry.CauseWatchStatusError)
}
