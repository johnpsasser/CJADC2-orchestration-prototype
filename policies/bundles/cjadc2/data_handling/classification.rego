# Data Handling Policy
# Controls access to data based on classification and agent clearance

package cjadc2.data_handling

import future.keywords.if
import future.keywords.in

import data.cjadc2.clearances
import data.cjadc2.allowed_processors
import data.cjadc2.classification_levels

# Default deny
default allow := false

# Allow if all conditions are met
allow if {
    agent_has_clearance
    agent_can_process_data_type
    handling_requirements_met
}

# Check if agent has sufficient clearance for the data classification
agent_has_clearance if {
    agent_level := clearances[input.agent_id]
    data_level := classification_levels[input.data.classification]
    agent_level >= data_level
}

# Fallback: allow if classification is unclassified
agent_has_clearance if {
    input.data.classification == "unclassified"
}

# Check if agent type is authorized to process this data type
agent_can_process_data_type if {
    input.agent_type in allowed_processors[input.data.type]
}

# Handling requirements based on classification
handling_requirements_met if {
    # Unclassified and confidential: basic requirements
    input.data.classification in ["unclassified", "confidential"]
    input.audit_enabled == true
}

handling_requirements_met if {
    # Secret and above: require encryption
    input.data.classification in ["secret", "top_secret"]
    input.audit_enabled == true
    input.encryption_enabled == true
}

# For MVP, allow if audit is not explicitly disabled
handling_requirements_met if {
    not input.audit_enabled == false
}

# Denial reasons
deny[msg] if {
    not agent_has_clearance
    msg := sprintf("Agent '%s' lacks clearance for '%s' data",
                   [input.agent_id, input.data.classification])
}

deny[msg] if {
    not agent_can_process_data_type
    msg := sprintf("Agent type '%s' not authorized to process '%s' data type",
                   [input.agent_type, input.data.type])
}

deny[msg] if {
    input.data.classification in ["secret", "top_secret"]
    input.encryption_enabled != true
    msg := sprintf("Encryption required for '%s' data", [input.data.classification])
}

# Decision metadata
decision := {
    "allowed": allow,
    "reasons": deny,
    "agent_id": input.agent_id,
    "data_type": input.data.type,
    "classification": input.data.classification
}
