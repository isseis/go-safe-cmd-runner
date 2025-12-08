---
applyTo: '**'
---
1. Get staged change `git --no-pager diff --staged`
2. Propose commit message for it.
  - The commit message must be in English
  - The commit message must not contain backquote characters (`).
  - The commit message should be concise and descriptive.
  - One line summary + 3-5 bullets points would be expected. If the change is complex and large, longer and more detailed message is acceptable.
  - The commit message should be broken down into lines every 80 characters.
3. Ask confirmation for proceeding commit with y/n prompt
4. If a user lets move forward, commit the change `git commit` with the proposed commit message.
