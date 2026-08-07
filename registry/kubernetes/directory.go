package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"reflect"
	"strings"
	"sync"

	"github.com/NeKiro-project/NeKiro/registry"
)

// DirectoryConfig supplies immutable Bindings and the required constrained
// Kubernetes request executor. An empty binding set is valid and reports the
// provider-neutral missing outcome for every target.
type DirectoryConfig struct {
	Bindings []Binding
	Executor KubernetesRequestExecutor
}

// Config is a short compatibility-friendly alias for DirectoryConfig.
type Config = DirectoryConfig

// Directory is a read/watch-only Kubernetes EndpointSlice InstanceDirectory.
type Directory struct {
	mu sync.Mutex

	executor   KubernetesRequestExecutor
	bindings   map[registry.ReleaseTarget]Binding
	capability registry.Capabilities
	closed     bool
	sessions   map[*observationSession]struct{}
}

// NewDirectory validates all explicit provider configuration and constructs a
// Kubernetes EndpointSlice directory. It never creates a default client.
func NewDirectory(config DirectoryConfig) (*Directory, error) {
	if isNilExecutor(config.Executor) {
		return nil, invalidInput()
	}
	if err := config.Executor.Guarantees().Validate(); err != nil {
		return nil, err
	}
	bindings := make(map[registry.ReleaseTarget]Binding, len(config.Bindings))
	for _, binding := range config.Bindings {
		if err := binding.Validate(); err != nil {
			return nil, err
		}
		target := binding.Target()
		if _, exists := bindings[target]; exists {
			return nil, invalidInput()
		}
		bindings[target] = binding
	}
	capability, err := registry.NewCapabilities(registry.CapabilitySnapshot, registry.CapabilityObserve)
	if err != nil {
		return nil, err
	}
	return &Directory{
		executor:   config.Executor,
		bindings:   bindings,
		capability: capability,
		sessions:   make(map[*observationSession]struct{}),
	}, nil
}

// NewEndpointSliceDirectory is an explicit provider-name alias.
func NewEndpointSliceDirectory(config DirectoryConfig) (*Directory, error) {
	return NewDirectory(config)
}

func (d *Directory) Capabilities() registry.Capabilities { return d.capability }

func (d *Directory) Snapshot(ctx context.Context, target registry.ReleaseTarget) (registry.InstanceSnapshot, error) {
	if err := validateContext(ctx); err != nil {
		return registry.InstanceSnapshot{}, err
	}
	binding, err := d.bindingFor(target)
	if err != nil {
		return registry.InstanceSnapshot{}, err
	}
	state, err := d.acquireLists(ctx, binding)
	if err != nil {
		return registry.InstanceSnapshot{}, err
	}
	if d.isClosed() {
		return registry.InstanceSnapshot{}, registry.ErrClosed
	}
	return state.snapshot(target, 0, binding)
}

func (d *Directory) Observe(ctx context.Context, target registry.ReleaseTarget) (registry.InstanceObservation, error) {
	if err := validateContext(ctx); err != nil {
		return registry.InstanceObservation{}, err
	}
	binding, err := d.bindingFor(target)
	if err != nil {
		return registry.InstanceObservation{}, err
	}
	state, err := d.acquireLists(ctx, binding)
	if err != nil {
		return registry.InstanceObservation{}, err
	}
	initial, err := state.snapshot(target, 0, binding)
	if err != nil {
		return registry.InstanceObservation{}, err
	}
	if err := validateContext(ctx); err != nil {
		return registry.InstanceObservation{}, err
	}
	watch, publisher, err := registry.NewInstanceWatch(binding.bounds.PendingChanges)
	if err != nil {
		return registry.InstanceObservation{}, err
	}
	session := newObservationSession(d, binding, target, state, initial, watch, publisher)
	if !d.registerSession(session) {
		_ = session.close()
		return registry.InstanceObservation{}, registry.ErrClosed
	}

	serviceBody, err := d.openWatch(ctx, binding, watchService, state.serviceResourceVersion)
	if err != nil {
		session.fail(err)
		return registry.InstanceObservation{}, err
	}
	if !session.attachBody(watchService, serviceBody) {
		return registry.InstanceObservation{}, session.terminalError()
	}
	go session.readSource(watchService, serviceBody)

	endpointBody, err := d.openWatch(ctx, binding, watchEndpointSlice, state.endpointResourceVersion)
	if err != nil {
		session.fail(err)
		return registry.InstanceObservation{}, err
	}
	if !session.attachBody(watchEndpointSlice, endpointBody) {
		return registry.InstanceObservation{}, session.terminalError()
	}
	go session.readSource(watchEndpointSlice, endpointBody)

	if err := session.markWatchOpen(watchService); err != nil {
		session.fail(err)
		return registry.InstanceObservation{}, err
	}
	if err := session.markWatchOpen(watchEndpointSlice); err != nil {
		session.fail(err)
		return registry.InstanceObservation{}, err
	}
	if err := session.awaitBarrier(ctx); err != nil {
		session.fail(err)
		return registry.InstanceObservation{}, err
	}
	return registry.NewInstanceObservation(initial, session.watch)
}

func (d *Directory) Close() error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil
	}
	d.closed = true
	sessions := make([]*observationSession, 0, len(d.sessions))
	for session := range d.sessions {
		sessions = append(sessions, session)
	}
	d.sessions = make(map[*observationSession]struct{})
	// No later operation can use these bindings. Existing sessions retain only
	// their private immutable Binding copy until they terminate.
	d.bindings = nil
	d.mu.Unlock()
	for _, session := range sessions {
		_ = session.close()
	}
	return nil
}

func (d *Directory) bindingFor(target registry.ReleaseTarget) (Binding, error) {
	if err := target.Validate(); err != nil {
		return Binding{}, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return Binding{}, registry.ErrClosed
	}
	binding, found := d.bindings[target]
	if !found {
		return Binding{}, registry.ErrMissing
	}
	return binding, nil
}

func (d *Directory) registerSession(session *observationSession) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return false
	}
	d.sessions[session] = struct{}{}
	return true
}

func (d *Directory) unregisterSession(session *observationSession) {
	d.mu.Lock()
	delete(d.sessions, session)
	d.mu.Unlock()
}

func (d *Directory) isClosed() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.closed
}

func (d *Directory) acquireLists(ctx context.Context, binding Binding) (topologyState, error) {
	servicePayload, err := d.executeList(ctx, binding, watchService)
	if err != nil {
		return topologyState{}, err
	}
	servicePresent, serviceResourceVersion, err := decodeServiceList(servicePayload, binding)
	if err != nil {
		return topologyState{}, err
	}
	endpointPayload, err := d.executeList(ctx, binding, watchEndpointSlice)
	if err != nil {
		return topologyState{}, err
	}
	slices, endpointResourceVersion, err := decodeEndpointSliceList(endpointPayload, binding)
	if err != nil {
		return topologyState{}, err
	}
	state := topologyState{
		servicePresent:          servicePresent,
		serviceResourceVersion:  serviceResourceVersion,
		endpointResourceVersion: endpointResourceVersion,
		slices:                  slices,
	}
	// Validate the aggregate even for a missing Service. This ensures a
	// malformed second source is never hidden by an empty first source.
	if _, err := state.instances(binding); err != nil {
		return topologyState{}, err
	}
	return state, nil
}

type watchSource string

const (
	watchService       watchSource = "service"
	watchEndpointSlice watchSource = "endpointslice"
)

func (d *Directory) executeList(ctx context.Context, binding Binding, source watchSource) ([]byte, error) {
	if err := validateContext(ctx); err != nil {
		return nil, err
	}
	request := requestFor(binding, source, false, "")
	response, err := d.executor.Execute(ctx, request)
	if err != nil {
		_ = closeBody(response.Body)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, canceledError(ctxErr)
		}
		return nil, registry.NewOutcomeError(registry.OutcomeUnavailable, registry.CauseProviderUnavailable)
	}
	if response.StatusCode != 200 {
		_ = closeBody(response.Body)
		return nil, outcomeFromStatus(response.StatusCode, false)
	}
	if response.Body == nil {
		return nil, invalidInput()
	}
	defer func() { _ = response.Body.Close() }()
	payload, err := readBounded(response.Body, binding.bounds.ListResponseBytes)
	if err != nil {
		if errors.Is(err, errWatchEnvelopeTooLarge) {
			return nil, invalidInput()
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, canceledError(ctxErr)
		}
		return nil, registry.NewOutcomeError(registry.OutcomeUnavailable, registry.CauseProviderUnavailable)
	}
	return payload, nil
}

func (d *Directory) openWatch(ctx context.Context, binding Binding, source watchSource, resourceVersion string) (io.ReadCloser, error) {
	if err := validateContext(ctx); err != nil {
		return nil, err
	}
	if !validResourceVersion(resourceVersion) {
		return nil, invalidInput()
	}
	request := requestFor(binding, source, true, resourceVersion)
	response, err := d.executor.Execute(ctx, request)
	if err != nil {
		_ = closeBody(response.Body)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, canceledError(ctxErr)
		}
		return nil, registry.NewOutcomeError(registry.OutcomeUnavailable, registry.CauseProviderUnavailable)
	}
	if response.StatusCode != 200 {
		_ = closeBody(response.Body)
		return nil, outcomeFromStatus(response.StatusCode, true)
	}
	if response.Body == nil {
		return nil, invalidInput()
	}
	return response.Body, nil
}

func requestFor(binding Binding, source watchSource, watch bool, resourceVersion string) KubernetesRequest {
	var path string
	query := make(url.Values)
	switch source {
	case watchService:
		path = "/api/v1/namespaces/" + url.PathEscape(binding.namespace) + "/services"
		query.Set("fieldSelector", "metadata.name="+binding.serviceName)
	case watchEndpointSlice:
		path = "/apis/discovery.k8s.io/v1/namespaces/" + url.PathEscape(binding.namespace) + "/endpointslices"
		labels := binding.EndpointSliceSelectorLabels()
		keys := sortedLabelKeys(labels)
		selector := make([]string, 0, len(keys))
		for _, key := range keys {
			selector = append(selector, key+"="+labels[key])
		}
		query.Set("labelSelector", strings.Join(selector, ","))
	}
	if watch {
		query.Set("resourceVersion", resourceVersion)
		query.Set("watch", "true")
	}
	return newKubernetesRequest("GET", binding.apiOrigin+path+"?"+query.Encode(), map[string][]string{
		"Accept": {"application/json"},
	})
}

func outcomeFromStatus(statusCode int, _ bool) error {
	switch statusCode {
	case 401:
		return registry.NewOutcomeError(registry.OutcomeUnauthorized, registry.CauseHTTPUnauthorized)
	case 403:
		return registry.NewOutcomeError(registry.OutcomeUnauthorized, registry.CauseHTTPForbidden)
	case 410:
		return registry.NewOutcomeError(registry.OutcomeStale, registry.CauseResourceVersionExpired)
	case 429:
		return registry.NewOutcomeError(registry.OutcomeUnavailable, registry.CauseRateLimited)
	default:
		if statusCode >= 500 && statusCode <= 599 {
			return registry.NewOutcomeError(registry.OutcomeUnavailable, registry.CauseProviderUnavailable)
		}
		return invalidInput()
	}
}

var errWatchEnvelopeTooLarge = errors.New("watch envelope too large")
var errWatchEnvelopeInvalid = errors.New("watch envelope invalid")

func readBounded(reader io.Reader, maximum int) ([]byte, error) {
	if maximum <= 0 {
		return nil, invalidInput()
	}
	limit := int64(maximum)
	if limit < int64(^uint64(0)>>1) {
		limit++
	}
	limited := io.LimitReader(reader, limit)
	payload, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(payload) > maximum {
		return nil, errWatchEnvelopeTooLarge
	}
	return payload, nil
}

func closeBody(body io.Closer) error {
	if body == nil {
		return nil
	}
	return body.Close()
}

func validateContext(ctx context.Context) error {
	if ctx == nil {
		return invalidInput()
	}
	if err := ctx.Err(); err != nil {
		return canceledError(err)
	}
	return nil
}

func canceledError(cause error) error {
	if cause == nil {
		cause = context.Canceled
	}
	return fmt.Errorf("%w: %w", registry.ErrCanceled, cause)
}

func isNilExecutor(executor KubernetesRequestExecutor) bool {
	if executor == nil {
		return true
	}
	value := reflect.ValueOf(executor)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func readWatchEnvelope(reader *bufioReader, maximum int) ([]byte, error) {
	return reader.read(maximum)
}

// bufioReader is a tiny bounded line reader. Keeping the bound in this layer
// avoids an unbounded scanner token limit while preserving arbitrary JSON
// envelope bytes up to the explicit Binding limit.
type bufioReader struct {
	reader io.ByteReader
}

func (r *bufioReader) read(maximum int) ([]byte, error) {
	if maximum <= 0 {
		return nil, invalidInput()
	}
	data := make([]byte, 0, minInt(maximum, 4096))
	for {
		value, err := r.reader.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) && len(data) > 0 {
				return data, nil
			}
			return nil, err
		}
		if value == '\n' {
			if len(data) == 0 {
				return nil, errWatchEnvelopeInvalid
			}
			return data, nil
		}
		data = append(data, value)
		if len(data) > maximum {
			return nil, errWatchEnvelopeTooLarge
		}
	}
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

var _ registry.InstanceDirectory = (*Directory)(nil)
