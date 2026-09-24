package authorization

import (
	"errors"
	"reflect"
	"slices"

	"github.com/not-for-prod/proterror/proterror"
)

// PolicyNode is an immutable configuration expression. It holds drivers only
// during service construction; persisted domain policies contain IDs, not drivers.
// Its zero value is invalid. Use All, Any, and Method to construct a tree.
type PolicyNode struct {
	kind      policyNodeKind
	id        RequirementID
	children  []PolicyNode
	binding   *methodBinding
	providers []ProviderID
	err       error
}

type policyNodeKind uint8

const (
	methodNode policyNodeKind = iota + 1
	anyNode
	allNode
)

type methodBinding struct{ driver AuthenticationMethod }
type methodOptions struct{ providers []ProviderID }

// MethodOption configures one policy leaf, not the provider's credentials.
type MethodOption interface{ applyMethod(*methodOptions) error }
type methodOptionFunc func(*methodOptions) error

func (f methodOptionFunc) applyMethod(c *methodOptions) error { return f(c) }

// WithProviders restricts a method leaf to the listed provider IDs. Provider
// credentials and provider availability remain the supplied driver's concern.
func WithProviders(providers ...ProviderID) MethodOption {
	providers = slices.Clone(providers)

	return methodOptionFunc(func(c *methodOptions) error {
		if len(providers) == 0 || c.providers != nil {
			return errors.Join(&proterror.InvalidArgument{}, errors.New("providers must be nonempty and configured once"))
		}

		c.providers = providers

		return nil
	})
}

// Method binds a concrete driver to a policy leaf. Reuse the same driver instance
// across policies in a tenant. Distinct instances with the same Name are rejected.
// For non-comparable value drivers, reuse the same PolicyNode or pass a pointer.
func Method(driver AuthenticationMethod, opts ...MethodOption) PolicyNode {
	cfg := methodOptions{}

	for _, opt := range opts {
		if opt == nil {
			return PolicyNode{err: errors.Join(&proterror.InvalidArgument{}, errors.New("nil method option"))}
		}

		if err := opt.applyMethod(&cfg); err != nil {
			return PolicyNode{err: err}
		}
	}

	return PolicyNode{kind: methodNode, binding: &methodBinding{driver: driver}, providers: cfg.providers}
}

// Any names an evidence requirement satisfied by at least one child. An empty
// ID inherits an enclosing requirement ID, following the domain policy rules.
func Any(id RequirementID, children ...PolicyNode) PolicyNode {
	return PolicyNode{kind: anyNode, id: id, children: slices.Clone(children)}
}

// All requires every child. It does not prescribe an execution order.
func All(children ...PolicyNode) PolicyNode {
	return PolicyNode{kind: allNode, children: slices.Clone(children)}
}

func (n PolicyNode) compile(bindings map[MethodID]*methodBinding, remaining *int, depth int) (Requirement, error) {
	*remaining--
	if *remaining < 0 || depth > 64 {
		return nil, errors.Join(&proterror.InvalidArgument{}, errors.New("policy exceeds configured size"))
	}

	if n.err != nil {
		return nil, n.err
	}

	if n.kind == methodNode {
		if n.binding == nil || isNil(n.binding.driver) {
			return nil, errors.Join(&proterror.InvalidArgument{}, errors.New("policy method driver is required"))
		}

		id := n.binding.driver.Name()
		if id == "" {
			return nil, errors.Join(&proterror.InvalidArgument{}, errors.New("policy method name is required"))
		}

		if old, exists := bindings[id]; exists && old != n.binding {
			previous, current := reflect.ValueOf(old.driver), reflect.ValueOf(n.binding.driver)
			if previous.Type() != current.Type() || !previous.Comparable() || !previous.Equal(current) {
				return nil, errors.Join(&proterror.InvalidArgument{}, errors.New("conflicting drivers for tenant method: "+string(id)))
			}
		}

		*remaining -= len(n.providers)
		if *remaining < 0 {
			return nil, errors.Join(&proterror.InvalidArgument{}, errors.New("policy exceeds configured size"))
		}

		bindings[id] = n.binding

		return MethodRequirement{Method: id, Providers: slices.Clone(n.providers)}, nil
	}

	if n.kind != anyNode && n.kind != allNode {
		return nil, errors.Join(&proterror.InvalidArgument{}, errors.New("invalid policy node"))
	}

	children := make([]Requirement, 0, len(n.children))
	for _, child := range n.children {
		compiled, err := child.compile(bindings, remaining, depth+1)
		if err != nil {
			return nil, err
		}

		children = append(children, compiled)
	}

	if n.kind == anyNode {
		return AnyOfRequirement{ID: n.id, Children: children}, nil
	}

	return AllOfRequirement{Children: children}, nil
}
