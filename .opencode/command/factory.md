---
description: Run the local software factory for a feature, the backlog or a rework.
agent: factory
---

Run the software factory. The user arguments are: $ARGUMENTS

If the arguments are empty, ask for a feature or a command. Supported forms:

- `/factory <feature>` starts a new job.
- `/factory next` claims the first unchecked item in `factory/backlog.md`.
- `/factory rework <job-id> <note>` sends a needs-human job back to the builder.
- `/factory approve <job-id>` explains that auto-merge is disabled and gives the
  branch and worktree for a manual merge.
