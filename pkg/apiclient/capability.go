package apiclient

// This file is the single source of truth for features whose behaviour differs
// between Jira Cloud and Data Center. Jira exposes the same concepts through
// different request shapes, and the recurring bug class in this CLI family is
// a Cloud branch honouring an input that the DC branch silently drops (or vice
// versa).
//
// Anything NOT listed here is assumed uniformly supported on both flavors. List
// a capability only when the flavors diverge, then drive the runtime guard, the
// help/docs, and the parity test from this one table so they cannot drift.

// Capability names a flavor-sensitive feature.
type Capability string

const (
	// CapBodyRichFormat: how rich-text bodies (description, comments) are
	// represented on the wire.
	CapBodyRichFormat Capability = "body.rich-format"
	// CapUserIdentifier: how users are identified in assignment requests.
	CapUserIdentifier Capability = "user.identifier"
	// CapProjectListPagination: whether the project listing paginates.
	CapProjectListPagination Capability = "project.list-pagination"
)

// SupportLevel describes how a flavor provides a capability.
type SupportLevel string

const (
	// SupportNative: the API does it directly (a single request / native field).
	SupportNative SupportLevel = "native"
	// SupportEmulated: the CLI reproduces the behaviour with extra calls or a
	// lossy conversion.
	SupportEmulated SupportLevel = "emulated"
	// SupportUnsupported: not available on this flavor yet.
	SupportUnsupported SupportLevel = "unsupported"
)

// Support is a flavor's support for a capability, with a reason for the
// non-native cases (shown in errors / help).
type Support struct {
	Level  SupportLevel `json:"level"`
	Reason string       `json:"reason,omitempty"`
}

// Supported reports whether the capability can be used at all on this flavor.
func (s Support) Supported() bool { return s.Level != SupportUnsupported }

// capabilitySupport is the divergence table. A flavor missing from a row (or a
// capability missing entirely) defaults to native support.
var capabilitySupport = map[Capability]map[Flavor]Support{
	CapBodyRichFormat: {
		FlavorCloud: {Level: SupportEmulated,
			Reason: "Cloud (REST v3) requires ADF documents; plain text is converted to ADF paragraphs on write and ADF is flattened to text on read, so rich nodes (tables, panels) degrade"},
		FlavorDataCenter: {Level: SupportNative,
			Reason: "DC (REST v2) takes plain strings verbatim; Jira wiki markup is rendered server-side"},
	},
	CapUserIdentifier: {
		FlavorCloud: {Level: SupportEmulated,
			Reason: "Cloud assigns by accountId; non-accountId selectors are resolved via user search (needs Browse Users permission) and must match exactly one active user"},
		FlavorDataCenter: {Level: SupportNative,
			Reason: "DC assigns by username, passed through as given"},
	},
	CapProjectListPagination: {
		FlavorCloud: {Level: SupportNative},
		FlavorDataCenter: {Level: SupportEmulated,
			Reason: "DC's /project endpoint is unpaginated; the full list arrives in one response and --query filters client-side"},
	},
}

// supportFor returns this client's flavor's support for a capability.
func (c *apiClient) supportFor(cap Capability) Support {
	return capabilitySupportFor(cap, c.flavor)
}

func capabilitySupportFor(cap Capability, flavor Flavor) Support {
	if byFlavor, ok := capabilitySupport[cap]; ok {
		if s, ok := byFlavor[flavor]; ok {
			return s
		}
	}
	return Support{Level: SupportNative}
}

// CapabilityMatrix returns a copy of the full divergence table, for rendering in
// help / docs. Keys are stable capability identifiers.
func CapabilityMatrix() map[Capability]map[Flavor]Support {
	out := make(map[Capability]map[Flavor]Support, len(capabilitySupport))
	for cap, byFlavor := range capabilitySupport {
		row := make(map[Flavor]Support, len(byFlavor))
		for f, s := range byFlavor {
			row[f] = s
		}
		out[cap] = row
	}
	return out
}
