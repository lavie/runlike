#!/usr/bin/env python3
"""Check a release proposal (release-notes/vX.Y.Z.md). Used by the release
workflow and, before a release PR is opened, by whoever prepares it.

  scripts/release-check.py release-notes/v2.1.0.md
      check this proposal as committed at HEAD (before opening the PR)
  scripts/release-check.py --commit SHA
      release check for master commit SHA: find the pending proposal in it
  scripts/release-check.py --commit SHA --version 2.1.0
      the same, but 2.1.0 must be the pending proposal (manual rerun)
  scripts/release-check.py --republish 2.1.0
      publish the existing tag v2.1.0 again, from its own commit

A proposal is pending when the notes file is in the commit and its tag does
not exist. It must be the only pending one, its version must be greater than
every existing version tag, and its notes must be non-empty and name the
version. The release commit is SHA itself (any commit on the first-parent
history of --branch, default origin/master), so a proposal whose release
failed is released by the next push to master that still contains it.

Prints key=value lines for $GITHUB_OUTPUT: tag, version, notes, commit,
prerelease, latest. Prints nothing when no proposal is pending. Exit 1 on
any failed check. Run from the repository root with all tags fetched.
"""
import argparse
import re
import subprocess
import sys

SEMVER = r"(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*))*))?"
NOTES = re.compile(r"release-notes/v(" + SEMVER + r")\.md")
# Old tags: "0.1", "1.4.14", "v1.6.3". Missing parts count as 0.
TAG = re.compile(r"v?(\d+)\.(\d+)(?:\.(\d+))?(?:-([0-9A-Za-z.-]+))?")


def key(major, minor, patch, pre):
    """Semver precedence: a pre-release sorts before its release; numeric
    identifiers before alphanumeric ones."""
    ids = tuple((0, int(p), "") if p.isdigit() else (1, 0, p) for p in pre.split(".")) if pre else ()
    return (int(major), int(minor), int(patch or 0), not pre, ids)


def git(*args, ok=False):
    r = subprocess.run(["git", *args], capture_output=True, text=True)
    if r.returncode and not ok:
        fail(f"git {' '.join(args)}: {r.stderr.strip()}")
    return r.stdout.strip() if not r.returncode else None


def fail(msg):
    sys.exit(f"release-check: {msg}")


def tags():
    return set(git("tag", "-l").split())


def resolve(rev):
    sha = git("rev-parse", "-q", "--verify", rev + "^{commit}", ok=True)
    if not sha:
        fail(f"{rev} is not a commit")
    return sha


def pending(commit, existing):
    files = git("ls-tree", "--name-only", commit, "release-notes/").split()
    return [p for p in sorted(files) if p.startswith("release-notes/v") and p.endswith(".md")
            and "v" + p[len("release-notes/v"):-3] not in existing]


def check(path, commit, republish=False, branch=None):
    m = NOTES.fullmatch(path)
    if not m:
        fail(f"{path}: name must be release-notes/vMAJOR.MINOR.PATCH[-PRERELEASE].md")
    version, tag = m[1], "v" + m[1]
    ver = key(*m.groups()[1:])
    existing = tags()
    if republish:
        if tag not in existing:
            fail(f"tag {tag} does not exist: nothing to publish again")
        commit = resolve(tag)
    text = git("show", f"{commit}:{path}", ok=True)
    if text is None:
        fail(f"{path} is not in commit {commit}")
    if not text.strip():
        fail(f"{path} is empty")
    if not re.search(r"(?<![\w.])v?" + re.escape(version) + r"(?![\w.-])", text):
        fail(f"{path} does not mention version {version}")

    if not republish:
        if tag in existing:
            fail(f"tag {tag} exists already: pick a new version")
        others = [p for p in pending(commit, existing) if p != path]
        if others:
            fail(f"more than one pending release: {path}, {', '.join(others)}")
        versions = [key(*t.groups()) for t in map(TAG.fullmatch, existing) if t]
        newest = max(versions, default=None)
        if newest is not None and ver <= newest:
            fail(f"{version} is not greater than the latest tag")

    if branch:
        on_branch(commit, branch)
    prerelease = "-" in version
    stable = [key(*t.groups()) for t in map(TAG.fullmatch, existing | {tag}) if t and not t[4]]
    latest = not prerelease and ver == max(stable, default=None)
    print(f"tag={tag}\nversion={version}\nnotes={path}\ncommit={commit}")
    print(f"prerelease={str(prerelease).lower()}\nlatest={str(latest).lower()}")


def on_branch(commit, branch):
    if commit not in git("rev-list", "--first-parent", resolve(branch)).split():
        fail(f"{commit} is not on the first-parent history of {branch}")


def main(argv):
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("notes", nargs="?", help="notes file to check at HEAD (before opening the PR)")
    p.add_argument("--commit", help="the master commit to release")
    p.add_argument("--version", help="with --commit: the version that must be pending")
    p.add_argument("--republish", metavar="VERSION", help="publish this existing tag again")
    p.add_argument("--branch", default="origin/master", help="the release branch (default origin/master)")
    a = p.parse_args(argv)
    modes = [bool(a.notes), bool(a.commit), bool(a.republish)]
    if sum(modes) != 1 or (a.version and not a.commit):
        p.error("give exactly one of NOTES, --commit or --republish; --version needs --commit")

    if a.notes:
        check(a.notes.removeprefix("./"), resolve("HEAD"))
    elif a.republish:
        path = f"release-notes/v{a.republish.removeprefix('v')}.md"
        check(path, None, republish=True, branch=a.branch)
    else:
        commit = resolve(a.commit)
        on_branch(commit, a.branch)
        if a.version:
            check(f"release-notes/v{a.version.removeprefix('v')}.md", commit)
            return
        found = pending(commit, tags())
        if len(found) > 1:
            fail(f"more than one pending release: {', '.join(found)}")
        elif found:
            check(found[0], commit)


if __name__ == "__main__":
    main(sys.argv[1:])
