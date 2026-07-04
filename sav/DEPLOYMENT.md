# SAV Application Platform Deployment Guide

This document outlines the deployment strategy, environment prerequisites, and bootstrapping order for the **Smart Asset Valuation (SAV)** application domain.

## 1. External Dependencies & Bootstrapping
SAV services rely entirely on a running Icarus IAM instance.
- **Dependency Checks:** Downstream services must execute a startup health probe checking:
  - JWKS verification endpoint: `http://icarus-auth-ms:8080/certs`
  - Governance registration endpoint: `http://icarus-admin-ms:8081/health`
- **Module Self-Registration:** On startup, `sav-platform-ms` and `retire-ms` register their modules with the Icarus Governance endpoint (`PLATFORM_MS_REGISTRATION_URL`). Ensure register credentials are injected correctly.

## 2. Horizontal Scaling & Microservice Separation
- **`retire-ms` (Calculation Engine):** High-CPU/Memory bound. Scale horizontally during periods of high financial/valuation requests. Configure appropriate horizontal pod autoscalers (HPAs) based on CPU utilization.
- **`sav-platform-ms`:** Scales based on concurrent HTTP requests.
- **Static Assets:** Build and package `sav-frontend` and `sav-admin-frontend` into optimized static containers, offloading static asset delivery to Nginx or a global Content Delivery Network (CDN).

## 3. Networking & Traffic Routing
- Public entry is managed via the top-level API gateway.
- `sav-platform-ms` communication to database systems should be restricted to VPC private subnets.
- Downstream endpoints do not require direct public access; routing should be configured exclusively through path rules `/sav-admin/*` and `/*` in Nginx.
