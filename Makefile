CUR_VER ?= $(shell poetry run ./current_version.py)
SHELL := bash
IMAGE_NAME ?= assaflavie/runlike

.PHONY: test
test:
	poetry run pytest -v

.PHONY: pypi
pypi:
	poetry build
	poetry publish -u __token__ -p $(POETRY_PYPI_TOKEN_PYPI)

.PHONY: release
release: push pypi

.PHONY: docker-build
docker-build:
	docker build \
	  -t $(IMAGE_NAME) \
	  -t $(IMAGE_NAME):$(CUR_VER) \
	  --build-arg VERSION=$(CUR_VER) \
	  .

.PHONY: docker-rebuild
docker-rebuild:
	docker build \
	  -t $(IMAGE_NAME) \
	  -t $(IMAGE_NAME):$(CUR_VER) \
	  --build-arg VERSION=$(CUR_VER) \
	  --no-cache=true \
	  .

.PHONY: docker-push
docker-push: docker-rebuild
	docker push $(IMAGE_NAME) 
	docker push $(IMAGE_NAME):$(CUR_VER)
