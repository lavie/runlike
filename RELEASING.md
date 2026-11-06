# Releasing runlike

A release is a pull request that adds a file `release-notes/vX.Y.Z.md`.
When the maintainer merges that pull request, the release is approved. The
workflow [.github/workflows/release.yml](.github/workflows/release.yml) then
publishes it. No other step and no second approval is needed. A pull request
that adds no new notes file releases nothing. AI agents and people use the
same process. No AI runs in GitHub Actions.

## Pick the version

runlike uses [semantic versioning](https://semver.org). The version counts
**runlike's own command line and library API**, not the exact text it
prints. Output may change in any release when the change makes it more
correct.

- **Patch** (2.0.1): bug fixes, including output that was wrong.
- **Minor** (2.1.0): new options, new features, or a new Docker CLI pin
  (`spec/vX.Y.Z/`).
- **Major** (3.0.0): a change to the command line or the library that breaks
  existing callers. runlike's command line stays compatible with 1.x
  (`test/compat`), so this must not happen.
- **Pre-release** (2.1.0-rc.1): allowed. The GitHub release is marked as a
  pre-release, and the image tag `:latest` does not move.

The version must be greater than every existing tag.

## Write the notes

Create `release-notes/vX.Y.Z.md`. Its content becomes the GitHub release
text exactly as written. It must name the version. Use these sections:

1. **Summary**: one short paragraph: what this release is for.
2. **Changes**: what a user sees, one bullet each. Link fixed issues as
   `#123`.
3. **Docker CLI pin**: the pinned version, and whether it changed.
4. **Thanks**: the people who reported issues or sent pull requests, by
   their GitHub `@handle`. Copy each handle from the issue or the pull
   request; never guess one.
5. **Upgrade notes**: anything users must do or check. Write "None" if
   there is nothing.

Write plain sentences. Do not use internal tracker IDs.

## Open the pull request

1. Put the code change and the notes file in one pull request, or put the
   notes file alone in a pull request after the code is merged.
2. Commit the notes file, then run:

   ```
   scripts/release-check.py release-notes/vX.Y.Z.md
   ```

   The script checks the notes file as committed at `HEAD`: the file name,
   the version (it must be greater than the latest tag, and no other
   release may be pending) and the notes (they must not be empty and must
   name the version). Fetch the tags first: `git fetch --tags upstream`.
3. Run the tests (see [CONTRIBUTING.md](CONTRIBUTING.md)), then open the pull
   request against `master`. Agents push to their own fork; they have no
   write access to this repository.
4. CI runs after the maintainer approves the run. The maintainer reviews the
   pull request and merges it. "Squash and merge" is recommended, because it
   keeps one master commit per change, but the release works with any merge
   method: it always releases the master commit that the merge pushed.

## What the workflow does

On every push to `master`:

1. **plan**: `scripts/release-check.py --commit <pushed commit>` looks for
   a notes file in the pushed commit whose tag does not exist yet (a
   *pending* version). If none is pending, the workflow stops. If more than
   one is pending, or the version is not greater than the latest tag, or
   the commit is not on master's first-parent history, it fails.
2. **test**: the full CI ([.github/workflows/ci.yml](.github/workflows/ci.yml))
   runs again on the pushed master commit.
3. **build**, on a macOS runner in the environment `release`: it builds
   the static binaries for linux/amd64, linux/arm64, darwin/arm64,
   windows/amd64 and windows/arm64 once, signs `runlike-darwin-arm64` with the Developer ID
   Application certificate (hardened runtime, secure timestamp) and has
   Apple notarize it ([scripts/macos-sign.sh](scripts/macos-sign.sh)). If
   signing or notarization fails, nothing is tagged or published. A bare
   binary cannot be stapled, so macOS checks the notarization online on
   first run. GitHub's macOS runners are sometimes slow to start; the
   release waits for one.
4. **publish**, in the environment `release`:
   - It tags that commit `vX.Y.Z`.
   - It takes the binaries and `SHA256SUMS` from build, unchanged.
   - It creates the GitHub release with the notes file as its text and
     attaches the binaries.
   - It pushes the image `assaflavie/runlike:X.Y.Z` for linux/amd64 and
     linux/arm64. It also moves `:latest`, unless the version is a
     pre-release or an older version than the latest release.
   - For the same newest non-prerelease, it writes `Formula/runlike.rb` in
     the tap [lavie/homebrew-tap](https://github.com/lavie/homebrew-tap)
     ([scripts/homebrew-formula.sh](scripts/homebrew-formula.sh)): the
     release binaries for macOS arm64 and Linux, with their checksums.
     Users install with `brew install lavie/tap/runlike`.

The release commit is the master commit the merge pushed, never a commit
from the pull request's branch. With a merge commit, that is the merge
commit.

## When a release fails

- **Before the tag exists** (the check, the tests, or the tag step
  failed): the version stays pending. Fix it forward: merge a pull request
  that fixes the problem and leaves the notes file in place (you may edit
  it). That push to `master` releases the version on the new commit. To
  retry without a new commit (a flaky runner), start the workflow by hand:
  Actions > release > Run workflow, on `master`, with the version and
  "republish" off. It releases the current master head.
- **To abandon a version**: merge a pull request that deletes its notes
  file. Then no version is pending, and the next release may use a new
  number. A pending notes file blocks every other release ("more than one
  pending release") until it ships or is deleted.
- **After the tag exists** (the GitHub release or the image failed): the
  version is no longer pending, and later pushes do not retry it. Start
  the workflow by hand on `master` with the version and "republish" on. It
  uses the tagged commit, updates the release, and pushes the image again.

Never publish to PyPI. The Python 1.x package is frozen.

## One-time repository setup (maintainer)

- [ ] **Settings > Actions > General > Fork pull request workflows from
      outside collaborators**: select "Require approval for all external
      contributors".
- [ ] **Settings > Actions > General > Workflow permissions**: select "Read
      repository contents and packages permissions". Leave "Allow GitHub
      Actions to create and approve pull requests" unchecked. (release.yml
      asks for write access only in its publish job.)
- [ ] **Settings > Actions > General > Actions permissions**: allow actions
      by lavie, plus GitHub-made and verified actions (the workflows use
      `actions/*` and `docker/*`, pinned by commit), or allow all actions.
- [ ] **Settings > Environments > New environment `release`**:
  - Deployment branches and tags: "Selected branches and tags", rule
    `master` (branch) only.
  - No required reviewers: merging the pull request is the approval.
  - Environment secret `DOCKERHUB_TOKEN`: a Docker Hub access token for
    `assaflavie` with read and write access to `assaflavie/runlike`.
  - Environment secrets for signing and notarizing the macOS binary:
    - `MACOS_CERT_P12_BASE64`: a "Developer ID Application" certificate
      with its private key, exported from Keychain Access as `.p12`, then
      `base64 -i cert.p12 | tr -d '\n'`. Only the Account Holder can
      create it (developer.apple.com > Certificates > + > Developer ID
      Application, G2 Sub-CA). It is valid for one year: renew it before
      it expires, or releases fail at build.
    - `MACOS_CERT_PASSWORD`: the `.p12` export password.
    - `ASC_KEY_ID`, `ASC_ISSUER_ID`, `ASC_KEY_P8`: an App Store Connect
      **team** API key with the Developer role (App Store Connect > Users
      and Access > Integrations > Team Keys), its issuer ID, and the full
      text of its `AuthKey_<id>.p8` file. notarytool uses it; individual
      keys cannot notarize.
  - Environment secret `HOMEBREW_TAP_TOKEN`: a fine-grained GitHub token
    owned by `lavie`, with access to the repository `lavie/homebrew-tap`
    only and the permission Contents: Read and write. Renew it before it
    expires, or releases fail before the tag.
- [ ] **The tap repository `lavie/homebrew-tap`**: public. The release
      workflow writes its `Formula/runlike.rb`; do not edit that file by
      hand.
- [ ] **Settings > Secrets and variables > Actions**: no repository-level
      secrets. Delete `DOCKERHUB_TOKEN` there if it exists.
- [ ] **Settings > Rules > Rulesets > New branch ruleset** for `master`
      (or a classic branch protection rule):
  - Require a pull request before merging.
  - Require status checks to pass, and require branches to be up to date
    before merging. The checks are `linux (ubuntu-24.04)`,
    `linux (ubuntu-24.04-arm)`, `windows`, `windows-arm` and `macos`.
  - Block force pushes and deletions.
  - Do not add the GitHub Actions app to the bypass list.
- [ ] **Settings > General > Pull Requests**: allow squash merging only
      (recommended; releases also work with merge commits). Optional:
      "Automatically delete head branches".
- [ ] **Tags**: if you add a tag ruleset for `v*`, the release workflow
      must still be able to create tags. Add "GitHub Actions" to that
      ruleset's bypass list, or do not add a tag ruleset.
- [ ] **Collaborators**: do not give the agent's GitHub account write
      access. It works from its own fork.
