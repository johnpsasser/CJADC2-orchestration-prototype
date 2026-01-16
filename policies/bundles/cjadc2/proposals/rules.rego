# Proposal Rules Policy
# Validates action proposals before they enter the approval queue

package cjadc2.proposals

import future.keywords.if
import future.keywords.in

# Valid action types
valid_actions := ["engage", "track", "identify", "ignore", "intercept", "monitor"]

# Threat level to minimum priority mapping
threat_priority_map := {
    "critical": 8,
    "high": 6,
    "medium": 4,
    "low": 2
}

# Default deny
default allow := false

# Allow if proposal is valid
allow if {
    valid_action_type
    valid_priority
    valid_rationale
    valid_track_reference
    not conflicting_proposal
}

# Validate action type
valid_action_type if {
    input.proposal.action_type in valid_actions
}

# Validate priority range
valid_priority if {
    input.proposal.priority >= 1
    input.proposal.priority <= 10
}

# Validate rationale is provided
valid_rationale if {
    count(input.proposal.rationale) > 10
}

# Validate track reference exists
valid_track_reference if {
    input.proposal.track_id != ""
    input.track_exists == true
}

# Check for conflicting pending proposals
conflicting_proposal if {
    input.pending_proposals[_].track_id == input.proposal.track_id
    input.pending_proposals[_].action_type == input.proposal.action_type
}

# Denial reasons
deny[msg] if {
    not valid_action_type
    msg := sprintf("Invalid action type: '%s'. Valid types: %v",
                   [input.proposal.action_type, valid_actions])
}

deny[msg] if {
    not valid_priority
    msg := sprintf("Priority %d out of range. Must be 1-10", [input.proposal.priority])
}

deny[msg] if {
    not valid_rationale
    msg := "Rationale must be at least 10 characters"
}

deny[msg] if {
    input.proposal.track_id == ""
    msg := "Track ID is required"
}

deny[msg] if {
    input.track_exists != true
    msg := sprintf("Track '%s' does not exist", [input.proposal.track_id])
}

deny[msg] if {
    conflicting_proposal
    msg := sprintf("Conflicting proposal already pending for track '%s' with action '%s'",
                   [input.proposal.track_id, input.proposal.action_type])
}

# Warnings (don't block, but note in decision)
warnings[msg] if {
    threat_priority_map[input.track.threat_level] > input.proposal.priority
    msg := sprintf("Priority %d may be too low for threat level '%s' (suggested minimum: %d)",
                   [input.proposal.priority, input.track.threat_level, threat_priority_map[input.track.threat_level]])
}

warnings[msg] if {
    input.proposal.action_type == "engage"
    input.track.classification != "hostile"
    msg := sprintf("Engage action proposed for non-hostile track (classification: %s)",
                   [input.track.classification])
}

# Decision metadata
decision := {
    "allowed": allow,
    "reasons": deny,
    "warnings": warnings,
    "action_type": input.proposal.action_type,
    "priority": input.proposal.priority,
    "track_id": input.proposal.track_id
}
