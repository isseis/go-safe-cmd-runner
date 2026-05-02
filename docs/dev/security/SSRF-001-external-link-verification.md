# SSRF-001: External Link Verification Security Risk

## Vulnerability Summary

**Title**: Server-Side Request Forgery (SSRF) in External Link Verification
**Severity**: High
**Component**: `scripts/verification/verify_links.go`
**Status**: Identified (Awaiting Mitigation)

## Description

The external link verification feature (`verify_links --external`) makes unchecked HTTP HEAD/GET requests to all URLs found in documentation files. This creates an SSRF vulnerability when:

1. The verification runs in CI/CD environments with access to internal networks
2. Untrusted contributors can modify documentation (e.g., pull requests)
3. The `--external` flag is enabled for untrusted branches

## Attack Scenarios

### Scenario 1: Cloud Metadata Access
```markdown
<!-- In a malicious PR's documentation -->
[Legitimate link](http://169.254.169.254/latest/meta-data/iam/security-credentials/)
```
- Attacker gains access to cloud instance credentials
- Can escalate to full cloud account compromise

### Scenario 2: Internal Network Scanning
```markdown
[Port scan](http://internal-service:8080)
[Database](http://db.internal:5432)
[Admin panel](http://admin.local:3000)
```
- Reveals internal network topology
- Identifies running services and ports
- Maps internal infrastructure

### Scenario 3: SSRF to Internal APIs
```markdown
[API](http://internal-api/admin/users)
[Webhook](http://localhost:9000/trigger-deployment)
```
- Triggers unauthorized actions
- Accesses sensitive internal endpoints
- Bypasses authentication/authorization

## Affected Code

### Primary Vulnerability
**File**: [`scripts/verification/verify_links.go:257-286`](../../scripts/verification/verify_links.go#L257-L286)

```go
func verifyExternalLink(url string, timeoutSec int) (bool, error) {
    client := &http.Client{
        Timeout: time.Duration(timeoutSec) * time.Second,
        // ...
    }

    // ⚠️ NO VALIDATION - Makes request to any URL
    resp, err := client.Head(url)
    if err != nil {
        resp, err = client.Get(url)  // ⚠️ Falls back to GET
        // ...
    }
    // ...
}
```

### Trigger Point
**File**: [`scripts/verification/run_all.sh:154-156`](../../scripts/verification/run_all.sh#L154-L156)

```bash
if [ $CHECK_EXTERNAL -eq 1 ]; then
    LINK_OPTS="$LINK_OPTS --external"
fi
```

## Recommended Mitigations

### Priority 1: URL Allowlist (Immediate)

Implement strict host allowlisting before making any external requests:

```go
var allowedHosts = map[string]bool{
    "github.com":        true,
    "golang.org":        true,
    "pkg.go.dev":        true,
    "docs.rs":           true,
    "en.wikipedia.org":  true,
    "ja.wikipedia.org":  true,
    // Add other trusted domains
}

func isAllowedHost(urlStr string) error {
    u, err := url.Parse(urlStr)
    if err != nil {
        return fmt.Errorf("invalid URL: %w", err)
    }

    // Block all private/internal IP ranges
    if isPrivateIP(u.Hostname()) {
        return fmt.Errorf("private IP addresses are not allowed")
    }

    // Check against allowlist
    if !allowedHosts[u.Host] {
        return fmt.Errorf("host not in allowlist: %s", u.Host)
    }

    return nil
}

func isPrivateIP(host string) bool {
    ip := net.ParseIP(host)
    if ip == nil {
        // Try resolving hostname
        addrs, err := net.LookupIP(host)
        if err != nil || len(addrs) == 0 {
            return false
        }
        ip = addrs[0]
    }

    // Check for private ranges
    // 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, 127.0.0.0/8, 169.254.0.0/16
    return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}
```

### Priority 2: Disable External Checks for Untrusted Input

**CI Configuration** (`.github/workflows/` or equivalent):

```yaml
# For pull requests from forks - NEVER check external links
- name: Verify Documentation (PR from fork)
  if: github.event.pull_request.head.repo.fork == true
  run: ./scripts/verification/run_all.sh
  # Note: NO -e flag for forks

# For trusted branches - can check external links
- name: Verify Documentation (trusted)
  if: github.event.pull_request.head.repo.fork != true || github.ref == 'refs/heads/main'
  run: ./scripts/verification/run_all.sh -e
```

### Priority 3: Enhanced Security Headers

Add security-focused HTTP client configuration:

```go
client := &http.Client{
    Timeout: time.Duration(timeoutSec) * time.Second,
    CheckRedirect: func(req *http.Request, via []*http.Request) error {
        // Validate redirect targets
        if err := isAllowedHost(req.URL.String()); err != nil {
            return fmt.Errorf("redirect blocked: %w", err)
        }
        if len(via) >= 10 {
            return fmt.Errorf("too many redirects")
        }
        return nil
    },
}
```

### Priority 4: DNS Rebinding Protection

Implement time-of-check/time-of-use (TOCTOU) protection:

```go
func verifyExternalLink(url string, timeoutSec int) (bool, error) {
    // First check: validate URL and resolve DNS
    if err := isAllowedHost(url); err != nil {
        return false, err
    }

    parsedURL, _ := url.Parse(url)

    // Resolve DNS and validate resulting IP
    addrs, err := net.LookupIP(parsedURL.Hostname())
    if err != nil {
        return false, fmt.Errorf("DNS lookup failed: %w", err)
    }

    for _, addr := range addrs {
        if isPrivateIP(addr.String()) {
            return false, fmt.Errorf("URL resolves to private IP: %s", addr)
        }
    }

    // Now make the request with a custom dialer that re-checks
    client := &http.Client{
        Transport: &http.Transport{
            DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
                host, _, err := net.SplitHostPort(addr)
                if err != nil {
                    return nil, err
                }
                if isPrivateIP(host) {
                    return nil, fmt.Errorf("connection blocked to private IP: %s", host)
                }
                return (&net.Dialer{}).DialContext(ctx, network, addr)
            },
        },
        Timeout: time.Duration(timeoutSec) * time.Second,
    }

    // Make request...
}
```

## Implementation Plan

### Phase 1: Immediate Risk Reduction
1. **Document the risk** ✅ (this document)
2. **Update CI configuration** to disable `-e` for pull requests
3. **Add warning to run_all.sh** help text about SSRF risk

### Phase 2: Short-term Mitigation (Week 1)
1. Implement URL allowlist with common legitimate domains
2. Add private IP range blocking
3. Add unit tests for URL validation
4. Update documentation

### Phase 3: Long-term Hardening (Week 2-3)
1. Implement DNS rebinding protection
2. Add redirect validation
3. Add comprehensive security tests
4. Consider rate limiting for external requests
5. Add audit logging for all external requests

## Workarounds (Until Fixed)

### For Maintainers
- **NEVER** use `-e` flag on untrusted branches
- Run external link checks manually only on main/release branches
- Review all documentation changes in PRs for suspicious URLs

### For CI/CD
```bash
# Safe: Only check local files for PRs
./scripts/verification/run_all.sh

# Unsafe: External checks only for trusted commits
if [ "$GITHUB_EVENT_NAME" != "pull_request" ]; then
    ./scripts/verification/run_all.sh -e
fi
```

## References

- **OWASP SSRF**: https://owasp.org/www-community/attacks/Server_Side_Request_Forgery
- **CWE-918**: Server-Side Request Forgery (SSRF)
- **AWS Metadata Endpoint**: http://169.254.169.254/latest/meta-data/
- **RFC 1918**: Private IP Address Ranges

## Timeline

- **2025-12-14**: Vulnerability identified and documented
- **TBD**: Mitigation implementation scheduled
- **TBD**: Security fix released

## Related Issues

- Consider adding similar validation to any other network-calling code
- Review all HTTP client usage in the codebase for similar issues
