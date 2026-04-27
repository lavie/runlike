# Runlike completeness implementation plan

This document is the implementation handoff for completing `runlike` on the first maintained Linux Docker target while preserving the released user-facing interface.

---

## 1. Goal

Complete `runlike` for the first maintained target with the following end state:

- the released CLI stays stable;
- support is measured separately for the container-name path and the `--stdin` path;
- support status is generated from probe results and committed artifacts;
- support is proven through automated round-trip recreation checks;
- detected unsupported option-states produce runtime warnings on stderr;
- the internal model, dictionary, and renderer structure support additional Docker targets and Docker Compose output after this pass.

---

## 2. Public compatibility contract

The release produced by this work preserves the existing user-facing interface.

Preserve these entrypoints and invocation forms:

- console entrypoint: `runlike`
- Python entrypoint: `runlike.runlike:main`
- container-name path: `runlike CONTAINER`
- stdin path: `docker inspect CONTAINER | runlike --stdin`

Preserve these flags:

- `--no-name`
- `--use-volume-id`
- `-p` / `--pretty`
- `-s` / `--stdin`
- `-l` / `--no-labels`

Preserve these output contracts:

- successful execution writes one `docker run ...` command to stdout;
- fatal failures continue to surface as CLI errors.

Use deterministic canonical output ordering and canonical flag spelling where that produces equivalent observable container state.

---

## 3. First maintained target

Implement and validate this first maintained target:

- Linux containers
- Docker Engine 25.0.5
- Docker CLI 25.0.5
- Docker API 1.44
- `DOCKER_API_VERSION=1.44`

Store this target in checked-in metadata and use it in CI probe jobs.

---

## 4. Correctness model

Support on an input path is established through canonicalized round-trip inspect comparison.

For each option-state probe:

1. create an original container with the option-state under test;
2. inspect the original container;
3. generate a command through `runlike CONTAINER`;
4. create a clone from that command;
5. inspect the clone;
6. compare canonicalized inspect projections;
7. repeat the same process through `docker inspect CONTAINER | runlike --stdin` when that path is in scope.

The canonical projection covers the stable observable configuration required to decide support, including:

- entrypoint and command;
- environment variables and labels;
- user, working directory, and hostname;
- restart policy;
- capabilities, privilege, runtime, pid, IPC, UTS, userns, cgroupns, and security settings;
- memory, CPU, pids, ulimit, sysctl, and related resource controls;
- mounts, volumes, tmpfs, and devices;
- exposed ports, published ports, and normalized network configuration;
- logging configuration;
- extra hosts and other inspect-visible host configuration.

Canonicalization removes environment-specific values that do not define the option-state under test, including generated identifiers, timestamps, runtime status, generated names, dynamic address assignments, dynamic host port assignments, log paths, and anonymous resource identifiers unless a probe targets them directly.

Use compare profiles so each probe checks the fields that determine correctness for that option-state.

---

## 5. Completion branch strategy

Carry out the work on a dedicated completion branch.

1. tag the current repository state as the baseline reference;
2. create the completion branch;
3. preserve the public compatibility contract throughout the branch;
4. treat the current implementation, tests, fixtures, and manual support lists as reference material on that branch;
5. replace internals, tests, fixtures, and support documentation as needed to reach the end state;
6. merge the completion branch back after the merge gate passes.

Keep the baseline tag available for differential debugging during implementation.

---

## 6. Internal architecture

Implement the new completion work around one shared data pipeline.

### 6.1 Pipeline

1. **source loader**
   - container-name input
   - stdin inspect JSON input

2. **image enrichment**
   - load image facts when available so image-defined defaults can be recognized and handled consistently

3. **normalized model builder**
   - convert inspect JSON into a stable internal representation
   - normalize ordering and shape differences up front

4. **option dictionary resolution**
   - determine which canonical option-states are present
   - determine path coverage and support status

5. **renderer**
   - emit canonical `docker run` tokens in deterministic order

6. **warning engine**
   - emit grouped warnings for detected unsupported option-states

7. **probe and comparator engine**
   - use the same normalized model and dictionary for probe execution, comparison, and support reporting

### 6.2 Shared source of truth

Use the normalized model and option dictionary as the shared source of truth for:

- command rendering;
- canonical comparison;
- support reporting;
- runtime warnings;
- later Docker Compose rendering.

---

## 7. Option inventory and dictionary

Implement two checked-in artifacts: a generated Docker option manifest and a repository-owned runlike option dictionary.

### 7.1 Docker option manifest

Generate the Docker option manifest from:

```bash
docker container run --help
docker container create --help
```

Each manifest entry contains:

- canonical long flag;
- short flag when present;
- help text from the pinned CLI;
- command family (`run`, `create`, or both);
- target metadata.

The manifest defines the option universe for the first maintained target.

### 7.2 Runlike option dictionary

Implement the runlike option dictionary as the core repository asset.

Each dictionary entry covers one canonical option or one tightly related option group and records:

- canonical output form;
- equivalent spellings and aliases;
- observability from inspect;
- inspect fields and detection profile;
- renderer profile;
- comparator profile;
- container-name path coverage;
- stdin path coverage;
- priority;
- scope classification;
- warning behavior;
- reason metadata for out-of-scope or runner-blocked entries.

Use the dictionary to answer these implementation questions for every option-state:

1. how the state is detected from inspect;
2. how the state is rendered;
3. how the state is compared in probes;
4. how support is reported;
5. whether runtime warnings are emitted when the state is detected but unsupported.

Use Docker CLI help text in the manifest and encode implementation semantics in the dictionary. Fill any missing semantics from Docker documentation while writing dictionary entries.

---

## 8. Unsupported option warnings

Implement runtime warnings for detected unsupported option-states.

Runtime flow:

1. build the normalized model;
2. resolve all detectable option-states through the dictionary;
3. render every supported option-state;
4. emit one grouped canonical warning to stderr for every detectable unsupported option-state on the active input path.

Warnings use canonical dictionary names and deterministic ordering. Stdout continues to contain only the generated `docker run` command.

Mark non-observable option-states explicitly in the dictionary so warning behavior remains evidence-based.

---

## 9. Probe design

Synthetic probes are the authoritative support tests.

### 9.1 Probe unit

Use one primary probe per option-state by default. Use one probe per tightly related option group when those states share one natural inspect and rendering unit.

Each probe uses:

- one main container;
- `busybox` or another equally small image by default;
- helper resources only when the option-state inherently requires them, such as an extra network, extra volume, linked container, or host device.

### 9.2 Probe flow

For every probe:

1. create required helper resources;
2. create the original container with the option-state under test;
3. inspect the original container;
4. run `runlike CONTAINER` and capture stdout and stderr;
5. create the clone from stdout and inspect it;
6. compare canonical projections;
7. run the stdin path with live inspect JSON, create the second clone, inspect it, and compare canonical projections when that path is in scope;
8. record results per path.

### 9.3 Container lifetime

Use long-lived commands so the clone remains inspectable during the probe. A standard default is:

```bash
busybox sh -c 'sleep 600'
```

### 9.4 Probe metadata

Each probe definition records:

- canonical option id;
- image;
- setup resources;
- original `docker run` arguments;
- cleanup rules;
- compare profile;
- path coverage;
- runner requirements.

---

## 10. Test strategy

The final authoritative test strategy consists of the synthetic round-trip probes.

Keep lightweight helper tests only where they accelerate development of shared infrastructure, such as:

- canonicalization helpers;
- shell tokenization and quoting helpers;
- dictionary validation.

Use the legacy substring tests and shared fixtures only as transition aids until equivalent focused probes are in place. The authoritative CI path uses the round-trip probes and the lightweight helper tests that support them.

---

## 11. Support status model

Use a generated support matrix as the source of truth.

Statuses:

- `supported`
- `partial`
- `unsupported`
- `out_of_scope`
- `blocked_by_runner`

Create one support row for every canonical dictionary entry and every supported input path.

Attach reason metadata where applicable, including:

- `windows_only`
- `client_side_only`
- `non_observable_from_inspect`
- `needs_gpu_runner`
- `needs_special_host_device`

### 11.1 Meaning of `supported`

`supported` means the round-trip probe passed for that path.

### 11.2 Meaning of `partial`

`partial` means one of:

- only some subforms are implemented;
- one input path passes and another does not;
- probe comparison passes with an explicitly documented limitation.

### 11.3 Meaning of `unsupported`

`unsupported` means the option-state is in scope and detectable, but the round-trip probe does not yet pass.

### 11.4 Meaning of `out_of_scope`

`out_of_scope` means the option exists in Docker but is not part of the first-pass support promise, for reasons such as:

- Windows-only;
- client-side only;
- not observable from inspect;
- intentionally deferred design choice.

### 11.5 Meaning of `blocked_by_runner`

`blocked_by_runner` means the option is in scope but requires a special execution environment to prove, such as:

- GPU runner;
- host device;
- unusual kernel capability.

---

## 12. Generated artifacts

Keep the generated artifact set minimal and practical.

```text
spec/
  current-target.json
  docker-option-manifest.json
  option-dictionary/
    ... one file per canonical option or option group ...

tools/
  dump_docker_option_manifest.py
  validate_option_dictionary.py
  canonicalize_inspect.py
  run_probes.py
  build_support_matrix.py
  check_generated_files.py

generated/
  probe-results.json
  support-matrix.json
  support-matrix.md
  mismatch-report.md

tests/
  probes/
    ... one probe per option or option group ...
```

When multi-version support is added, move these under per-target directories.

---

## 13. Prioritization

### 13.1 Priority rule

Prioritize trustworthy support claims, common Linux option-states, and both input paths before expanding through the rest of the manifest.

### 13.2 P0

P0 covers the option-states already closest to the current feature set and the highest-value basics:

- name / hostname / user / workdir
- env / labels
- entrypoint / cmd
- publish / expose
- mounts and volumes
- `--volumes-from`
- network mode / network / IP / IPv6 / aliases
- restart / tty / auto-remove / attach-open-stdin basics where observable
- capabilities / privileged / devices / pid / runtime / shm-size
- add-host / log-driver / log-opt
- memory / memory-reservation / cpuset and other already-adjacent resource flags

### 13.3 P1

P1 covers the remaining common Linux options that are in scope but not yet proven complete, such as:

- healthcheck family
- `--init`
- stop signal / stop timeout
- `--group-add`
- `--ipc`, `--uts`, `--userns`, `--cgroupns`
- `--security-opt`
- `--read-only`, `--tmpfs`, `--mount` forms not already covered
- CPU and pids controls not covered by P0
- `--ulimit`, `--sysctl`

### 13.4 P2

P2 covers runner-sensitive and host-sensitive options:

- GPU-related flags
- device-cgroup-rule
- advanced blkio and device-rate flags
- other flags that require special runners or kernel setup

---

## 14. Implementation phases

### Phase 0 — branch, baseline, and compatibility freeze

#### Goal

Create a safe implementation starting point.

#### Tasks

1. Tag the current repo state as a baseline.
2. Create a dedicated completion branch.
3. Record the public compatibility contract.
4. Capture baseline outputs from the current implementation on a small reference set.
5. Record the current README support lists, test layout, and CI behavior for reference.

#### Exit criteria

- completion branch exists;
- baseline reference is tagged;
- compatibility contract is written down.

### Phase 1 — pin the first maintained target in CI and local dev

#### Goal

Make the execution environment reproducible.

#### Tasks

1. Add checked-in target metadata.
2. Pin the CI Docker target.
3. Set `DOCKER_API_VERSION=1.44` for Docker probe jobs.
4. Add a CI verification step that prints and validates Docker versions.

#### Exit criteria

- CI uses the pinned target consistently.

### Phase 2 — generate the Docker option manifest

#### Goal

Define the complete option universe for the first pass.

#### Tasks

1. Implement `dump_docker_option_manifest.py`.
2. Parse `docker container run --help` and `docker container create --help`.
3. Generate `spec/docker-option-manifest.json`.
4. Deduplicate aliases and grouped entries.

#### Exit criteria

- every target Docker option is represented in the manifest exactly once in canonical form.

### Phase 3 — create the option dictionary skeleton

#### Goal

Create the implementation dictionary used by the whole completion effort.

#### Tasks

1. Create one dictionary entry per canonical option or tightly related option group.
2. For each entry, define:
   - observability;
   - detection fields;
   - compare profile;
   - render profile;
   - path coverage;
   - scope;
   - priority;
   - warning behavior.
3. Classify out-of-scope and runner-blocked options.
4. Validate dictionary coverage against the manifest.

#### Exit criteria

- manifest coverage is complete;
- every manifest option has a canonical dictionary home or an explicit out-of-scope reason.

### Phase 4 — build canonicalization and probe infrastructure

#### Goal

Create the round-trip correctness machinery.

#### Tasks

1. Implement inspect canonicalization.
2. Implement compare profiles.
3. Implement the probe runner.
4. Support both input paths.
5. Support setup and cleanup helpers.
6. Support deterministic stderr capture for warnings.
7. Support structured JSON output for probe results.

#### Exit criteria

- a probe can create an original container, run `runlike`, create a clone, inspect both, canonicalize both, compare both, and emit a structured result.

### Phase 5 — build the first per-option probes and replace the old fixture strategy

#### Goal

Move to one-probe-per-option coverage.

#### Tasks

1. Add P0 probes first.
2. Prefer `busybox` or a similarly tiny image.
3. Split the current large fixtures into focused per-option probes.
4. Keep only the minimal legacy checks needed until equivalent probes exist.
5. Move the authoritative CI path to the new probes as soon as P0 coverage exists.

#### Exit criteria

- active authoritative testing uses the focused probe layout.

### Phase 6 — implement runtime warning support

#### Goal

Warn on detectable unsupported option-states.

#### Tasks

1. Resolve detected option-states through the dictionary at runtime.
2. Emit grouped canonical warnings to stderr.
3. Keep stdout as a clean command string.
4. Add probe assertions for warning behavior where appropriate.

#### Exit criteria

- unsupported detectable option-states are warned about deterministically.

### Phase 7 — move rendering to the dictionary and normalized model

#### Goal

Produce the released command from the new pipeline.

#### Tasks

1. Implement the normalized model builder.
2. Implement renderer profiles.
3. Implement deterministic token ordering.
4. Preserve the current CLI surface while replacing internals.
5. Verify both container-name and stdin paths.

#### Exit criteria

- the released command is produced by the normalized model and dictionary pipeline.

### Phase 8 — generate support artifacts and replace manual docs

#### Goal

Make the support matrix the official source of truth.

#### Tasks

1. Build `probe-results.json`.
2. Build `support-matrix.json`.
3. Build `support-matrix.md`.
4. Update `README.md` to point to the generated support matrix.
5. Remove hand-maintained supported and unsupported option tables from the README.
6. Add generated-file checks to CI.

#### Exit criteria

- support claims are generated and current.

### Phase 9 — complete remaining options in priority order

#### Goal

Drive support work from the dictionary and probes until the first pass is complete.

#### Per-option workflow

For each option-state:

1. confirm the dictionary entry;
2. confirm scope and priority;
3. add or refine the focused probe;
4. record the failing baseline;
5. implement detection, render, and compare behavior;
6. rerun the probe for both paths;
7. update warnings if needed;
8. regenerate support artifacts;
9. commit code, probe, and generated support updates together.

#### Exit criteria

- every in-scope observable option-state has an explicit support status;
- support is earned by passing probes.

### Phase 10 — merge gate and release review

#### Goal

Decide when the completion branch is ready to merge back.

#### Merge gate

Before merge, verify:

- public CLI contract still holds;
- generated support matrix exists and is current;
- every manifest option is classified;
- every in-scope observable option-state has a status for both paths;
- old README support lists are gone;
- warnings work for detectable unsupported states;
- CI passes on the pinned target.

---

## 15. CI end state

Keep CI simple and aligned with the new test strategy.

Required jobs:

1. **dictionary and manifest validation**
   - validates complete coverage and schema consistency

2. **authoritative probe suite**
   - pinned target
   - synthetic round-trip probes
   - source of support truth

3. **generated-file check**
   - fails if generated support artifacts are stale

4. **runner-specific probe jobs**
   - for hardware and host-sensitive options

The final authoritative gate is the dictionary and manifest validation, the pinned probe suite, the generated-file check, and the runner-specific jobs required for blocked options.

---

## 16. Repository-level decisions

1. **Canonical output is allowed.**
   Equivalent observable container state is the compatibility target.

2. **The dictionary is the core asset.**
   The generated manifest defines the Docker option universe; the dictionary defines what `runlike` detects, renders, compares, warns about, and reports.

3. **Legacy material stays on the completion branch as reference material.**
   Current code, tests, fixtures, and manual support lists guide implementation and debugging while the new system is built.

4. **Probe-first implementation defines support.**
   An option-state becomes supported when its round-trip probe passes on the relevant input path.

5. **Warnings remain evidence-based.**
   Runtime warnings are emitted for detectable unsupported option-states.

---

## 17. Post-completion extensions

### 17.1 Additional Docker targets

After the first maintained target is complete, add new Docker targets by:

1. pinning a new target;
2. regenerating the Docker option manifest;
3. reusing and extending the option dictionary where valid;
4. rerunning the same probe framework against the new target;
5. generating a support matrix for the new target.

### 17.2 Docker Compose output

After the first maintained target is complete, add a second renderer that emits Docker Compose service definitions from the same normalized model and option dictionary.

---

## 18. Immediate next actions for the new Codex agent

Start in this order:

1. tag the current repo as a baseline and create the completion branch;
2. record the public compatibility contract;
3. pin the first maintained Docker target in CI;
4. generate the Docker option manifest;
5. create the full option dictionary skeleton;
6. implement inspect canonicalization and the probe runner;
7. create the first focused P0 probes;
8. move the authoritative test path to the new probes;
9. implement runtime unsupported-option warnings;
10. generate the first support matrix and update the README to point to it;
11. continue option-by-option until the matrix is complete for the first pass;
12. run the merge gate and merge the completion branch back.
