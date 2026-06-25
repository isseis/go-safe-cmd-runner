Invoke the `fixpr` workflow using the Workflow tool:

```
Workflow({ name: "fixpr" })
```

The workflow runs in the background and returns a task ID. It handles everything:
fetch → triage → fix → build → reply → push.

Once it completes you will be notified. Report the summary result to the user,
and list any skipped ("unclear") threads with their URLs so they can be resolved manually.
