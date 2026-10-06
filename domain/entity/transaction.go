package entity

import (
	"errors"
	"slices"
	"time"
)

var (
	ErrInvalidTransaction   = errors.New("invalid authentication transaction")
	ErrTransactionExpired   = errors.New("authentication transaction expired")
	ErrTransactionBusy      = errors.New("authentication transaction has an operation in flight")
	ErrTransactionAbandoned = errors.New("authentication transaction abandoned; create a new transaction")
	ErrInvalidTransition    = errors.New("authentication operation is not eligible")
	ErrSubjectMismatch      = errors.New("authentication subject mismatch")
	ErrAttemptLimit         = errors.New("authentication attempt limit reached")
	ErrInvalidProof         = errors.New("invalid authentication proof")
)

type TransactionID string
type TenantID string

type TransactionStatus string

const (
	TransactionActive     TransactionStatus = "active"
	TransactionFinalizing TransactionStatus = "finalizing"
	TransactionCompleted  TransactionStatus = "completed"
	TransactionAbandoned  TransactionStatus = "abandoned"
)

type OperationKind string

const (
	OperationStart    OperationKind = "start"
	OperationVerify   OperationKind = "verify"
	OperationCallback OperationKind = "callback"
)

// TransactionSettings are fixed for the lifetime of an authentication transaction.
type TransactionSettings struct {
	Lifetime          time.Duration
	OperationTimeout  time.Duration
	MaxAttempts       int
	MaxChallenges     int
	MaxChallengeBytes int
	MaxPolicyNodes    int
	IssueSession      bool
}

func (s TransactionSettings) Validate() error {
	if s.Lifetime <= 0 || s.OperationTimeout <= 0 || s.OperationTimeout > s.Lifetime ||
		s.MaxAttempts <= 0 || s.MaxChallenges <= 0 || s.MaxChallengeBytes <= 0 || s.MaxPolicyNodes <= 0 {
		return ErrInvalidTransaction
	}

	return nil
}

// OperationClaim elects one dispatcher and fences late method results.
type OperationClaim struct {
	AttemptID   string
	ID          string
	Kind        OperationKind
	Selection   MethodSelection
	ChallengeID ChallengeID
	ExpiresAt   time.Time
}

// MethodAttempt is bounded transaction-owned state, including a recoverable start result.
// RedirectURL is private driver result data and is excluded from published events.
type MethodAttempt struct {
	ID          string
	Selection   MethodSelection
	ChallengeID ChallengeID
	Ready       bool
	Verified    bool
	Kind        OperationKind
	RedirectURL string
}

// VerifiedIdentity identifies a proven subject, whether registered or unresolved. It is private
// transaction state, not a credential and not a published event payload.
type VerifiedIdentity struct {
	Kind    IdentityKind
	Issuer  string
	Subject string
}

// Transaction owns progress, bounded challenges, and completion intent.
// Repositories persist the whole root under one lock. No raw session token is stored.
type Transaction struct {
	VerifiedIdentity *VerifiedIdentity
	Version          uint64
	Pending          *MethodAttempt
	Receipts         []MethodAttempt
	events           []TransactionEvent
	ID               TransactionID
	TenantID         TenantID
	Intent           Intent
	OwnerKey         string
	UserID           *UserID
	Evidence         []Evidence
	Policy           Policy
	Settings         TransactionSettings
	Challenges       []Challenge
	Status           TransactionStatus
	Attempts         int
	Claim            *OperationClaim
	Completion       *OperationClaim
	Principal        *Principal
	CreatedAt        time.Time
	ExpiresAt        time.Time
}

func NewTransaction(id TransactionID, owner string, policy Policy, userID *UserID,
	settings TransactionSettings, now time.Time) (*Transaction, error) {
	if id == "" || owner == "" || policy.TenantID == "" || !policy.Intent.Valid() || now.IsZero() ||
		(userID != nil && *userID == "") {
		return nil, ErrInvalidTransaction
	}

	if err := settings.Validate(); err != nil {
		return nil, err
	}

	snapshot, err := policy.Snapshot(settings.MaxPolicyNodes)
	if err != nil {
		return nil, err
	}

	tx := &Transaction{
		ID:        id,
		TenantID:  policy.TenantID,
		Intent:    policy.Intent,
		OwnerKey:  owner,
		Policy:    snapshot,
		Settings:  settings,
		Status:    TransactionActive,
		CreatedAt: now.UTC(),
		ExpiresAt: now.Add(settings.Lifetime).UTC(),
	}

	if userID != nil {
		value := *userID
		tx.UserID = &value
	}

	tx.Version = 1

	return tx, nil
}

// Copy returns independent state for repositories and external drivers.
func (t Transaction) Copy() Transaction {
	t.events = nil
	if t.Pending != nil {
		pending := *t.Pending
		t.Pending = &pending
	}

	t.Receipts = slices.Clone(t.Receipts)
	if t.VerifiedIdentity != nil {
		identity := *t.VerifiedIdentity
		t.VerifiedIdentity = &identity
	}
	if t.UserID != nil {
		id := *t.UserID
		t.UserID = &id
	}

	t.Evidence = slices.Clone(t.Evidence)
	t.Policy.Root = copyRequirement(t.Policy.Root)

	t.Challenges = slices.Clone(t.Challenges)
	for i := range t.Challenges {
		t.Challenges[i].State = slices.Clone(t.Challenges[i].State)
	}

	if t.Claim != nil {
		claim := *t.Claim
		t.Claim = &claim
	}

	if t.Completion != nil {
		claim := *t.Completion
		t.Completion = &claim
	}

	if t.Principal != nil {
		principal := *t.Principal
		principal.Evidence = slices.Clone(principal.Evidence)
		t.Principal = &principal
	}

	return t
}

func (t Transaction) CheckUsable(now time.Time) error {
	if !now.Before(t.ExpiresAt) {
		return ErrTransactionExpired
	}

	if t.Status == TransactionAbandoned {
		return ErrTransactionAbandoned
	}

	if t.Status != TransactionActive && t.Status != TransactionFinalizing && t.Status != TransactionCompleted {
		return ErrInvalidTransition
	}

	return nil
}

// AbandonExpiredClaim is called by a mutating retry, never a query.
func (t *Transaction) AbandonExpiredClaim(now time.Time) bool {
	if t.Status != TransactionActive || t.Claim == nil || now.Before(t.Claim.ExpiresAt) {
		return false
	}

	t.Status = TransactionAbandoned

	t.Pending = nil

	t.Claim = nil
	for i := range t.Challenges {
		t.Challenges[i].State = nil
		t.Challenges[i].Status = ChallengeRetired
	}

	t.Version++

	return true
}

// CheckAttempt checks that id identifies the transaction's pending method attempt.
func (t Transaction) CheckAttempt(id string) error {
	if t.Pending == nil || t.Pending.ID != id {
		return ErrInvalidTransition
	}

	return nil
}

func (t Transaction) Challenge(id ChallengeID) (*Challenge, error) {
	for _, c := range t.Challenges {
		if c.ID == id {
			c.State = slices.Clone(c.State)
			return &c, nil
		}
	}

	return nil, ErrInvalidTransition
}

func (t *Transaction) ClaimOperation(id string, kind OperationKind, selection MethodSelection,
	challengeID ChallengeID, now time.Time) error {
	if err := t.CheckUsable(now); err != nil {
		return err
	}

	if t.Status != TransactionActive {
		return ErrInvalidTransition
	}

	if t.Claim != nil {
		return ErrTransactionBusy
	}

	if t.Attempts >= t.Settings.MaxAttempts {
		return ErrAttemptLimit
	}

	if id == "" || selection.Requirement == "" || selection.Method == "" {
		return ErrInvalidTransition
	}

	if kind != OperationStart && kind != OperationVerify && kind != OperationCallback {
		return ErrInvalidTransition
	}

	if kind == OperationStart && (challengeID != "" || len(t.Challenges) >= t.Settings.MaxChallenges) {
		return ErrAttemptLimit
	}

	if kind == OperationCallback && challengeID == "" {
		return ErrInvalidTransition
	}

	if challengeID != "" {
		c, err := t.Challenge(challengeID)
		if err != nil {
			return err
		}

		if c.Status != ChallengePending || c.Selection != selection || !now.Before(c.ExpiresAt) {
			return ErrInvalidTransition
		}
	}

	if kind == OperationStart && t.Pending != nil {
		return ErrInvalidTransition
	}

	if kind != OperationStart &&
		(t.Pending == nil || !t.Pending.Ready || t.Pending.Selection != selection || t.Pending.ChallengeID != challengeID) {
		return ErrInvalidTransition
	}

	expires := now.Add(t.Settings.OperationTimeout)
	if expires.After(t.ExpiresAt) {
		expires = t.ExpiresAt
	}

	t.Claim = &OperationClaim{ID: id, Kind: kind, Selection: selection, ChallengeID: challengeID, ExpiresAt: expires}
	if kind == OperationStart {
		t.Pending = &MethodAttempt{ID: id, Selection: selection}
	}

	t.Claim.AttemptID = t.Pending.ID
	t.Attempts++

	t.Version++

	return nil
}

func (t Transaction) CheckClaim(id string, now time.Time) error {
	if err := t.CheckUsable(now); err != nil {
		return err
	}

	if t.Status != TransactionActive || t.Claim == nil || t.Claim.ID != id {
		return ErrInvalidTransition
	}

	if !now.Before(t.Claim.ExpiresAt) {
		return ErrTransactionAbandoned
	}

	return nil
}

func (t *Transaction) AttachChallenge(id string, challenge *Challenge, now time.Time) error {
	return t.RecordMethodStarted(id, challenge, "", now)
}

func (t *Transaction) RecordMethodStarted(id string, challenge *Challenge, redirectURL string, now time.Time) error {
	if err := t.CheckClaim(id, now); err != nil {
		return err
	}

	if t.Claim.Kind != OperationStart {
		return ErrInvalidTransition
	}

	if challenge != nil {
		if challenge.ID == "" || challenge.TransactionID != t.ID || challenge.Selection != t.Claim.Selection ||
			challenge.Status != ChallengePending || challenge.CreatedAt.Before(t.CreatedAt) || challenge.CreatedAt.After(now) ||
			!now.Before(challenge.ExpiresAt) || challenge.ExpiresAt.After(t.ExpiresAt) ||
			len(challenge.State) > t.Settings.MaxChallengeBytes || len(t.Challenges) >= t.Settings.MaxChallenges {
			return ErrInvalidTransition
		}

		for _, c := range t.Challenges {
			if c.ID == challenge.ID {
				return ErrInvalidTransition
			}
		}

		c := *challenge
		c.State = slices.Clone(challenge.State)
		t.Challenges = append(t.Challenges, c)
	}

	t.Pending.Ready = true

	t.Pending.RedirectURL = redirectURL
	if challenge != nil {
		t.Pending.ChallengeID = challenge.ID
	}

	t.Claim = nil

	t.Version++

	return nil
}

// RejectOperation releases a definitive rejection or pre-dispatch failure.
// Retiring a challenge prevents replay after a verified but unusable result.
func (t *Transaction) RejectOperation(id string, retire bool, now time.Time) error {
	if err := t.CheckClaim(id, now); err != nil {
		return err
	}

	if retire && t.Claim.ChallengeID != "" {
		for i := range t.Challenges {
			if t.Challenges[i].ID == t.Claim.ChallengeID {
				t.Challenges[i].Status = ChallengeRetired
				t.Challenges[i].State = nil
			}
		}
	}

	if retire || t.Claim.Kind == OperationStart {
		t.Pending = nil
	}

	t.Claim = nil

	t.Version++

	return nil
}

func (t *Transaction) AcceptEvidence(id string, userID UserID, evidence Evidence, now time.Time) error {
	if err := t.CheckClaim(id, now); err != nil {
		return err
	}

	if t.Claim.Kind == OperationStart || (userID == "" && t.VerifiedIdentity == nil) ||
		evidence.VerifiedAt.Before(t.CreatedAt) ||
		evidence.VerifiedAt.After(now) ||
		(MethodSelection{Requirement: evidence.Requirement, Method: evidence.Method, Provider: evidence.Provider}) != t.Claim.Selection {
		return ErrInvalidProof
	}

	if t.UserID != nil && *t.UserID != userID {
		return ErrSubjectMismatch
	}

	if t.Claim.ChallengeID != "" {
		c, err := t.Challenge(t.Claim.ChallengeID)
		if err != nil {
			return err
		}

		if c.Status != ChallengePending || !now.Before(c.ExpiresAt) {
			return ErrInvalidTransition
		}
	}

	for _, accepted := range t.Evidence {
		if accepted.Requirement == evidence.Requirement && accepted.Method == evidence.Method &&
			accepted.Provider == evidence.Provider {
			return ErrInvalidProof
		}
	}

	if userID != "" {
		t.UserID = &userID
	}

	t.Evidence = append(t.Evidence, evidence)
	if t.Claim.ChallengeID != "" {
		for i := range t.Challenges {
			if t.Challenges[i].ID == t.Claim.ChallengeID {
				t.Challenges[i].Status = ChallengeConsumed
				t.Challenges[i].State = nil
			}
		}
	}

	receipt := *t.Pending
	receipt.Verified = true
	receipt.Kind = t.Claim.Kind
	t.Receipts = append(t.Receipts, receipt)
	t.Pending = nil
	completion := *t.Claim
	t.Completion = &completion
	t.Claim = nil

	t.Version++

	return nil
}

func (t *Transaction) PrepareCompletion(decision Decision, now time.Time) error {
	if err := t.CheckUsable(now); err != nil {
		return err
	}

	if (t.Status != TransactionActive && t.Status != TransactionFinalizing) || t.Principal != nil || t.Claim != nil ||
		!decision.Satisfied ||
		t.UserID == nil ||
		t.Completion == nil {
		return ErrInvalidTransition
	}

	t.Principal = &Principal{
		TenantID:        t.TenantID,
		UserID:          *t.UserID,
		Evidence:        slices.Clone(t.Evidence),
		AuthenticatedAt: now.UTC(),
	}

	t.Status = TransactionFinalizing
	for i := range t.Challenges {
		t.Challenges[i].State = nil
		if t.Challenges[i].Status == ChallengePending {
			t.Challenges[i].Status = ChallengeRetired
		}
	}

	t.Version++

	return nil
}

func (t *Transaction) Complete(now time.Time) error {
	if err := t.CheckUsable(now); err != nil {
		return err
	}

	if (t.Status != TransactionFinalizing && t.Status != TransactionCompleted) || t.Principal == nil {
		return ErrInvalidTransition
	}

	if t.Status == TransactionCompleted {
		return nil
	}

	before := t.eventSnapshot()

	t.Status = TransactionCompleted

	t.recordEvent("AuthenticationCompleted", &before, now)

	return nil
}

// Evidence records a verified proof for exactly one requirement scope.
type Evidence struct {
	Requirement RequirementID
	Method      MethodID
	Provider    ProviderID
	VerifiedAt  time.Time
}
