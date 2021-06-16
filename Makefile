CUR_VER := $(shell ./current_version.py)
SHELL := bash

.PHONY: build
build:
	docker build -t assaflavie/runlike .
	docker tag assaflavie/runlike assaflavie/runlike:$(CUR_VER)

.PHONY: rebuild
rebuild:
	docker build -t assaflavie/runlike --no-cache=true .
	docker tag assaflavie/runlike assaflavie/runlike:$(CUR_VER)

.PHONY: push
push: rebuild
	docker push assaflavie/runlike
	docker push assaflavie/runlike:$(CUR_VER)

.PHONY: test
test:
	poetry run pytest

.PHONY: pypi
pypi:
	poetry build
	poetry publish

.PHONY: release
release: push pypi


