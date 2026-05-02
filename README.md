# Xpass

A GUI for the [standard unix password manager](https://www.passwordstore.org/).

Made with [Go](https://go.dev) and the [Gio](https://gioui.org) UI toolkit.

Planned as a part of ADE (yet a mythical desktop environment).

## Features

- Fast search (by filter) and navigation in your password database.
- Copy passwords and other fields into the clipboard.
- Clipboard is automatically cleared after 60 seconds (customizable).
- Support markdown formatting for the fields.
- Per-field masking in the formatted view: prefix a metadata key with `.` to
  hide its value (e.g. `.pin: 1234`).
- Keep state between runs for selecting fields of the same record.

![screenshot](/docs/sshot1.png)

### Keyboard shortcuts

Shortcuts apply globally unless you are in **edit mode** (where arrows and Escape
behave like a normal editor).

| Shortcut       | Action                                                                                                                 |
|----------------|------------------------------------------------------------------------------------------------------------------------|
| **Escape**     | Quit the app (saves last selection to cache). In edit mode: cancel editing (or abandon a new card — see below).        |
| **↑** / **↓**  | Move selection in the list.                                                                                            |
| **Ctrl+Enter** | Toggle **Formatted view** ↔ **Not Masked Source**. If the “add new record” row is visible, also creates the new entry. |
| **Ctrl+C**     | In **Not Masked Source** only: copy the current text **selection** to the clipboard (starts the same clear timer).     |
| **Ctrl+L**     | Copy the first matching field among `login`, `user`, `username` (case-insensitive keys).                               |
| **Ctrl+E**     | Copy the first matching field among `email`, `mail`, `e-mail`.                                                         |
| **Ctrl+O**     | Open a URL: looks for `url` or `link`, otherwise the first metadata value containing `://`. Uses `xdg-open`.           |
| **Ctrl+M**     | Open **edit mode** for the selected entry (must be decrypted first).                                                   |
| **Ctrl+R**     | After a failed decrypt (“Wrong key”), retry decryption for the selected entry.                                         |

### Configuration (environment)

Compatible with common `pass` variables used by this app:

| Variable                   | Role                                                                                       |
|----------------------------|--------------------------------------------------------------------------------------------|
| `PASSWORD_STORE_DIR`       | Password store root (default `~/.password-store`).                                         |
| `PASSWORD_STORE_KEY`       | Optional GPG key id for encryption when saving (otherwise recipients come from `.gpg-id`). |
| `PASSWORD_STORE_CLIP_TIME` | Seconds until the clipboard is cleared after a copy (default `60`).                        |

## Status

Usable but still in development.
