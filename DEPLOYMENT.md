# Icarus Identity & Access Management (IAM) Deployment Guide

This document outlines the deployment strategy, environment prerequisites, and bootstrapping order for the **Icarus** core authentication and governance domain.

## 1. Bootstrapping Order
Icarus services **MUST** be deployed and fully operational before any application platform services (e.g., SAV) start.
1. **Persistent Database/Identity Store:** Ensure SQLite/PostgreSQL is initialized.
2. **`icarus-auth-ms` (Authentication Service):** Starts first to generate public/private key pairs and serve the JWKS (JSON Web Key Set) endpoint.
3. **`icarus-admin-ms` (Identity & Policy Engine):** Starts once `icarus-auth-ms` is healthy to verify admin tokens and host the governance registration APIs.
4. **`icarus-frontend`:** Deployed last to allow administrator dashboard access.

## 2. Secrets & Certificate Management
> [!IMPORTANT]
> Never hardcode or check in cryptographic keys. Use a secure vaults provider (e.g., HashiCorp Vault, AWS Secrets Manager, Google Secret Manager).

- **Token Signing Keys (`KEYS_DIR`):** The auth microservice requires a read-write directory path (`/app/keys` in Docker) to load/generate token signing keys.
- **Certificates Distribution:** Public certs are exposed at `/certs?format=pem` for downstream services (like SAV) to verify signature authenticity.

## 3. High-Availability (HA) Strategy
- **`icarus-auth-ms` Scaling:** Stateless and can be scaled horizontally. Session affinity is not required as verification is token-based.
- **Database Layer:** Deploy the DB with replica nodes. Ensure write queries route to primary and read validation queries (when non-cached) route to read-replicas.
- **Ingress Restrictions:** Limit exposure of `icarus-auth-ms` and `icarus-admin-ms` internal endpoints. Only `/certs` and user authentication paths should be publicly reachable.
