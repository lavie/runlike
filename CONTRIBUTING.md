Please make sure to add test coverage in `test_runlike.py` before submitting a PR.

Thanks for contributing!

Assaf

# How to release a version

(I keep getting it wrong)

1. Bump version in `pyproject.toml`
1. Tag Git with same version
1. `make release`
1. Push, wait for Travis build to pass
1. Publish GitHub release
