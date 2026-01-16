# Origin Attestation Policy
# Verifies that messages come from valid, authorized sources

package cjadc2.origin

import future.keywords.if
import future.keywords.in

# Valid agent types and their allowed source ID patterns
valid_source_patterns := {
    "sensor": "sensor-*",
    "classifier": "classifier-*",
    "correlator": "correlator-*",
    "planner": "planner-*",
    "authorizer": "authorizer-*",
    "effector": "effector-*",
    "api": "api-*"
}

# Default deny
default allow := false

# Allow if source type is valid and source ID matches expected pattern
allow if {
    input.envelope.source_type in valid_source_patterns
    glob.match(valid_source_patterns[input.envelope.source_type], [], input.envelope.source)
    valid_signature
}

# Signature validation (simplified for MVP - checks presence)
valid_signature if {
    input.envelope.signature != ""
}

# Also allow if signature validation is disabled (for local dev)
valid_signature if {
    input.skip_signature_check == true
}

# Denial reasons for explainability
deny[msg] if {
    not input.envelope.source_type in valid_source_patterns
    msg := sprintf("Unknown source type: %s", [input.envelope.source_type])
}

deny[msg] if {
    input.envelope.source_type in valid_source_patterns
    not glob.match(valid_source_patterns[input.envelope.source_type], [], input.envelope.source)
    msg := sprintf("Source ID '%s' does not match allowed pattern for type '%s'",
                   [input.envelope.source, input.envelope.source_type])
}

deny[msg] if {
    input.envelope.signature == ""
    input.skip_signature_check != true
    msg := "Missing message signature"
}

# Metadata for decision explanation
decision := {
    "allowed": allow,
    "reasons": deny,
    "source": input.envelope.source,
    "source_type": input.envelope.source_type
}
