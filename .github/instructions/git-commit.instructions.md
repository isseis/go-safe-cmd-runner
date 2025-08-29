---
applyTo: '**'
---
1. Select GPT-5 mini LLM model.
2. Run `pre-commit` (without `--all-files` option) to check staged files, and if fails, stop the process.
3. Get staged change `git --no-pager diff --staged`
4. Propose commit message for it.
  - The commit message must be in English
  - The commit message must not contain backquote characters (`).
  - The commit message should be concise and descriptive.
  - One line summary + 3-5 bullets points would be expected. If the change is complex and large, longer and more detailed message is acceptable.
  - The commit message should be broken down into lines every 80 characters.
5. Ask confirmation for proceeding commit with y/n prompt
6. If a user lets move forward, commit the change `git commit` with the proposed commit message.
