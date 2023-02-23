FROM python:3.8-alpine

ARG VERSION

RUN pip install runlike==$VERSION

ENTRYPOINT ["runlike"]
