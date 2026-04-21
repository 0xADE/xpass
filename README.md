# Xpass

A GUI for the [standard unix password manager](https://www.passwordstore.org/).

Made with [Go](https://go.dev) and the [Gio](https://gioui.org) UI toolkit.

## Features

- Fast search (by filter) and navigation of your password database.
- Copy passwords to the clipboard.
- Clipboard is automatically cleared after 60 seconds (customizable).
- Support markdown formatting in the password data.
- Per-field masking in the formatted view: prefix a metadata key with `.` to
  hide its value (e.g. `.pin: 1234`). Keys without a leading dot are shown
  unmasked. The first line of each entry (the password) is always masked in
  formatted view. The "Not Masked Source" view shows everything verbatim.

## Status

Usable but still in development.

