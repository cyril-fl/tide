---
name: tide-issue-done
description: Finish a GitHub issue of Notes without being asked - commit, tag a version, close the issue, open the follow-up issue for what is left. Use as soon as the work on an issue is done and its tests pass, or when the user says "issue finie", "ferme l'issue", "termine l'issue".
---

# Finish an issue

Run these steps in order, once the work on the issue is done and its tests pass. The details are in the `tide-release` skill: follow it. You make the commit (step 1); steps 2 to 4 go to the `tide-release-checker` subagent, which runs the checks first.

1. **Commit** (you): on `develop`, only your own hunks staged, one commit per change, `Fixes #n` in the body.
2. **Version** (subagent): tell it minor if the issue adds a `feat:`, patch otherwise, and the tag message (what the user gets).
3. **Close** (subagent): give it the issue number and what was done. It closes it by hand, since `Fixes #n` closes nothing on `develop`.
4. **Follow-up issue** (subagent): give it one task per thing left, in French. Something left on the user's side counts too, such as an `ALTER` on the dev base or the server: it goes in the follow-up issue and in the closing comment.
5. **Report** to the user in French: the commit, the tag, the closed issue, the new issue numbers, and what they must do (`git push origin develop --follow-tags`, a manual step). Never push `develop`. No wiki: it is updated only when the user asks (`tide-wiki`).
