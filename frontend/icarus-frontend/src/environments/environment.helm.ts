// Helm / Kubernetes production environment
// API calls are relative paths routed by the NGINX Ingress controller
export const environment = {
  production: true,
  apiAuth: '/api/auth',
  apiAdmin: '/api/admin',
  apiWorkflow: '/api/workflow',
};
