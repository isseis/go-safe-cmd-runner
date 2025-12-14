# CI Configuration Example for Safe External Link Verification

This document provides example CI configurations that safely use the documentation verification tools while preventing SSRF attacks.

## Key Security Principles

1. **NEVER** enable external link checking (`-e` flag) for pull requests from forks
2. **NEVER** enable external link checking for untrusted branches
3. **ONLY** enable external link checking for:
   - Main/release branches after merge
   - Pull requests from trusted team members (same repository)
   - Manual runs by maintainers

## GitHub Actions Configuration

### Complete Example

Create `.github/workflows/docs-verification.yml`:

```yaml
name: Documentation Verification

on:
  pull_request:
    paths:
      - 'docs/**'
      - 'scripts/verification/**'
  push:
    branches:
      - main
      - 'release/**'
  workflow_dispatch:  # Allow manual triggering

permissions:
  contents: read

jobs:
  verify-docs:
    name: Verify Documentation
    runs-on: ubuntu-latest

    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.23'

      # SECURITY: Determine if external link checking is safe
      - name: Determine verification flags
        id: flags
        run: |
          # Default: no external link checking (safe)
          FLAGS=""

          # Only enable external checking for trusted scenarios
          if [[ "${{ github.event_name }}" == "push" ]] && [[ "${{ github.ref }}" == "refs/heads/main" ]]; then
            echo "Running on main branch - enabling external link checks"
            FLAGS="-e"
          elif [[ "${{ github.event_name }}" == "pull_request" ]] && [[ "${{ github.event.pull_request.head.repo.fork }}" != "true" ]]; then
            echo "Running on PR from same repository - enabling external link checks"
            FLAGS="-e"
          else
            echo "SECURITY: External link checking disabled (untrusted source)"
          fi

          echo "flags=$FLAGS" >> $GITHUB_OUTPUT

      - name: Run documentation verification
        run: |
          cd scripts/verification
          ./run_all.sh -v ${{ steps.flags.outputs.flags }}

      - name: Upload verification reports
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: verification-reports
          path: build/verification-reports/
          retention-days: 30

  # Separate job for security testing
  verify-security:
    name: Verify Security Tests
    runs-on: ubuntu-latest

    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.23'

      - name: Run security tests for verify_links
        run: |
          cd scripts/verification
          go test -v verify_links.go verify_links_test.go
```

### Simplified Example (Conservative)

For maximum security, never enable external link checking in CI:

```yaml
name: Documentation Verification (Safe)

on:
  pull_request:
    paths:
      - 'docs/**'
  push:
    branches:
      - main

jobs:
  verify-docs:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'

      # SECURITY: Never use -e flag in CI
      - name: Verify documentation
        run: ./scripts/verification/run_all.sh -v
```

## GitLab CI Configuration

Create `.gitlab-ci.yml`:

```yaml
stages:
  - verify

variables:
  # SECURITY: Default to no external link checking
  VERIFY_FLAGS: ""

.verify-docs-base:
  stage: verify
  image: golang:1.23
  script:
    - cd scripts/verification
    - ./run_all.sh -v $VERIFY_FLAGS
  artifacts:
    when: always
    paths:
      - build/verification-reports/
    expire_in: 30 days

# Safe verification for merge requests (no external checks)
verify-docs-mr:
  extends: .verify-docs-base
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
  variables:
    VERIFY_FLAGS: ""  # No -e flag

# Full verification for main branch only
verify-docs-main:
  extends: .verify-docs-base
  rules:
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
  variables:
    VERIFY_FLAGS: "-e"  # Enable external checks for trusted branch

# Manual verification with external checks (maintainers only)
verify-docs-manual:
  extends: .verify-docs-base
  rules:
    - if: $CI_PIPELINE_SOURCE == "web"
      when: manual
  variables:
    VERIFY_FLAGS: "-e"
```

## Jenkins Pipeline

Create `Jenkinsfile`:

```groovy
pipeline {
    agent any

    environment {
        // SECURITY: Determine if external link checking is safe
        VERIFY_FLAGS = "${env.CHANGE_ID ? '' : (env.BRANCH_NAME == 'main' ? '-e' : '')}"
    }

    stages {
        stage('Setup') {
            steps {
                sh 'go version'
            }
        }

        stage('Verify Documentation') {
            steps {
                script {
                    def isPR = env.CHANGE_ID != null
                    def isMain = env.BRANCH_NAME == 'main'

                    if (isPR) {
                        echo "SECURITY: Pull request detected - external link checking disabled"
                    } else if (isMain) {
                        echo "Main branch detected - enabling external link checking"
                    } else {
                        echo "Branch build - external link checking disabled"
                    }
                }

                sh """
                    cd scripts/verification
                    ./run_all.sh -v \${VERIFY_FLAGS}
                """
            }
        }
    }

    post {
        always {
            archiveArtifacts artifacts: 'build/verification-reports/**', allowEmptyArchive: true
        }
    }
}
```

## CircleCI Configuration

Create `.circleci/config.yml`:

```yaml
version: 2.1

executors:
  golang:
    docker:
      - image: cimg/go:1.23

jobs:
  verify-docs-safe:
    executor: golang
    steps:
      - checkout
      - run:
          name: Verify documentation (no external checks)
          command: |
            cd scripts/verification
            ./run_all.sh -v
      - store_artifacts:
          path: build/verification-reports
          destination: reports

  verify-docs-full:
    executor: golang
    steps:
      - checkout
      - run:
          name: Verify documentation (with external checks)
          command: |
            cd scripts/verification
            ./run_all.sh -v -e
      - store_artifacts:
          path: build/verification-reports
          destination: reports

workflows:
  version: 2
  verify:
    jobs:
      # Safe verification for PRs
      - verify-docs-safe:
          filters:
            branches:
              ignore: main

      # Full verification only for main branch
      - verify-docs-full:
          filters:
            branches:
              only: main
```

## Manual Verification for Maintainers

Maintainers can run full verification locally on trusted content:

```bash
# Before merging a PR, maintainers can check out the branch and run:
git fetch origin pull/123/head:pr-123
git checkout pr-123

# Review the documentation changes first
git diff main -- docs/

# If the changes look safe, run full verification
./scripts/verification/run_all.sh -v -e

# Review the reports
cat build/verification-reports/links_report.txt
```

## Security Checklist for CI Setup

Before enabling external link checking in CI, verify:

- [ ] External checks are NEVER enabled for pull requests from forks
- [ ] External checks are NEVER enabled for untrusted branches
- [ ] The allowlist in `verify_links.go` contains only trusted domains
- [ ] Private IP blocking is implemented and tested
- [ ] DNS rebinding protection is in place
- [ ] CI runners do NOT have access to sensitive internal networks
- [ ] Security tests pass successfully
- [ ] Logs do not expose sensitive information from blocked requests

## Incident Response

If SSRF exploitation is suspected:

1. **Immediately disable** external link checking in all CI pipelines
2. Review CI logs for suspicious HTTP requests
3. Check for unauthorized access to internal services
4. Rotate any credentials that may have been exposed
5. Review and restrict CI runner network access
6. Update the allowlist and security controls
7. Re-enable external checking only after fixes are verified

## References

- [SSRF-001 Security Advisory](./SSRF-001-external-link-verification.md)
- [OWASP SSRF Prevention Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Server_Side_Request_Forgery_Prevention_Cheat_Sheet.html)
- [GitHub Actions Security Hardening](https://docs.github.com/en/actions/security-guides/security-hardening-for-github-actions)
