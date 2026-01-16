# Policy Unit Tests for CJADC2
# Run with: opa test policies/bundles/cjadc2/ -v

package cjadc2_test

import data.cjadc2.origin
import data.cjadc2.data_handling
import data.cjadc2.proposals
import data.cjadc2.effects

import future.keywords.if
import future.keywords.in

#############################
# Origin Attestation Tests
#############################

# Test valid sensor source
test_origin_valid_sensor if {
    origin.allow with input as {
        "envelope": {
            "source": "sensor-001",
            "source_type": "sensor",
            "signature": "valid-signature"
        },
        "skip_signature_check": false
    }
}

# Test valid sensor with wildcard pattern
test_origin_valid_sensor_wildcard if {
    origin.allow with input as {
        "envelope": {
            "source": "sensor-radar-coastal-001",
            "source_type": "sensor",
            "signature": "sig"
        },
        "skip_signature_check": false
    }
}

# Test valid classifier source
test_origin_valid_classifier if {
    origin.allow with input as {
        "envelope": {
            "source": "classifier-prod-001",
            "source_type": "classifier",
            "signature": "sig"
        },
        "skip_signature_check": false
    }
}

# Test valid correlator source
test_origin_valid_correlator if {
    origin.allow with input as {
        "envelope": {
            "source": "correlator-main",
            "source_type": "correlator",
            "signature": "sig"
        },
        "skip_signature_check": false
    }
}

# Test valid planner source
test_origin_valid_planner if {
    origin.allow with input as {
        "envelope": {
            "source": "planner-tactical",
            "source_type": "planner",
            "signature": "sig"
        },
        "skip_signature_check": false
    }
}

# Test valid authorizer source
test_origin_valid_authorizer if {
    origin.allow with input as {
        "envelope": {
            "source": "authorizer-human",
            "source_type": "authorizer",
            "signature": "sig"
        },
        "skip_signature_check": false
    }
}

# Test valid effector source
test_origin_valid_effector if {
    origin.allow with input as {
        "envelope": {
            "source": "effector-weapons-system",
            "source_type": "effector",
            "signature": "sig"
        },
        "skip_signature_check": false
    }
}

# Test valid API source
test_origin_valid_api if {
    origin.allow with input as {
        "envelope": {
            "source": "api-gateway",
            "source_type": "api",
            "signature": "sig"
        },
        "skip_signature_check": false
    }
}

# Test invalid source type
test_origin_invalid_source_type if {
    not origin.allow with input as {
        "envelope": {
            "source": "unknown-001",
            "source_type": "unknown",
            "signature": "sig"
        },
        "skip_signature_check": false
    }
}

# Test source ID pattern mismatch
test_origin_source_pattern_mismatch if {
    not origin.allow with input as {
        "envelope": {
            "source": "invalid-source-001",
            "source_type": "sensor",
            "signature": "sig"
        },
        "skip_signature_check": false
    }
}

# Test missing signature
test_origin_missing_signature if {
    not origin.allow with input as {
        "envelope": {
            "source": "sensor-001",
            "source_type": "sensor",
            "signature": ""
        },
        "skip_signature_check": false
    }
}

# Test skip signature check
test_origin_skip_signature_check if {
    origin.allow with input as {
        "envelope": {
            "source": "sensor-001",
            "source_type": "sensor",
            "signature": ""
        },
        "skip_signature_check": true
    }
}

# Test denial reason for unknown source type
test_origin_deny_unknown_source_type if {
    count(origin.deny) > 0 with input as {
        "envelope": {
            "source": "unknown-001",
            "source_type": "unknown",
            "signature": "sig"
        },
        "skip_signature_check": false
    }
}

# Test denial reason for pattern mismatch
test_origin_deny_pattern_mismatch if {
    count(origin.deny) > 0 with input as {
        "envelope": {
            "source": "invalid-pattern",
            "source_type": "sensor",
            "signature": "sig"
        },
        "skip_signature_check": false
    }
}

# Test denial reason for missing signature
test_origin_deny_missing_signature if {
    count(origin.deny) > 0 with input as {
        "envelope": {
            "source": "sensor-001",
            "source_type": "sensor",
            "signature": ""
        },
        "skip_signature_check": false
    }
}

#############################
# Data Handling Tests
#############################

# Test unclassified data allowed with audit
test_data_handling_unclassified_with_audit if {
    data_handling.allow with input as {
        "agent_id": "classifier-001",
        "agent_type": "classifier",
        "data": {
            "type": "track",
            "classification": "unclassified"
        },
        "audit_enabled": true,
        "encryption_enabled": false
    }
    with data.cjadc2.clearances as {"classifier-001": 1}
    with data.cjadc2.classification_levels as {"unclassified": 0, "confidential": 1, "secret": 2, "top_secret": 3}
    with data.cjadc2.allowed_processors as {"track": ["sensor", "classifier", "correlator"]}
}

# Test confidential data with sufficient clearance
test_data_handling_confidential_allowed if {
    data_handling.allow with input as {
        "agent_id": "correlator-001",
        "agent_type": "correlator",
        "data": {
            "type": "correlated_track",
            "classification": "confidential"
        },
        "audit_enabled": true,
        "encryption_enabled": false
    }
    with data.cjadc2.clearances as {"correlator-001": 2}
    with data.cjadc2.classification_levels as {"unclassified": 0, "confidential": 1, "secret": 2, "top_secret": 3}
    with data.cjadc2.allowed_processors as {"correlated_track": ["correlator", "planner"]}
}

# Test secret data requires encryption
test_data_handling_secret_requires_encryption if {
    not data_handling.allow with input as {
        "agent_id": "planner-001",
        "agent_type": "planner",
        "data": {
            "type": "proposal",
            "classification": "secret"
        },
        "audit_enabled": true,
        "encryption_enabled": false
    }
    with data.cjadc2.clearances as {"planner-001": 3}
    with data.cjadc2.classification_levels as {"unclassified": 0, "confidential": 1, "secret": 2, "top_secret": 3}
    with data.cjadc2.allowed_processors as {"proposal": ["planner", "authorizer"]}
}

# Test secret data allowed with encryption
test_data_handling_secret_with_encryption if {
    data_handling.allow with input as {
        "agent_id": "planner-001",
        "agent_type": "planner",
        "data": {
            "type": "proposal",
            "classification": "secret"
        },
        "audit_enabled": true,
        "encryption_enabled": true
    }
    with data.cjadc2.clearances as {"planner-001": 3}
    with data.cjadc2.classification_levels as {"unclassified": 0, "confidential": 1, "secret": 2, "top_secret": 3}
    with data.cjadc2.allowed_processors as {"proposal": ["planner", "authorizer"]}
}

# Test top secret data requires encryption
test_data_handling_top_secret_requires_encryption if {
    not data_handling.allow with input as {
        "agent_id": "effector-001",
        "agent_type": "effector",
        "data": {
            "type": "effect",
            "classification": "top_secret"
        },
        "audit_enabled": true,
        "encryption_enabled": false
    }
    with data.cjadc2.clearances as {"effector-001": 4}
    with data.cjadc2.classification_levels as {"unclassified": 0, "confidential": 1, "secret": 2, "top_secret": 3}
    with data.cjadc2.allowed_processors as {"effect": ["authorizer", "effector"]}
}

# Test insufficient clearance denied
test_data_handling_insufficient_clearance if {
    not data_handling.allow with input as {
        "agent_id": "sensor-low-clearance",
        "agent_type": "sensor",
        "data": {
            "type": "detection",
            "classification": "secret"
        },
        "audit_enabled": true,
        "encryption_enabled": true
    }
    with data.cjadc2.clearances as {"sensor-low-clearance": 1}
    with data.cjadc2.classification_levels as {"unclassified": 0, "confidential": 1, "secret": 2, "top_secret": 3}
    with data.cjadc2.allowed_processors as {"detection": ["sensor", "classifier"]}
}

# Test unauthorized processor denied
test_data_handling_unauthorized_processor if {
    not data_handling.allow with input as {
        "agent_id": "sensor-001",
        "agent_type": "sensor",
        "data": {
            "type": "decision",
            "classification": "unclassified"
        },
        "audit_enabled": true,
        "encryption_enabled": false
    }
    with data.cjadc2.clearances as {"sensor-001": 1}
    with data.cjadc2.classification_levels as {"unclassified": 0}
    with data.cjadc2.allowed_processors as {"decision": ["authorizer"]}
}

#############################
# Proposal Validation Tests
#############################

# Test valid engage proposal
test_proposals_valid_engage if {
    proposals.allow with input as {
        "proposal": {
            "action_type": "engage",
            "priority": 8,
            "rationale": "Hostile aircraft approaching protected zone - engagement authorized",
            "track_id": "track-001"
        },
        "track_exists": true,
        "pending_proposals": [],
        "track": {
            "threat_level": "high",
            "classification": "hostile"
        }
    }
}

# Test valid track proposal
test_proposals_valid_track if {
    proposals.allow with input as {
        "proposal": {
            "action_type": "track",
            "priority": 5,
            "rationale": "Unknown aircraft requires tracking for identification",
            "track_id": "track-002"
        },
        "track_exists": true,
        "pending_proposals": [],
        "track": {
            "threat_level": "medium",
            "classification": "unknown"
        }
    }
}

# Test valid identify proposal
test_proposals_valid_identify if {
    proposals.allow with input as {
        "proposal": {
            "action_type": "identify",
            "priority": 6,
            "rationale": "Unidentified contact requires IFF challenge and identification",
            "track_id": "track-003"
        },
        "track_exists": true,
        "pending_proposals": [],
        "track": {
            "threat_level": "medium",
            "classification": "unknown"
        }
    }
}

# Test valid ignore proposal
test_proposals_valid_ignore if {
    proposals.allow with input as {
        "proposal": {
            "action_type": "ignore",
            "priority": 2,
            "rationale": "Friendly aircraft confirmed via IFF - no action required",
            "track_id": "track-004"
        },
        "track_exists": true,
        "pending_proposals": [],
        "track": {
            "threat_level": "low",
            "classification": "friendly"
        }
    }
}

# Test valid intercept proposal
test_proposals_valid_intercept if {
    proposals.allow with input as {
        "proposal": {
            "action_type": "intercept",
            "priority": 7,
            "rationale": "Suspected hostile entering restricted airspace - intercept required",
            "track_id": "track-005"
        },
        "track_exists": true,
        "pending_proposals": [],
        "track": {
            "threat_level": "high",
            "classification": "unknown"
        }
    }
}

# Test valid monitor proposal
test_proposals_valid_monitor if {
    proposals.allow with input as {
        "proposal": {
            "action_type": "monitor",
            "priority": 3,
            "rationale": "Civilian aircraft deviation from flight plan - monitoring required",
            "track_id": "track-006"
        },
        "track_exists": true,
        "pending_proposals": [],
        "track": {
            "threat_level": "low",
            "classification": "unknown"
        }
    }
}

# Test invalid action type
test_proposals_invalid_action_type if {
    not proposals.allow with input as {
        "proposal": {
            "action_type": "destroy",
            "priority": 8,
            "rationale": "Invalid action type should be rejected by the system",
            "track_id": "track-001"
        },
        "track_exists": true,
        "pending_proposals": [],
        "track": {}
    }
}

# Test priority too low
test_proposals_priority_too_low if {
    not proposals.allow with input as {
        "proposal": {
            "action_type": "track",
            "priority": 0,
            "rationale": "Priority zero is below minimum allowed value",
            "track_id": "track-001"
        },
        "track_exists": true,
        "pending_proposals": [],
        "track": {}
    }
}

# Test priority too high
test_proposals_priority_too_high if {
    not proposals.allow with input as {
        "proposal": {
            "action_type": "track",
            "priority": 11,
            "rationale": "Priority eleven exceeds maximum allowed value",
            "track_id": "track-001"
        },
        "track_exists": true,
        "pending_proposals": [],
        "track": {}
    }
}

# Test rationale too short
test_proposals_rationale_too_short if {
    not proposals.allow with input as {
        "proposal": {
            "action_type": "track",
            "priority": 5,
            "rationale": "Too short",
            "track_id": "track-001"
        },
        "track_exists": true,
        "pending_proposals": [],
        "track": {}
    }
}

# Test missing track ID
test_proposals_missing_track_id if {
    not proposals.allow with input as {
        "proposal": {
            "action_type": "track",
            "priority": 5,
            "rationale": "This is a valid rationale with sufficient length",
            "track_id": ""
        },
        "track_exists": false,
        "pending_proposals": [],
        "track": {}
    }
}

# Test track does not exist
test_proposals_track_not_exists if {
    not proposals.allow with input as {
        "proposal": {
            "action_type": "track",
            "priority": 5,
            "rationale": "This is a valid rationale with sufficient length",
            "track_id": "nonexistent-track"
        },
        "track_exists": false,
        "pending_proposals": [],
        "track": {}
    }
}

# Test conflicting pending proposal
test_proposals_conflicting_pending if {
    not proposals.allow with input as {
        "proposal": {
            "action_type": "engage",
            "priority": 8,
            "rationale": "This is a valid rationale with sufficient length",
            "track_id": "track-001"
        },
        "track_exists": true,
        "pending_proposals": [
            {
                "track_id": "track-001",
                "action_type": "engage"
            }
        ],
        "track": {}
    }
}

# Test warning for low priority with critical threat
test_proposals_warning_priority_too_low_for_threat if {
    count(proposals.warnings) > 0 with input as {
        "proposal": {
            "action_type": "engage",
            "priority": 5,
            "rationale": "This is a valid rationale with sufficient length",
            "track_id": "track-001"
        },
        "track_exists": true,
        "pending_proposals": [],
        "track": {
            "threat_level": "critical",
            "classification": "hostile"
        }
    }
}

# Test warning for engage on non-hostile
test_proposals_warning_engage_non_hostile if {
    count(proposals.warnings) > 0 with input as {
        "proposal": {
            "action_type": "engage",
            "priority": 8,
            "rationale": "This is a valid rationale with sufficient length",
            "track_id": "track-001"
        },
        "track_exists": true,
        "pending_proposals": [],
        "track": {
            "threat_level": "high",
            "classification": "unknown"
        }
    }
}

#############################
# Effect Release Tests
#############################

# Test valid human approval
test_effects_valid_human_approval if {
    effects.allow_effect with input as {
        "decision": {
            "approved": true,
            "approved_by": "commander-alpha",
            "proposal_id": "prop-001"
        },
        "proposal": {
            "proposal_id": "prop-001",
            "expires_at": "2099-12-31T23:59:59Z"
        },
        "action_type": "engage",
        "already_executed": false
    }
}

# Test human approval always required
test_effects_require_human_always if {
    effects.require_human with input as {
        "decision": {
            "approved": true,
            "approved_by": "commander-alpha",
            "proposal_id": "prop-001"
        },
        "proposal": {
            "proposal_id": "prop-001",
            "expires_at": "2099-12-31T23:59:59Z"
        },
        "action_type": "engage",
        "already_executed": false
    }
}

# Test denied when not approved
test_effects_denied_when_not_approved if {
    not effects.allow_effect with input as {
        "decision": {
            "approved": false,
            "approved_by": "commander-alpha",
            "proposal_id": "prop-001"
        },
        "proposal": {
            "proposal_id": "prop-001",
            "expires_at": "2099-12-31T23:59:59Z"
        },
        "action_type": "engage",
        "already_executed": false
    }
}

# Test denied when no approver
test_effects_denied_no_approver if {
    not effects.allow_effect with input as {
        "decision": {
            "approved": true,
            "approved_by": "",
            "proposal_id": "prop-001"
        },
        "proposal": {
            "proposal_id": "prop-001",
            "expires_at": "2099-12-31T23:59:59Z"
        },
        "action_type": "engage",
        "already_executed": false
    }
}

# Test denied when system approver
test_effects_denied_system_approver if {
    not effects.allow_effect with input as {
        "decision": {
            "approved": true,
            "approved_by": "system",
            "proposal_id": "prop-001"
        },
        "proposal": {
            "proposal_id": "prop-001",
            "expires_at": "2099-12-31T23:59:59Z"
        },
        "action_type": "engage",
        "already_executed": false
    }
}

# Test denied when already executed (idempotency)
test_effects_denied_already_executed if {
    not effects.allow_effect with input as {
        "decision": {
            "approved": true,
            "approved_by": "commander-alpha",
            "proposal_id": "prop-001"
        },
        "proposal": {
            "proposal_id": "prop-001",
            "expires_at": "2099-12-31T23:59:59Z"
        },
        "action_type": "engage",
        "already_executed": true
    }
}

# Test denied when proposal/decision mismatch
test_effects_denied_proposal_mismatch if {
    not effects.allow_effect with input as {
        "decision": {
            "approved": true,
            "approved_by": "commander-alpha",
            "proposal_id": "prop-002"
        },
        "proposal": {
            "proposal_id": "prop-001",
            "expires_at": "2099-12-31T23:59:59Z"
        },
        "action_type": "engage",
        "already_executed": false
    }
}

# Test denied when proposal expired
test_effects_denied_expired_proposal if {
    not effects.allow_effect with input as {
        "decision": {
            "approved": true,
            "approved_by": "commander-alpha",
            "proposal_id": "prop-001"
        },
        "proposal": {
            "proposal_id": "prop-001",
            "expires_at": "2000-01-01T00:00:00Z"
        },
        "action_type": "engage",
        "already_executed": false
    }
}

# Test deny reasons populated when not approved
test_effects_deny_reasons_not_approved if {
    count(effects.deny) > 0 with input as {
        "decision": {
            "approved": false,
            "approved_by": "commander-alpha",
            "proposal_id": "prop-001"
        },
        "proposal": {
            "proposal_id": "prop-001",
            "expires_at": "2099-12-31T23:59:59Z"
        },
        "action_type": "engage",
        "already_executed": false
    }
}

# Test deny reasons populated when no approver
test_effects_deny_reasons_no_approver if {
    count(effects.deny) > 0 with input as {
        "decision": {
            "approved": true,
            "approved_by": "",
            "proposal_id": "prop-001"
        },
        "proposal": {
            "proposal_id": "prop-001",
            "expires_at": "2099-12-31T23:59:59Z"
        },
        "action_type": "engage",
        "already_executed": false
    }
}

# Test deny reasons populated when system approver
test_effects_deny_reasons_system_approver if {
    count(effects.deny) > 0 with input as {
        "decision": {
            "approved": true,
            "approved_by": "system",
            "proposal_id": "prop-001"
        },
        "proposal": {
            "proposal_id": "prop-001",
            "expires_at": "2099-12-31T23:59:59Z"
        },
        "action_type": "engage",
        "already_executed": false
    }
}

# Test deny reasons populated when already executed
test_effects_deny_reasons_already_executed if {
    count(effects.deny) > 0 with input as {
        "decision": {
            "approved": true,
            "approved_by": "commander-alpha",
            "proposal_id": "prop-001"
        },
        "proposal": {
            "proposal_id": "prop-001",
            "expires_at": "2099-12-31T23:59:59Z"
        },
        "action_type": "engage",
        "already_executed": true
    }
}

# Test approval reasons for human-required actions
test_effects_approval_reasons_engage if {
    count(effects.approval_reasons) > 0 with input as {
        "decision": {
            "approved": true,
            "approved_by": "commander-alpha",
            "proposal_id": "prop-001"
        },
        "proposal": {
            "proposal_id": "prop-001",
            "expires_at": "2099-12-31T23:59:59Z"
        },
        "action_type": "engage",
        "already_executed": false,
        "threat_level": "critical",
        "priority": 9
    }
    with data.cjadc2.human_approval_required as ["engage", "intercept"]
}

# Test approval reasons for critical threat
test_effects_approval_reasons_critical_threat if {
    count(effects.approval_reasons) > 0 with input as {
        "decision": {
            "approved": true,
            "approved_by": "commander-alpha",
            "proposal_id": "prop-001"
        },
        "proposal": {
            "proposal_id": "prop-001",
            "expires_at": "2099-12-31T23:59:59Z"
        },
        "action_type": "track",
        "already_executed": false,
        "threat_level": "critical",
        "priority": 5
    }
    with data.cjadc2.human_approval_required as []
}

# Test approval reasons for high priority
test_effects_approval_reasons_high_priority if {
    count(effects.approval_reasons) > 0 with input as {
        "decision": {
            "approved": true,
            "approved_by": "commander-alpha",
            "proposal_id": "prop-001"
        },
        "proposal": {
            "proposal_id": "prop-001",
            "expires_at": "2099-12-31T23:59:59Z"
        },
        "action_type": "track",
        "already_executed": false,
        "threat_level": "medium",
        "priority": 8
    }
    with data.cjadc2.human_approval_required as []
}
