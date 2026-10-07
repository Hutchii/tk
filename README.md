# tk

A small terminal planner for people who work across many repos. Tasks are markdown files, the app is one Go binary, and the read commands have JSON output so a coding agent can read today's plan and pick up the work.

```
 tk   1 calendar  2 board                                                                    tue 06 oct
BACKLOG 2                 TODAY tue 06 2            wed 07 1                  thu 08 1
────────────────────────  ────────────────────────  ────────────────────────  ────────────────────────
blog          due 15 oct  events                    acme-web                  bookings
  Seo audit               from sun 04 · due 05 oct    Pricing page copy         Export csv
bookings                    Old slipped task
  Backlog idea            acme-web      due 07 oct
                            Fix login redirect lo…
                          ── done ────────────────
                          acme-web
                          ✓ Deploy env vars
```

## Install

```
go install github.com/hutchii/tk@latest
```

Needs Go 1.27 or newer. Run `tk` to open the app, or `tk help` for the commands.

## How it works

Two views:

- **Calendar** (default): a backlog column, then today and the next days across all projects. Past days show what got done.
- **Board**: to do and done for one project or all of them. Done is collapsed until you press `d`.

Where a task shows up:

- Planned for a day and not done: on that day.
- Planned for a past day and not done: on today, marked `from <day>`.
- Done: on the day it was finished, whatever day it was planned for.
- No planned day: in the backlog.
- A deadline is separate from the planned day and shows as `due <date>`. It turns yellow two days before and red once it has passed.

## Keys

| Key | Action |
| --- | --- |
| `a` | add: `title @project ^day !deadline` |
| `x` / space | toggle done |
| `p` / `u` | set planned day / deadline |
| `[` `]` | move a task a day back or forward |
| `e` | edit the description in `$VISUAL`, `$EDITOR` or nvim |
| enter | read the rendered description |
| `r` / `m` / `X` | rename / move to project / delete |
| arrows / `h` `j` `k` `l` | move; left of the first day and right of the last scroll a day |
| `,` `.` / `g` / `b` | scroll a week / back to today / backlog |
| `1` `2` tab | calendar / board |
| `?` / `q` | all keys / quit |

Days can be typed as `t`, `m` (tomorrow), `+3`, `-1`, `fri`, `2026-10-12` or `none`.

## CLI

```
tk today [--json]
tk day fri [--json]
tk list [-p project] [--done|--all] [--json]
tk show <id> [--json]
tk add <title> -p <project> [--plan <day>] [--deadline <day>] [--desc <text> | --desc -]
tk plan <id> <day|none>
tk deadline <id> <day|none>
tk done <id> [--on <day>]
tk undo <id>
tk edit <id> [--title t] [-p project] [--desc <text> | --desc -]
tk rm <id>
tk projects [--json]
```

`--desc -` reads the description from stdin. JSON output includes each task's repo path and file, which is what an agent needs to start work.

## Posts

`tk post` opens the same app on a social post calendar in `$TK_SOCIAL_DIR` (default `~/social`), and `tk post <command>` runs any command above there. A post is a task with a platform: planned is the day it goes out, done is the day it was published.

```
tk post add "Replika case study" -p codapi --platform linkedin --status approved --plan fri --desc -
tk post done <id> --url https://www.linkedin.com/posts/... [--on <day>]
tk post list -p codapi --all
```

Platforms: linkedin, instagram, facebook, tiktok. Status: draft or approved; published is `done_at`. The post text comes first in the body; notes go under a `##` heading after it. Images sit beside the posts in `~/social/<project>/media/<id>/` and `brand/`, which tk does not read.

## Data

One file per task in `$TK_DIR` (default `~/tasks`):

```
~/tasks/acme-web/1fxu98-fix-login-redirect.md
```

```
---
id: 1fxu98
title: Fix login redirect
project: acme-web
planned: "2026-10-07"
deadline: "2026-10-10"
created: "2026-10-06"
---
Any markdown: steps, links, acceptance checks.
```

`done_at` appears once the task is done. Dates are local calendar days. Extra frontmatter keys you add by hand are kept. The folder works as an Obsidian vault if you don't rename the files and keep dates as plain dates (a time on a date drops the task off the calendar).

A project's repo is `~/dev/<project>` when that folder exists. For anything else, map it in `~/tasks/projects.yaml`:

```
bookings: ~/dev/next-bookings
```

Writers take a lock on the folder and re-read a task before saving it, so the app, the CLI and an agent can all edit at the same time.
