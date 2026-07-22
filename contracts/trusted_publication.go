package contracts

import (
	"fmt"
	"time"
)

type TrustedPublicationErrorCode string

const (
	TrustedErrorValidation          TrustedPublicationErrorCode = "VALIDATION_ERROR"
	TrustedErrorUnauthenticated     TrustedPublicationErrorCode = "UNAUTHENTICATED"
	TrustedErrorForbidden           TrustedPublicationErrorCode = "FORBIDDEN"
	TrustedErrorNotFound            TrustedPublicationErrorCode = "NOT_FOUND"
	TrustedErrorConflict            TrustedPublicationErrorCode = "CONFLICT"
	TrustedErrorInvalidEndpoint     TrustedPublicationErrorCode = "INVALID_ENDPOINT"
	TrustedErrorDisallowedNetwork   TrustedPublicationErrorCode = "DISALLOWED_NETWORK"
	TrustedErrorEndpointUnavailable TrustedPublicationErrorCode = "ENDPOINT_UNAVAILABLE"
	TrustedErrorWrongProof          TrustedPublicationErrorCode = "WRONG_PROOF"
	TrustedErrorChallengeExpired    TrustedPublicationErrorCode = "CHALLENGE_EXPIRED"
	TrustedErrorChallengeReused     TrustedPublicationErrorCode = "CHALLENGE_REUSED"
	TrustedErrorRedirectNotAllowed  TrustedPublicationErrorCode = "REDIRECT_NOT_ALLOWED"
	TrustedErrorDependency          TrustedPublicationErrorCode = "DEPENDENCY_ERROR"
	TrustedErrorInternal            TrustedPublicationErrorCode = "INTERNAL_ERROR"
)

var trustedPublicationErrorMessages = map[TrustedPublicationErrorCode]string{
	TrustedErrorValidation:          "The trusted publication request is invalid.",
	TrustedErrorUnauthenticated:     "Authentication is required.",
	TrustedErrorForbidden:           "The trusted publication operation is not allowed.",
	TrustedErrorNotFound:            "The trusted publication resource was not found.",
	TrustedErrorConflict:            "The trusted publication operation conflicts with current state.",
	TrustedErrorInvalidEndpoint:     "The declared Agent endpoint is invalid.",
	TrustedErrorDisallowedNetwork:   "The declared Agent endpoint resolves to a disallowed network.",
	TrustedErrorEndpointUnavailable: "The declared Agent endpoint is unavailable.",
	TrustedErrorWrongProof:          "The Agent endpoint did not return the required ownership proof.",
	TrustedErrorChallengeExpired:    "The endpoint ownership challenge has expired.",
	TrustedErrorChallengeReused:     "The endpoint ownership challenge has already been used.",
	TrustedErrorRedirectNotAllowed:  "The endpoint ownership challenge redirected to another location.",
	TrustedErrorDependency:          "A required trusted publication dependency failed.",
	TrustedErrorInternal:            "The platform could not complete the trusted publication request.",
}

type TrustedPublicationError struct {
	Code    TrustedPublicationErrorCode `json:"code"`
	Message string                      `json:"message"`
	TraceID TraceID                     `json:"traceId"`
}

func NewTrustedPublicationError(code TrustedPublicationErrorCode, traceID TraceID) (TrustedPublicationError, error) {
	if _, err := ParseTraceID(string(traceID)); err != nil {
		return TrustedPublicationError{}, fmt.Errorf("invalid trace id")
	}
	message, exists := trustedPublicationErrorMessages[code]
	if !exists {
		return TrustedPublicationError{}, fmt.Errorf("unknown trusted publication error code %q", code)
	}
	return TrustedPublicationError{Code: code, Message: message, TraceID: traceID}, nil
}

const (
	ReleaseStateDraft               = "draft"
	ReleaseStatePendingVerification = "pending_verification"
	ReleaseStateVerified            = "verified"
	ReleaseStatePublished           = "published"
	ReleaseStateSuspended           = "suspended"
	ReleaseStateRevoked             = "revoked"
)

type CreateAgentReleaseRequest struct {
	Version           string `json:"version"`
	EndpointBindingID string `json:"endpointBindingId"`
}

type AgentReleaseResponse struct {
	ReleaseID                  string     `json:"releaseId"`
	ProviderID                 string     `json:"providerId"`
	AgentID                    string     `json:"agentId"`
	AgentCardVersion           string     `json:"agentCardVersion"`
	CardDigest                 string     `json:"cardDigest"`
	EndpointBindingID          string     `json:"endpointBindingId"`
	EndpointOrigin             string     `json:"endpointOrigin"`
	EndpointPath               string     `json:"endpointPath"`
	VerificationMethod         string     `json:"verificationMethod"`
	VerificationEvidenceDigest *string    `json:"verificationEvidenceDigest,omitempty"`
	State                      string     `json:"state"`
	CreatedAt                  time.Time  `json:"createdAt"`
	UpdatedAt                  time.Time  `json:"updatedAt"`
	VerifiedAt                 *time.Time `json:"verifiedAt,omitempty"`
	PublishedAt                *time.Time `json:"publishedAt,omitempty"`
	SuspendedAt                *time.Time `json:"suspendedAt,omitempty"`
	RevokedAt                  *time.Time `json:"revokedAt,omitempty"`
}
