# runlike command-line compatibility suite

Black-box tests of runlike's own interface (flags, arguments, exit codes,
`--stdin`, runnable output, the Docker image). Any runlike build must pass
them. Output wording and option order are not checked: the tests compare the
argv that `sh` hands to `docker`, with options before the image sorted.

Setup (once):

    test/lima/up.sh
    test/compat/install-py.sh    # runlike 1.6.3 from git tag v1.6.3 into test/compat/.venv
    PATH=$PWD/test/bin:$PATH DOCKER_CONTEXT=runlike-test-1 test/compat/build-image-py.sh

Run (from the repo root):

    export PATH=$PWD/test/bin:$PATH DOCKER_CONTEXT=runlike-test-1
    go test -count=1 ./test/compat/                                 # Python 1.6.3
    RUNLIKE_IMAGE=runlike-compat:py go test -count=1 ./test/compat/  # plus the image test
    make build && RUNLIKE_BIN="$PWD/build/runlike" go test -count=1 ./test/compat/   # the Go build
    docker build -t runlike-local:dev . && \
      RUNLIKE_BIN="$PWD/build/runlike" RUNLIKE_IMAGE=runlike-local:dev go test -count=1 ./test/compat/

runlike 1.6.3 is installed from the git tag, not PyPI: PyPI stops at 1.5.4
(1.6.x was never uploaded). The Python code lives on branch `python-1.x`.

- `RUNLIKE_BIN`: the runlike command line (split on spaces). Default
  `test/compat/runlike-py.sh`, which also sets `RUNLIKE_IMPL=python`.
- `RUNLIKE_IMPL=python`: tests that fail on a known 1.6.3 bug skip as
  "expected fail" and error if they pass. Unset for other builds.
- `RUNLIKE_IMAGE`: image to test as `docker run --rm -v
  /var/run/docker.sock:/var/run/docker.sock IMAGE CONTAINER`; skipped if unset.

The suite creates containers labelled `compat.suite=1` and volume
`compat-vol` (names `compat-*`) and removes them before and after the run.
