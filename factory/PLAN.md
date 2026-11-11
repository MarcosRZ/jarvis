# Software Factory Plan

Goal: add a local, recoverable software factory to jarvis: a task queue, a
deterministic runner, and opencode agents that implement, check and review one
job at a time. The first real use is replacing M4nhattan mock data with real
data.

## How to resume

1. Read this file before doing anything else.
2. Run `git status --short --branch` and `git log --oneline -5`.
3. Find the first unchecked checkpoint below. Continue from its **Next action**.
4. Read `factory/jobs/` only when the current checkpoint needs job state.
5. Do not start the local Home Assistant container unless the user asks.
6. Auto-merge is disabled until a later checkpoint. Jobs stop at `needs-human`.

## Decisions

- The queue is local, not GitHub issues. No `gh` dependency.
- A deterministic Python runner owns the loop. LLM agents do one stage at a
  time; the runner validates files and advances state.
- Worktrees live in `.factory/worktrees/<job-id>` inside the repo. This keeps
  opencode inside the project boundary and avoids external-directory access.
- Runtime state lives in `factory/jobs/<job-id>/job.json`. `factory/jobs/`,
  `.factory/`, and factory logs are gitignored.
- `factory/checks.sh` is the gate. A failing check stops the job before review.
- Auto-merge is off. The approver always leaves the job at `needs-human` until
  checkpoint 7 is explicitly enabled.
- Agents are model-agnostic. They inherit the current opencode model (DeepSeek
  in this setup). Per-agent model routing can be added later.
- The local Home Assistant container is for tests only. Development uses the
  real home Home Assistant; do not start the local one unasked.
- The dashboard remains the M4nhattan dashboard. The factory dashboard is a
  separate static page under `factory/`.

## Checkpoints

- [x] **CP0 - Plan and recovery baseline**
  - Create this file and link it from `AGENTS.md`.
  - Record decisions, resume protocol and verification commands.
  - Verification: `factory/PLAN.md` exists and `AGENTS.md` points to it.

- [x] **CP1 - opencode factory skeleton**
  - Add `.opencode/command/factory.md`.
  - Add the `factory` primary agent and the spec, builder, four reviewers and
    approver agents. The stage agents use `mode: all` so both the task tool and
    the headless runner can invoke them.
  - Use the opencode task tool, not Claude `SendMessage`.
  - Use `.factory/worktrees/<job-id>` and keep auto-merge disabled.
  - Add `.factory/` and `factory/jobs/` to `.gitignore`.
  - Verification: `opencode agent list` shows the new agents; a dry read of the
    command and agent prompts has no Claude-only tool names.

- [x] **CP2 - Deterministic checks and config**
  - Add `factory/checks.sh` with the real Go and dashboard checks.
  - Add `factory/config.json` with concurrency, rounds, paths and auto-merge
    policy.
  - Document the validation commands and worktree env rules in `AGENTS.md`.
  - Verification: run `factory/checks.sh` against the current checkout and it
    exits 0.

- [x] **CP3 - Local queue and runner**
  - Add `factory/queue.json` with the queue schema and an empty queue.
  - Add `factory/queue.example.json` as a runnable schema example.
  - Add `factory/runner.py`: lock, pick ready task, create worktree, run stages,
    run checks, update `job.json`, stop at `needs-human`, resume safely.
  - Add a dry-run mode that prints commands without calling a model.
  - Require the factory agents to be committed on the base branch before a real
    run, because worktrees do not include untracked files.
  - Verification: dry-run a sample task and inspect the generated state.

- [x] **CP4 - Queue dashboard**
  - Add `factory/dashboard.html` with queued and running jobs.
  - Read `factory/queue.json` and `factory/jobs/*/job.json`.
  - Verification: serve it with `python3 -m http.server` from `factory/` and
    check the queued cards render.

- [x] **CP5 - Pilot job**
  - Add one small dashboard task to the queue.
  - Run it end to end with auto-merge off.
  - Inspect spec, build, reviews and decision artifacts.
  - Verification: job reaches `needs-human` or `merged` with all artifacts and
    checks passing.

- [ ] **CP6 - M4nhattan real-data design task**
  - Add a design/ADR task to the queue: data source, Docker access model,
    contracts, failure states and security.
  - Run only the design/spec part and stop for human approval.
  - Verification: a reviewed design artifact exists before any real-data code
    task is queued.

- [ ] **CP7 - Autonomy hardening**
  - Enable concurrency only after CP5 and CP6 pass.
  - Add retries, timeouts, logs and a queue dashboard refresh.
  - Consider auto-merge only for low-risk, explicitly marked tasks.
  - Verification: a deliberate failure leaves state recoverable and a restart
    resumes the queue without duplicating work.

## Current state

- Current checkpoint: CP5 complete, CP6 queued.
- Next action: run the `real-data-design` task and inspect the design artifact.
- Pilot: `prompt-api-docs` reached `needs-human` with `APPROVE`, then was merged
  as `30a4fae`. The queue task and job are `done`.
- Auto-merge: disabled. This merge was a manual decision after the pilot.
- Queue: `factory/queue.json` has `real-data-design` queued with a dependency on
  the completed pilot; `factory/queue.example.json` remains the schema example.
- Known requirement: commit `.opencode` and `AGENTS.md` before a real job run.
  The runner stops at `needs-human` if the worktree lacks the agents.

## Verification commands

```sh
factory/checks.sh                 # Go checks + dashboard lint/build
opencode agent list               # factory agents load
python3 factory/runner.py --dry-run --queue factory/queue.example.json
python3 factory/runner.py --all   # run ready tasks until the queue is empty
```

## Log

- 2026-11-11: CP0 and CP1 completed. Added the plan, opencode command and
  agents, and gitignore entries.
- 2026-11-11: CP2 completed. Added `factory/checks.sh`,
  `factory/config.json`, and validation/worktree notes in `AGENTS.md`.
  `factory/checks.sh` passes on the current checkout.
- 2026-11-11: CP3 completed. Added the empty queue, schema example and
  `factory/runner.py`. Dry-run generates `job.json` and prints the worktree,
  agent, checks and review commands without calling a model. A real-path test
  created a worktree, stopped at `needs-human` because `.opencode` is not
  committed, and left recoverable state. The lock rejects concurrent runs.
- 2026-11-11: CP4 completed. Added `factory/dashboard.html` and served it with
  `python3 -m http.server`. A headless Brave screenshot showed one queued and
  one running card, the summary counts and the job details. The test queue was
  restored to empty.
- 2026-11-11: CP5 completed. The `prompt-api-docs` pilot reached `needs-human`
  with `APPROVE`; branch `factory/prompt-api-docs` has one docs commit
  (`f36a696`), checks passed, and all four round-1 reviews passed. Headless
  fixes: builder/reviewers run `factory/checks.sh` from the worktree, and the
  approver is told not to read `factory/logs` or the main checkout because
  external-directory permissions auto-reject it.
- 2026-11-11: Pilot merged as `30a4fae`; queue and job are `done`. Added
  `--all` to run ready tasks until the queue is empty. CP6 is queued as
  `real-data-design`, depending on the pilot.
