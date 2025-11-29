# Sudo Authorization Workflow - Petri Net Edition

A demonstration of modeling authorization and privilege escalation workflows using Petri nets.

## Overview

This example models a Linux-like sudo authorization workflow, demonstrating how Petri nets can represent:
- State transitions in security workflows
- Authorization decision points
- Session management
- Privilege escalation and de-escalation

## The Model

The sudo authorization workflow is modeled as a Petri net where:
- **Places**: Represent states in the authorization process
  - `UserSession`: Active user session (unprivileged)
  - `SudoRequest`: Request for elevated privileges pending
  - `AuthCheck`: Authentication validation in progress
  - `AdminSession`: Session with elevated privileges
  - `Denied`: Authorization denied
  - `Expired`: Session expired (timeout)
  - `AuditLog`: Logged authorization events

- **Transitions**: Represent actions/events
  - `request_sudo`: User initiates sudo request
  - `authenticate`: System validates credentials
  - `grant_access`: Authorization granted
  - `deny_access`: Authorization denied
  - `timeout`: Session expires
  - `drop_privileges`: User returns to normal session
  - `log_event`: Event logging

## Quick Start

```bash
# Build
go build -o sudo ./cmd

# Run a single authorization simulation
./sudo

# Analyze the authorization model
./sudo --analyze

# Simulate with different scenarios
./sudo --scenario auth-fail     # Authentication failure
./sudo --scenario timeout       # Session timeout
./sudo --scenario success       # Successful authorization

# Run multiple simulations for statistics
./sudo --simulate --count 100

# Verbose mode to see state changes
./sudo --v
```

## Authorization Scenarios

### 1. Successful Authorization
```
UserSession → request_sudo → SudoRequest → authenticate → AuthCheck → grant_access → AdminSession
```

### 2. Failed Authorization
```
UserSession → request_sudo → SudoRequest → authenticate → AuthCheck → deny_access → Denied → return → UserSession
```

### 3. Session Timeout
```
AdminSession → timeout → Expired → return → UserSession
```

### 4. Privilege Drop
```
AdminSession → drop_privileges → UserSession
```

## Petri Net Visualization

```
                              ┌─────────────┐
                              │   AuditLog  │
                              └──────▲──────┘
                                     │
    ┌───────────────┐        ┌───────┴───────┐
    │  UserSession  │───────►│  SudoRequest  │
    └───────────────┘        └───────┬───────┘
           ▲                         │
           │                         ▼
    ┌──────┴──────┐          ┌───────────────┐
    │   Denied    │◄─────────│   AuthCheck   │
    └─────────────┘          └───────┬───────┘
                                     │
                                     ▼
    ┌─────────────┐          ┌───────────────┐
    │   Expired   │◄─────────│ AdminSession  │
    └─────────────┘          └───────────────┘
```

## Example Output

### Normal Mode
```
=== Sudo Authorization Workflow Demo ===
Loaded Petri net with 7 places, 7 transitions, 14 arcs

Starting authorization simulation...

Current state: UserSession (unprivileged)
[>] Transition: request_sudo
Current state: SudoRequest (pending)
[>] Transition: authenticate
Current state: AuthCheck (validating)
[>] Transition: grant_access
Current state: AdminSession (elevated)

=== Authorization Complete ===
Result: Access GRANTED
Session duration: 2.5s
Events logged: 4
```

### Analysis Mode
```
=== Sudo Authorization Model Analysis ===

Model Structure:
  Places: 7
  Transitions: 7
  Arcs: 14

Reachability Analysis:
  Reachable states: 7
  Terminal states: 0 (all states can transition)
  Bounded: true
  Maximum tokens per place: 1

Security Properties:
  ✓ No unauthorized access: Admin requires auth
  ✓ Audit trail: All transitions logged
  ✓ Session timeout: Enforced expiration
  ✓ Privilege drop: Can return to user mode
```

## Model Properties

### Safety Properties (verified by reachability analysis)
1. **No Unauthorized Access**: AdminSession is only reachable through AuthCheck with valid credentials
2. **Bounded Tokens**: Maximum 1 token in any place (single session)
3. **No Deadlocks**: All states have valid outgoing transitions

### Liveness Properties
1. **Authorization Progress**: Request always leads to decision
2. **Session Recovery**: Denied/Expired states can return to UserSession
3. **Audit Completeness**: All authorization events are logged

## Extending the Model

### Multi-User Support
Add places for multiple concurrent user sessions:
```go
net.AddPlace("User1_Session", 1.0, nil, 100, 100, nil)
net.AddPlace("User2_Session", 1.0, nil, 100, 200, nil)
```

### Role-Based Access Control (RBAC)
Add places for different privilege levels:
```go
net.AddPlace("RootPrivilege", 0.0, nil, 400, 100, nil)
net.AddPlace("AdminPrivilege", 0.0, nil, 400, 200, nil)
net.AddPlace("UserPrivilege", 1.0, nil, 400, 300, nil)
```

### Two-Factor Authentication
Add additional authentication places:
```go
net.AddPlace("PasswordCheck", 0.0, nil, 200, 150, nil)
net.AddPlace("TwoFactorCheck", 0.0, nil, 300, 150, nil)
```

## Connection to Real Systems

This model demonstrates concepts applicable to:
- **Linux PAM (Pluggable Authentication Modules)**: Token-based state machine
- **OAuth 2.0 Flows**: Authorization code grant patterns
- **Session Management**: Web application session lifecycles
- **Access Control Systems**: Physical and logical access decisions

## Files

- `cmd/main.go` - Main program and CLI interface
- `cmd/auth.go` - Authorization workflow implementation
- `sudo_model.json` - Generated Petri net model
- `README.md` - This file

## References

- [Petri Nets and Security](https://en.wikipedia.org/wiki/Petri_net#Applications)
- [Linux sudo](https://en.wikipedia.org/wiki/Sudo)
- [pflow.dev Documentation](https://pflow.dev/docs)
