# Product Requirements Document
## Talos

## 1. Overview

Talos is a self-hosted deployment platform for Dockerized applications running on a single VPS. It is designed for a solo operator or small technical team that wants a simple control plane for deploying and operating apps without adopting a larger platform.

This PRD is split into:

- `v1 Launch`: the first complete product that should be built and stabilized
- `v2 Expansion`: the next layer of capabilities that grow Talos without changing its core identity

Talos is not intended to become a general-purpose PaaS, Kubernetes replacement, or multi-tenant platform.

## 2. Product Vision

Talos deploys and operates Dockerized applications from GitHub onto one VPS with a simple, reliable, self-hosted control plane.

## 3. Product Principles

- One obvious happy path
- Reliability before breadth
- Opinionated defaults over feature sprawl
- Talos owns its runtime layout
- Advanced capabilities must strengthen the same product identity
- Host integration stays bounded and supportable

## 4. Target User

The primary user is a solo operator, founder, or small engineering team deploying apps to one VPS.

The user is expected to:

- understand GitHub and Docker at a practical level
- have shell access to a Linux VPS
- be comfortable running a bootstrap installer
- prefer simplicity and coherence over maximum flexibility

## 5. Non-Goals

Talos is not intended to support:

- Kubernetes
- multi-node orchestration
- multi-tenant hosting
- complex RBAC in the initial product
- arbitrary reverse proxy adoption
- arbitrary infrastructure topology adoption
- general CI/CD system replacement

## 6. v1 Launch

### 6.1 Summary

`v1 Launch` is a narrow, reliable deployment platform for one VPS.

The launch contract is:

- one VPS
- local admin/password auth
- GitHub Actions webhook flow as the default deployment path
- Talos-managed Traefik
- domain-based access with IP:port fallback
- host-local installer
- Ansible-backed bootstrap internally
- reuse existing Docker when present

### 6.2 Product Goals

- provide a reliable single-VPS application deployment platform
- keep the primary operator workflow easy to understand
- support both empty-host bootstrap and reuse of an existing Docker host
- keep routing and public access consistent through Talos-managed Traefik
- keep the codebase small enough to evolve without major rewrites

### 6.3 Core User Flow

1. User SSHes into the VPS and runs the installer locally on the host.
2. Talos prepares the host if required.
3. User opens Talos in the browser and creates a local admin account.
4. User creates an app with:
   - app name
   - GitHub repository URL
   - production branch
   - internal port
   - optional domain
5. Talos provides the GitHub Actions deployment contract.
6. GitHub Actions builds and publishes the image.
7. GitHub Actions sends a signed webhook to Talos.
8. Talos deploys the image, updates routing, and records deploy state.
9. User views status, logs, deploy history, and rollback controls in Talos.

### 6.4 Functional Requirements

#### Authentication

Talos must:

- support local admin/password login
- store sessions server-side
- protect management routes behind authentication
- support secure logout and session invalidation

#### App Management

Talos must allow operators to:

- create an app
- view app details
- update app configuration
- delete an app
- inspect deployment status and history
- trigger rollback

The v1 app model must support:

- app name
- source type
- repository URL
- production branch
- internal container port
- image reference
- optional domain
- optional fallback public port
- current access URL
- deployment metadata

Talos must enforce uniqueness where needed, including:

- app name
- domain
- fallback public port

#### Deployment

Talos must:

- accept signed deploy webhooks from GitHub Actions
- verify authenticity before acting
- create deploy records for every deploy attempt
- prevent overlapping deploys per app
- pull the target image
- start a candidate container
- verify health before switching traffic
- preserve the last healthy deployment when a new deploy fails
- store deploy metadata, outcome, and logs
- support redeploy and rollback

#### Routing and Public Access

Talos always routes public app traffic through Talos-managed Traefik.

Talos must support two access modes:

##### Domain Mode

- app is exposed through a configured domain
- Traefik routes by host rule
- app is reachable by domain
- HTTPS is expected here

##### Port Fallback Mode

- used when the app has no domain yet
- Talos assigns a unique public fallback port
- Traefik exposes the app on VPS public IP and assigned port
- app is reachable as `http://VPS_IP:PORT`
- this mode is intended for bootstrap, testing, and early usage

Talos must allow migration from port fallback mode to domain mode later.

#### Observability

Talos must show:

- current app status
- current public access URL
- current active deploy
- recent deploy history
- runtime logs from the active container

#### Rollback

Talos must:

- allow rollback to the previous successful deployment
- record rollback actions in deploy history
- use a known-good existing artifact when rolling back

### 6.5 Host Bootstrap and Lifecycle

Talos must provide a complete launch-ready host lifecycle story.

#### Install

Talos must support a primary installation flow where:

- user SSHes into the VPS
- user runs one installer locally on the host

Implementation may include:

- shell entrypoint
- Ansible-backed bootstrap internals

Ansible is an implementation detail, not the product concept.

#### Supported Host States

Talos must support:

##### Empty VPS

- install missing host dependencies
- prepare Talos runtime layout
- install Talos and Traefik

##### Existing Docker Host

- detect compatible Docker installation
- reuse Docker when present
- install or configure only missing Talos-owned components

#### Reuse Policy

Talos may reuse:

- Docker Engine
- Docker Compose or required Docker plugins

Talos should not try to adopt:

- arbitrary existing Traefik setups
- arbitrary reverse proxy configuration
- arbitrary custom host networking layouts

#### Talos-Owned Host Components

Talos always manages:

- its own Traefik instance and configuration
- its own runtime directories
- its own Docker network
- its own systemd services
- its own public routing state

#### Operations

Talos should provide:

- scripted upgrade
- scripted backup
- uninstall while preserving data
- uninstall and purge data

### 6.6 UX Requirements

The v1 UI should:

- make the main deployment workflow obvious
- keep the dashboard focused on app state and next actions
- clearly show whether an app uses:
  - domain mode
  - IP:port fallback
- surface deploy, rollback, and access details clearly
- avoid presenting too many competing advanced choices

### 6.7 Non-Functional Requirements

#### Reliability

- deploy and app state must persist across restarts
- failed deploys must not destroy working production state
- supported install and upgrade flows must be retryable

#### Simplicity

- the product must have one obvious default deploy workflow
- the installer must have one primary operational story
- the codebase must avoid speculative abstractions

#### Security

- secrets must be encrypted at rest
- sessions must be validated server-side
- webhooks must be signed and verified
- sensitive values should not be repeatedly exposed in the UI

### 6.8 Architecture Direction

Recommended subsystem boundaries for `v1 Launch`:

- `internal/server`
- `internal/store`
- `internal/auth`
- `internal/deploy`
- `internal/runtime/docker`
- `internal/proxy/traefik`
- `internal/bootstrap`
- `internal/github`

### 6.9 Launch Success Criteria

`v1 Launch` is successful if:

- a clean VPS can be bootstrapped with one installer
- an existing Docker host can run Talos without reinstalling Docker
- one GitHub repository can deploy automatically through GitHub Actions
- failed deploys do not destroy the working production state
- apps can be accessed before DNS is ready through IP:port fallback

## 7. v2 Expansion

### 7.1 Summary

`v2 Expansion` adds smoother onboarding and optional managed platform capabilities without changing the core Talos identity.

The expansion areas are:

- preview environments
- managed PostgreSQL
- managed MySQL
- managed Redis
- managed Garage object storage
- later GitHub-native onboarding

### 7.2 Product Goals

- reduce operator friction after the core runtime is stable
- add common backing services without turning Talos into a general infrastructure suite
- improve GitHub onboarding after the deploy/runtime path is proven
- keep all advanced features optional and app-scoped

### 7.3 Preview Environments

Talos should support preview environments in a later phase.

Preview environments should:

- be derived from pull request activity
- remain logically separate from production
- have unique public access URLs
- support create, update, and cleanup lifecycle
- be clearly marked as preview in the UI and runtime state

Previews may start with GitHub Actions-driven orchestration first, then later move toward GitHub App-native events.

### 7.4 Managed Services

Talos will support optional managed backing services.

These are app-scoped attachments, not the primary product identity.

#### Managed PostgreSQL

Talos should support:

- one database per app
- generated database, user, and password
- environment variable injection
- lifecycle state:
  - pending
  - provisioned
  - retained
  - failed

#### Managed MySQL

Talos should support:

- one database/schema per app
- generated credentials
- environment variable injection
- lifecycle behavior comparable to PostgreSQL

#### Managed Redis

Talos should support:

- per-app Redis user or app-specific binding
- generated credentials
- environment variable injection
- operator-visible lifecycle state

#### Managed Garage Object Storage

Talos should support:

- a Talos-managed shared Garage instance
- per-app bucket and credential binding
- generated S3-compatible access credentials
- environment variable injection
- bucket lifecycle visibility

#### Managed Service Constraints

Managed services must remain:

- optional
- app-oriented
- operationally bounded
- clearly visible in lifecycle state

Talos should not drift into a generic data platform.

### 7.5 GitHub-Native Onboarding

After the core deploy/runtime path is mature, Talos may add GitHub-native onboarding.

This may include:

- GitHub App installation
- repository picker onboarding
- installation and repository permission tracking
- direct GitHub webhook handling for managed events
- optional Talos-managed build flow

This is a later product enhancement, not part of the initial launch dependency chain.

### 7.6 UX Expansion

Later UI phases may expand:

- GitHub-native onboarding
- preview environment visibility
- managed service configuration
- operational settings and host summaries
- clearer service and preview lifecycle messaging

### 7.7 Expanded Architecture Direction

Recommended additional subsystem boundaries later:

- `internal/services/postgres`
- `internal/services/mysql`
- `internal/services/redis`
- `internal/services/garage`
- deeper `internal/github` integration for GitHub App flows

The system should continue to favor:

- explicit state transitions
- small modules
- clear ownership of host and runtime concerns
- minimal speculative abstraction

### 7.8 Expansion Success Criteria

`v2 Expansion` is successful if:

- preview environments do not complicate production deploy reliability
- managed services can be attached without redefining Talos as a database platform
- advanced onboarding reduces operator friction without creating multiple conflicting primary workflows
- the codebase remains small enough that feature additions do not require major rewrites

## 8. Public Interfaces and Product Contracts

Talos should maintain clear product contracts for:

### App Definition

- repository URL
- branch
- internal port
- source type
- optional domain
- optional fallback public port
- managed service attachments

### Deployment Contract

- signed webhook input from GitHub Actions
- stable deploy state lifecycle
- predictable routing switch behavior
- rollback behavior

### Bootstrap Contract

- local on-VPS install
- empty-host bootstrap
- Docker reuse where compatible
- Talos-owned Traefik/runtime/service layout

### Service Attachment Contract

Each managed service must define:

- app attachment state
- provisioning or binding lifecycle
- generated environment variables
- retention behavior on app deletion
- operator-visible service state

### Operations Contract

- install
- backup
- upgrade
- uninstall
- rollback

## 9. Release Phasing

### Phase 1: Core Runtime

Deliver:

- local auth
- app CRUD
- SQLite metadata
- Docker deployment runtime
- Traefik routing
- logs
- rollback

### Phase 2: GitHub Deploy Contract

Deliver:

- GitHub Actions deployment contract
- signed webhook verification
- production deploy flow

### Phase 3: Host Lifecycle

Deliver:

- local installer
- Ansible-backed bootstrap
- Docker reuse support
- Traefik provisioning
- backup, upgrade, and uninstall flows

### Phase 4: Managed Services

Deliver in recommended order:

1. PostgreSQL
2. Redis
3. MySQL
4. Garage

### Phase 5: Preview Environments

Deliver:

- pull-request-based preview lifecycle
- preview access routing
- preview UI visibility

### Phase 6: GitHub-Native Onboarding

Deliver:

- GitHub App integration
- repository picker
- installation permission awareness
- optional GitHub-managed build flow

## 10. Assumptions and Defaults

- Talos remains a single-VPS product
- local admin auth is the initial primary auth model
- GitHub Actions webhook flow is the launch deploy path
- Traefik remains mandatory and Talos-managed
- domain is optional
- no-domain access uses Traefik IP:port fallback
- Docker reuse is supported; arbitrary Traefik reuse is not
- bootstrap is Ansible-backed internally
- Ubuntu is the initial supported bootstrap target
- MySQL, PostgreSQL, Redis, and Garage are optional managed services
- GitHub-native onboarding is a later phase, not a launch dependency
