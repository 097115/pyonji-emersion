# pyonji

An easy-to-use tool to send e-mail patches.

- Opinionated: you will need to use a feature branch workflow similar to
  GitHub/GitLab. Other workflows and niche setups are out-of-scope.
- Auto-detect mail settings: instead of filling the Git config manually, you
  just need to enter your e-mail address and password the first time pyonji is
  invoked.
- No amnesia: the last version, cover letter, and other settings are saved
  on-disk. No need to manually pass `-v2` when sending a new version. Your
  cover letter is not lost if the network is flaky.
