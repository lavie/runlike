CUR_VER := $(shell poetry run $(PWD)/current_version.py)
SHELL := bash

.PHONY: build
build:
	docker build -t assaflavie/runlike --build-arg VERSION=$(CUR_VER) .
	docker tag assaflavie/runlike assaflavie/runlike:$(CUR_VER)

.PHONY: rebuild
rebuild:
	docker build -t assaflavie/runlike --build-arg VERSION=$(CUR_VER) --no-cache=true .
	docker tag assaflavie/runlike assaflavie/runlike:$(CUR_VER)

.PHONY: push
push: rebuild
	docker push assaflavie/runlike
	docker push assaflavie/runlike:$(CUR_VER)

.PHONY: test
test:
	poetry run pytest -v

.PHONY: pypi
pypi:
	poetry build
	@if ! poetry publish -u __token__ -p $(POETRY_PYPI_TOKEN_PYPI) 2>&1 | tee /dev/stderr | grep -q "HTTP Error 400: File already exists"; then \
		if [ $$? -ne 0 ]; then \
			echo "Error occurred during publish that was not 'File already exists'. Exiting."; \
			exit 1; \
		fi; \
	else \
		echo "Version $(CUR_VER) already exists on PyPI. Continuing..."; \
	fi

.PHONY: release
release: push pypi


