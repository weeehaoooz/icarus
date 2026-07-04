# Icarus - Identity & Access Management (IAM) Domain

This directory contains the services and interfaces responsible for managing global identity, core security policies, credentials, token generation/verification, and microservice governance/registration across the system.

## Domain Architecture

### Backend Services
- **[icarus-auth-ms](file:///Users/tengweihao/Projects/icarus/icarus/backend/icarus-auth-ms):** Issues authentication tokens, rotating JSON Web Key Sets (JWKS), and handles credentials verification.
- **[icarus-admin-ms](file:///Users/tengweihao/Projects/icarus/icarus/backend/icarus-admin-ms):** Orchestrates module governance (registration) and security policies/admin actions.

### Frontend Consoles
- **[icarus-admin-frontend](file:///Users/tengweihao/Projects/icarus/icarus/frontend/icarus-admin-frontend):** An administrative user interface for managing system users, security parameters, and registration audits.

## Documentation
- Detailed deployment instructions can be found in the [DEPLOYMENT.md](file:///Users/tengweihao/Projects/icarus/icarus/DEPLOYMENT.md) file.
