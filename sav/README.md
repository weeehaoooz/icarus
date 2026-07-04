# SAV - Smart Asset Valuation Domain

This directory contains the services and interfaces for the Smart Asset Valuation (SAV) application platform. It implements client interfaces, advisor dashboards, and calculation/business logic.

## Domain Architecture

### Backend Services
- **[sav-platform-ms](file:///Users/tengweihao/Projects/icarus/sav/backend/sav-platform-ms):** Orchestrates financial data, asset listings, and general SAV application states.
- **[retire-ms](file:///Users/tengweihao/Projects/icarus/sav/backend/retire-ms):** Downstream calculation microservice for retirement modeling and wealth projection.

### Frontend Consoles
- **[sav-frontend](file:///Users/tengweihao/Projects/icarus/sav/frontend/sav-frontend):** Customer-facing portal for modeling retirement goals and managing portfolios.
- **[sav-admin-frontend](file:///Users/tengweihao/Projects/icarus/sav/frontend/sav-admin-frontend):** Internal platform admin portal for managers and financial advisors.

## Documentation
- Detailed deployment instructions can be found in the [DEPLOYMENT.md](file:///Users/tengweihao/Projects/icarus/sav/DEPLOYMENT.md) file.
