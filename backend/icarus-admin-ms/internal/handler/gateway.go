package handler

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"icarus-admin-ms/internal/models"
	"strings"
)

// matchesPattern matches request path against API path pattern.
func matchesPattern(path, pattern string) bool {
	if pattern == "*" || pattern == "/*" {
		return true
	}
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		return strings.HasPrefix(path, prefix)
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(path, prefix)
	}
	return path == pattern
}

// GatewayHandler handles reverse-proxying.
func (s *HandlerServer) GatewayHandler(w http.ResponseWriter, r *http.Request) {
	siteDomain := r.PathValue("site_domain")
	if siteDomain == "" {
		s.respondWithError(w, http.StatusBadRequest, "missing site domain in route")
		return
	}

	path := "/" + r.PathValue("path")

	// 1. Fetch Module
	module, err := s.Repo.GetModuleByCode(siteDomain)
	if err != nil {
		s.respondWithError(w, http.StatusNotFound, "module not found: "+siteDomain)
		return
	}

	// 2. Fetch Permissions for this module
	permissions, err := s.Repo.GetPermissionsByModuleID(module.ID)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to query permissions: "+err.Error())
		return
	}

	// 3. Find matching Permission/Route
	var matchedPermission *models.Permission
	for i := range permissions {
		p := &permissions[i]
		methodMatch := p.Method == "*" || strings.ToUpper(p.Method) == "ANY" || strings.ToUpper(p.Method) == strings.ToUpper(r.Method)
		if methodMatch && matchesPattern(path, p.PathPattern) {
			matchedPermission = p
			break
		}
	}

	// If no matching permission/route is onboarded, we don't know where to proxy
	if matchedPermission == nil {
		s.respondWithError(w, http.StatusNotFound, "API endpoint not found on this module")
		return
	}

	// 4. Reverse Proxy to Destination
	targetURLStr := module.BaseURL
	targetURL, err := url.Parse(targetURLStr)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "invalid destination target URL: "+err.Error())
		return
	}

	// Create reverse proxy
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = targetURL.Scheme
			req.URL.Host = targetURL.Host
			req.URL.Path = singleJoiningSlash(targetURL.Path, path)
			req.Host = targetURL.Host
			req.Header.Del("Connection")
			log.Printf("[Gateway Proxy] Proxying to %s%s", targetURLStr, req.URL.Path)
		},
	}

	proxy.ServeHTTP(w, r)
}

func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b
}
