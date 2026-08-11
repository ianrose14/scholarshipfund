#!/usr/bin/env python3
"""Parse cadence.md and open GitHub issues for today's tasks."""

import os
import re
import subprocess
from datetime import datetime

CADENCE_FILE = os.path.join(os.path.dirname(__file__), "..", "cadence.md")


def parse_cadence(path: str) -> list[dict]:
    """Return list of task dicts parsed from cadence.md."""
    tasks = []
    current: dict | None = None

    with open(path) as f:
        for line in f:
            line = line.rstrip()
            # Top-level bullet = task title
            if re.match(r"^\* .+", line):
                if current:
                    tasks.append(current)
                current = {"title": line[2:].strip()}
            # Sub-bullet fields
            elif current and re.match(r"^\s+\* (When|What|Who): .+", line):
                m = re.match(r"^\s+\* (When|What|Who): (.+)", line)
                if m:
                    current[m.group(1).lower()] = m.group(2).strip()

    if current:
        tasks.append(current)

    return tasks


def tasks_for_today(tasks: list[dict]) -> list[dict]:
    today = datetime.now()
    matches = []
    for task in tasks:
        when = task.get("when", "")
        try:
            task_date = datetime.strptime(f"{when} {today.year}", "%B %d %Y")
            if task_date.month == today.month and task_date.day == today.day:
                matches.append(task)
        except ValueError:
            pass

    return matches


def build_body(task: dict) -> str:
    lines = []
    if "what" in task:
        lines.append(f"**What:** {task['what']}")
    if "who" in task:
        lines.append(f"**Who:** {task['who']}")
    lines.append("")
    lines.append("[View full cadence](https://github.com/ianrose14/scholarshipfund/blob/main/cadence.md)")
    return "\n".join(lines)


def create_issue(title: str, body: str) -> None:
    subprocess.run(
        [
            "gh", "issue", "create",
            "--title", title,
            "--body", body,
            "--label", "cadence",
        ],
        check=True,
    )


def main():
    tasks = parse_cadence(CADENCE_FILE)
    todays = tasks_for_today(tasks)

    if not todays:
        print("No cadence items for today.")
        return

    print(f"Found {len(todays)} item(s) for today — creating issues.")

    for task in todays:
        create_issue(task["title"], build_body(task))

    print("Issue(s) created.")


if __name__ == "__main__":
    main()
