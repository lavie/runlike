FROM docker

ARG VERSION

RUN apk add --no-cache python3 py3-pip

RUN python3 -m venv /app/venv
ENV PATH="/app/venv/bin:$PATH"
RUN pip3 install runlike==$VERSION

ENTRYPOINT ["runlike"]
