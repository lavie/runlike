#!/usr/bin/env python3
"""Self-test of scripts/release-check.py: python3 scripts/test_release_check.py

Semver ordering, then release scenarios in a throwaway git repository:
squash merge, true merge commit, a failed release fixed forward, a manual
rerun, a republish, a prerelease with no stable tags, and the failures."""
import importlib.util, os, pathlib, subprocess, sys, tempfile

SCRIPT = pathlib.Path(__file__).resolve().with_name("release-check.py")
spec = importlib.util.spec_from_file_location("rc", SCRIPT)
rc = importlib.util.module_from_spec(spec); spec.loader.exec_module(rc)
k = lambda t: rc.key(*rc.TAG.fullmatch(t).groups())
order = ["0.1", "1.4.2", "1.4.14", "v1.6.3", "2.0.0-alpha", "2.0.0-alpha.1", "2.0.0-alpha.beta",
         "2.0.0-beta.2", "2.0.0-beta.11", "2.0.0-rc.1", "2.0.0", "2.0.1", "2.1.0", "10.0.0"]
assert sorted(order, key=k) == order, sorted(order, key=k)
assert rc.NOTES.fullmatch("release-notes/v2.0.0-rc.1.md") and not rc.NOTES.fullmatch("release-notes/v2.0.md")
assert not rc.NOTES.fullmatch("release-notes/v02.0.0.md")

repo = tempfile.mkdtemp(prefix="release-check-")
env = {**os.environ, "GIT_CONFIG_GLOBAL": os.devnull, "GIT_CONFIG_NOSYSTEM": "1",
       "GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t", "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t"}


def sh(*cmd):
    return subprocess.run(cmd, cwd=repo, env=env, check=True, capture_output=True, text=True).stdout.strip()


def commit(msg, **files):
    for name, text in files.items():
        path = pathlib.Path(repo, name)
        if text is None:
            sh("git", "rm", "-q", name)
            continue
        path.parent.mkdir(exist_ok=True)
        path.write_text(text)
        sh("git", "add", name)
    sh("git", "commit", "-q", "--allow-empty", "-m", msg)
    return sh("git", "rev-parse", "HEAD")


def run(*args, ok=True):
    r = subprocess.run([sys.executable, SCRIPT, "--branch", "master", *args] if args and args[0].startswith("--")
                       else [sys.executable, SCRIPT, *args], cwd=repo, env=env, capture_output=True, text=True)
    assert (r.returncode == 0) == ok, (args, r.stdout, r.stderr)
    return dict(l.split("=", 1) for l in r.stdout.split()) if ok else r.stderr


def release(out):  # what publish does: tag the release commit
    sh("git", "tag", out["tag"], out["commit"])


sh("git", "init", "-q", "-b", "master")
commit("python 1.x")
# Prerelease, no stable tag at all.
c = commit("rc notes", **{"release-notes/v2.0.0-rc.1.md": "2.0.0-rc.1\n"})
o = run("--commit", c)
assert o["commit"] == c and o["prerelease"] == "true" and o["latest"] == "false", o
release(o)
assert run("--commit", c) == {}, "a tagged version is not pending"
assert run("--commit", "HEAD") == {}

# Pre-PR check on a feature branch: notes must be committed at HEAD.
sh("git", "checkout", "-q", "-b", "feature")
commit("code")
pathlib.Path(repo, "release-notes/v2.0.0.md").write_text("Release v2.0.0\n")
assert "not in commit" in run("release-notes/v2.0.0.md", ok=False)
feat = commit("notes", **{"release-notes/v2.0.0.md": "Release v2.0.0\n"})
assert run("release-notes/v2.0.0.md")["commit"] == feat
# A feature commit is not a master commit.
assert "first-parent" in run("--commit", feat, ok=False)

# True merge commit: the release commit is the merge, not the commit that added the notes.
sh("git", "checkout", "-q", "master")
commit("other work on master")
sh("git", "merge", "-q", "--no-ff", "-m", "merge feature", "feature")
merge = sh("git", "rev-parse", "HEAD")
o = run("--commit", merge)
assert o["commit"] == merge and o["latest"] == "true" and o["tag"] == "v2.0.0", o

# Release failed (no tag). A fix-forward push releases the same notes on the new commit.
fix = commit("fix the build")
o = run("--commit", fix)
assert o["commit"] == fix and o["tag"] == "v2.0.0", o
# Manual rerun on master head: the version must still be pending.
assert run("--commit", fix, "--version", "2.0.0")["commit"] == fix
release(o)
assert "exists already" in run("--commit", fix, "--version", "2.0.0", ok=False)
# Republish an existing tag: its own commit, wherever master is now.
commit("later")
assert run("--republish", "2.0.0")["commit"] == fix
assert "does not exist" in run("--republish", "2.9.9", ok=False)

# Squash merge of a new version.
sq = commit("squash: 2.0.1", **{"release-notes/v2.0.1.md": "2.0.1 fixes\n"})
o = run("--commit", sq)
assert o["commit"] == sq and o["latest"] == "true", o
# Two pending versions fail loudly; deleting one (abandon) unblocks.
two = commit("2.0.2 too", **{"release-notes/v2.0.2.md": "2.0.2\n"})
assert "more than one pending" in run("--commit", two, ok=False)
one = commit("abandon 2.0.1", **{"release-notes/v2.0.1.md": None})
assert run("--commit", one)["tag"] == "v2.0.2"
# Bad proposals.
bad = commit("old", **{"release-notes/v2.0.2.md": None, "release-notes/v1.9.0.md": "1.9.0\n"})
assert "not greater" in run("--commit", bad, ok=False)
bad = commit("stray", **{"release-notes/v1.9.0.md": None, "release-notes/vNEXT.md": "x\n"})
assert "name must be" in run("--commit", bad, ok=False)
bad = commit("unnamed", **{"release-notes/vNEXT.md": None, "release-notes/v2.1.0.md": "notes\n"})
assert "does not mention" in run("--commit", bad, ok=False)
assert "more than one" not in run("--commit", bad, "--version", "2.1.0", ok=False)
print("ok")
