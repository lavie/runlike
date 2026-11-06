# runlike 1.6.3 (the last Python release) in the shape of the old repo
# Dockerfile, from source: PyPI stops at 1.5.4. Build with build-image-py.sh.
FROM docker:29.9.3-cli
RUN apk add --no-cache python3 py3-pip
RUN python3 -m venv /app/venv
ENV PATH="/app/venv/bin:$PATH"
COPY src /src
RUN pip3 install /src
ENTRYPOINT ["runlike"]
