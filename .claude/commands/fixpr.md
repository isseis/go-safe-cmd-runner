Invoke the `fixpr` workflow using the Workflow tool:

```
Workflow({ name: "fixpr" })
```

The workflow runs in the background and returns a task ID. It handles everything:
fetch → triage → fix → build → reply → push.

Once it completes you will be notified. Report the result to the user with **both**
of the following — bare counts alone are not enough:

1. **Summary line**: PR, fixed / invalid / unclear counts, clusters, and commit SHA.

2. **Bot-comment assessment** (from the workflow result's `assessment` array): a
   table grouping every thread by `severity`, so the user sees what each comment
   raised and how much it mattered — not just that N were "fixed". Use these levels:
   - 🔴 **must-fix** — real bug / correctness or security defect
   - 🟡 **worth-fixing** — legitimate improvement, not a bug
   - 🟢 **no-harm** — cosmetic, or invalid/inapplicable: safe to ignore

   For each thread show its `topic`, whether a fix was `applied`, and (for
   unresolved/unclear ones) the `url`. Then give a one-line overall read: was this
   round substantive or noise, and is it worth running again or safe to merge.

3. **Skipped threads**: list any "unclear" or unapplied threads with their URLs so
   the user can resolve them manually.
