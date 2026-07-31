package paymentworker

import (
	"time"

	"lapangango-api/internal/paymentoutbox"
	"lapangango-api/internal/payments"
)

type providerIdentityState string

const (
	providerIdentityNone           providerIdentityState = "NONE"
	providerIdentitySession        providerIdentityState = "SESSION"
	providerIdentityPaymentRequest providerIdentityState = "PAYMENT_REQUEST"
	providerIdentityPayment        providerIdentityState = "PAYMENT"
	providerIdentityInvalid        providerIdentityState = "INVALID"
)

type executionAction string

const (
	executionCallCreate          executionAction = "CALL_CREATE"
	executionCallInquiry         executionAction = "CALL_INQUIRY"
	executionLocalCreateRecovery executionAction = "LOCAL_CREATE_RECOVERY"
	executionLocalRetry          executionAction = "LOCAL_RETRY"
	executionLocalNoop           executionAction = "LOCAL_NOOP"
	executionLocalTerminal       executionAction = "LOCAL_TERMINAL"
	executionRejectLease         executionAction = "REJECT_LEASE"
)

type executionDecision struct {
	Action        executionAction
	IdentityState providerIdentityState
	Reason        string
}

// decideCommandExecution is the only pre-provider lifecycle decision. It
// freezes provider-call authorization across create and inquiry instead of
// allowing individual processor branches to reinterpret attempt/identity
// state independently.
func decideCommandExecution(command paymentoutbox.Command, attempt payments.PaymentAttempt, now time.Time) executionDecision {
	identityState := classifyProviderIdentity(attempt)
	if !commandLeaseActive(command, now) {
		return executionDecision{Action: executionRejectLease, IdentityState: identityState}
	}
	if isTerminalAttemptState(attempt.State) {
		return executionDecision{Action: executionLocalNoop, IdentityState: identityState}
	}
	switch command.CommandType {
	case paymentoutbox.CommandPaymentCreate:
		if attempt.State != payments.AttemptStateCreated && attempt.State != payments.AttemptStatePending {
			return executionDecision{Action: executionLocalTerminal, IdentityState: identityState, Reason: "PROVIDER_CONTRACT_BLOCKED"}
		}
		switch identityState {
		case providerIdentityNone:
			return executionDecision{Action: executionCallCreate, IdentityState: identityState}
		case providerIdentitySession, providerIdentityPaymentRequest, providerIdentityPayment:
			return executionDecision{Action: executionLocalCreateRecovery, IdentityState: identityState}
		default:
			return executionDecision{Action: executionLocalTerminal, IdentityState: identityState, Reason: "PROVIDER_CONTRACT_BLOCKED"}
		}
	case paymentoutbox.CommandPaymentInquiry:
		if attempt.State != payments.AttemptStatePending {
			return executionDecision{Action: executionLocalTerminal, IdentityState: identityState, Reason: "PROVIDER_CONTRACT_BLOCKED"}
		}
		switch identityState {
		case providerIdentityNone:
			return executionDecision{Action: executionLocalRetry, IdentityState: identityState}
		case providerIdentitySession, providerIdentityPaymentRequest, providerIdentityPayment:
			return executionDecision{Action: executionCallInquiry, IdentityState: identityState}
		default:
			return executionDecision{Action: executionLocalTerminal, IdentityState: identityState, Reason: "PROVIDER_CONTRACT_BLOCKED"}
		}
	default:
		return executionDecision{Action: executionLocalTerminal, IdentityState: identityState, Reason: "PROVIDER_CONTRACT_BLOCKED"}
	}
}

func commandLeaseActive(command paymentoutbox.Command, now time.Time) bool {
	return command.State == paymentoutbox.StateLeased &&
		command.PaymentAttemptID != nil &&
		command.LeaseOwner != nil &&
		command.LeaseToken != nil &&
		command.LeaseExpiresAt != nil &&
		command.LeaseExpiresAt.After(now)
}

func classifyProviderIdentity(attempt payments.PaymentAttempt) providerIdentityState {
	for _, identity := range []*string{
		attempt.ProviderSessionID,
		attempt.ProviderPaymentReqID,
		attempt.ProviderPaymentID,
	} {
		if identity != nil && !payments.ValidProviderIdentity(*identity, true) {
			return providerIdentityInvalid
		}
	}
	if attempt.ProviderPaymentID != nil && attempt.ProviderPaymentReqID == nil {
		return providerIdentityInvalid
	}
	if attempt.ProviderPaymentID != nil {
		return providerIdentityPayment
	}
	if attempt.ProviderPaymentReqID != nil {
		return providerIdentityPaymentRequest
	}
	if attempt.ProviderSessionID != nil {
		return providerIdentitySession
	}
	return providerIdentityNone
}

func preferredProviderIdentity(attempt payments.PaymentAttempt) string {
	if attempt.ProviderPaymentID != nil {
		return *attempt.ProviderPaymentID
	}
	if attempt.ProviderPaymentReqID != nil {
		return *attempt.ProviderPaymentReqID
	}
	if attempt.ProviderSessionID != nil {
		return *attempt.ProviderSessionID
	}
	return ""
}
