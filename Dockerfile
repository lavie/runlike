FROM python:3.11-alpine

RUN pip install runlike

ENTRYPOINT ["runlike"]
