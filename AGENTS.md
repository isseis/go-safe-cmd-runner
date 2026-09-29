# AGENTS.md

Guidance for automated agents, including code reviewers, working on this
repository. The project's conventions, design principles and testing rules are
in [CLAUDE.md](CLAUDE.md); follow them.

## Review guidelines

- Report defects by their worst outcome when the code runs: broken behavior, a
  security hole, a failing build, or a test that cannot fail for its stated
  reason.
- Do not re-raise a finding that has been answered in its review thread unless
  the code it concerns has changed since the answer.

### Static guards in `internal/errmsg/errmsg_guard_test.go`

These guards detect mistakes that developers and agents of this repository make
while writing ordinary code. They are not a sandbox against code written to get
around them. The threat model is in
`docs/tasks/0178_structured_error_message_redaction/03_detailed_specification.md`
§9.0.

- Do not report bypasses that need deliberately contrived code, for example a
  type alias declared only to route around a rule, `reflect`, or `unsafe`.
- The guards list the forms they allow and reject the rest. A form the guards
  reject although ordinary code would legitimately write it (a false positive)
  is worth reporting. An accepted form is worth reporting only if ordinary code
  would plausibly write it by mistake; say why.
- No production package uses `internal/errmsg` yet; later phases add the
  callers. Prefer findings about the guards' rules over findings about
  hypothetical code that no phase plans to write.
