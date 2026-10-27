package authz

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// PolicyVersion is the only policy language version the engine accepts.
const PolicyVersion = "2026-10-25"

// Effect is Allow or Deny.
type Effect string

// The two effects a statement can have.
const (
	Allow Effect = "Allow"
	Deny  Effect = "Deny"
)

// Policy is a policy document in the style of AWS IAM.
type Policy struct {
	Version   string      `json:"Version"`
	Statement []Statement `json:"Statement"`
}

// Statement grants or denies a set of actions on a set of resources, optionally only when a
// condition holds.
type Statement struct {
	Sid       string     `json:"Sid,omitempty"`
	Effect    Effect     `json:"Effect"`
	Action    []string   `json:"Action"`
	Resource  []string   `json:"Resource,omitempty"` // empty means every resource
	Condition *Condition `json:"Condition,omitempty"`
}

// Condition narrows a statement. Every key that is set must hold. Keys whose data the request
// does not provide count as unknown: an Allow with an unknown condition does not grant, and a
// Deny with an unknown condition still denies, so missing data always fails closed.
type Condition struct {
	// Self compares the resource's owner (the staff user it belongs to or is assigned to)
	// with the principal.
	Self *bool `json:"self,omitempty"`
	// Actor compares the principal with the staff users who already acted on the resource,
	// for example who checked a payroll run. Used for separation of duties.
	Actor *bool `json:"actor,omitempty"`
	// Property set to "assigned" requires the resource's property to be one the principal is
	// assigned to.
	Property string `json:"property,omitempty"`
	// AmountAtMost requires the request amount, in rupiah, to be at most this value.
	AmountAtMost *int64 `json:"amountAtMost,omitempty"`
}

// ParsePolicy decodes and validates a policy document. Unknown fields are rejected.
func ParsePolicy(data []byte) (Policy, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var p Policy
	if err := dec.Decode(&p); err != nil {
		return Policy{}, fmt.Errorf("decoding policy: %w", err)
	}
	if dec.More() {
		return Policy{}, errors.New("decoding policy: trailing data after the document")
	}
	if err := p.Validate(); err != nil {
		return Policy{}, err
	}
	return p, nil
}

// Validate reports the first problem in the policy, if any.
func (p Policy) Validate() error {
	if p.Version != PolicyVersion {
		return fmt.Errorf("policy version must be %q, got %q", PolicyVersion, p.Version)
	}
	if len(p.Statement) == 0 {
		return errors.New("policy has no statements")
	}
	for i, st := range p.Statement {
		if err := st.validate(); err != nil {
			return fmt.Errorf("statement %d (%s): %w", i, st.Sid, err)
		}
	}
	return nil
}

func (st Statement) validate() error {
	if st.Effect != Allow && st.Effect != Deny {
		return fmt.Errorf("effect must be Allow or Deny, got %q", st.Effect)
	}
	if len(st.Action) == 0 {
		return errors.New("no actions")
	}
	for _, a := range st.Action {
		if !validActionPattern(a) {
			return fmt.Errorf("action %q is not of the form resource:verb", a)
		}
		if !matchesAnyAction(a) {
			return fmt.Errorf("action %q matches no known action", a)
		}
	}
	for _, r := range st.Resource {
		if !validResourcePattern(r) {
			return fmt.Errorf("resource %q has characters outside a-z, 0-9, -, _, / and *", r)
		}
	}
	if c := st.Condition; c != nil {
		if c.Property != "" && c.Property != "assigned" {
			return fmt.Errorf(`condition property must be "assigned", got %q`, c.Property)
		}
		if c.AmountAtMost != nil && *c.AmountAtMost < 0 {
			return errors.New("condition amountAtMost must not be negative")
		}
	}
	return nil
}

func validResourcePattern(s string) bool {
	if s == "" {
		return false
	}
	return strings.IndexFunc(s, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9') && !strings.ContainsRune("-_/*", r)
	}) == -1
}
