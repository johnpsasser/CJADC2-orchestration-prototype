# Effect Release Policy
# Controls when effects can be executed - REQUIRES human approval

package cjadc2.effects

import future.keywords.if
import future.keywords.in

import data.cjadc2.human_approval_required
import data.cjadc2.auto_approve_actions

# Default: effect not allowed, human approval required
default allow_effect := false
default require_human := true

# Effect is allowed only with valid approval chain
allow_effect if {
    valid_approval_chain
    not expired_decision
    idempotency_verified
}

# Valid approval chain: human has approved
valid_approval_chain if {
    input.decision.approved == true
    input.decision.approved_by != ""
    input.decision.approved_by != "system"
    input.decision.proposal_id == input.proposal.proposal_id
}

# Check if decision has expired
expired_decision if {
    time.parse_rfc3339_ns(input.proposal.expires_at) < time.now_ns()
}

# Check idempotency - effect hasn't been executed
idempotency_verified if {
    not input.already_executed == true
}

# Human approval is ALWAYS required (safety constraint)
require_human := true

# Denial reasons for explainability
deny[msg] if {
    input.decision.approved != true
    msg := "Effect requires human approval - proposal was not approved"
}

deny[msg] if {
    input.decision.approved_by == ""
    msg := "Effect requires human approval - no approver identified"
}

deny[msg] if {
    input.decision.approved_by == "system"
    msg := "Effect requires human approval - system approvals not allowed"
}

deny[msg] if {
    expired_decision
    msg := sprintf("Proposal has expired at %s", [input.proposal.expires_at])
}

deny[msg] if {
    input.already_executed == true
    msg := "Effect has already been executed (idempotency check)"
}

deny[msg] if {
    input.decision.proposal_id != input.proposal.proposal_id
    msg := "Decision does not match proposal (integrity check)"
}

# Reasons why human approval is required
approval_reasons[reason] if {
    input.action_type in human_approval_required
    reason := sprintf("Action type '%s' always requires human approval", [input.action_type])
}

approval_reasons[reason] if {
    input.threat_level == "critical"
    reason := "Critical threat level requires human verification"
}

approval_reasons[reason] if {
    input.priority >= 7
    reason := sprintf("High priority (%d) requires human approval", [input.priority])
}

# Always include the safety constraint reason
approval_reasons["Human-in-the-loop is mandatory for all effects (safety constraint)"] := true

# Decision metadata for audit trail
decision := {
    "allowed": allow_effect,
    "require_human": require_human,
    "reasons": deny,
    "approval_reasons": approval_reasons,
    "action_type": input.action_type,
    "approver": input.decision.approved_by,
    "proposal_id": input.proposal.proposal_id
}
