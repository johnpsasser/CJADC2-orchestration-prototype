#!/usr/bin/env bash
#
# CJADC2 Platform Demo Script
# Demonstrates the end-to-end flow from sensor detection to approved action
#
# Usage: ./scripts/demo.sh
#

set -e

# Colors
CYAN='\033[36m'
GREEN='\033[32m'
YELLOW='\033[33m'
RED='\033[31m'
BOLD='\033[1m'
RESET='\033[0m'

# Configuration
API_URL="${API_URL:-http://localhost:8080}"
MAX_WAIT=60
POLL_INTERVAL=2

#------------------------------------------------------------------------------
# Helper Functions
#------------------------------------------------------------------------------

log_info() {
    echo -e "${CYAN}[INFO]${RESET} $1"
}

log_success() {
    echo -e "${GREEN}[OK]${RESET} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${RESET} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${RESET} $1"
}

log_step() {
    echo ""
    echo -e "${BOLD}${CYAN}=== $1 ===${RESET}"
    echo ""
}

wait_for_service() {
    local url=$1
    local name=$2
    local waited=0

    while [ $waited -lt $MAX_WAIT ]; do
        if curl -sf "$url" > /dev/null 2>&1; then
            return 0
        fi
        sleep $POLL_INTERVAL
        waited=$((waited + POLL_INTERVAL))
    done
    return 1
}

pretty_json() {
    if command -v jq > /dev/null 2>&1; then
        jq '.'
    else
        cat
    fi
}

#------------------------------------------------------------------------------
# Demo Steps
#------------------------------------------------------------------------------

check_prerequisites() {
    log_step "Checking Prerequisites"

    if ! command -v curl > /dev/null 2>&1; then
        log_error "curl is required but not installed"
        exit 1
    fi
    log_success "curl is available"

    if ! command -v jq > /dev/null 2>&1; then
        log_warn "jq is not installed - JSON output will not be formatted"
    else
        log_success "jq is available"
    fi

    if ! command -v docker > /dev/null 2>&1; then
        log_error "docker is required but not installed"
        exit 1
    fi
    log_success "docker is available"
}

start_platform() {
    log_step "Starting CJADC2 Platform"

    log_info "Starting services with docker compose..."
    docker compose up -d

    log_info "Waiting for services to become healthy..."

    echo -n "  API Gateway: "
    if wait_for_service "$API_URL/health" "API Gateway"; then
        echo -e "${GREEN}ready${RESET}"
    else
        echo -e "${RED}timeout${RESET}"
        log_error "API Gateway failed to start"
        exit 1
    fi

    echo -n "  NATS:        "
    if wait_for_service "http://localhost:8222/healthz" "NATS"; then
        echo -e "${GREEN}ready${RESET}"
    else
        echo -e "${RED}timeout${RESET}"
        log_error "NATS failed to start"
        exit 1
    fi

    echo -n "  OPA:         "
    if wait_for_service "http://localhost:8181/health" "OPA"; then
        echo -e "${GREEN}ready${RESET}"
    else
        echo -e "${RED}timeout${RESET}"
        log_error "OPA failed to start"
        exit 1
    fi

    log_success "All services are healthy"
}

show_service_urls() {
    log_step "Service URLs"

    echo "  Web UI:        http://localhost:3000"
    echo "  API Gateway:   http://localhost:8080"
    echo "  NATS Monitor:  http://localhost:8222"
    echo "  Prometheus:    http://localhost:9090"
    echo "  Jaeger:        http://localhost:16686"
    echo "  OPA:           http://localhost:8181"
}

wait_for_tracks() {
    log_step "Waiting for Tracks"

    log_info "Waiting for sensor to generate tracks..."

    local waited=0
    local track_count=0

    while [ $waited -lt $MAX_WAIT ]; do
        track_count=$(curl -sf "$API_URL/api/v1/tracks" | jq '.total // 0' 2>/dev/null || echo "0")

        if [ "$track_count" -gt 0 ]; then
            log_success "Found $track_count active tracks"
            return 0
        fi

        echo -n "."
        sleep $POLL_INTERVAL
        waited=$((waited + POLL_INTERVAL))
    done

    echo ""
    log_warn "No tracks found within timeout - sensor may still be starting"
}

show_tracks() {
    log_step "Current Tracks"

    log_info "Fetching active tracks..."

    local response
    response=$(curl -sf "$API_URL/api/v1/tracks?limit=5")

    if [ -n "$response" ]; then
        echo "$response" | jq '.tracks[] | {
            id: .external_track_id,
            classification: .classification,
            type: .type,
            threat_level: .threat_level,
            confidence: .confidence,
            position: {lat: .position.lat, lon: .position.lon},
            detections: .detection_count
        }' 2>/dev/null || echo "$response"
    else
        log_warn "No tracks available"
    fi
}

wait_for_proposals() {
    log_step "Waiting for Proposals"

    log_info "Waiting for planner to generate proposals..."

    local waited=0
    local proposal_count=0

    while [ $waited -lt $MAX_WAIT ]; do
        proposal_count=$(curl -sf "$API_URL/api/v1/proposals?status=pending" | jq '.total // 0' 2>/dev/null || echo "0")

        if [ "$proposal_count" -gt 0 ]; then
            log_success "Found $proposal_count pending proposals"
            return 0
        fi

        echo -n "."
        sleep $POLL_INTERVAL
        waited=$((waited + POLL_INTERVAL))
    done

    echo ""
    log_warn "No proposals found - this may be expected if no high-threat tracks exist"
}

show_proposals() {
    log_step "Pending Proposals"

    log_info "Fetching pending proposals..."

    local response
    response=$(curl -sf "$API_URL/api/v1/proposals?status=pending&limit=5")

    if [ -n "$response" ]; then
        local count
        count=$(echo "$response" | jq '.total // 0' 2>/dev/null || echo "0")

        if [ "$count" -gt 0 ]; then
            echo "$response" | jq '.proposals[] | {
                proposal_id: .proposal_id,
                track_id: .external_track_id,
                action_type: .action_type,
                priority: .priority,
                threat_level: .threat_level,
                rationale: .rationale,
                expires_at: .expires_at
            }' 2>/dev/null || echo "$response"
        else
            log_info "No pending proposals at this time"
        fi
    else
        log_warn "Could not fetch proposals"
    fi
}

approve_proposal() {
    log_step "Approving a Proposal"

    log_info "Fetching first pending proposal..."

    local proposal_id
    proposal_id=$(curl -sf "$API_URL/api/v1/proposals?status=pending&limit=1" | jq -r '.proposals[0].proposal_id // empty' 2>/dev/null)

    if [ -z "$proposal_id" ] || [ "$proposal_id" = "null" ]; then
        log_warn "No pending proposals to approve"
        log_info "Proposals require high-threat tracks to be generated"
        return 0
    fi

    log_info "Approving proposal: $proposal_id"

    local response
    response=$(curl -sf -X POST "$API_URL/api/v1/proposals/$proposal_id/approve" \
        -H "Content-Type: application/json" \
        -d '{
            "approved_by": "demo-operator",
            "reason": "Approved during demo - verified threat assessment",
            "conditions": ["Maintain safe distance", "Report on contact"]
        }')

    if [ -n "$response" ]; then
        log_success "Proposal approved!"
        echo ""
        echo "$response" | jq '{
            decision_id: .decision_id,
            approved: .approved,
            approved_by: .approved_by,
            reason: .reason
        }' 2>/dev/null || echo "$response"
    else
        log_error "Failed to approve proposal"
    fi
}

show_decisions() {
    log_step "Recent Decisions"

    log_info "Fetching recent decisions..."

    local response
    response=$(curl -sf "$API_URL/api/v1/decisions?limit=5")

    if [ -n "$response" ]; then
        local count
        count=$(echo "$response" | jq '.total // 0' 2>/dev/null || echo "0")

        if [ "$count" -gt 0 ]; then
            echo "$response" | jq '.decisions[] | {
                decision_id: .decision_id,
                approved: .approved,
                approved_by: .approved_by,
                action_type: .action_type,
                approved_at: .approved_at,
                effect_status: .effect_status
            }' 2>/dev/null || echo "$response"
        else
            log_info "No decisions recorded yet"
        fi
    else
        log_warn "Could not fetch decisions"
    fi
}

show_effects() {
    log_step "Executed Effects"

    log_info "Fetching executed effects..."

    local response
    response=$(curl -sf "$API_URL/api/v1/effects?limit=5")

    if [ -n "$response" ]; then
        local count
        count=$(echo "$response" | jq '.total // 0' 2>/dev/null || echo "0")

        if [ "$count" -gt 0 ]; then
            echo "$response" | jq '.effects[] | {
                effect_id: .effect_id,
                action_type: .action_type,
                status: .status,
                result: .result,
                executed_at: .executed_at
            }' 2>/dev/null || echo "$response"
        else
            log_info "No effects executed yet"
        fi
    else
        log_warn "Could not fetch effects"
    fi
}

show_pipeline_summary() {
    log_step "Pipeline Summary"

    local tracks proposals decisions effects

    tracks=$(curl -sf "$API_URL/api/v1/tracks" | jq '.total // 0' 2>/dev/null || echo "?")
    proposals=$(curl -sf "$API_URL/api/v1/proposals?status=pending" | jq '.total // 0' 2>/dev/null || echo "?")
    decisions=$(curl -sf "$API_URL/api/v1/decisions" | jq '.total // 0' 2>/dev/null || echo "?")
    effects=$(curl -sf "$API_URL/api/v1/effects" | jq '.total // 0' 2>/dev/null || echo "?")

    echo "  +-------------------+-------------------+"
    echo "  |     Stage         |      Count        |"
    echo "  +-------------------+-------------------+"
    printf "  | Active Tracks     | %17s |\n" "$tracks"
    printf "  | Pending Proposals | %17s |\n" "$proposals"
    printf "  | Total Decisions   | %17s |\n" "$decisions"
    printf "  | Executed Effects  | %17s |\n" "$effects"
    echo "  +-------------------+-------------------+"
}

show_next_steps() {
    log_step "Next Steps"

    echo "  1. Open the Web UI at http://localhost:3000"
    echo "     - View real-time track positions on the map"
    echo "     - Review and approve/deny proposals"
    echo "     - Monitor the decision audit trail"
    echo ""
    echo "  2. Explore the API at http://localhost:8080"
    echo "     - GET /api/v1/tracks - List active tracks"
    echo "     - GET /api/v1/proposals - List proposals"
    echo "     - POST /api/v1/proposals/:id/approve - Approve"
    echo ""
    echo "  3. View distributed traces at http://localhost:16686"
    echo "     - Search for service: sensor-sim"
    echo "     - Follow a detection through the pipeline"
    echo ""
    echo "  4. View metrics at http://localhost:9090"
    echo "     - Query: agent_messages_total"
    echo "     - Query: agent_processing_latency_seconds"
    echo ""
    echo "  5. Stop the platform:"
    echo "     docker compose down"
    echo ""
    echo "  6. View logs:"
    echo "     docker compose logs -f"
}

#------------------------------------------------------------------------------
# Main
#------------------------------------------------------------------------------

main() {
    echo ""
    echo -e "${BOLD}${CYAN}"
    echo "   ____   _    _    ____   ____  ____  "
    echo "  / ___| | |  / \  |  _ \ / ___|/ ___| "
    echo " | |     | | / _ \ | | | | |    \___ \ "
    echo " | |___  | |/ ___ \| |_| | |___  ___) |"
    echo "  \____| |_/_/   \_\____/ \____||____/ "
    echo ""
    echo "  Combined Joint All-Domain Command and Control"
    echo -e "${RESET}"
    echo ""
    echo "  This demo will:"
    echo "  1. Start the CJADC2 platform"
    echo "  2. Wait for synthetic tracks to be generated"
    echo "  3. Show tracks, proposals, and decisions"
    echo "  4. Demonstrate human approval of a proposal"
    echo "  5. Show the executed effect"
    echo ""
    read -p "Press Enter to continue or Ctrl+C to cancel..."

    check_prerequisites
    start_platform
    show_service_urls

    # Give agents time to start
    log_info "Waiting for agents to initialize (10 seconds)..."
    sleep 10

    wait_for_tracks
    show_tracks

    wait_for_proposals
    show_proposals

    approve_proposal

    # Give effector time to execute
    sleep 3

    show_decisions
    show_effects
    show_pipeline_summary
    show_next_steps

    log_success "Demo complete!"
}

# Run main function
main "$@"
