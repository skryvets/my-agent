# Decisions - the terminal reads with ergochat/readline

## Why a library at all
**Q:** Why does the terminal connector take a dependency now?
**A:** It was a feature gap, not handrolled code to delete: the plain `bufio.Scanner` had no line editing, no history and no tab completion. The library-recommendations survey ranked it below the clear wins, and the user asked for it.
**Source:** user (2026-09-21)

## Which library
**Q:** Which readline library?
**A:** [ergochat/readline](https://github.com/ergochat/readline) v0.1.3, the maintained fork of the unmaintained chzyer/readline the survey named. MIT, two small dependencies (`x/sys`, `x/text`). The API was verified against pkg.go.dev and the tagged source, not from memory.
**Source:** model (2026-09-21)

## When the library is used
**Q:** Does every input go through readline?
**A:** No. Stdin is a terminal only when `-cli` runs interactively; a pipe or a redirected file keeps the plain line reader, which is also how the tests drive the session. The choice is `os.Stat` on stdin looking for a character device.
**Source:** model (2026-09-21)

## Streaming through the editor
**Q:** The model streams its answer while readline owns the terminal. Who wins?
**A:** The answer streams through `editor.Stdout()`, which redraws the prompt and buffer around what is written. The chat happens between two `ReadLine` calls, so the terminal is in its normal mode while the answer arrives.
**Source:** model (2026-09-21)

## Ctrl-C and Ctrl-D
**Q:** What do they do now?
**A:** In raw mode Ctrl-C reaches readline as an interrupt instead of a SIGINT, and readline answers with `ErrInterrupt`. Both interrupt and end of input end the session, which is what the banner has always promised. Before, Ctrl-C cancelled the context but the loop went on waiting for a line.
**Source:** model (2026-09-21)

## History
**Q:** Where does the history live?
**A:** In memory, for the life of the process. `HistoryFile` stays empty, so nothing is persisted; a restart clears it, matching the Telegram connector's history rule.
**Source:** model (2026-09-21)

## Coverage
**Q:** The package coverage dropped below 90%. Why?
**A:** `Run`'s interactive branch needs a real terminal: the library's default interactivity check reads the process file descriptors directly, so no test can drive it. The branch is the package's wiring, like `main.go`, which the project also leaves uncovered. Everything testable is at 100%.
**Source:** model (2026-09-21)
