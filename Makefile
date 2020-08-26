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
push: build
	docker push assaflavie/runlike
	docker push assaflavie/runlike:$(CUR_VER)

.PHONY: test
test:
	pipenv run pytest

.PHONY: pypi
pypi:
	python3 setup.py sdist bdist_wheel
	python3 -m twine upload --repository runlike dist/*

.PHONY: release
release: push pypi


