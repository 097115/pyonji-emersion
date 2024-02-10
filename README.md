# pyonji

An easy-to-use tool to send e-mail patches.

- Opinionated: you will need to use a feature branch workflow similar to
  GitHub/GitLab. Other workflows and niche setups are out-of-scope.
- Auto-detect mail settings: instead of filling the Git config manually, you
  just need to enter your e-mail address and password the first time pyonji is
  invoked. (If you've already set up git-send-email, pyonji will pick up your
  mail settings from there.)
- No amnesia: the last version, cover letter, and other settings are saved
  on-disk. No need to manually pass `-v2` when sending a new version. Your
  cover letter is not lost if the network is flaky.

[![asciicast](https://asciinema.org/a/620880.svg)](https://asciinema.org/a/620880)

# Usage

Create a branch, commit changes, and then type `pyonji`. pyonji will ask for
your e-mail address and password the first time it's used, and will then
display an interface to submit your patches.

## Installation

Use your distribution's package manager, or:

    go install 'git.sr.ht/~emersion/pyonji@latest'

## Building

    go build

# Contributing

Send patches on the [mailing list], report bugs on the [issue tracker]. Discuss
in [#emersion on Libera Chat].

# License

GPLv3 only, see LICENSE.

Copyright © 2023 Simon Ser

[mailing list]: https://lists.sr.ht/~emersion/public-inbox
[issue tracker]: https://todo.sr.ht/~emersion/pyonji
[#emersion on Libera Chat]: ircs://irc.libera.chat/#emersion
