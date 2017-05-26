.PHONY: test

test:
	docker build .
	docker rm test || echo "creating test container"
	docker run --name test busybox
	python runlike.py test
	docker rm test
