package psacampaign

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/platform/canonjson"
)

// approvalEnvelopeVersion prefixes every signing input. A future change to the
// envelope's field set or order must bump this, so a signature minted under the
// old layout can never be reinterpreted under the new one.
const approvalEnvelopeVersion = "psa-push-approval.v1"

// ApprovalFreshness bounds how long an approval stays executable. This is not
// the replay defense — a database writer can restore a whole approved envelope
// verbatim, and the signature would still verify because it cannot cover the
// mutable status column. Replay is prevented at the portal instead (see
// drainCreate's existence check and PushCampaign's compare-and-swap). The
// window is defense in depth and operational hygiene: an approval the operator
// has forgotten about should not fire a week later.
const ApprovalFreshness = 24 * time.Hour

var (
	// ErrApprovalUnsigned marks a row that reached the drain without a
	// signature. Rows approved before signing was introduced land here.
	ErrApprovalUnsigned = errors.New("psacampaign: approved row carries no signature")
	// ErrApprovalSignatureInvalid marks a row whose envelope does not match its
	// signature — the row was altered after approval.
	ErrApprovalSignatureInvalid = errors.New("psacampaign: approval signature does not verify")
	// ErrApprovalPayloadMismatch marks a row whose stored diff does not hash to
	// the digest that was signed — the payload was swapped under a valid
	// envelope.
	ErrApprovalPayloadMismatch = errors.New("psacampaign: proposed diff does not match the approved payload digest")
	// ErrApprovalExpired marks a row whose approval is older than ApprovalFreshness.
	ErrApprovalExpired = errors.New("psacampaign: approval is older than the freshness window")
	// ErrApprovalFieldUnencodable marks an envelope component containing a
	// newline, which would make the newline-joined signing input ambiguous.
	ErrApprovalFieldUnencodable = errors.New("psacampaign: approval envelope field contains a newline")
)

// PayloadDigest is the hex SHA-256 of the canonical encoding of d. Both the
// approve path (from the typed value it just built) and the drain path (from
// what came back out of jsonb) call this, and canonjson.Digest routes each
// through the same decode step so the two agree.
func PayloadDigest(d ProposedDiff) (string, error) {
	return canonjson.Digest(d)
}

// SigningInput renders the approval envelope for row as the exact bytes that
// get authenticated: a fixed field order, newline-joined.
//
// status is deliberately absent. It changes legitimately across the row's
// lifecycle, so including it would invalidate the signature at every
// transition. Everything that must not change between approval and execution is
// here — which portal campaign is touched, what payload is applied, who
// approved it and when.
func SigningInput(row PushRow) (string, error) {
	fields := []string{
		approvalEnvelopeVersion,
		row.ID,
		string(row.Operation),
		row.InternalCampaignID,
		row.PSACampaignID,
		row.PayloadDigest,
		row.ApprovedBy,
		row.ApprovedAt.UTC().Format(time.RFC3339),
	}
	for i, f := range fields {
		if strings.ContainsAny(f, "\n\r") {
			return "", fmt.Errorf("%w (component %d)", ErrApprovalFieldUnencodable, i)
		}
	}
	return strings.Join(fields, "\n"), nil
}

// ApprovalSigner authenticates approval envelopes. The key lives in the
// application's environment and never in the database — that separation is the
// entire security property, since the adversary this defends against is a
// database writer.
type ApprovalSigner interface {
	// KeyID labels the key used, so rotating it does not invalidate rows
	// already in flight under the previous key.
	KeyID() string
	Sign(input string) (string, error)
	// Verify reports whether signature authenticates input, in constant time.
	Verify(input, signature string) bool
}

// Approval is everything Approve writes alongside the status transition. It is
// grouped into a value so the four signature columns cannot be written apart
// from each other — a row carrying a signature but no digest, or an approver
// but no signature, is not a state the store should be able to produce.
type Approval struct {
	ApprovedBy        string
	ApprovedAt        time.Time
	PayloadDigest     string
	ApprovalSignature string
	SignatureKeyID    string
}

// SignApproval builds the signed approval for row.
//
// The digest is taken from the diff as it stands at approval time, and the
// envelope is signed over that digest rather than over the diff itself. That
// indirection is what lets the drain re-derive the digest from whatever the
// database hands back and compare, without needing the payload bytes to survive
// jsonb byte-for-byte.
func SignApproval(row PushRow, approvedBy string, now time.Time, signer ApprovalSigner) (Approval, error) {
	if signer == nil {
		return Approval{}, ErrApprovalUnsigned
	}

	digest, err := PayloadDigest(row.Diff)
	if err != nil {
		return Approval{}, fmt.Errorf("psacampaign: digest diff for approval: %w", err)
	}

	// Sign the row as it will be stored, not as it was read: the envelope must
	// cover the approval fields this call is about to write.
	row.PayloadDigest = digest
	row.ApprovedBy = approvedBy
	row.ApprovedAt = now.UTC().Truncate(time.Second)

	input, err := SigningInput(row)
	if err != nil {
		return Approval{}, err
	}
	sig, err := signer.Sign(input)
	if err != nil {
		return Approval{}, fmt.Errorf("psacampaign: sign approval: %w", err)
	}

	return Approval{
		ApprovedBy:        row.ApprovedBy,
		ApprovedAt:        row.ApprovedAt,
		PayloadDigest:     digest,
		ApprovalSignature: sig,
		SignatureKeyID:    signer.KeyID(),
	}, nil
}

// VerifyApproval checks that row is executable: it carries a signature, its
// stored diff still hashes to the digest that was signed, the envelope
// authenticates under signer, and the approval is inside the freshness window.
//
// Every failure is terminal by design. A row that fails here has been altered
// or has expired, and no amount of retrying makes it valid — callers must not
// route these through the transient-retry path, which would turn a tamper
// signal into a loop that re-attempts the push every hour.
func VerifyApproval(row PushRow, signer ApprovalSigner, now time.Time) error {
	if signer == nil {
		return ErrApprovalUnsigned
	}
	if row.ApprovalSignature == "" || row.PayloadDigest == "" || row.ApprovedAt.IsZero() {
		return ErrApprovalUnsigned
	}

	// Re-derive the digest from the payload actually stored, so swapping the
	// diff under an otherwise-valid envelope is caught here.
	actual, err := PayloadDigest(row.Diff)
	if err != nil {
		return fmt.Errorf("psacampaign: digest stored diff: %w", err)
	}
	if actual != row.PayloadDigest {
		return fmt.Errorf("%w (stored %s, approved %s)", ErrApprovalPayloadMismatch, actual, row.PayloadDigest)
	}

	input, err := SigningInput(row)
	if err != nil {
		return err
	}
	if !signer.Verify(input, row.ApprovalSignature) {
		return ErrApprovalSignatureInvalid
	}

	if age := now.Sub(row.ApprovedAt); age > ApprovalFreshness {
		return fmt.Errorf("%w (approved %s ago)", ErrApprovalExpired, age.Truncate(time.Minute))
	}
	return nil
}
