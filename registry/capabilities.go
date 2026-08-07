package registry

import "sort"

// Capability identifies an optional Instance Registry behavior.
type Capability string

const (
	CapabilitySnapshot       Capability = "snapshot"
	CapabilityObserve        Capability = "observe"
	CapabilityRegistration   Capability = "registration"
	CapabilityDeregistration Capability = "deregistration"
	CapabilityLease          Capability = "lease"
	CapabilityHeartbeat      Capability = "heartbeat"
)

// Capabilities is an immutable set of directory capabilities.
type Capabilities struct {
	set map[Capability]struct{}
}

// NewCapabilities validates and copies an immutable capability set.
func NewCapabilities(values ...Capability) (Capabilities, error) {
	set := make(map[Capability]struct{}, len(values))
	for _, value := range values {
		if !validCapability(value) {
			return Capabilities{}, newInvalidError("capability")
		}
		if _, exists := set[value]; exists {
			return Capabilities{}, newInvalidError("duplicate_capability")
		}
		set[value] = struct{}{}
	}
	return Capabilities{set: set}, nil
}

func validCapability(value Capability) bool {
	switch value {
	case CapabilitySnapshot, CapabilityObserve, CapabilityRegistration,
		CapabilityDeregistration, CapabilityLease, CapabilityHeartbeat:
		return true
	default:
		return false
	}
}

// Supports reports whether this capability was explicitly advertised.
func (c Capabilities) Supports(value Capability) bool {
	_, ok := c.set[value]
	return ok
}

// Values returns a sorted copy of the advertised capability set.
func (c Capabilities) Values() []Capability {
	values := make([]Capability, 0, len(c.set))
	for value := range c.set {
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	return values
}

// Equal reports whether both capability sets contain exactly the same values.
func (c Capabilities) Equal(other Capabilities) bool {
	if len(c.set) != len(other.set) {
		return false
	}
	for value := range c.set {
		if !other.Supports(value) {
			return false
		}
	}
	return true
}
