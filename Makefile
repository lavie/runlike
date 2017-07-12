.PHONY: build
build:
	docker build -t assaflavie/runlike .

.PHONY: push
push:
	docker push assaflavie/runlike

.PHONY: test
test:
	nosetests
