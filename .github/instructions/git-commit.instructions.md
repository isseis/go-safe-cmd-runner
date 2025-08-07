---
applyTo: '**'
---
1. Switch to GPT-4.1 agent mode
2. Run `pre-commit`, and if fails, stop the process.
3. Get staged change `git --no-pager diff --staged`
4. Propose commit message for it.
5. Ask confirmation for proceeding commit with y/n prompt
6. If a user lets move forward, commit the change `git commit` with the proposed commit message.
