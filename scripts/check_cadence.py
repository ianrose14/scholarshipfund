#!/usr/bin/env python3
"""Parse cadence.md and send email reminders for today's tasks."""

import os
import re
import sys
from datetime import datetime

import resend

resend.api_key = os.environ["RESEND_API_KEY"]

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


def build_html(tasks: list[dict]) -> str:
    items = ""
    for t in tasks:
        items += f"<li><strong>{t['title']}</strong><br>"
        if "what" in t:
            items += f"What: {t['what']}<br>"
        if "who" in t:
            items += f"Who: {t['who']}"
        items += "</li>\n"
    return f"""
<h2>Scholarship Fund Reminders for Today</h2>
<ul>
{items}
</ul>
<p><a href="https://github.com/ianrose14/scholarshipfund/blob/main/cadence.md">View full cadence</a></p>
"""


def main():
    tasks = parse_cadence(CADENCE_FILE)
    todays = tasks_for_today(tasks)

    if not todays:
        print("No cadence items for today.")
        return

    print(f"Found {len(todays)} item(s) for today — sending email.")

    html = build_html(todays)
    subject = f"Scholarship Fund Reminder — {datetime.now().strftime('%B %-d')}"

    resend.Emails.send({
        "from": "ianrose@allisonrosememorialfund.org",
        "to": "ianrose14@gmail.com",
        "subject": subject,
        "html": html,
    })

    print("Email sent.")


if __name__ == "__main__":
    main()
