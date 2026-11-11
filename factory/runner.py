#!/usr/bin/env python3
import argparse
import fcntl
import json
import os
import re
import shlex
import shutil
import subprocess
import sys
import tempfile
from datetime import datetime, timezone
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DEFAULT_CONFIG = ROOT / "factory" / "config.json"
ID_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]*$")
TERMINAL = {"needs-human", "done", "failed", "cancelled"}
TASK_STATUSES = {"queued", "running", "needs-human", "done", "failed", "cancelled"}
AGENTS = (
    "factory-spec-writer",
    "factory-builder",
    "factory-reviewer-security",
    "factory-reviewer-ux",
    "factory-reviewer-ui",
    "factory-reviewer-code",
    "factory-approver",
)


class RunnerError(RuntimeError):
    pass


def utc_now():
    return datetime.now(timezone.utc).isoformat(timespec="seconds")


def read_json(path):
    try:
        return json.loads(Path(path).read_text(encoding="utf-8"))
    except FileNotFoundError as exc:
        raise RunnerError(f"missing file: {path}") from exc
    except json.JSONDecodeError as exc:
        raise RunnerError(f"invalid JSON in {path}: {exc}") from exc


def write_json(path, data):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, tmp_name = tempfile.mkstemp(prefix=path.name + ".", dir=path.parent)
    tmp = Path(tmp_name)
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as handle:
            json.dump(data, handle, indent=2)
            handle.write("\n")
        os.replace(tmp, path)
    except Exception:
        tmp.unlink(missing_ok=True)
        raise


def resolve_path(value):
    path = Path(value)
    if not path.is_absolute():
        path = ROOT / path
    return path.resolve()


def run(cmd, cwd=ROOT, timeout=None, check=True):
    try:
        proc = subprocess.run(
            cmd,
            cwd=cwd,
            text=True,
            capture_output=True,
            timeout=timeout,
        )
    except FileNotFoundError as exc:
        raise RunnerError(f"command not found: {cmd[0]}") from exc
    except OSError as exc:
        raise RunnerError(f"cannot run {cmd[0]}: {exc}") from exc
    except subprocess.TimeoutExpired as exc:
        raise RunnerError(f"command timed out after {timeout}s: {' '.join(cmd)}") from exc
    if check and proc.returncode != 0:
        detail = (proc.stderr or proc.stdout or "").strip()
        raise RunnerError(f"command failed ({proc.returncode}): {' '.join(cmd)}\n{detail}")
    return proc


def git(args, cwd=ROOT, check=True):
    return run(["git", *args], cwd=cwd, check=check)


def load_config(path):
    config = read_json(path)
    required = (
        "concurrency",
        "max_rounds",
        "max_resumes",
        "base_branch",
        "auto_merge",
        "queue_file",
        "jobs_dir",
        "worktrees_dir",
        "logs_dir",
        "lock_file",
        "checks",
        "checks_timeout_seconds",
        "agent_timeout_seconds",
        "reviewers",
        "agents",
    )
    missing = [key for key in required if key not in config]
    if missing:
        raise RunnerError(f"config is missing: {', '.join(missing)}")
    if config["concurrency"] != 1:
        raise RunnerError("concurrency must be 1 until CP7")
    if config["auto_merge"]:
        raise RunnerError("auto_merge is not implemented; keep it false")
    if int(config["max_rounds"]) < 1:
        raise RunnerError("max_rounds must be at least 1")
    if not isinstance(config["reviewers"], dict) or not config["reviewers"]:
        raise RunnerError("reviewers must be a non-empty object")
    for lens in ("security", "ux", "ui", "code"):
        if lens not in config["reviewers"]:
            raise RunnerError(f"reviewers is missing {lens}")
    if not isinstance(config["agents"], dict):
        raise RunnerError("agents must be an object")
    for role in ("spec", "builder", "approver"):
        if role not in config["agents"]:
            raise RunnerError(f"agents is missing {role}")
    return config


def validate_task(task):
    if not isinstance(task, dict):
        raise RunnerError("every task must be an object")
    task_id = task.get("id")
    if not isinstance(task_id, str) or not ID_RE.match(task_id):
        raise RunnerError(f"invalid task id: {task_id!r}")
    for key in ("title", "request"):
        if not isinstance(task.get(key), str) or not task[key].strip():
            raise RunnerError(f"task {task_id} is missing {key}")
    status = task.get("status", "queued")
    if status not in TASK_STATUSES:
        raise RunnerError(f"task {task_id} has invalid status: {status}")
    if not isinstance(task.get("priority", 0), int):
        raise RunnerError(f"task {task_id} has a non-integer priority")
    depends_on = task.get("depends_on", [])
    if not isinstance(depends_on, list) or not all(
        isinstance(dep, str) for dep in depends_on
    ):
        raise RunnerError(f"task {task_id} has invalid depends_on")
    if not isinstance(task.get("attempts", 0), int):
        raise RunnerError(f"task {task_id} has invalid attempts")


def load_queue(path):
    queue = read_json(path)
    if not isinstance(queue, dict) or queue.get("version") != 1:
        raise RunnerError("queue must be an object with version 1")
    if not isinstance(queue.get("tasks"), list):
        raise RunnerError("queue tasks must be an array")
    seen = set()
    for task in queue["tasks"]:
        validate_task(task)
        if task["id"] in seen:
            raise RunnerError(f"duplicate task id: {task['id']}")
        seen.add(task["id"])
    return queue


def save_queue(path, queue):
    queue["updated_at"] = utc_now()
    write_json(path, queue)


def set_task(path, queue, task, status, **fields):
    task["status"] = status
    task.update(fields)
    save_queue(path, queue)


def select_task(queue, task_id):
    tasks = queue["tasks"]
    if task_id:
        matches = [task for task in tasks if task["id"] == task_id]
        if not matches:
            raise RunnerError(f"task not found: {task_id}")
        return matches[0]
    running = [task for task in tasks if task.get("status") == "running"]
    if len(running) > 1:
        raise RunnerError("multiple running tasks; fix the queue first")
    if running:
        return running[0]
    done = {task["id"] for task in tasks if task.get("status") == "done"}
    ready = [
        task
        for task in tasks
        if task.get("status", "queued") == "queued"
        and all(dep in done for dep in task.get("depends_on", []))
    ]
    if not ready:
        return None
    ready.sort(key=lambda task: (-task.get("priority", 0), task.get("created_at", ""), task["id"]))
    return ready[0]


def job_paths(task, config):
    job_id = task["id"]
    job_dir = resolve_path(config["jobs_dir"]) / job_id
    worktree = resolve_path(config["worktrees_dir"]) / job_id
    branch = f"factory/{job_id}"
    if not job_dir.is_relative_to(ROOT):
        raise RunnerError("jobs_dir must be inside the repository")
    if not worktree.is_relative_to(ROOT):
        raise RunnerError("worktrees_dir must be inside the repository")
    return job_dir, branch, worktree


def save_job(job_dir, job):
    if job.get("_no_write"):
        return
    job["updated_at"] = utc_now()
    write_json(job_dir / "job.json", job)


def load_or_create_job(task, config, dry_run):
    job_dir, branch, worktree = job_paths(task, config)
    job_file = job_dir / "job.json"
    real_job = False
    if job_file.exists():
        job = read_json(job_file)
        if job.get("id") != task["id"]:
            raise RunnerError(f"job id mismatch in {job_file}")
        real_job = not bool(job.get("dry_run"))
        if job.get("dry_run"):
            job["status"] = "new"
            job["dry_run"] = False
            job.pop("commands", None)
    else:
        job = {
            "id": task["id"],
            "title": task["title"],
            "request": task["request"],
            "branch": branch,
            "worktree": str(worktree.relative_to(ROOT)),
            "status": "new",
            "round": 1,
            "attempts": 0,
            "created_at": utc_now(),
            "updated_at": utc_now(),
            "sessions": {},
            "reviews": {},
            "checks": None,
            "decision": None,
            "last_error": None,
            "dry_run": False,
        }
    job.setdefault("branch", branch)
    job.setdefault("worktree", str(worktree.relative_to(ROOT)))
    if not isinstance(job.get("sessions"), dict):
        job["sessions"] = {}
    if not isinstance(job.get("reviews"), dict):
        job["reviews"] = {}
    if not dry_run:
        job["attempts"] = int(job.get("attempts", 0)) + 1
    if not (dry_run and real_job):
        save_job(job_dir, job)
    return job, job_dir, branch, worktree, real_job


def ensure_worktree(job, config, dry_run, commands):
    worktree = resolve_path(job["worktree"])
    if not worktree.is_relative_to(ROOT):
        raise RunnerError("worktree must be inside the repository")
    branch = job["branch"]
    base = config["base_branch"]
    if worktree.exists():
        if not (worktree / ".git").exists():
            raise RunnerError(f"worktree path exists but is not a worktree: {worktree}")
        current = git(["rev-parse", "--abbrev-ref", "HEAD"], cwd=worktree).stdout.strip()
        if current != branch:
            raise RunnerError(f"worktree is on {current}, expected {branch}")
        return worktree
    if dry_run:
        commands.append(
            {
                "stage": "worktree",
                "command": ["git", "worktree", "add", "-b", branch, str(worktree), base],
            }
        )
        return worktree
    if config.get("require_clean_main", True):
        status = git(["status", "--porcelain", "--untracked-files=no"]).stdout.strip()
        if status:
            raise RunnerError("main checkout has tracked changes; commit or stash them first")
    if git(["rev-parse", "--verify", base], check=False).returncode != 0:
        raise RunnerError(f"base branch does not exist: {base}")
    worktree.parent.mkdir(parents=True, exist_ok=True)
    branch_exists = (
        git(["show-ref", "--verify", "--quiet", f"refs/heads/{branch}"], check=False).returncode
        == 0
    )
    if branch_exists:
        git(["worktree", "add", str(worktree), branch])
    else:
        git(["worktree", "add", "-b", branch, str(worktree), base])
    return worktree


def ensure_agents(worktree, config, dry_run):
    if dry_run:
        return
    names = set(AGENTS)
    names.update(config["agents"].values())
    names.update(config["reviewers"].values())
    missing = [
        name
        for name in sorted(names)
        if not (worktree / ".opencode" / "agent" / f"{name}.md").exists()
    ]
    if missing:
        raise RunnerError(
            "factory agents are not in the worktree; commit .opencode on the base branch: "
            + ", ".join(missing)
        )


def opencode_command(agent, worktree, prompt, session=None):
    command = [
        "opencode",
        "run",
        "--format",
        "json",
        "--agent",
        agent,
        "--dir",
        str(worktree),
    ]
    if session:
        command.extend(["--session", session])
    command.append(prompt)
    return command


def parse_session_id(stdout):
    for line in stdout.splitlines():
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        session_id = event.get("sessionID")
        if isinstance(session_id, str) and session_id:
            return session_id
    return None


def parse_agent_error(stdout):
    for line in stdout.splitlines():
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        if event.get("type") == "error":
            return event.get("error") or event
        part = event.get("part")
        if isinstance(part, dict) and part.get("type") == "error":
            return part
    return None


def run_agent(agent, job, job_dir, worktree, stage, prompt, config, dry_run, commands):
    sessions = job.setdefault("sessions", {})
    command = opencode_command(agent, worktree, prompt, sessions.get(stage))
    if dry_run:
        commands.append({"stage": stage, "agent": agent, "command": command})
        return
    logs_dir = resolve_path(config["logs_dir"]) / job["id"]
    logs_dir.mkdir(parents=True, exist_ok=True)
    stamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    log_path = logs_dir / f"{stage}-{stamp}.log"
    proc = run(
        command,
        cwd=ROOT,
        timeout=int(config["agent_timeout_seconds"]),
        check=False,
    )
    log_path.write_text(
        f"$ {' '.join(shlex.quote(part) for part in command)}\n\n"
        f"{proc.stdout}\n{proc.stderr}",
        encoding="utf-8",
    )
    agent_error = parse_agent_error(proc.stdout)
    fallback = "Falling back to default agent" in proc.stderr
    if proc.returncode != 0 or agent_error or fallback:
        detail = agent_error or (proc.stderr or proc.stdout).strip()
        raise RunnerError(
            f"{stage} failed; see {log_path.relative_to(ROOT)}\n{detail}"
        )
    session_id = parse_session_id(proc.stdout)
    if session_id:
        sessions[stage] = session_id
        save_job(job_dir, job)


def context(job, job_dir, worktree):
    return (
        f"Job folder: {job_dir}\n"
        f"Worktree: {worktree}\n"
        f"Branch: {job['branch']}\n"
    )


def stage_spec(job, job_dir, worktree, config, dry_run, commands):
    spec = job_dir / "spec.md"
    if spec.exists() and spec.stat().st_size > 0:
        return
    job["status"] = "spec"
    save_job(job_dir, job)
    prompt = (
        f"You are the spec writer for factory job {job['id']}.\n\n"
        f"{context(job, job_dir, worktree)}\n"
        f"Request:\n{job['request']}\n\n"
        f"Read AGENTS.md in the worktree. Write {spec}. Do not edit code. "
        "Do not commit. Reply with the spec path."
    )
    run_agent(
        config["agents"]["spec"],
        job,
        job_dir,
        worktree,
        "spec",
        prompt,
        config,
        dry_run,
        commands,
    )
    if not dry_run and (not spec.exists() or spec.stat().st_size == 0):
        raise RunnerError("spec writer did not write spec.md")


def commits_ahead(worktree, base, dry_run=False):
    if dry_run:
        return 1
    output = git(["rev-list", "--count", f"{base}..HEAD"], cwd=worktree).stdout.strip()
    return int(output)


def stage_build(job, job_dir, worktree, config, dry_run, commands, round_no):
    build = job_dir / "build.md"
    rework = job_dir / f"rework-{round_no}.md"
    if round_no == 1:
        if (
            build.exists()
            and build.stat().st_size > 0
            and commits_ahead(worktree, config["base_branch"], dry_run) > 0
        ):
            return
        prompt = (
            f"You are the builder for factory job {job['id']}.\n\n"
            f"{context(job, job_dir, worktree)}\n"
            f"Read {job_dir / 'spec.md'} and, if it exists, {build}. "
            "Implement the spec in the worktree. Run the relevant checks. "
            "Commit your code. Write build.md with what changed and the checks "
            "you ran. Do not merge."
        )
    else:
        prompt = (
            f"You are the builder for factory job {job['id']}, rework round {round_no}.\n\n"
            f"{context(job, job_dir, worktree)}\n"
            f"Read {job_dir / 'spec.md'}, {build}, and every review in "
            f"{job_dir / f'round-{round_no - 1}'}. Fix all findings. Run the "
            f"relevant checks. Commit the fix. Write {rework} with what changed "
            "and the checks you ran. Do not merge."
        )
    job["status"] = "build"
    job["round"] = round_no
    save_job(job_dir, job)
    run_agent(
        config["agents"]["builder"],
        job,
        job_dir,
        worktree,
        "build",
        prompt,
        config,
        dry_run,
        commands,
    )
    if dry_run:
        return
    if not build.exists() or build.stat().st_size == 0:
        raise RunnerError("builder did not write build.md")
    if round_no > 1 and (not rework.exists() or rework.stat().st_size == 0):
        raise RunnerError(f"builder did not write {rework.name}")
    if commits_ahead(worktree, config["base_branch"]) == 0:
        raise RunnerError("builder did not commit any changes")


def stage_checks(job, job_dir, worktree, config, dry_run, commands, round_no):
    checks = resolve_path(config["checks"])
    command = [str(checks), str(worktree)]
    if dry_run:
        commands.append({"stage": f"checks-r{round_no}", "command": command})
        return
    job["status"] = "checks"
    save_job(job_dir, job)
    logs_dir = resolve_path(config["logs_dir"]) / job["id"]
    logs_dir.mkdir(parents=True, exist_ok=True)
    stamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    log_path = logs_dir / f"checks-r{round_no}-{stamp}.log"
    try:
        proc = run(
            command,
            cwd=ROOT,
            timeout=int(config["checks_timeout_seconds"]),
            check=False,
        )
    except RunnerError as exc:
        job["checks"] = {
            "ok": False,
            "log": str(log_path.relative_to(ROOT)),
            "error": str(exc),
            "at": utc_now(),
        }
        save_job(job_dir, job)
        raise
    log_path.write_text(
        f"$ {' '.join(shlex.quote(part) for part in command)}\n\n"
        f"{proc.stdout}\n{proc.stderr}",
        encoding="utf-8",
    )
    ok = proc.returncode == 0
    job["checks"] = {
        "ok": ok,
        "log": str(log_path.relative_to(ROOT)),
        "at": utc_now(),
    }
    save_job(job_dir, job)
    if not ok:
        raise RunnerError(f"checks failed; see {log_path.relative_to(ROOT)}")


def review_verdict(path):
    if not path.exists():
        return None
    lines = path.read_text(encoding="utf-8").splitlines()
    if not lines:
        return None
    first = lines[0].strip()
    if first == "VERDICT: PASS":
        return "PASS"
    if first == "VERDICT: CHANGES":
        return "CHANGES"
    return None


def stage_review(job, job_dir, worktree, config, dry_run, commands, round_no):
    round_dir = job_dir / f"round-{round_no}"
    if not dry_run:
        round_dir.mkdir(parents=True, exist_ok=True)
    verdicts = {}
    for lens, agent in config["reviewers"].items():
        review = round_dir / f"review-{lens}.md"
        verdict = review_verdict(review)
        if verdict is None:
            job["status"] = "review"
            save_job(job_dir, job)
            prompt = (
                f"You are the {lens} reviewer for factory job {job['id']}, "
                f"round {round_no}.\n\n"
                f"{context(job, job_dir, worktree)}\n"
                f"Read {job_dir / 'spec.md'}, {job_dir / 'build.md'}, and the "
                f"branch diff. Write {review}. Do not edit code."
            )
            run_agent(
                agent,
                job,
                job_dir,
                worktree,
                f"review-{lens}-r{round_no}",
                prompt,
                config,
                dry_run,
                commands,
            )
            verdict = review_verdict(review) if not dry_run else "DRY-RUN"
            if not dry_run and verdict is None:
                raise RunnerError(f"{agent} did not write a valid {review.name}")
        verdicts[lens] = verdict
    if dry_run:
        return "dry-run"
    job.setdefault("reviews", {})[str(round_no)] = verdicts
    save_job(job_dir, job)
    if all(verdict == "PASS" for verdict in verdicts.values()):
        return "pass"
    if any(verdict == "CHANGES" for verdict in verdicts.values()):
        return "changes"
    raise RunnerError(f"invalid review verdicts in round {round_no}: {verdicts}")


def decision_verdict(path):
    if not path.exists():
        return None
    lines = path.read_text(encoding="utf-8").splitlines()
    if not lines:
        return None
    first = lines[0].strip()
    if first == "APPROVE":
        return "APPROVE"
    if first == "ESCALATE":
        return "ESCALATE"
    return None


def stage_approve(job, job_dir, worktree, config, dry_run, commands, round_no):
    decision = job_dir / "decision.md"
    verdict = decision_verdict(decision)
    if verdict is None:
        job["status"] = "approve"
        save_job(job_dir, job)
        prompt = (
            f"You are the approver for factory job {job['id']}, final round "
            f"{round_no}.\n\n"
            f"{context(job, job_dir, worktree)}\n"
            f"Read all artifacts in the job folder. Write {decision}."
        )
        run_agent(
            config["agents"]["approver"],
            job,
            job_dir,
            worktree,
            "approver",
            prompt,
            config,
            dry_run,
            commands,
        )
        if dry_run:
            return
        verdict = decision_verdict(decision)
        if verdict is None:
            raise RunnerError("approver did not write a valid decision.md")
    if dry_run:
        return
    job["decision"] = {"verdict": verdict, "at": utc_now()}
    job["status"] = "needs-human"
    save_job(job_dir, job)


def save_dry_run(job, job_dir, commands, real_job):
    data = {
        "id": job["id"],
        "status": "dry-run",
        "dry_run": True,
        "generated_at": utc_now(),
        "commands": commands,
    }
    if real_job:
        write_json(job_dir / "dry-run.json", data)
        return job_dir / "dry-run.json"
    job.update(data)
    save_job(job_dir, job)
    return job_dir / "job.json"


def run_job(task, queue, queue_path, config, dry_run, commands):
    job, job_dir, _branch, worktree, real_job = load_or_create_job(task, config, dry_run)
    if dry_run and real_job:
        job["_no_write"] = True
    if not dry_run and job.get("status") in TERMINAL:
        if task.get("status") != job["status"]:
            set_task(queue_path, queue, task, job["status"])
        print(f"job {job['id']}: already {job['status']} ({job_dir})")
        return job, job_dir
    worktree = ensure_worktree(job, config, dry_run, commands)
    ensure_agents(worktree, config, dry_run)
    stage_spec(job, job_dir, worktree, config, dry_run, commands)
    round_no = max(1, int(job.get("round", 1)))
    if round_no > int(config["max_rounds"]):
        raise RunnerError(f"job round {round_no} exceeds max_rounds")
    if round_no == 1 or not (job_dir / f"rework-{round_no}.md").exists():
        stage_build(job, job_dir, worktree, config, dry_run, commands, round_no)
    stage_checks(job, job_dir, worktree, config, dry_run, commands, round_no)
    if dry_run:
        result = stage_review(job, job_dir, worktree, config, dry_run, commands, round_no)
        if result != "changes":
            stage_approve(job, job_dir, worktree, config, dry_run, commands, round_no)
        path = save_dry_run(job, job_dir, commands, real_job)
        print(f"dry-run job {job['id']}: {path}")
        for entry in commands:
            print(f"# {entry['stage']}")
            print(" ".join(shlex.quote(part) for part in entry["command"]))
        return job, job_dir
    while True:
        result = stage_review(job, job_dir, worktree, config, dry_run, commands, round_no)
        if result == "pass":
            break
        if round_no >= int(config["max_rounds"]):
            raise RunnerError(f"reviews still request changes after {round_no} rounds")
        round_no += 1
        job["round"] = round_no
        save_job(job_dir, job)
        stage_build(job, job_dir, worktree, config, dry_run, commands, round_no)
        stage_checks(job, job_dir, worktree, config, dry_run, commands, round_no)
    stage_approve(job, job_dir, worktree, config, dry_run, commands, round_no)
    job["status"] = "needs-human"
    job["last_error"] = None
    save_job(job_dir, job)
    set_task(queue_path, queue, task, "needs-human", finished_at=utc_now(), last_error=None)
    print(f"job {job['id']}: needs-human ({job_dir})")
    return job, job_dir


def mark_needs_human(queue, queue_path, task, job, job_dir, message):
    job["status"] = "needs-human"
    job["last_error"] = message
    save_job(job_dir, job)
    set_task(
        queue_path,
        queue,
        task,
        "needs-human",
        finished_at=utc_now(),
        last_error=message,
    )


def acquire_lock(path):
    path.parent.mkdir(parents=True, exist_ok=True)
    try:
        handle = path.open("w", encoding="utf-8")
        fcntl.flock(handle, fcntl.LOCK_EX | fcntl.LOCK_NB)
    except BlockingIOError as exc:
        handle.close()
        raise RunnerError("another runner is active") from exc
    except OSError as exc:
        raise RunnerError(f"cannot open runner lock: {exc}") from exc
    return handle


def main(argv=None):
    parser = argparse.ArgumentParser(description="Run one jarvis factory job.")
    parser.add_argument("--config", default=str(DEFAULT_CONFIG))
    parser.add_argument("--queue", help="queue file to use instead of the configured one")
    parser.add_argument("--task", help="task id to run instead of the next ready task")
    parser.add_argument("--dry-run", action="store_true", help="write state and print commands")
    args = parser.parse_args(argv)

    lock = None
    config = None
    queue = None
    queue_path = None
    task = None
    commands = []
    try:
        config_path = Path(args.config)
        if not config_path.is_absolute():
            config_path = ROOT / config_path
        config = load_config(config_path)
        lock = acquire_lock(resolve_path(config["lock_file"]))
        queue_path = Path(args.queue).resolve() if args.queue else resolve_path(config["queue_file"])
        queue = load_queue(queue_path)
        task = select_task(queue, args.task)
        if task is None:
            print("no ready tasks")
            return 0
        if not args.dry_run:
            if not shutil.which("opencode"):
                raise RunnerError("opencode is not installed")
            if task.get("status") not in {"queued", "running"}:
                raise RunnerError(f"task {task['id']} is {task.get('status')}")
            done = {item["id"] for item in queue["tasks"] if item.get("status") == "done"}
            missing = [dep for dep in task.get("depends_on", []) if dep not in done]
            if missing:
                raise RunnerError(f"task dependencies are not done: {', '.join(missing)}")
            set_task(
                queue_path,
                queue,
                task,
                "running",
                started_at=utc_now(),
                finished_at=None,
                last_error=None,
                attempts=int(task.get("attempts", 0)) + 1,
            )
        run_job(task, queue, queue_path, config, args.dry_run, commands)
        return 0
    except RunnerError as exc:
        if not args.dry_run and config is not None and task is not None and queue is not None:
            candidate = resolve_path(config["jobs_dir"]) / task["id"] / "job.json"
            if candidate.exists():
                try:
                    job = read_json(candidate)
                    mark_needs_human(
                        queue,
                        queue_path,
                        task,
                        job,
                        candidate.parent,
                        str(exc),
                    )
                except RunnerError:
                    pass
        print(f"runner: {exc}", file=sys.stderr)
        return 1
    finally:
        if lock is not None:
            fcntl.flock(lock, fcntl.LOCK_UN)
            lock.close()


if __name__ == "__main__":
    sys.exit(main())
