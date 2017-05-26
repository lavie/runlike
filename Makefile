.PHONY: test

cont_name := $(shell bash -c 'echo $$RANDOM')

test:
	docker build .
	docker run --name $(cont_name) busybox
	python runlike.py $(cont_name)
	docker rm $(cont_name)
