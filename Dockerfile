FROM docker

ARG VERSION

RUN apk add --no-cache python3 py3-pip
RUN pip install runlike==$VERSION

ENTRYPOINT ["runlike"]
