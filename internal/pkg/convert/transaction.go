package convert

import (
	"encoding/json"
	"errors"

	"github.com/not-for-prod/proterror/proterror"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/generated/xo"
)

// The explicit policy node discriminator makes interface-valued trees durable.
type requirement struct {
	Kind      string               `json:"kind"`
	ID        entity.RequirementID `json:"id,omitempty"`
	Method    entity.MethodID      `json:"method,omitempty"`
	Providers []entity.ProviderID  `json:"providers,omitempty"`
	Children  []requirement        `json:"children,omitempty"`
}
type transactionSnapshot struct {
	Version int                `json:"version"`
	State   entity.Transaction `json:"state"`
	Root    requirement        `json:"root"`
}

func encodeRequirement(node entity.Requirement) requirement {
	switch n := node.(type) {
	case entity.MethodRequirement:
		return requirement{Kind: "method", Method: n.Method, Providers: n.Providers}
	case entity.AnyOfRequirement:
		r := requirement{Kind: "any", ID: n.ID}
		for _, c := range n.Children {
			r.Children = append(r.Children, encodeRequirement(c))
		}
		return r
	case entity.AllOfRequirement:
		r := requirement{Kind: "all", ID: n.ID}
		for _, c := range n.Children {
			r.Children = append(r.Children, encodeRequirement(c))
		}
		return r
	default:
		panic("validated policy has unsupported node")
	}
}
func decodeRequirement(r requirement, remaining *int, depth int) (entity.Requirement, error) {
	*remaining--
	if *remaining < 0 || depth > 64 {
		return nil, errors.New("stored policy exceeds bounds")
	}
	children := make([]entity.Requirement, 0, len(r.Children))
	for _, c := range r.Children {
		node, err := decodeRequirement(c, remaining, depth+1)
		if err != nil {
			return nil, err
		}
		children = append(children, node)
	}
	switch r.Kind {
	case "method":
		return entity.MethodRequirement{Method: r.Method, Providers: r.Providers}, nil
	case "any":
		return entity.AnyOfRequirement{ID: r.ID, Children: children}, nil
	case "all":
		return entity.AllOfRequirement{ID: r.ID, Children: children}, nil
	default:
		return nil, errors.New("unknown stored policy node")
	}
}
func TransactionToXO(t entity.Transaction) (xo.AuthTransaction, error) {
	policy, err := t.Policy.Snapshot(t.Settings.MaxPolicyNodes)
	if err != nil {
		return xo.AuthTransaction{}, errors.Join(&proterror.InvalidArgument{}, err)
	}
	state := t.Copy()
	state.Policy.Root = nil
	snapshot, err := json.Marshal(transactionSnapshot{Version: 1, State: state, Root: encodeRequirement(policy.Root)})
	if err != nil {
		return xo.AuthTransaction{}, errors.Join(&proterror.InvalidArgument{}, err)
	}
	return xo.AuthTransaction{
		TenantID: string(t.TenantID),
		ID:       string(t.ID),
		OwnerKey: t.OwnerKey,
		Snapshot: snapshot,
	}, nil
}
func TransactionFromXO(row xo.AuthTransaction) (entity.Transaction, error) {
	var snapshot transactionSnapshot
	if err := json.Unmarshal(row.Snapshot, &snapshot); err != nil {
		return entity.Transaction{}, errors.Join(&proterror.Internal{}, err)
	}
	t := snapshot.State
	if snapshot.Version != 1 || string(t.ID) != row.ID || string(t.TenantID) != row.TenantID ||
		t.OwnerKey != row.OwnerKey ||
		t.Policy.Root != nil {
		return entity.Transaction{}, errors.Join(
			&proterror.Internal{},
			errors.New("invalid stored transaction envelope"),
		)
	}
	remaining := t.Settings.MaxPolicyNodes
	root, err := decodeRequirement(snapshot.Root, &remaining, 1)
	if err != nil {
		return entity.Transaction{}, errors.Join(&proterror.Internal{}, err)
	}
	t.Policy.Root = root
	policy, err := t.Policy.Snapshot(t.Settings.MaxPolicyNodes)
	if err != nil {
		return entity.Transaction{}, errors.Join(&proterror.Internal{}, err)
	}
	t.Policy = policy
	return t, nil
}
